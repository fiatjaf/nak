package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/fiatjaf/cli/v3"
	"github.com/mailru/easyjson"
	"github.com/nbd-wtf/go-nostr"
)

const (
	CATEGORY_FILTER_ATTRIBUTES = "FILTER ATTRIBUTES"
	// CATEGORY_SIGNER            = "SIGNER OPTIONS" -- defined at event.go as the same (yes, I know)
)

var req = &cli.Command{
	Name:  "req",
	Usage: "generates encoded REQ messages and optionally use them to talk to relays",
	Description: `outputs a NIP-01 Nostr filter. when a relay is not given, will print the filter, otherwise will connect to the given relay and send the filter.

example:
		nak req -k 1 -l 15 wss://nostr.wine wss://nostr-pub.wellorder.net
		nak req -k 0 -a 3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d wss://nos.lol | jq '.content | fromjson | .name'

it can also take a filter from stdin, optionally modify it with flags and send it to specific relays (or just print it).

example:
		echo '{"kinds": [1], "#t": ["test"]}' | nak req -l 5 -k 4549 --tag t=spam wss://nostr-pub.wellorder.net`,
	DisableSliceFlagSeparator: true,
	Flags: append(defaultKeyFlags,
		append(reqFilterFlags,
			&cli.BoolFlag{
				Name:        "stream",
				Usage:       "keep the subscription open, printing all events as they are returned",
				DefaultText: "false, will close on EOSE",
			},
			&cli.BoolFlag{
				Name:        "paginate",
				Usage:       "make multiple REQs to the relay decreasing the value of 'until' until 'limit' or 'since' conditions are met",
				DefaultText: "false",
			},
			&cli.DurationFlag{
				Name:  "paginate-interval",
				Usage: "time between queries when using --paginate",
			},
			&cli.UintFlag{
				Name:        "paginate-global-limit",
				Usage:       "global limit at which --paginate should stop",
				DefaultText: "uses the value given by --limit/-l or infinite",
			},
			&cli.BoolFlag{
				Name:  "bare",
				Usage: "when printing the filter, print just the filter, not enveloped in a [\"REQ\", ...] array",
			},
			&cli.BoolFlag{
				Name:  "auth",
				Usage: "always perform NIP-42 \"AUTH\" when facing an \"auth-required: \" rejection and try again",
			},
			&cli.BoolFlag{
				Name:     "force-pre-auth",
				Aliases:  []string{"fpa"},
				Usage:    "after connecting, for a NIP-42 \"AUTH\" message to be received, act on it and only then send the \"REQ\"",
				Category: CATEGORY_SIGNER,
			},
		)...,
	),
	ArgsUsage: "[relay...]",
	Action: func(ctx context.Context, c *cli.Command) error {
		relayUrls := c.Args().Slice()
		if len(relayUrls) > 0 {
			relays := connectToAllRelays(ctx,
				relayUrls,
				c.Bool("force-pre-auth"),
				nostr.WithAuthHandler(
					func(ctx context.Context, authEvent nostr.RelayEvent) error {
						if !c.Bool("auth") && !c.Bool("force-pre-auth") {
							return fmt.Errorf("auth not authorized")
						}
						kr, err := gatherKeyerFromArguments(ctx, c)
						if err != nil {
							return err
						}

						pk, _ := kr.GetPublicKey(ctx)
						log("performing auth as %s... ", pk)

						return kr.SignEvent(ctx, authEvent.Event)
					},
				),
			)
			if len(relays) == 0 {
				log("failed to connect to any of the given relays.\n")
				os.Exit(3)
			}
			relayUrls = make([]string, len(relays))
			for i, relay := range relays {
				relayUrls[i] = relay.URL
			}

			defer func() {
				for _, relay := range relays {
					relay.Close()
				}
			}()
		}

		for stdinFilter := range getStdinLinesOrBlank() {
			filter := nostr.Filter{}
			if stdinFilter != "" {
				if err := easyjson.Unmarshal([]byte(stdinFilter), &filter); err != nil {
					ctx = lineProcessingError(ctx, "invalid filter '%s' received from stdin: %s", stdinFilter, err)
					continue
				}
			}

			if err := applyFlagsToFilter(c, &filter); err != nil {
				return err
			}

			if len(relayUrls) > 0 {
				fn := sys.Pool.SubManyEose
				if c.Bool("paginate") {
					fn = paginateWithParams(c.Duration("paginate-interval"), c.Uint("paginate-global-limit"))
				} else if c.Bool("stream") {
					fn = sys.Pool.SubMany
				}

				for ie := range fn(ctx, relayUrls, nostr.Filters{filter}) {
					stdout(ie.Event)
				}
			} else {
				// no relays given, will just print the filter
				var result string
				if c.Bool("bare") {
					result = filter.String()
				} else {
					j, _ := json.Marshal(nostr.ReqEnvelope{SubscriptionID: "nak", Filters: nostr.Filters{filter}})
					result = string(j)
				}

				stdout(result)
			}
		}

		exitIfLineProcessingError(ctx)
		return nil
	},
}

var reqFilterFlags = []cli.Flag{
	&cli.StringSliceFlag{
		Name:     "author",
		Aliases:  []string{"a"},
		Usage:    "only accept events from these authors (pubkey as hex)",
		Category: CATEGORY_FILTER_ATTRIBUTES,
	},
	&cli.StringSliceFlag{
		Name:     "id",
		Aliases:  []string{"i"},
		Usage:    "only accept events with these ids (hex)",
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
	&cli.StringSliceFlag{
		Name:     "d",
		Usage:    "shortcut for --tag d=<value>",
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
	&cli.UintFlag{
		Name:     "limit",
		Aliases:  []string{"l"},
		Usage:    "only accept up to this number of events",
		Category: CATEGORY_FILTER_ATTRIBUTES,
	},
	&cli.StringFlag{
		Name:     "search",
		Usage:    "a NIP-50 search query, use it only with relays that explicitly support it",
		Category: CATEGORY_FILTER_ATTRIBUTES,
	},
}

func applyFlagsToFilter(c *cli.Command, filter *nostr.Filter) error {
	if authors := c.StringSlice("author"); len(authors) > 0 {
		filter.Authors = append(filter.Authors, authors...)
	}
	if ids := c.StringSlice("id"); len(ids) > 0 {
		filter.IDs = append(filter.IDs, ids...)
	}
	for _, kind64 := range c.IntSlice("kind") {
		filter.Kinds = append(filter.Kinds, int(kind64))
	}
	if search := c.String("search"); search != "" {
		filter.Search = search
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
	for _, dtag := range c.StringSlice("d") {
		tags = append(tags, []string{"d", dtag})
	}

	if len(tags) > 0 && filter.Tags == nil {
		filter.Tags = make(nostr.TagMap)
	}

	for _, tag := range tags {
		if _, ok := filter.Tags[tag[0]]; !ok {
			filter.Tags[tag[0]] = make([]string, 0, 3)
		}
		filter.Tags[tag[0]] = append(filter.Tags[tag[0]], tag[1])
	}

	if c.IsSet("since") {
		nts := getNaturalDate(c, "since")
		filter.Since = &nts
	}

	if c.IsSet("until") {
		nts := getNaturalDate(c, "until")
		filter.Until = &nts
	}

	if limit := c.Uint("limit"); limit != 0 {
		filter.Limit = int(limit)
	} else if c.IsSet("limit") {
		filter.LimitZero = true
	}

	return nil
}
