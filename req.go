package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip42"
	"fiatjaf.com/nostr/nip77"
	"github.com/fatih/color"
	"github.com/mailru/easyjson"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

const (
	CATEGORY_FILTER_ATTRIBUTES = "FILTER ATTRIBUTES"
	// CATEGORY_SIGNER            = "SIGNER OPTIONS" -- defined at event.go as the same (yes, I know)
)

var req = &cli.Command{
	Name:  "req",
	Usage: "generates encoded REQ messages and optionally use them to talk to relays",
	Description: `outputs a nip01 Nostr filter. when a relay is not given, will print the filter, otherwise will connect to the given relay and send the filter.

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
				Name:  "ids-only",
				Usage: "use nip77 to fetch just a list of ids",
			},
			&cli.BoolFlag{
				Name:        "stream",
				Usage:       "keep the subscription open, printing all events as they are returned",
				DefaultText: "false, will close on EOSE",
			},
			&cli.BoolFlag{
				Name:        "outbox",
				Usage:       "use outbox relays from specified public keys",
				DefaultText: "false, will only use manually-specified relays",
			},
			&cli.UintFlag{
				Name:    "outbox-relays-number",
				Aliases: []string{"n"},
				Usage:   "number of outbox relays to use for each pubkey",
				Value:   3,
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
				Usage: "always perform nip42 \"AUTH\" when facing an \"auth-required: \" rejection and try again",
			},
			&cli.BoolFlag{
				Name:     "force-pre-auth",
				Aliases:  []string{"fpa"},
				Usage:    "after connecting, for a nip42 \"AUTH\" message to be received, act on it and only then send the \"REQ\"",
				Category: CATEGORY_SIGNER,
			},
		)...,
	),
	ArgsUsage: "[relay...]",
	Action: func(ctx context.Context, c *cli.Command) error {
		if c.Bool("paginate") && c.Bool("stream") {
			return fmt.Errorf("incompatible flags --paginate and --stream")
		}

		if c.Bool("paginate") && c.Bool("outbox") {
			return fmt.Errorf("incompatible flags --paginate and --outbox")
		}

		relayUrls := c.Args().Slice()
		if len(relayUrls) > 0 {
			// this is used both for the normal AUTH (after "auth-required:" is received) or forced pre-auth
			// connect to all relays we expect to use in this call in parallel
			forcePreAuthSigner := authSigner
			if !c.Bool("force-pre-auth") {
				forcePreAuthSigner = nil
			}
			relays := connectToAllRelays(
				ctx,
				c,
				relayUrls,
				forcePreAuthSigner,
				nostr.PoolOptions{
					AuthHandler: func(ctx context.Context, authEvent *nostr.Event) error {
						return authSigner(ctx, c, func(s string, args ...any) {
							if strings.HasPrefix(s, "authenticating as") {
								cleanUrl, _ := strings.CutPrefix(
									nip42.GetRelayURLFromAuthEvent(*authEvent),
									"wss://",
								)
								s = "authenticating to " + color.CyanString(cleanUrl) + " as" + s[len("authenticating as"):]
							}
							log(s+"\n", args...)
						}, authEvent)
					},
				})

			// stop here already if all connections failed
			if len(relays) == 0 {
				log("failed to connect to any of the given relays.\n")
				os.Exit(3)
			}
			relayUrls = make([]string, len(relays))
			for i, relay := range relays {
				relayUrls[i] = relay.URL
			}
		}

		// go line by line from stdin or run once with input from flags
		for stdinFilter := range getJsonsOrBlank() {
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

			if len(relayUrls) > 0 || c.Bool("outbox") {
				if c.Bool("ids-only") {
					seen := make(map[nostr.ID]struct{}, max(500, filter.Limit))
					for _, url := range relayUrls {
						ch, err := nip77.FetchIDsOnly(ctx, url, filter)
						if err != nil {
							log("negentropy call to %s failed: %s", url, err)
							continue
						}
						for id := range ch {
							if _, ok := seen[id]; ok {
								continue
							}
							seen[id] = struct{}{}
							stdout(id)
						}
					}
				} else {
					var results chan nostr.RelayEvent
					opts := nostr.SubscriptionOptions{
						Label: "nak-req",
					}

					if c.Bool("paginate") {
						paginator := sys.Pool.PaginatorWithInterval(c.Duration("paginate-interval"))
						results = paginator(ctx, relayUrls, filter, opts)
					} else if c.Bool("outbox") {
						defs := make([]nostr.DirectedFilter, 0, len(filter.Authors)*2)

						// hardcoded relays, if any
						for _, relayUrl := range relayUrls {
							defs = append(defs, nostr.DirectedFilter{
								Filter: filter,
								Relay:  relayUrl,
							})
						}

						// relays for each pubkey
						errg := errgroup.Group{}
						errg.SetLimit(16)
						mu := sync.Mutex{}
						for _, pubkey := range filter.Authors {
							errg.Go(func() error {
								n := int(c.Uint("outbox-relays-number"))
								for _, url := range sys.FetchOutboxRelays(ctx, pubkey, n) {
									if slices.Contains(relayUrls, url) {
										// already hardcoded, ignore
										continue
									}
									if !nostr.IsValidRelayURL(url) {
										continue
									}

									matchUrl := func(def nostr.DirectedFilter) bool { return def.Relay == url }
									idx := slices.IndexFunc(defs, matchUrl)
									if idx == -1 {
										// new relay, add it
										mu.Lock()
										// check again after locking to prevent races
										idx = slices.IndexFunc(defs, matchUrl)
										if idx == -1 {
											// then add it
											filter := filter.Clone()
											filter.Authors = []nostr.PubKey{pubkey}
											defs = append(defs, nostr.DirectedFilter{
												Filter: filter,
												Relay:  url,
											})
											mu.Unlock()
											continue // done with this relay url
										}

										// otherwise we'll just use the idx
										mu.Unlock()
									}

									// existing relay, add this pubkey
									defs[idx].Authors = append(defs[idx].Authors, pubkey)
								}

								return nil
							})
						}
						errg.Wait()

						if c.Bool("stream") {
							results = sys.Pool.BatchedSubscribeMany(ctx, defs, opts)
						} else {
							results = sys.Pool.BatchedQueryMany(ctx, defs, opts)
						}
					} else {
						if c.Bool("stream") {
							results = sys.Pool.SubscribeMany(ctx, relayUrls, filter, opts)
						} else {
							results = sys.Pool.FetchMany(ctx, relayUrls, filter, opts)
						}
					}

					for ie := range results {
						stdout(ie.Event)
					}
				}
			} else {
				// no relays given, will just print the filter
				var result string
				if c.Bool("bare") {
					result = filter.String()
				} else {
					j, _ := json.Marshal(nostr.ReqEnvelope{SubscriptionID: "nak", Filters: []nostr.Filter{filter}})
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
	&PubKeySliceFlag{
		Name:     "author",
		Aliases:  []string{"a"},
		Usage:    "only accept events from these authors (pubkey as hex)",
		Category: CATEGORY_FILTER_ATTRIBUTES,
	},
	&IDSliceFlag{
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
		Usage:    "a nip50 search query, use it only with relays that explicitly support it",
		Category: CATEGORY_FILTER_ATTRIBUTES,
	},
}

func applyFlagsToFilter(c *cli.Command, filter *nostr.Filter) error {
	if authors := getPubKeySlice(c, "author"); len(authors) > 0 {
		filter.Authors = append(filter.Authors, authors...)
	}
	if ids := getIDSlice(c, "id"); len(ids) > 0 {
		filter.IDs = append(filter.IDs, ids...)
	}
	for _, kind64 := range c.IntSlice("kind") {
		filter.Kinds = append(filter.Kinds, nostr.Kind(kind64))
	}
	if search := c.String("search"); search != "" {
		filter.Search = search
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
		filter.Since = getNaturalDate(c, "since")
	}
	if c.IsSet("until") {
		filter.Until = getNaturalDate(c, "until")
	}

	if limit := c.Uint("limit"); limit != 0 {
		filter.Limit = int(limit)
	} else if c.IsSet("limit") {
		filter.LimitZero = true
	}

	return nil
}
