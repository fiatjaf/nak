package main

import (
	"context"

	"github.com/fiatjaf/cli/v3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	sdk "github.com/nbd-wtf/nostr-sdk"
)

var fetch = &cli.Command{
	Name:  "fetch",
	Usage: "fetches events related to the given nip19 code from the included relay hints or the author's NIP-65 relays.",
	Description: `example usage:
        nak fetch nevent1qqsxrwm0hd3s3fddh4jc2574z3xzufq6qwuyz2rvv3n087zvym3dpaqprpmhxue69uhhqatzd35kxtnjv4kxz7tfdenju6t0xpnej4
        echo npub1h8spmtw9m2huyv6v2j2qd5zv956z2zdugl6mgx02f2upffwpm3nqv0j4ps | nak fetch --relay wss://relay.nostr.band`,
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "relay",
			Aliases: []string{"r"},
			Usage:   "also use these relays to fetch from",
		},
	},
	ArgsUsage: "[nip19code]",
	Action: func(ctx context.Context, c *cli.Command) error {
		sys := sdk.NewSystem()

		defer func() {
			sys.Pool.Relays.Range(func(_ string, relay *nostr.Relay) bool {
				relay.Close()
				return true
			})
		}()

		for code := range getStdinLinesOrArguments(c.Args()) {
			filter := nostr.Filter{}

			prefix, value, err := nip19.Decode(code)
			if err != nil {
				ctx = lineProcessingError(ctx, "failed to decode: %s", err)
				continue
			}

			relays := c.StringSlice("relay")
			if err := normalizeAndValidateRelayURLs(relays); err != nil {
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
				relays := sys.FetchOutboxRelays(ctx, authorHint, 3)
				for _, url := range relays {
					relays = append(relays, url)
				}
			}

			if len(relays) == 0 {
				ctx = lineProcessingError(ctx, "no relay hints found")
				continue
			}

			for ie := range sys.Pool.SubManyEose(ctx, relays, nostr.Filters{filter}) {
				stdout(ie.Event)
			}
		}

		exitIfLineProcessingError(ctx)
		return nil
	},
}
