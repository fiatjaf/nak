package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mailru/easyjson"
	"github.com/nbd-wtf/go-nostr"
	"github.com/urfave/cli/v2"
)

const CATEGORY_FILTER_ATTRIBUTES = "FILTER ATTRIBUTES"

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
	Flags: []cli.Flag{
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
		&cli.StringFlag{
			Name:     "since",
			Aliases:  []string{"s"},
			Usage:    "only accept events newer than this (unix timestamp)",
			Category: CATEGORY_FILTER_ATTRIBUTES,
		},
		&cli.StringFlag{
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
		&cli.StringFlag{
			Name:     "search",
			Usage:    "a NIP-50 search query, use it only with relays that explicitly support it",
			Category: CATEGORY_FILTER_ATTRIBUTES,
		},
		&cli.BoolFlag{
			Name:        "stream",
			Usage:       "keep the subscription open, printing all events as they are returned",
			DefaultText: "false, will close on EOSE",
		},
		&cli.BoolFlag{
			Name:  "bare",
			Usage: "when printing the filter, print just the filter, not enveloped in a [\"REQ\", ...] array",
		},
		&cli.BoolFlag{
			Name:  "auth",
			Usage: "always perform NIP-42 \"AUTH\" when facing an \"auth-required: \" rejection and try again",
		},
		&cli.StringFlag{
			Name:        "sec",
			Usage:       "secret key to sign the AUTH challenge, as hex or nsec",
			DefaultText: "the key '1'",
			Value:       "0000000000000000000000000000000000000000000000000000000000000001",
		},
		&cli.BoolFlag{
			Name:  "prompt-sec",
			Usage: "prompt the user to paste a hex or nsec with which to sign the AUTH challenge",
		},
		&cli.StringFlag{
			Name:  "connect",
			Usage: "sign AUTH using NIP-46, expects a bunker://... URL",
		},
	},
	ArgsUsage: "[relay...]",
	Action: func(c *cli.Context) error {
		var pool *nostr.SimplePool
		relayUrls := c.Args().Slice()
		if len(relayUrls) > 0 {
			var relays []*nostr.Relay
			pool, relays = connectToAllRelays(c.Context, relayUrls, nostr.WithAuthHandler(func(evt *nostr.Event) error {
				if !c.Bool("auth") {
					return fmt.Errorf("auth not authorized")
				}
				sec, bunker, err := gatherSecretKeyOrBunkerFromArguments(c)
				if err != nil {
					return err
				}

				var pk string
				if bunker != nil {
					pk, err = bunker.GetPublicKey(c.Context)
					if err != nil {
						return fmt.Errorf("failed to get public key from bunker: %w", err)
					}
				} else {
					pk, _ = nostr.GetPublicKey(sec)
				}
				log("performing auth as %s...\n", pk)

				if bunker != nil {
					return bunker.SignEvent(c.Context, evt)
				} else {
					return evt.Sign(sec)
				}
			}))
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
					lineProcessingError(c, "invalid filter '%s' received from stdin: %s", stdinFilter, err)
					continue
				}
			}

			if authors := c.StringSlice("author"); len(authors) > 0 {
				filter.Authors = append(filter.Authors, authors...)
			}
			if ids := c.StringSlice("id"); len(ids) > 0 {
				filter.IDs = append(filter.IDs, ids...)
			}
			if kinds := c.IntSlice("kind"); len(kinds) > 0 {
				filter.Kinds = append(filter.Kinds, kinds...)
			}
			if search := c.String("search"); search != "" {
				filter.Search = search
			}
			tags := make([][]string, 0, 5)
			for _, tagFlag := range c.StringSlice("tag") {
				spl := strings.Split(tagFlag, "=")
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

			if since := c.String("since"); since != "" {
				if since == "now" {
					ts := nostr.Now()
					filter.Since = &ts
				} else if i, err := strconv.Atoi(since); err == nil {
					ts := nostr.Timestamp(i)
					filter.Since = &ts
				} else {
					return fmt.Errorf("parse error: Invalid numeric literal %q", since)
				}
			}
			if until := c.String("until"); until != "" {
				if until == "now" {
					ts := nostr.Now()
					filter.Until = &ts
				} else if i, err := strconv.Atoi(until); err == nil {
					ts := nostr.Timestamp(i)
					filter.Until = &ts
				} else {
					return fmt.Errorf("parse error: Invalid numeric literal %q", until)
				}
			}
			if limit := c.Int("limit"); limit != 0 {
				filter.Limit = limit
			}

			if len(relayUrls) > 0 {
				fn := pool.SubManyEose
				if c.Bool("stream") {
					fn = pool.SubMany
				}
				for ie := range fn(c.Context, relayUrls, nostr.Filters{filter}) {
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

		exitIfLineProcessingError(c)
		return nil
	},
}
