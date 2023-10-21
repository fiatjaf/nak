package main

import (
	"fmt"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/sdk"
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
		filter := nostr.Filter{}
		code := getStdinOrFirstArgument(c)

		prefix, value, err := nip19.Decode(code)
		if err != nil {
			return err
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
			relays = v.Relays
		case "naddr":
			v := value.(nostr.EntityPointer)
			filter.Tags = nostr.TagMap{"d": []string{v.Identifier}}
			filter.Kinds = append(filter.Kinds, v.Kind)
			filter.Authors = append(filter.Authors, v.PublicKey)
			authorHint = v.PublicKey
			relays = v.Relays
		case "nprofile":
			v := value.(nostr.ProfilePointer)
			filter.Authors = append(filter.Authors, v.PublicKey)
			filter.Kinds = append(filter.Kinds, 0)
			authorHint = v.PublicKey
			relays = v.Relays
		case "npub":
			v := value.(string)
			filter.Authors = append(filter.Authors, v)
			filter.Kinds = append(filter.Kinds, 0)
			authorHint = v
		}

		pool := nostr.NewSimplePool(c.Context)
		if authorHint != "" {
			relayList := sdk.FetchRelaysForPubkey(c.Context, pool, authorHint,
				"wss://purplepag.es", "wss://offchain.pub", "wss://public.relaying.io")
			for _, relayListItem := range relayList {
				if relayListItem.Outbox {
					relays = append(relays, relayListItem.URL)
				}
			}
		}

		if len(relays) == 0 {
			return fmt.Errorf("no relay hints found")
		}

		for ie := range pool.SubManyEose(c.Context, relays, nostr.Filters{filter}) {
			fmt.Println(ie.Event)
		}

		return nil
	},
}
