package main

import (
	"fmt"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/sdk"
	"github.com/urfave/cli/v2"
)

var fetch = &cli.Command{
	Name:        "fetch",
	Usage:       "fetches events related to the given nip19 code from the included relay hints",
	Description: ``,
	Flags:       []cli.Flag{},
	ArgsUsage:   "[nip19code]",
	Action: func(c *cli.Context) error {
		filter := nostr.Filter{}
		code := getStdinOrFirstArgument(c)

		prefix, value, err := nip19.Decode(code)
		if err != nil {
			return err
		}

		var relays []string
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
			authorHint = v.PublicKey
			relays = v.Relays
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
