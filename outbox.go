package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/sdk"
	"fiatjaf.com/nostr/sdk/hints/badgerh"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

var (
	hintsFilePath   string
	hintsFileExists bool
)

func initializeOutboxHintsDB(c *cli.Command, sys *sdk.System) error {
	configPath := c.String("config-path")
	if configPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configPath = filepath.Join(home, ".config/nak")
		}
	}
	if configPath != "" {
		hintsFilePath = filepath.Join(configPath, "outbox/hints.bg")
	}
	if hintsFilePath != "" {
		if _, err := os.Stat(hintsFilePath); !os.IsNotExist(err) {
			hintsFileExists = true
		} else if err != nil {
			return err
		}
	}
	if hintsFileExists && hintsFilePath != "" {
		hintsdb, err := badgerh.NewBadgerHints(hintsFilePath)
		if err == nil {
			sys.Hints = hintsdb
		}
	}

	return nil
}

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

				os.MkdirAll(hintsFilePath, 0755)
				_, err := badgerh.NewBadgerHints(hintsFilePath)
				if err != nil {
					return fmt.Errorf("failed to create badger hints db at '%s': %w", hintsFilePath, err)
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
					log(color.YellowString("running with temporary fragile data.\n"))
					log(color.YellowString("call `nak outbox init` to setup persistence.\n"))
				}

				if c.Args().Len() != 1 {
					return fmt.Errorf("expected exactly one argument (pubkey)")
				}

				pk, err := nostr.PubKeyFromHex(c.Args().First())
				if err != nil {
					return fmt.Errorf("invalid public key '%s': %w", c.Args().First(), err)
				}

				for _, relay := range sys.FetchOutboxRelays(ctx, pk, 6) {
					stdout(relay)
				}

				return nil
			},
		},
	},
}
