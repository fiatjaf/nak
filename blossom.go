package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
					if pk, err := nostr.PubKeyFromHex(pubkey); err != nil {
						return fmt.Errorf("invalid public key '%s': %w", pubkey, err)
					} else {
						client = blossom.NewClient(c.String("server"), keyer.NewReadOnlySigner(pk))
					}
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
			Usage:                     "",
			Description:               ``,
			DisableSliceFlagSeparator: true,
			ArgsUsage:                 "",
			Action: func(ctx context.Context, c *cli.Command) error {
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
