package main

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/eventstore"
	"fiatjaf.com/nostr/eventstore/slicestore"
	"fiatjaf.com/nostr/eventstore/wrappers"
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
			&cli.StringFlag{
				Name:      "only-missing",
				Usage:     "use nip77 negentropy to only fetch events that aren't present in the given jsonl file",
				TakesFile: true,
			},
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
				Name:    "outbox-relays-per-pubkey",
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
			&cli.BoolFlag{
				Name:  "spell",
				Usage: "output a spell event (kind 777) instead of a filter",
			},
		)...,
	),
	ArgsUsage: "[relay...]",
	Action: func(ctx context.Context, c *cli.Command) error {
		negentropy := c.Bool("ids-only") || c.IsSet("only-missing")
		if negentropy {
			if c.Bool("paginate") || c.Bool("stream") || c.Bool("outbox") {
				return fmt.Errorf("negentropy is incompatible with --stream, --outbox or --paginate")
			}
		}

		if c.Bool("paginate") && c.Bool("stream") {
			return fmt.Errorf("incompatible flags --paginate and --stream")
		}

		if c.Bool("paginate") && c.Bool("outbox") {
			return fmt.Errorf("incompatible flags --paginate and --outbox")
		}

		if c.Bool("bare") && c.Bool("spell") {
			return fmt.Errorf("incompatible flags --bare and --spell")
		}

		relayUrls := c.Args().Slice()

		if len(relayUrls) > 0 && (c.Bool("bare") || c.Bool("spell")) {
			return fmt.Errorf("relay URLs are incompatible with --bare or --spell")
		}

		if len(relayUrls) > 0 && !negentropy {
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
				if negentropy {
					store := &slicestore.SliceStore{}
					store.Init()

					if syncFile := c.String("only-missing"); syncFile != "" {
						file, err := os.Open(syncFile)
						if err != nil {
							return fmt.Errorf("failed to open sync file: %w", err)
						}
						defer file.Close()
						scanner := bufio.NewScanner(file)
						scanner.Buffer(make([]byte, 16*1024*1024), 256*1024*1024)
						for scanner.Scan() {
							var evt nostr.Event
							if err := easyjson.Unmarshal([]byte(scanner.Text()), &evt); err != nil {
								continue
							}
							if err := store.SaveEvent(evt); err != nil || err == eventstore.ErrDupEvent {
								continue
							}
						}
						if err := scanner.Err(); err != nil {
							return fmt.Errorf("failed to read sync file: %w", err)
						}
					}

					target := PrintingQuerierPublisher{
						QuerierPublisher: wrappers.StorePublisher{Store: store, MaxLimit: math.MaxInt},
					}

					var source nostr.Querier = nil
					if c.IsSet("only-missing") {
						source = target
					}

					handle := nip77.SyncEventsFromIDs

					if c.Bool("ids-only") {
						seen := make(map[nostr.ID]struct{}, max(500, filter.Limit))
						handle = func(ctx context.Context, dir nip77.Direction) {
							for id := range dir.Items {
								if _, ok := seen[id]; ok {
									continue
								}
								seen[id] = struct{}{}
								stdout(id.Hex())
							}
						}
					}

					for _, url := range relayUrls {
						err := nip77.NegentropySync(ctx, url, filter, source, target, handle)
						if err != nil {
							log("negentropy sync from %s failed: %s", url, err)
						}
					}
				} else {
					performReq(ctx, filter, relayUrls, c.Bool("stream"), c.Bool("outbox"), c.Uint("outbox-relays-per-pubkey"), c.Bool("paginate"), c.Duration("paginate-interval"), "nak-req")
				}
			} else {
				// no relays given, will just print the filter or spell
				var result string
				if c.Bool("spell") {
					// output a spell event instead of a filter
					kr, _, err := gatherKeyerFromArguments(ctx, c)
					if err != nil {
						return err
					}
					spellEvent := createSpellEvent(ctx, filter, kr)
					j, _ := json.Marshal(spellEvent)
					result = string(j)
				} else if c.Bool("bare") {
					// bare filter output
					result = filter.String()
				} else {
					// normal filter
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

func performReq(
	ctx context.Context,
	filter nostr.Filter,
	relayUrls []string,
	stream bool,
	outbox bool,
	outboxRelaysPerPubKey uint64,
	paginate bool,
	paginateInterval time.Duration,
	label string,
) {
	var results chan nostr.RelayEvent
	var closeds chan nostr.RelayClosed

	opts := nostr.SubscriptionOptions{
		Label: label,
	}

	if paginate {
		paginator := sys.Pool.PaginatorWithInterval(paginateInterval)
		results = paginator(ctx, relayUrls, filter, opts)
	} else if outbox {
		defs := make([]nostr.DirectedFilter, 0, len(filter.Authors)*2)

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
		logverbose("gathering outbox relays for %d authors...\n", len(filter.Authors))
		for _, pubkey := range filter.Authors {
			errg.Go(func() error {
				n := int(outboxRelaysPerPubKey)
				for _, url := range sys.FetchOutboxRelays(ctx, pubkey, n) {
					if slices.Contains(relayUrls, url) {
						// already specified globally, ignore
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

		if stream {
			logverbose("running subscription with %d directed filters...\n", len(defs))
			results, closeds = sys.Pool.BatchedSubscribeManyNotifyClosed(ctx, defs, opts)
		} else {
			logverbose("running query with %d directed filters...\n", len(defs))
			results, closeds = sys.Pool.BatchedQueryManyNotifyClosed(ctx, defs, opts)
		}
	} else {
		if stream {
			logverbose("running subscription to %d relays...\n", len(relayUrls))
			results, closeds = sys.Pool.SubscribeManyNotifyClosed(ctx, relayUrls, filter, opts)
		} else {
			logverbose("running query to %d relays...\n", len(relayUrls))
			results, closeds = sys.Pool.FetchManyNotifyClosed(ctx, relayUrls, filter, opts)
		}
	}

readevents:
	for {
		select {
		case ie, ok := <-results:
			if !ok {
				break readevents
			}
			stdout(ie.Event)
		case closed := <-closeds:
			if closed.HandledAuth {
				logverbose("%s CLOSED: %s\n", closed.Relay.URL, closed.Reason)
			} else {
				log("%s CLOSED: %s\n", closed.Relay.URL, closed.Reason)
			}
		case <-ctx.Done():
			break readevents
		}
	}
}

var reqFilterFlags = []cli.Flag{
	&PubKeySliceFlag{
		Name:     "author",
		Aliases:  []string{"a"},
		Usage:    "only accept events from these authors",
		Category: CATEGORY_FILTER_ATTRIBUTES,
	},
	&IDSliceFlag{
		Name:     "id",
		Aliases:  []string{"i"},
		Usage:    "only accept events with these ids",
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
			tags = append(tags, []string{spl[0], decodeTagValue(spl[1])})
		} else {
			return fmt.Errorf("invalid --tag '%s'", tagFlag)
		}
	}
	for _, etag := range c.StringSlice("e") {
		tags = append(tags, []string{"e", decodeTagValue(etag)})
	}
	for _, ptag := range c.StringSlice("p") {
		tags = append(tags, []string{"p", decodeTagValue(ptag)})
	}
	for _, dtag := range c.StringSlice("d") {
		tags = append(tags, []string{"d", decodeTagValue(dtag)})
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

type PrintingQuerierPublisher struct {
	nostr.QuerierPublisher
}

func (p PrintingQuerierPublisher) Publish(ctx context.Context, evt nostr.Event) error {
	if err := p.QuerierPublisher.Publish(ctx, evt); err == nil {
		stdout(evt)
		return nil
	} else if err == eventstore.ErrDupEvent {
		return nil
	} else {
		return err
	}
}
