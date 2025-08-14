package main

import (
	"context"
	"fmt"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip05"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/sdk/hints"
	"github.com/urfave/cli/v3"
)

var fetch = &cli.Command{
	Name:  "fetch",
	Usage: "fetches events related to the given nip19 or nip05 code from the included relay hints or the author's outbox relays.",
	Description: `example usage:
        nak fetch nevent1qqsxrwm0hd3s3fddh4jc2574z3xzufq6qwuyz2rvv3n087zvym3dpaqprpmhxue69uhhqatzd35kxtnjv4kxz7tfdenju6t0xpnej4
        echo npub1h8spmtw9m2huyv6v2j2qd5zv956z2zdugl6mgx02f2upffwpm3nqv0j4ps | nak fetch --relay wss://relay.nostr.band`,
	DisableSliceFlagSeparator: true,
	Flags: append(reqFilterFlags,
		&cli.StringSliceFlag{
			Name:    "relay",
			Aliases: []string{"r"},
			Usage:   "also use these relays to fetch from",
		},
	),
	ArgsUsage: "[nip05_or_nip19_code]",
	Action: func(ctx context.Context, c *cli.Command) error {
		for code := range getStdinLinesOrArguments(c.Args()) {
			filter := nostr.Filter{}
			var authorHint nostr.PubKey
			relays := c.StringSlice("relay")

			if nip05.IsValidIdentifier(code) {
				pp, err := nip05.QueryIdentifier(ctx, code)
				if err != nil {
					ctx = lineProcessingError(ctx, "failed to fetch nip05: %s", err)
					continue
				}
				authorHint = pp.PublicKey
				relays = append(relays, pp.Relays...)
				filter.Authors = append(filter.Authors, pp.PublicKey)
			} else {
				prefix, value, err := nip19.Decode(code)
				if err != nil {
					ctx = lineProcessingError(ctx, "failed to decode: %s", err)
					continue
				}

				if err := normalizeAndValidateRelayURLs(relays); err != nil {
					return err
				}

				switch prefix {
				case "nevent":
					v := value.(nostr.EventPointer)
					filter.IDs = append(filter.IDs, v.ID)
					if v.Author != nostr.ZeroPK {
						authorHint = v.Author
					}
					relays = append(relays, v.Relays...)
				case "note":
					filter.IDs = append(filter.IDs, value.([32]byte))
				case "naddr":
					v := value.(nostr.EntityPointer)
					filter.Kinds = []nostr.Kind{v.Kind}
					filter.Tags = nostr.TagMap{"d": []string{v.Identifier}}
					filter.Authors = append(filter.Authors, v.PublicKey)
					authorHint = v.PublicKey
					relays = append(relays, v.Relays...)
				case "nprofile":
					v := value.(nostr.ProfilePointer)
					filter.Authors = append(filter.Authors, v.PublicKey)
					authorHint = v.PublicKey
					relays = append(relays, v.Relays...)
				case "npub":
					v := value.(nostr.PubKey)
					filter.Authors = append(filter.Authors, v)
					authorHint = v
				default:
					return fmt.Errorf("unexpected prefix %s", prefix)
				}
			}

			if authorHint != nostr.ZeroPK {
				for _, url := range relays {
					sys.Hints.Save(authorHint, nostr.NormalizeURL(url), hints.LastInHint, nostr.Now())
				}

				for _, url := range sys.FetchOutboxRelays(ctx, authorHint, 3) {
					relays = append(relays, url)
				}
			}

			if err := applyFlagsToFilter(c, &filter); err != nil {
				return err
			}

			if len(filter.Authors) > 0 && len(filter.Kinds) == 0 {
				filter.Kinds = append(filter.Kinds, 0)
			}

			if len(relays) == 0 {
				ctx = lineProcessingError(ctx, "no relay hints found")
				continue
			}

			for ie := range sys.Pool.FetchMany(ctx, relays, filter, nostr.SubscriptionOptions{
				Label: "nak-fetch",
			}) {
				stdout(ie.Event)
			}
		}

		exitIfLineProcessingError(ctx)
		return nil
	},
}
