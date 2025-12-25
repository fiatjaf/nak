package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/keyer"
	"fiatjaf.com/nostr/nipb0/blossom"
	"github.com/urfave/cli/v3"
)

var blossomCmd = &cli.Command{
	Name:                      "blossom",
	Suggest:                   true,
	UseShortOptionHandling:    true,
	Usage:                     "an army knife for blossom things",
	DisableSliceFlagSeparator: true,
	Flags: append(defaultKeyFlags,
		&cli.StringFlag{
			Name:     "server",
			Aliases:  []string{"s"},
			Usage:    "the hostname of the target mediaserver",
			Required: true,
		},
	),
	Commands: []*cli.Command{
		{
			Name:                      "list",
			Usage:                     "lists blobs from a pubkey",
			Description:               `takes one pubkey passed as an argument or derives one from the --sec supplied. if that is given then it will also pre-authorize the list, which some servers may require.`,
			DisableSliceFlagSeparator: true,
			ArgsUsage:                 "[pubkey]",
			Action: func(ctx context.Context, c *cli.Command) error {
				var client *blossom.Client
				pubkey := c.Args().First()
				if pubkey != "" {
					pk, err := parsePubKey(pubkey)
					if err != nil {
						return fmt.Errorf("invalid public key '%s': %w", pubkey, err)
					}
					client = blossom.NewClient(c.String("server"), keyer.NewReadOnlySigner(pk))
				} else {
					var err error
					client, err = getBlossomClient(ctx, c)
					if err != nil {
						return err
					}
				}

				bds, err := client.List(ctx)
				if err != nil {
					return err
				}

				for _, bd := range bds {
					stdout(bd)
				}

				return nil
			},
		},
		{
			Name:                      "upload",
			Usage:                     "uploads a file to a specific mediaserver.",
			Description:               `takes any number of local file paths and uploads them to a mediaserver, printing the resulting blob descriptions when successful.`,
			DisableSliceFlagSeparator: true,
			ArgsUsage:                 "[files...]",
			Action: func(ctx context.Context, c *cli.Command) error {
				client, err := getBlossomClient(ctx, c)
				if err != nil {
					return err
				}

				if isPiped() {
					// get file from stdin
					if c.Args().Len() > 0 {
						return fmt.Errorf("do not pass arguments when piping from stdin")
					}

					data, err := io.ReadAll(os.Stdin)
					if err != nil {
						return fmt.Errorf("failed to read stdin: %w", err)
					}

					bd, err := client.UploadBlob(ctx, bytes.NewReader(data), "")
					if err != nil {
						return err
					}

					j, _ := json.Marshal(bd)
					stdout(string(j))
				} else {
					// get filenames from arguments
					hasError := false
					for _, fpath := range c.Args().Slice() {
						bd, err := client.UploadFilePath(ctx, fpath)
						if err != nil {
							fmt.Fprintf(os.Stderr, "%s\n", err)
							hasError = true
							continue
						}

						j, _ := json.Marshal(bd)
						stdout(string(j))
					}

					if hasError {
						os.Exit(3)
					}
				}

				return nil
			},
		},
		{
			Name:                      "download",
			Usage:                     "downloads files from mediaservers",
			Description:               `takes any number of sha256 hashes as hex, downloads them and prints them to stdout (unless --output is specified).`,
			DisableSliceFlagSeparator: true,
			ArgsUsage:                 "[sha256...]",
			Flags: []cli.Flag{
				&cli.StringSliceFlag{
					Name:    "output",
					Aliases: []string{"o"},
					Usage:   "file name to save downloaded file to, can be passed multiple times when downloading multiple hashes",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				client, err := getBlossomClient(ctx, c)
				if err != nil {
					return err
				}

				outputs := c.StringSlice("output")

				hasError := false
				for i, hash := range c.Args().Slice() {
					if len(outputs)-1 >= i && outputs[i] != "--" {
						// save to this file
						err := client.DownloadToFile(ctx, hash, outputs[i])
						if err != nil {
							fmt.Fprintf(os.Stderr, "%s\n", err)
							hasError = true
						}
					} else {
						// if output wasn't specified, print to stdout
						data, err := client.Download(ctx, hash)
						if err != nil {
							fmt.Fprintf(os.Stderr, "%s\n", err)
							hasError = true
							continue
						}
						stdout(data)
					}
				}

				if hasError {
					os.Exit(2)
				}
				return nil
			},
		},
		{
			Name:    "del",
			Aliases: []string{"delete"},
			Usage:   "deletes a file from a mediaserver",
			Description: `takes any number of sha256 hashes, signs authorizations and deletes them from the current mediaserver.

if any of the files are not deleted command will fail, otherwise it will succeed. it will also print error messages to stderr and the hashes it successfully deletes to stdout.`,
			DisableSliceFlagSeparator: true,
			ArgsUsage:                 "[sha256...]",
			Action: func(ctx context.Context, c *cli.Command) error {
				client, err := getBlossomClient(ctx, c)
				if err != nil {
					return err
				}

				hasError := false
				for _, hash := range c.Args().Slice() {
					err := client.Delete(ctx, hash)
					if err != nil {
						fmt.Fprintf(os.Stderr, "%s\n", err)
						hasError = true
						continue
					}

					stdout(hash)
				}

				if hasError {
					os.Exit(3)
				}
				return nil
			},
		},
		{
			Name:  "check",
			Usage: "asks the mediaserver if it has the specified hashes.",
			Description: `uses the HEAD request to succintly check if the server has the specified sha256 hash.

if any of the files are not found the command will fail, otherwise it will succeed. it will also print error messages to stderr and the hashes it finds to stdout.`,
			DisableSliceFlagSeparator: true,
			ArgsUsage:                 "[sha256...]",
			Action: func(ctx context.Context, c *cli.Command) error {
				client, err := getBlossomClient(ctx, c)
				if err != nil {
					return err
				}

				hasError := false
				for _, hash := range c.Args().Slice() {
					err := client.Check(ctx, hash)
					if err != nil {
						hasError = true
						fmt.Fprintf(os.Stderr, "%s\n", err)
						continue
					}

					stdout(hash)
				}

				if hasError {
					os.Exit(2)
				}
				return nil
			},
		},
		{
			Name:                      "mirror",
			Usage:                     "mirrors blobs from source server to target server",
			Description:               `lists all blobs from the source server and mirrors them to the target server using BUD-04. requires --sec to sign the authorization event.`,
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				targetClient, err := getBlossomClient(ctx, c)
				if err != nil {
					return err
				}

				// Create client for source server
				sourceServer := c.Args().First()
				keyer, _, err := gatherKeyerFromArguments(ctx, c)
				if err != nil {
					return err
				}
				sourceClient := blossom.NewClient(sourceServer, keyer)

				// Get list of blobs from source server
				bds, err := sourceClient.List(ctx)
				if err != nil {
					return fmt.Errorf("failed to list blobs from source server: %w", err)
				}

				// Mirror each blob to target server
				hasError := false
				for _, bd := range bds {
					mirrored, err := mirrorBlob(ctx, targetClient, bd.URL)
					if err != nil {
						fmt.Fprintf(os.Stderr, "failed to mirror %s: %s\n", bd.SHA256, err)
						hasError = true
						continue
					}

					j, _ := json.Marshal(mirrored)
					stdout(string(j))
				}

				if hasError {
					os.Exit(3)
				}
				return nil
			},
		},
	},
}

func getBlossomClient(ctx context.Context, c *cli.Command) (*blossom.Client, error) {
	keyer, _, err := gatherKeyerFromArguments(ctx, c)
	if err != nil {
		return nil, err
	}
	return blossom.NewClient(c.String("server"), keyer), nil
}

// mirrorBlob mirrors a blob from a URL to the mediaserver using BUD-04
func mirrorBlob(ctx context.Context, client *blossom.Client, url string) (*blossom.BlobDescriptor, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download blob from URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download blob: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read blob content: %w", err)
	}

	hash := sha256.Sum256(data)
	hashHex := hex.EncodeToString(hash[:])

	signer := client.GetSigner()
	pubkey, _ := signer.GetPublicKey(ctx)

	evt := nostr.Event{
		Kind:      24242,
		CreatedAt: nostr.Now(),
		Tags: nostr.Tags{
			{"t", "upload"},
			{"x", hashHex},
			{"expiration", fmt.Sprintf("%d", nostr.Now()+60)},
		},
		Content: "blossom stuff",
		PubKey:  pubkey,
	}

	if err := signer.SignEvent(ctx, &evt); err != nil {
		return nil, fmt.Errorf("failed to sign authorization event: %w", err)
	}

	evtj, err := json.Marshal(evt)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal authorization event: %w", err)
	}
	auth := base64.StdEncoding.EncodeToString(evtj)

	mediaserver := client.GetMediaServer()
	mirrorURL := mediaserver + "mirror"

	requestBody := map[string]string{"url": url}
	requestJSON, _ := json.Marshal(requestBody)

	req, err := http.NewRequestWithContext(ctx, "PUT", mirrorURL, bytes.NewReader(requestJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create mirror request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Nostr "+auth)

	httpClient := &http.Client{}
	mirrorResp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send mirror request: %w", err)
	}
	defer mirrorResp.Body.Close()

	if mirrorResp.StatusCode < 200 || mirrorResp.StatusCode >= 300 {
		body, _ := io.ReadAll(mirrorResp.Body)
		return nil, fmt.Errorf("mirror request failed with HTTP %d: %s", mirrorResp.StatusCode, string(body))
	}

	var bd blossom.BlobDescriptor
	if err := json.NewDecoder(mirrorResp.Body).Decode(&bd); err != nil {
		return nil, fmt.Errorf("failed to decode blob descriptor: %w", err)
	}

	return &bd, nil
}
