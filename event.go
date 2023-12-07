package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mailru/easyjson"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nson"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/slices"
)

const CATEGORY_EVENT_FIELDS = "EVENT FIELDS"

var event = &cli.Command{
	Name:  "event",
	Usage: "generates an encoded event and either prints it or sends it to a set of relays",
	Description: `outputs an event built with the flags. if one or more relays are given as arguments, an attempt is also made to publish the event to these relays.

example:
		nak event -c hello wss://nos.lol
		nak event -k 3 -p 3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d

if an event -- or a partial event -- is given on stdin, the flags can be used to optionally modify it. if it is modified it is rehashed and resigned, otherwise it is just returned as given, but that can be used to just publish to relays.

example:
		echo '{"id":"a889df6a387419ff204305f4c2d296ee328c3cd4f8b62f205648a541b4554dfb","pubkey":"c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5","created_at":1698623783,"kind":1,"tags":[],"content":"hello from the nostr army knife","sig":"84876e1ee3e726da84e5d195eb79358b2b3eaa4d9bd38456fde3e8a2af3f1cd4cda23f23fda454869975b3688797d4c66e12f4c51c1b43c6d2997c5e61865661"}' | nak event wss://offchain.pub
		echo '{"tags": [["t", "spam"]]}' | nak event -c 'this is spam'`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "sec",
			Usage:       "secret key to sign the event, as hex or nsec",
			DefaultText: "the key '1'",
			Value:       "0000000000000000000000000000000000000000000000000000000000000001",
		},
		&cli.BoolFlag{
			Name:  "prompt-sec",
			Usage: "prompt the user to paste a hex or nsec with which to sign the event",
		},
		&cli.BoolFlag{
			Name:  "envelope",
			Usage: "print the event enveloped in a [\"EVENT\", ...] message ready to be sent to a relay",
		},
		&cli.BoolFlag{
			Name:  "auth",
			Usage: "always perform NIP-42 \"AUTH\" when facing an \"auth-required: \" rejection and try again",
		},
		&cli.BoolFlag{
			Name:  "nson",
			Usage: "encode the event using NSON",
		},
		&cli.IntFlag{
			Name:        "kind",
			Aliases:     []string{"k"},
			Usage:       "event kind",
			DefaultText: "1",
			Value:       0,
			Category:    CATEGORY_EVENT_FIELDS,
		},
		&cli.StringFlag{
			Name:        "content",
			Aliases:     []string{"c"},
			Usage:       "event content",
			DefaultText: "hello from the nostr army knife",
			Value:       "",
			Category:    CATEGORY_EVENT_FIELDS,
		},
		&cli.StringSliceFlag{
			Name:     "tag",
			Aliases:  []string{"t"},
			Usage:    "sets a tag field on the event, takes a value like -t e=<id>",
			Category: CATEGORY_EVENT_FIELDS,
		},
		&cli.StringSliceFlag{
			Name:     "e",
			Usage:    "shortcut for --tag e=<value>",
			Category: CATEGORY_EVENT_FIELDS,
		},
		&cli.StringSliceFlag{
			Name:     "p",
			Usage:    "shortcut for --tag p=<value>",
			Category: CATEGORY_EVENT_FIELDS,
		},
		&cli.StringFlag{
			Name:        "created-at",
			Aliases:     []string{"time", "ts"},
			Usage:       "unix timestamp value for the created_at field",
			DefaultText: "now",
			Value:       "",
			Category:    CATEGORY_EVENT_FIELDS,
		},
	},
	ArgsUsage: "[relay...]",
	Action: func(c *cli.Context) error {
		// try to connect to the relays here
		var relays []*nostr.Relay
		if relayUrls := c.Args().Slice(); len(relayUrls) > 0 {
			_, relays = connectToAllRelays(c.Context, relayUrls)
			if len(relays) == 0 {
				log("failed to connect to any of the given relays.\n")
				os.Exit(3)
			}
		}

		// gather the secret key
		sec, err := gatherSecretKeyFromArguments(c)
		if err != nil {
			return err
		}

		doAuth := c.Bool("auth")

		// then process input and generate events
	nextline:
		for stdinEvent := range getStdinLinesOrBlank() {
			evt := nostr.Event{
				Tags: make(nostr.Tags, 0, 3),
			}

			kindWasSupplied := false
			mustRehashAndResign := false

			if stdinEvent != "" {
				if err := easyjson.Unmarshal([]byte(stdinEvent), &evt); err != nil {
					lineProcessingError(c, "invalid event received from stdin: %s", err)
					continue
				}
				kindWasSupplied = strings.Contains(stdinEvent, `"kind"`)
			}

			if kind := c.Int("kind"); slices.Contains(c.FlagNames(), "kind") {
				evt.Kind = kind
				mustRehashAndResign = true
			} else if !kindWasSupplied {
				evt.Kind = 1
				mustRehashAndResign = true
			}

			if content := c.String("content"); content != "" {
				evt.Content = content
				mustRehashAndResign = true
			} else if evt.Content == "" && evt.Kind == 1 {
				evt.Content = "hello from the nostr army knife"
				mustRehashAndResign = true
			}

			tags := make(nostr.Tags, 0, 5)
			for _, tagFlag := range c.StringSlice("tag") {
				// tags are in the format key=value
				spl := strings.Split(tagFlag, "=")
				if len(spl) == 2 && len(spl[0]) > 0 {
					tag := nostr.Tag{spl[0]}
					// tags may also contain extra elements separated with a ";"
					spl2 := strings.Split(spl[1], ";")
					tag = append(tag, spl2...)
					// ~
					tags = append(tags, tag)
				}
			}
			for _, etag := range c.StringSlice("e") {
				tags = append(tags, []string{"e", etag})
				mustRehashAndResign = true
			}
			for _, ptag := range c.StringSlice("p") {
				tags = append(tags, []string{"p", ptag})
				mustRehashAndResign = true
			}
			if len(tags) > 0 {
				for _, tag := range tags {
					evt.Tags = append(evt.Tags, tag)
				}
				mustRehashAndResign = true
			}

			if createdAt := c.String("created-at"); createdAt != "" {
				ts := time.Now()
				if createdAt != "now" {
					if v, err := strconv.ParseInt(createdAt, 10, 64); err != nil {
						return fmt.Errorf("failed to parse timestamp '%s': %w", createdAt, err)
					} else {
						ts = time.Unix(v, 0)
					}
				}
				evt.CreatedAt = nostr.Timestamp(ts.Unix())
				mustRehashAndResign = true
			} else if evt.CreatedAt == 0 {
				evt.CreatedAt = nostr.Now()
				mustRehashAndResign = true
			}

			if evt.Sig == "" || mustRehashAndResign {
				if err := evt.Sign(sec); err != nil {
					return fmt.Errorf("error signing with provided key: %w", err)
				}
			}

			// print event as json
			var result string
			if c.Bool("envelope") {
				j, _ := json.Marshal(nostr.EventEnvelope{Event: evt})
				result = string(j)
			} else if c.Bool("nson") {
				result, _ = nson.Marshal(&evt)
			} else {
				j, _ := easyjson.Marshal(&evt)
				result = string(j)
			}
			fmt.Println(result)

			// publish to relays
			if len(relays) > 0 {
				os.Stdout.Sync()
				for _, relay := range relays {
					log("publishing to %s... ", relay.URL)
					if relay, err := nostr.RelayConnect(c.Context, relay.URL); err != nil {
						log("failed to connect: %s\n", err)
					} else {
					publish:
						ctx, cancel := context.WithTimeout(c.Context, 10*time.Second)
						defer cancel()

						status, err := relay.Publish(ctx, evt)
						if err == nil {
							// published fine probably
							log("%s.\n", status)
							continue nextline
						}

						// error publishing
						if isAuthRequired(err.Error()) && sec != "" && doAuth {
							// if the relay is requesting auth and we can auth, let's do it
							log("performing auth... ")
							st, err := relay.Auth(c.Context, func(evt *nostr.Event) error { return evt.Sign(sec) })
							if st == nostr.PublishStatusSucceeded {
								// try to publish again, but this time don't try to auth again
								doAuth = false
								goto publish
							} else {
								// auth error
								if err == nil {
									err = fmt.Errorf("no response from relay")
								}
								log("auth error: %s. ", err)
							}
						}
						log("failed: %s\n", err)
					}
				}
			}
		}

		exitIfLineProcessingError(c)
		return nil
	},
}
