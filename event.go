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
	"github.com/nbd-wtf/go-nostr/nip19"
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
		&cli.StringFlag{
			Name:  "connect",
			Usage: "sign event using NIP-46, expects a bunker://... URL",
		},
		&cli.StringFlag{
			Name:        "connect-as",
			Usage:       "private key to when communicating with the bunker given on --connect",
			DefaultText: "a random key",
		},
		// ~ these args are only for the convoluted musig2 signing process
		// they will be generally copy-shared-pasted across some manual coordination method between participants
		&cli.UintFlag{
			Name:        "musig2",
			Usage:       "number of signers to use for musig2",
			Value:       1,
			DefaultText: "1 -- i.e. do not use musig2 at all",
		},
		&cli.StringSliceFlag{
			Name:   "musig2-pubkey",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "musig2-nonce-secret",
			Hidden: true,
		},
		&cli.StringSliceFlag{
			Name:   "musig2-nonce",
			Hidden: true,
		},
		&cli.StringSliceFlag{
			Name:   "musig2-partial",
			Hidden: true,
		},
		// ~~~
		&cli.BoolFlag{
			Name:  "envelope",
			Usage: "print the event enveloped in a [\"EVENT\", ...] message ready to be sent to a relay",
		},
		&cli.BoolFlag{
			Name:  "auth",
			Usage: "always perform NIP-42 \"AUTH\" when facing an \"auth-required: \" rejection and try again",
		},
		&cli.BoolFlag{
			Name:  "nevent",
			Usage: "print the nevent code (to stderr) after the event is published",
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
		&cli.StringSliceFlag{
			Name:     "d",
			Usage:    "shortcut for --tag d=<value>",
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

		defer func() {
			for _, relay := range relays {
				relay.Close()
			}
		}()

		sec, bunker, err := gatherSecretKeyOrBunkerFromArguments(c)
		if err != nil {
			return err
		}

		doAuth := c.Bool("auth")

		// then process input and generate events
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
				tagName, tagValue, found := strings.Cut(tagFlag, "=")
				tag := []string{tagName}
				if found {
					// tags may also contain extra elements separated with a ";"
					tagValues := strings.Split(tagValue, ";")
					tag = append(tag, tagValues...)
				}
				tags = tags.AppendUnique(tag)
			}

			for _, etag := range c.StringSlice("e") {
				tags = tags.AppendUnique([]string{"e", etag})
				mustRehashAndResign = true
			}
			for _, ptag := range c.StringSlice("p") {
				tags = tags.AppendUnique([]string{"p", ptag})
				mustRehashAndResign = true
			}
			for _, dtag := range c.StringSlice("d") {
				tags = tags.AppendUnique([]string{"d", dtag})
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
				if bunker != nil {
					if err := bunker.SignEvent(c.Context, &evt); err != nil {
						return fmt.Errorf("failed to sign with bunker: %w", err)
					}
				} else if numSigners := c.Uint("musig2"); numSigners > 1 && sec != "" {
					pubkeys := c.StringSlice("musig2-pubkey")
					secNonce := c.String("musig2-nonce-secret")
					pubNonces := c.StringSlice("musig2-nonce")
					partialSigs := c.StringSlice("musig2-partial")
					signed, err := performMusig(c.Context,
						sec, &evt, int(numSigners), pubkeys, pubNonces, secNonce, partialSigs)
					if err != nil {
						return fmt.Errorf("musig2 error: %w", err)
					}
					if !signed {
						// we haven't finished signing the event, so the users still have to do more steps
						// instructions for what to do should have been printed by the performMusig() function
						return nil
					}
				} else if err := evt.Sign(sec); err != nil {
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
			stdout(result)

			// publish to relays
			successRelays := make([]string, 0, len(relays))
			if len(relays) > 0 {
				os.Stdout.Sync()
				for _, relay := range relays {
				publish:
					log("publishing to %s... ", relay.URL)
					ctx, cancel := context.WithTimeout(c.Context, 10*time.Second)
					defer cancel()

					err := relay.Publish(ctx, evt)
					if err == nil {
						// published fine
						log("success.\n")
						successRelays = append(successRelays, relay.URL)
						continue // continue to next relay
					}

					// error publishing
					if strings.HasPrefix(err.Error(), "msg: auth-required:") && (sec != "" || bunker != nil) && doAuth {
						// if the relay is requesting auth and we can auth, let's do it
						var pk string
						if bunker != nil {
							pk, err = bunker.GetPublicKey(c.Context)
							if err != nil {
								return fmt.Errorf("failed to get public key from bunker: %w", err)
							}
						} else {
							pk, _ = nostr.GetPublicKey(sec)
						}
						log("performing auth as %s... ", pk)
						if err := relay.Auth(c.Context, func(evt *nostr.Event) error {
							if bunker != nil {
								return bunker.SignEvent(c.Context, evt)
							}
							return evt.Sign(sec)
						}); err == nil {
							// try to publish again, but this time don't try to auth again
							doAuth = false
							goto publish
						} else {
							log("auth error: %s. ", err)
						}
					}
					log("failed: %s\n", err)
				}
				if len(successRelays) > 0 && c.Bool("nevent") {
					nevent, _ := nip19.EncodeEvent(evt.ID, successRelays, evt.PubKey)
					log(nevent + "\n")
				}
			}
		}

		exitIfLineProcessingError(c)
		return nil
	},
}
