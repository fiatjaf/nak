package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip90"
	"github.com/urfave/cli/v3"
)

var dvm = &cli.Command{
	Name:                      "dvm",
	Usage:                     "deal with nip90 data-vending-machine things (experimental)",
	DisableSliceFlagSeparator: true,
	Flags: append(defaultKeyFlags,
		&cli.StringSliceFlag{
			Name:    "relay",
			Aliases: []string{"r"},
		},
	),
	Commands: append([]*cli.Command{
		{
			Name:                      "list",
			Usage:                     "find DVMs that have announced themselves for a specific kind",
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				return fmt.Errorf("we don't know how to do this yet")
			},
		},
	}, (func() []*cli.Command {
		commands := make([]*cli.Command, len(nip90.Jobs))
		for i, job := range nip90.Jobs {
			flags := make([]cli.Flag, 0, 2+len(job.Params))

			if job.InputType != "" {
				flags = append(flags, &cli.StringSliceFlag{
					Name:     "input",
					Aliases:  []string{"i"},
					Category: "INPUT",
				})
			}

			for _, param := range job.Params {
				flags = append(flags, &cli.StringSliceFlag{
					Name:     param,
					Category: "PARAMETER",
				})
			}

			commands[i] = &cli.Command{
				Name:                      strconv.Itoa(job.InputKind),
				Usage:                     job.Name,
				Description:               job.Description,
				DisableSliceFlagSeparator: true,
				Flags:                     flags,
				Action: func(ctx context.Context, c *cli.Command) error {
					relayUrls := c.StringSlice("relay")
					relays := connectToAllRelays(ctx, c, relayUrls, nil)
					if len(relays) == 0 {
						log("failed to connect to any of the given relays.\n")
						os.Exit(3)
					}
					defer func() {
						for _, relay := range relays {
							relay.Close()
						}
					}()

					evt := nostr.Event{
						Kind:      job.InputKind,
						Tags:      make(nostr.Tags, 0, 2+len(job.Params)),
						CreatedAt: nostr.Now(),
					}

					for _, input := range c.StringSlice("input") {
						evt.Tags = append(evt.Tags, nostr.Tag{"i", input, job.InputType})
					}
					for _, paramN := range job.Params {
						for _, paramV := range c.StringSlice(paramN) {
							tag := nostr.Tag{"param", paramN, "", ""}[0:2]
							for _, v := range strings.Split(paramV, ";") {
								tag = append(tag, v)
							}
							evt.Tags = append(evt.Tags, tag)
						}
					}

					kr, _, err := gatherKeyerFromArguments(ctx, c)
					if err != nil {
						return err
					}
					if err := kr.SignEvent(ctx, &evt); err != nil {
						return err
					}

					logverbose("%s", evt)

					log("- publishing job request... ")
					first := true
					for res := range sys.Pool.PublishMany(ctx, relayUrls, evt) {
						cleanUrl, _ := strings.CutPrefix(res.RelayURL, "wss://")

						if !first {
							log(", ")
						}
						first = false

						if res.Error != nil {
							log("%s: %s", colors.errorf(cleanUrl), res.Error)
						} else {
							log("%s: ok", colors.successf(cleanUrl))
						}
					}

					log("\n- waiting for response...\n")
					for ie := range sys.Pool.SubscribeMany(ctx, relayUrls, nostr.Filter{
						Kinds: []int{7000, job.OutputKind},
						Tags:  nostr.TagMap{"e": []string{evt.ID}},
					}) {
						stdout(ie.Event)
					}

					return nil
				},
			}
		}
		return commands
	})()...),
}
