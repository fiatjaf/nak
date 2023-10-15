package main

import (
	"fmt"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
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
		code := c.Args().First()

		prefix, value, err := nip19.Decode(code)
		if err != nil {
			return err
		}

		var relays []string
		switch prefix {
		case "nevent":
			v := value.(nostr.EventPointer)
			filter.IDs = append(filter.IDs, v.ID)
			if v.Author != "" {
				// TODO fetch relays from nip65
			}
			relays = v.Relays
		case "naddr":
			v := value.(nostr.EntityPointer)
			filter.Tags = nostr.TagMap{"d": []string{v.Identifier}}
			filter.Kinds = append(filter.Kinds, v.Kind)
			filter.Authors = append(filter.Authors, v.PublicKey)
			// TODO fetch relays from nip65
			relays = v.Relays
		case "nprofile":
			v := value.(nostr.ProfilePointer)
			filter.Authors = append(filter.Authors, v.PublicKey)
			// TODO fetch relays from nip65
			relays = v.Relays
		}

		if len(relays) == 0 {
			return fmt.Errorf("no relay hints found")
		}

		pool := nostr.NewSimplePool(c.Context)
		for ie := range pool.SubManyEose(c.Context, relays, nostr.Filters{filter}) {
			fmt.Println(ie.Event)
		}

		return nil
	},
}
