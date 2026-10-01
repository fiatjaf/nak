package main

import (
	"context"

	"github.com/urfave/cli/v3"
)

var relaysCmd = &cli.Command{
	Name:      "relays",
	Usage:     "prints the nip65 relays for a given pubkey, one relay per line",
	ArgsUsage: "[pubkey]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "inbox",
			Aliases: []string{"read"},
			Usage:   "only print relays marked as inbox/read",
		},
		&cli.BoolFlag{
			Name:    "outbox",
			Aliases: []string{"write"},
			Usage:   "only print relays marked as outbox/write",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		wantInbox := c.Bool("inbox")
		wantOutbox := c.Bool("outbox")

		for pubkeyInput := range getStdinLinesOrArguments(c.Args()) {
			pk, err := parsePubKeyForCommand(ctx, c, pubkeyInput)
			if err != nil {
				ctx = lineProcessingError(ctx, "invalid pubkey '%s': %s", pubkeyInput, err)
				continue
			}

			relayList := sys.FetchRelayList(ctx, pk)
			for _, rl := range relayList.Items {
				if wantInbox && wantOutbox {
					if !(rl.Inbox && rl.Outbox) {
						continue
					}
				} else if wantInbox {
					if !rl.Inbox {
						continue
					}
				} else if wantOutbox {
					if !rl.Outbox {
						continue
					}
				}
				stdout(rl.URL)
			}
		}

		exitIfLineProcessingError(ctx)
		return nil
	},
}
