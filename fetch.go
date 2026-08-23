package main

import (
	"context"
	"fmt"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip05"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nipad"
	"fiatjaf.com/nostr/sdk/hints"
	"github.com/urfave/cli/v3"
)

var fetch = &cli.Command{
	Name:  "fetch",
	Usage: "fetches events related to the given nip19 or nip05 code, or nostr web address, from the included relay hints or the author's outbox relays.",
	Description: `example usage:
        nak fetch nevent1qqsxrwm0hd3s3fddh4jc2574z3xzufq6qwuyz2rvv3n087zvym3dpaqprpmhxue69uhhqatzd35kxtnjv4kxz7tfdenju6t0xpnej4
        echo npub1h8spmtw9m2huyv6v2j2qd5zv956z2zdugl6mgx02f2upffwpm3nqv0j4ps | nak fetch --relay wss://relay.nostr.band`,
	DisableSliceFlagSeparator: true,
	Flags: combineFlags([][]cli.Flag{reqFilterFlags},
		&cli.StringSliceFlag{
			Name:    "relay",
			Aliases: []string{"r"},
			Usage:   "also use these relays to fetch from",
		},
		&cli.StringFlag{
			Name:  "jq",
			Usage: "filter returned events with jq expression",
		},
		&cli.BoolFlag{
			Name:  "jq-raw",
			Usage: "print --jq string results without JSON quoting, like `jq -r`",
		},
	),
	ArgsUsage: "[nip05_or_nip19_code_or_web_address]",
	Action: func(ctx context.Context, c *cli.Command) error {
		jq, err := jqPrepare(c.String("jq"), c.Bool("jq-raw"))
		if err != nil {
			return err
		}

		for code := range getStdinLinesOrArguments(c.Args()) {
			filter := nostr.Filter{}
			var authorHint nostr.PubKey
			relays := c.StringSlice("relay")
			authoritativeFilter := false

			if strings.HasPrefix(code, "http://") || strings.HasPrefix(code, "https://") ||
				isWebAddress(code) {
				if !strings.HasPrefix(code, "http://") && !strings.HasPrefix(code, "https://") {
					code = guessWebScheme(code) + code
				}

				webPath, err := nipad.Resolve(ctx, code)
				if err != nil {
					ctx = lineProcessingError(ctx, "failed to resolve nostr web address %s: %s", code, err)
					continue
				}
				filter = webPath.Filter
				authoritativeFilter = true
				if len(webPath.Filter.Authors) > 0 {
					authorHint = webPath.Filter.Authors[0]
				}
				for _, url := range webPath.Relays {
					relays = append(relays, nostr.NormalizeURL(url))
				}
			} else if nip05.IsValidIdentifier(code) {
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
					filter.IDs = append(filter.IDs, value.(nostr.EventPointer).ID)
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

				for _, url := range sys.FetchOutboxRelays(ctx, authorHint, 6) {
					relays = append(relays, url)
				}
			}

			if err := applyFlagsToFilter(c, &filter); err != nil {
				return err
			}

			// default to fetching just the profile when given only an author,
			// but not when the filter came ready-made from a web address
			if !authoritativeFilter && len(filter.Authors) > 0 && len(filter.Kinds) == 0 {
				filter.Kinds = append(filter.Kinds, 0)
			}

			if len(relays) == 0 {
				ctx = lineProcessingError(ctx, "no relay hints found")
				continue
			}

			found := false
			for ie := range sys.Pool.FetchMany(ctx, relays, filter, nostr.SubscriptionOptions{
				Label: "nak-fetch",
			}) {
				found = true
				var out string
				if jq == nil {
					out = ie.Event.String()
				} else {
					v, matches, err := jq(ie.Event)
					if err != nil {
						return fmt.Errorf("jq filter failed: %w", err)
					}
					if !matches {
						continue
					}
					out = v
				}
				stdout(out)
			}

			if !found {
				ctx = lineProcessingError(ctx, "no events found for %s", code)
			}
		}

		exitIfLineProcessingError(ctx)
		return nil
	},
}

func isWebAddress(code string) bool {
	return strings.Contains(code, "/") || strings.Contains(code, ":")
}

func guessWebScheme(code string) string {
	host, _, _ := strings.Cut(code, "/")
	if strings.HasPrefix(host, "[") {
		// ipv6 literal, keep only the bracketed part
		if i := strings.Index(host, "]"); i != -1 {
			host = host[:i+1]
		}
	} else if h, _, ok := strings.Cut(host, ":"); ok {
		// strip port
		host = h
	}
	if host == "localhost" || host == "127.0.0.1" || host == "[::1]" {
		return "http://"
	}
	return "https://"
}
