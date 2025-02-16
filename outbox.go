package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
	"github.com/nbd-wtf/go-nostr"
)

var outbox = &cli.Command{
	Name:                      "outbox",
	Usage:                     "manage outbox relay hints database",
	DisableSliceFlagSeparator: true,
	Commands: []*cli.Command{
		{
			Name:                      "init",
			Usage:                     "initialize the outbox hints database",
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				if hintsFileExists {
					return nil
				}
				if hintsFilePath == "" {
					return fmt.Errorf("couldn't find a place to store the hints, pass --config-path to fix.")
				}

				if err := os.MkdirAll(filepath.Dir(hintsFilePath), 0777); err == nil {
					if err := os.WriteFile(hintsFilePath, []byte("{}"), 0644); err != nil {
						return fmt.Errorf("failed to create hints database: %w", err)
					}
				}

				log("initialized hints database at %s\n", hintsFilePath)
				return nil
			},
		},
		{
			Name:                      "list",
			Usage:                     "list outbox relays for a given pubkey",
			ArgsUsage:                 "<pubkey>",
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				if !hintsFileExists {
					log("running with temporary fragile data.\n")
					log("call `nak outbox init` to setup persistence.\n")
				}

				if c.Args().Len() != 1 {
					return fmt.Errorf("expected exactly one argument (pubkey)")
				}

				pubkey := c.Args().First()
				if !nostr.IsValidPublicKey(pubkey) {
					return fmt.Errorf("invalid public key: %s", pubkey)
				}

				for _, relay := range sys.FetchOutboxRelays(ctx, pubkey, 6) {
					stdout(relay)
				}

				return nil
			},
		},
	},
}
