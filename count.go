package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/fiatjaf/cli/v3"
	"github.com/nbd-wtf/go-nostr"
)

var count = &cli.Command{
	Name:                      "count",
	Usage:                     "generates encoded COUNT messages and optionally use them to talk to relays",
	Description:               `outputs a NIP-45 request (the flags are mostly the same as 'nak req').`,
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
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
		&cli.IntFlag{
			Name:     "since",
			Aliases:  []string{"s"},
			Usage:    "only accept events newer than this (unix timestamp)",
			Category: CATEGORY_FILTER_ATTRIBUTES,
		},
		&cli.IntFlag{
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
		filter := nostr.Filter{}

		if authors := c.StringSlice("author"); len(authors) > 0 {
			filter.Authors = authors
		}
		if ids := c.StringSlice("id"); len(ids) > 0 {
			filter.IDs = ids
		}
		if kinds64 := c.IntSlice("kind"); len(kinds64) > 0 {
			kinds := make([]int, len(kinds64))
			for i, v := range kinds64 {
				kinds[i] = int(v)
			}
			filter.Kinds = kinds
		}

		tags := make([][]string, 0, 5)
		for _, tagFlag := range c.StringSlice("tag") {
			spl := strings.SplitN(tagFlag, "=", 2)
			if len(spl) == 2 && len(spl[0]) == 1 {
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

		if since := c.Int("since"); since != 0 {
			ts := nostr.Timestamp(since)
			filter.Since = &ts
		}
		if until := c.Int("until"); until != 0 {
			ts := nostr.Timestamp(until)
			filter.Until = &ts
		}
		if limit := c.Int("limit"); limit != 0 {
			filter.Limit = int(limit)
		}

		relays := c.Args().Slice()
		successes := 0
		failures := make([]error, 0, len(relays))
		if len(relays) > 0 {
			for _, relayUrl := range relays {
				relay, err := nostr.RelayConnect(ctx, relayUrl)
				if err != nil {
					failures = append(failures, err)
					continue
				}
				count, err := relay.Count(ctx, nostr.Filters{filter})
				if err != nil {
					failures = append(failures, err)
					continue
				}
				fmt.Printf("%s: %d\n", relay.URL, count)
				successes++
			}
			if successes == 0 {
				return errors.Join(failures...)
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
