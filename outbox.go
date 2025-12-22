package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

var outboxCmd = &cli.Command{
	Name:                      "outbox",
	Usage:                     "manage outbox relay hints database",
	DisableSliceFlagSeparator: true,
	Commands: []*cli.Command{
		{
			Name:                      "list",
			Usage:                     "list outbox relays for a given pubkey",
			ArgsUsage:                 "<pubkey>",
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				if c.Args().Len() != 1 {
					return fmt.Errorf("expected exactly one argument (pubkey)")
				}

				pk, err := parsePubKey(c.Args().First())
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
