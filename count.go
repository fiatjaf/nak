package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip45"
	"fiatjaf.com/nostr/nip45/hyperloglog"
	"github.com/urfave/cli/v3"
)

var count = &cli.Command{
	Name:                      "count",
	Usage:                     "generates encoded COUNT messages and optionally use them to talk to relays",
	Description:               `outputs a nip45 request (the flags are mostly the same as 'nak req').`,
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&PubKeySliceFlag{
			Name:     "author",
			Aliases:  []string{"a"},
			Usage:    "only accept events from these authors (pubkey as hex)",
			Category: CATEGORY_FILTER_ATTRIBUTES,
		},
		&cli.IntSliceFlag{
			Name:     "kind",
			Aliases:  []string{"k"},
			Usage:    "only accept events with these kind numbers",
			Category: CATEGORY_FILTER_ATTRIBUTES,
		},
		&cli.StringSliceFlag{
			Name:     "tag",
			Aliases:  []string{"t"},
			Usage:    "takes a tag like -t e=<id>, only accept events with these tags",
			Category: CATEGORY_FILTER_ATTRIBUTES,
		},
		&cli.StringSliceFlag{
			Name:     "e",
			Usage:    "shortcut for --tag e=<value>",
			Category: CATEGORY_FILTER_ATTRIBUTES,
		},
		&cli.StringSliceFlag{
			Name:     "p",
			Usage:    "shortcut for --tag p=<value>",
			Category: CATEGORY_FILTER_ATTRIBUTES,
		},
		&NaturalTimeFlag{
			Name:     "since",
			Aliases:  []string{"s"},
			Usage:    "only accept events newer than this (unix timestamp)",
			Category: CATEGORY_FILTER_ATTRIBUTES,
		},
		&NaturalTimeFlag{
			Name:     "until",
			Aliases:  []string{"u"},
			Usage:    "only accept events older than this (unix timestamp)",
			Category: CATEGORY_FILTER_ATTRIBUTES,
		},
		&cli.IntFlag{
			Name:     "limit",
			Aliases:  []string{"l"},
			Usage:    "only accept up to this number of events",
			Category: CATEGORY_FILTER_ATTRIBUTES,
		},
	},
	ArgsUsage: "[relay...]",
	Action: func(ctx context.Context, c *cli.Command) error {
		biggerUrlSize := 0
		relayUrls := c.Args().Slice()
		if len(relayUrls) > 0 {
			relays := connectToAllRelays(ctx, c, relayUrls, nil, nostr.PoolOptions{})
			if len(relays) == 0 {
				log("failed to connect to any of the given relays.\n")
				os.Exit(3)
			}
			relayUrls = make([]string, len(relays))
			for i, relay := range relays {
				relayUrls[i] = relay.URL
				if len(relay.URL) > biggerUrlSize {
					biggerUrlSize = len(relay.URL)
				}
			}
		}

		filter := nostr.Filter{}

		if authors := getPubKeySlice(c, "author"); len(authors) > 0 {
			filter.Authors = authors
		}
		if kinds64 := c.IntSlice("kind"); len(kinds64) > 0 {
			kinds := make([]nostr.Kind, len(kinds64))
			for i, v := range kinds64 {
				kinds[i] = nostr.Kind(v)
			}
			filter.Kinds = kinds
		}

		tags := make([][]string, 0, 5)
		for _, tagFlag := range c.StringSlice("tag") {
			spl := strings.SplitN(tagFlag, "=", 2)
			if len(spl) == 2 {
				tags = append(tags, spl)
			} else {
				return fmt.Errorf("invalid --tag '%s'", tagFlag)
			}
		}
		for _, etag := range c.StringSlice("e") {
			tags = append(tags, []string{"e", etag})
		}
		for _, ptag := range c.StringSlice("p") {
			tags = append(tags, []string{"p", ptag})
		}
		if len(tags) > 0 {
			filter.Tags = make(nostr.TagMap)
			for _, tag := range tags {
				if _, ok := filter.Tags[tag[0]]; !ok {
					filter.Tags[tag[0]] = make([]string, 0, 3)
				}
				filter.Tags[tag[0]] = append(filter.Tags[tag[0]], tag[1])
			}
		}

		if c.IsSet("since") {
			filter.Since = getNaturalDate(c, "since")
		}
		if c.IsSet("until") {
			filter.Until = getNaturalDate(c, "until")
		}

		if limit := c.Int("limit"); limit != 0 {
			filter.Limit = int(limit)
		}

		successes := 0
		if len(relayUrls) > 0 {
			var hll *hyperloglog.HyperLogLog
			if offset := nip45.HyperLogLogEventPubkeyOffsetForFilter(filter); offset != -1 && len(relayUrls) > 1 {
				hll = hyperloglog.New(offset)
			}
			for _, relayUrl := range relayUrls {
				relay, _ := sys.Pool.EnsureRelay(relayUrl)
				count, hllRegisters, err := relay.Count(ctx, filter, nostr.SubscriptionOptions{
					Label: "nak-count",
				})
				fmt.Fprintf(os.Stderr, "%s%s: ", strings.Repeat(" ", biggerUrlSize-len(relayUrl)), relayUrl)

				if err != nil {
					fmt.Fprintf(os.Stderr, "❌ %s\n", err)
					continue
				}

				var hasHLLStr string
				if hll != nil && len(hllRegisters) == 256 {
					hll.MergeRegisters(hllRegisters)
					hasHLLStr = " 📋"
				}

				fmt.Fprintf(os.Stderr, "%d%s\n", count, hasHLLStr)
				successes++
			}
			if successes == 0 {
				return fmt.Errorf("all relays have failed")
			} else if hll != nil {
				fmt.Fprintf(os.Stderr, "📋 HyperLogLog sum: %d\n", hll.Count())
			}
		} else {
			// no relays given, will just print the filter
			var result string
			j, _ := json.Marshal([]any{"COUNT", "nak", filter})
			result = string(j)
			stdout(result)
		}

		return nil
	},
}
