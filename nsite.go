package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/keyer"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nip5a"
	"fiatjaf.com/nostr/nipb0/blossom"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

var nsite = &cli.Command{
	Name:                      "nsite",
	Suggest:                   true,
	Usage:                     "publishes and downloads nip-5A static sites",
	ArgsUsage:                 "<directory> [relay...]",
	DisableSliceFlagSeparator: true,
	Flags:                     defaultKeyFlags,
	Commands: []*cli.Command{
		{
			Name:                      "upload",
			Usage:                     "uploads site files and publishes manifest event",
			ArgsUsage:                 "<directory> [relay...]",
			DisableSliceFlagSeparator: true,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:  "root",
					Usage: "publish root site as kind 15128",
				},
				&cli.StringFlag{
					Name:    "identifier",
					Aliases: []string{"d"},
					Usage:   "publish named site as kind 35128 with this d tag",
				},
				&cli.StringFlag{
					Name:  "description",
					Usage: "a human-readable description of the site",
				},
				&cli.StringFlag{
					Name:  "source",
					Usage: "a link to the source code of the site",
				},
				&cli.StringSliceFlag{
					Name:        "server",
					Aliases:     []string{"s"},
					Usage:       "blossom server hostname or URL, can be given multiple times",
					DefaultText: "defaults to the publisher's list of preferred blossom servers",
				},
				&cli.BoolFlag{
					Name:    "yes",
					Aliases: []string{"y"},
					Usage:   "skip upload confirmation prompt",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				dir := c.Args().First()
				if dir == "" {
					return fmt.Errorf("missing directory")
				}

				st, err := os.Stat(dir)
				if err != nil {
					return fmt.Errorf("failed to stat %s: %w", dir, err)
				}
				if !st.IsDir() {
					return fmt.Errorf("%s is not a directory", dir)
				}

				root := c.Bool("root")
				identifier := c.String("identifier")
				if root == (identifier != "") {
					return fmt.Errorf("pick exactly one of --root or --identifier/-d")
				}

				kr, _, err := gatherKeyerFromArguments(ctx, c)
				if err != nil {
					return err
				}
				pk, err := kr.GetPublicKey(ctx)
				if err != nil {
					return fmt.Errorf("failed to get public key: %w", err)
				}

				manifest := nip5a.SiteManifest{
					Pubkey:      pk,
					Root:        root,
					Identifier:  identifier,
					Paths:       make(map[string][32]byte),
					Description: c.String("description"),
					Source:      c.String("source"),
				}

				blossomServers := c.StringSlice("server")
				if len(blossomServers) != 0 {
					manifest.Servers = blossomServers
				} else {
					servers := sys.FetchBlossomServerList(ctx, pk)
					if len(servers.Items) == 0 {
						return fmt.Errorf("no blossom servers advertised in manifest or kind:10063")
					}
					blossomServers = make([]string, len(servers.Items))
					for i, s := range servers.Items {
						blossomServers[i] = s.Value()
					}
				}

				if !c.Bool("yes") {
					log("%s\n", color.CyanString("files:"))

					if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
						if err != nil {
							return err
						}
						if d.IsDir() || !d.Type().IsRegular() {
							return nil
						}

						relPath, err := filepath.Rel(dir, path)
						if err != nil {
							return fmt.Errorf("failed to get relative path for %s: %w", path, err)
						}

						log("  %s\n", color.GreenString("/%s", filepath.ToSlash(relPath)))
						return nil
					}); err != nil {
						return err
					}

					log("%s\n", color.CyanString("blossom servers:"))
					for _, server := range blossomServers {
						log("  %s\n", color.YellowString(server))
					}
					if !askConfirmation("upload nsite and publish manifest? [y/n] ") {
						return fmt.Errorf("aborted")
					}
				}

				if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.IsDir() || !d.Type().IsRegular() {
						return nil
					}

					relPath, err := filepath.Rel(dir, path)
					if err != nil {
						return fmt.Errorf("failed to get relative path for %s: %w", path, err)
					}

					var hhash string
					for _, server := range blossomServers {
						client := blossom.NewClient(server, kr)
						bd, err := client.UploadFilePath(ctx, path)
						if err != nil {
							return fmt.Errorf("failed to upload %s to %s: %w", path, server, err)
						}
						hhash = bd.SHA256
						log("uploaded %s to %s as %s\n", color.GreenString(path), color.YellowString(server), color.CyanString(hhash))
					}

					var hash [32]byte
					if _, err := hex.Decode(hash[:], []byte(hhash)); err != nil {
						return fmt.Errorf("invalid blob hash '%s': %w", hhash, err)
					}
					manifest.Paths["/"+filepath.ToSlash(relPath)] = hash

					return nil
				}); err != nil {
					return err
				}

				evt := manifest.ToEvent()
				if err := kr.SignEvent(ctx, &evt); err != nil {
					return fmt.Errorf("error signing manifest event: %w", err)
				}

				relayURLs := nostr.AppendUnique(sys.FetchWriteRelays(ctx, pk), c.Args().Slice()[1:]...)
				if len(relayURLs) == 0 {
					return fmt.Errorf("no relays to publish this nsite to")
				}

				sys.Pool.AuthRequiredHandler = func(ctx context.Context, authEvent *nostr.Event) error {
					return authSigner(ctx, c, func(string, ...any) {}, authEvent)
				}
				relays := connectToAllRelays(ctx, c, relayURLs, nil)
				if len(relays) == 0 {
					return fmt.Errorf("failed to connect to any of [ %v ]", relayURLs)
				}

				stdout(evt.String())
				if identifier == "" {
					stdout(nip19.EncodeNpub(pk))
				} else {
					stdout(nip5a.PubKeyToBase36(pk) + identifier)
				}

				return publishFlow(ctx, c, kr, evt, relays)
			},
		},
		{
			Name:                      "download",
			Usage:                     "downloads all files from a published nsite",
			ArgsUsage:                 "<site> [directory]",
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				input := c.Args().First()
				if input == "" {
					return fmt.Errorf("missing site")
				}

				outputDir := c.Args().Get(1)
				if outputDir == "" {
					return fmt.Errorf("missing write directory")
				}
				if st, err := os.Stat(outputDir); err == nil {
					if st.IsDir() {
						return fmt.Errorf("output directory %s already exists", outputDir)
					}
					return fmt.Errorf("output path %s already exists and is not a directory", outputDir)
				} else if !os.IsNotExist(err) {
					return fmt.Errorf("failed to stat output directory %s: %w", outputDir, err)
				}

				pk, identifier, isRoot, err := nip5a.DecodeSiteURL(input)
				if err != nil {
					return err
				}

				filter := nostr.Filter{
					Authors: []nostr.PubKey{pk},
					Limit:   1,
				}
				if isRoot {
					filter.Kinds = []nostr.Kind{nostr.KindNsiteRoot}
				} else {
					filter.Kinds = []nostr.Kind{nostr.KindNsiteNamed}
					filter.Tags = nostr.TagMap{"d": []string{identifier}}
				}

				res := sys.Pool.QuerySingle(ctx, sys.FetchWriteRelays(ctx, pk), filter, nostr.SubscriptionOptions{
					Label: "nak-nsite",
				})
				if res == nil {
					return fmt.Errorf("failed to fetch nsite with filter %v", filter)
				}

				mnf, err := nip5a.ParseSiteManifest(&res.Event)
				if err != nil {
					return fmt.Errorf("invalid nsite %s: %w", res.Event, err)
				}

				blossomServers := mnf.Servers
				if len(blossomServers) == 0 {
					servers := sys.FetchBlossomServerList(ctx, res.Event.PubKey)
					if len(servers.Items) == 0 {
						return fmt.Errorf("no blossom servers advertised in manifest or kind:10063")
					}
					blossomServers = make([]string, len(servers.Items))
					for i, s := range servers.Items {
						blossomServers[i] = s.Value()
					}
				}

				signer := keyer.NewReadOnlySigner(pk)

				for path, hash := range mnf.Paths {
					fullPath := filepath.Join(outputDir, filepath.FromSlash(strings.TrimPrefix(path, "/")))
					if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
						return fmt.Errorf("failed to create %s: %w", filepath.Dir(fullPath), err)
					}

					var downloadErr error
					for _, server := range blossomServers {
						client := blossom.NewClient(server, signer)
						data, err := client.Download(ctx, hash)
						if err != nil {
							downloadErr = err
							continue
						}
						if err := os.WriteFile(fullPath, data, 0o644); err != nil {
							return fmt.Errorf("failed to write %s: %w", fullPath, err)
						}
						stdout(path)
						downloadErr = nil
						break
					}
					if downloadErr != nil {
						return fmt.Errorf("failed to download '%s': %w", path, downloadErr)
					}
				}

				return nil
			},
		},
	},
}
