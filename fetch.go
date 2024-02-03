package main

import (
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	sdk "github.com/nbd-wtf/nostr-sdk"
	"github.com/urfave/cli/v2"
)

var fetch = &cli.Command{
	Name:  "fetch",
	Usage: "fetches events related to the given nip19 code from the included relay hints",
	Description: `example usage:
        nak fetch nevent1qqsxrwm0hd3s3fddh4jc2574z3xzufq6qwuyz2rvv3n087zvym3dpaqprpmhxue69uhhqatzd35kxtnjv4kxz7tfdenju6t0xpnej4
        echo npub1h8spmtw9m2huyv6v2j2qd5zv956z2zdugl6mgx02f2upffwpm3nqv0j4ps | nak fetch --relay wss://relay.nostr.band`,
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "relay",
			Aliases: []string{"r"},
			Usage:   "also use these relays to fetch from",
		},
	},
	ArgsUsage: "[nip19code]",
	Action: func(c *cli.Context) error {
		pool := nostr.NewSimplePool(c.Context)

		defer func() {
			pool.Relays.Range(func(_ string, relay *nostr.Relay) bool {
				relay.Close()
				return true
			})
		}()

		for code := range getStdinLinesOrFirstArgument(c.Args().First()) {
			filter := nostr.Filter{}

			prefix, value, err := nip19.Decode(code)
			if err != nil {
				lineProcessingError(c, "failed to decode: %s", err)
				continue
			}

			relays := c.StringSlice("relay")
			if err := validateRelayURLs(relays); err != nil {
				return err
			}
			var authorHint string

			switch prefix {
			case "nevent":
				v := value.(nostr.EventPointer)
				filter.IDs = append(filter.IDs, v.ID)
				if v.Author != "" {
					authorHint = v.Author
				}
				relays = append(relays, v.Relays...)
			case "naddr":
				v := value.(nostr.EntityPointer)
				filter.Tags = nostr.TagMap{"d": []string{v.Identifier}}
				filter.Kinds = append(filter.Kinds, v.Kind)
				filter.Authors = append(filter.Authors, v.PublicKey)
				authorHint = v.PublicKey
				relays = append(relays, v.Relays...)
			case "nprofile":
				v := value.(nostr.ProfilePointer)
				filter.Authors = append(filter.Authors, v.PublicKey)
				filter.Kinds = append(filter.Kinds, 0)
				authorHint = v.PublicKey
				relays = append(relays, v.Relays...)
			case "npub":
				v := value.(string)
				filter.Authors = append(filter.Authors, v)
				filter.Kinds = append(filter.Kinds, 0)
				authorHint = v
			}

			if authorHint != "" {
				relayList := sdk.FetchRelaysForPubkey(c.Context, pool, authorHint,
					"wss://purplepag.es", "wss://relay.damus.io", "wss://relay.noswhere.com",
					"wss://nos.lol", "wss://public.relaying.io", "wss://relay.nostr.band")
				for _, relayListItem := range relayList {
					if relayListItem.Outbox {
						relays = append(relays, relayListItem.URL)
					}
				}
			}

			if len(relays) == 0 {
				lineProcessingError(c, "no relay hints found")
				continue
			}

			for ie := range pool.SubManyEose(c.Context, relays, nostr.Filters{filter}) {
				stdout(ie.Event)
			}
		}

		exitIfLineProcessingError(c)
		return nil
	},
}
