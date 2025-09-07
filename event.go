package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/keyer"
	"fiatjaf.com/nostr/nip13"
	"fiatjaf.com/nostr/nip19"
	"github.com/fatih/color"
	"github.com/mailru/easyjson"
	"github.com/urfave/cli/v3"
)

const (
	CATEGORY_EVENT_FIELDS = "EVENT FIELDS"
	CATEGORY_SIGNER       = "SIGNER OPTIONS"
	CATEGORY_EXTRAS       = "EXTRAS"
)

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
	DisableSliceFlagSeparator: true,
	Flags: append(defaultKeyFlags,
		// ~ these args are only for the convoluted musig2 signing process
		// they will be generally copy-shared-pasted across some manual coordination method between participants
		&cli.UintFlag{
			Name:        "musig",
			Usage:       "number of signers to use for musig2",
			Value:       1,
			DefaultText: "1 -- i.e. do not use musig2 at all",
			Category:    CATEGORY_SIGNER,
		},
		&cli.StringSliceFlag{
			Name:   "musig-pubkey",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "musig-nonce-secret",
			Hidden: true,
		},
		&cli.StringSliceFlag{
			Name:   "musig-nonce",
			Hidden: true,
		},
		&cli.StringSliceFlag{
			Name:   "musig-partial",
			Hidden: true,
		},
		// ~~~
		&cli.UintFlag{
			Name:     "pow",
			Usage:    "nip13 difficulty to target when doing hash work on the event id",
			Category: CATEGORY_EXTRAS,
		},
		&cli.BoolFlag{
			Name:     "envelope",
			Usage:    "print the event enveloped in a [\"EVENT\", ...] message ready to be sent to a relay",
			Category: CATEGORY_EXTRAS,
		},
		&cli.BoolFlag{
			Name:     "auth",
			Usage:    "always perform nip42 \"AUTH\" when facing an \"auth-required: \" rejection and try again",
			Category: CATEGORY_EXTRAS,
		},
		&cli.BoolFlag{
			Name:     "nevent",
			Usage:    "print the nevent code (to stderr) after the event is published",
			Category: CATEGORY_EXTRAS,
		},
		&cli.UintFlag{
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
			Usage:       "event content (if it starts with an '@' will read from a file)",
			DefaultText: "hello from the nostr army knife",
			Value:       "",
			Category:    CATEGORY_EVENT_FIELDS,
		},
		&cli.StringSliceFlag{
			Name:     "tag",
			Aliases:  []string{"t"},
			Usage:    "sets a tag field on the event, takes a value like -t e=<id> or -t sometag=\"value one;value two;value three\"",
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
		&NaturalTimeFlag{
			Name:        "created-at",
			Aliases:     []string{"time", "ts"},
			Usage:       "unix timestamp value for the created_at field",
			DefaultText: "now",
			Value:       nostr.Now(),
			Category:    CATEGORY_EVENT_FIELDS,
		},
		&cli.BoolFlag{
			Name:     "confirm",
			Usage:    "ask before publishing the event",
			Category: CATEGORY_EXTRAS,
		},
	),
	ArgsUsage: "[relay...]",
	Action: func(ctx context.Context, c *cli.Command) error {
		// try to connect to the relays here
		var relays []*nostr.Relay

		if relayUrls := c.Args().Slice(); len(relayUrls) > 0 {
			relays = connectToAllRelays(ctx, c, relayUrls, nil,
				nostr.PoolOptions{
					AuthHandler: func(ctx context.Context, authEvent *nostr.Event) error {
						return authSigner(ctx, c, func(s string, args ...any) {}, authEvent)
					},
				},
			)
			if len(relays) == 0 {
				log("failed to connect to any of the given relays.\n")
				os.Exit(3)
			}
		}
		kr, sec, err := gatherKeyerFromArguments(ctx, c)
		if err != nil {
			return err
		}

		// then process input and generate events:

		// will reuse this
		var evt nostr.Event

		// this is called when we have a valid json from stdin
		handleEvent := func(stdinEvent string) error {
			evt.Content = ""

			kindWasSupplied := strings.Contains(stdinEvent, `"kind"`)
			contentWasSupplied := strings.Contains(stdinEvent, `"content"`)
			mustRehashAndResign := false

			if err := easyjson.Unmarshal([]byte(stdinEvent), &evt); err != nil {
				return fmt.Errorf("invalid event received from stdin: %s", err)
			}

			if kind := c.Uint("kind"); slices.Contains(c.FlagNames(), "kind") {
				evt.Kind = nostr.Kind(kind)
				mustRehashAndResign = true
			} else if !kindWasSupplied {
				evt.Kind = 1
				mustRehashAndResign = true
			}

			if c.IsSet("content") {
				content := c.String("content")
				if strings.HasPrefix(content, "@") {
					filedata, err := os.ReadFile(content[1:])
					if err != nil {
						return fmt.Errorf("failed to read file '%s' for content: %w", content[1:], err)
					}
					evt.Content = string(filedata)
				} else {
					evt.Content = content
				}
				mustRehashAndResign = true
			} else if !contentWasSupplied && evt.Content == "" && evt.Kind == 1 {
				evt.Content = "hello from the nostr army knife"
				mustRehashAndResign = true
			}

			tagFlags := c.StringSlice("tag")
			tags := make(nostr.Tags, 0, len(tagFlags)+2)
			for _, tagFlag := range tagFlags {
				// tags are in the format key=value
				tagName, tagValue, found := strings.Cut(tagFlag, "=")
				tag := []string{tagName}
				if found {
					// tags may also contain extra elements separated with a ";"
					tagValues := strings.Split(tagValue, ";")
					tag = append(tag, tagValues...)
				}
				tags = append(tags, tag)
			}

			for _, etag := range c.StringSlice("e") {
				if tags.FindWithValue("e", etag) == nil {
					tags = append(tags, nostr.Tag{"e", etag})
				}
			}
			for _, ptag := range c.StringSlice("p") {
				if tags.FindWithValue("p", ptag) == nil {
					tags = append(tags, nostr.Tag{"p", ptag})
				}
			}
			for _, dtag := range c.StringSlice("d") {
				if tags.FindWithValue("d", dtag) == nil {
					tags = append(tags, nostr.Tag{"d", dtag})
				}
			}
			if len(tags) > 0 {
				for _, tag := range tags {
					evt.Tags = append(evt.Tags, tag)
				}
				mustRehashAndResign = true
			}

			if c.IsSet("created-at") {
				evt.CreatedAt = getNaturalDate(c, "created-at")
				mustRehashAndResign = true
			} else if evt.CreatedAt == 0 {
				evt.CreatedAt = nostr.Now()
				mustRehashAndResign = true
			}

			if c.IsSet("musig") || c.IsSet("sec") || c.IsSet("prompt-sec") {
				mustRehashAndResign = true
			}

			if difficulty := c.Uint("pow"); difficulty > 0 {
				// before doing pow we need the pubkey
				if numSigners := c.Uint("musig"); numSigners > 1 {
					pubkeys := c.StringSlice("musig-pubkey")
					if int(numSigners) != len(pubkeys) {
						return fmt.Errorf("when doing a pow with musig we must know all signer pubkeys upfront")
					}
					evt.PubKey, err = getMusigAggregatedKey(ctx, pubkeys)
					if err != nil {
						return err
					}
				} else {
					evt.PubKey, _ = kr.GetPublicKey(ctx)
				}

				// try to generate work with this difficulty -- runs forever
				nonceTag, _ := nip13.DoWork(ctx, evt, int(difficulty))
				evt.Tags = append(evt.Tags, nonceTag)

				mustRehashAndResign = true
			}

			if evt.Sig == [64]byte{} || mustRehashAndResign {
				if numSigners := c.Uint("musig"); numSigners > 1 {
					// must do musig
					pubkeys := c.StringSlice("musig-pubkey")
					secNonce := c.String("musig-nonce-secret")
					pubNonces := c.StringSlice("musig-nonce")
					partialSigs := c.StringSlice("musig-partial")
					signed, err := performMusig(ctx,
						sec, &evt, int(numSigners), pubkeys, pubNonces, secNonce, partialSigs)
					if err != nil {
						return fmt.Errorf("musig error: %w", err)
					}
					if !signed {
						// we haven't finished signing the event, so the users still have to do more steps
						// instructions for what to do should have been printed by the performMusig() function
						return nil
					}
				} else if err := kr.SignEvent(ctx, &evt); err != nil {
					if _, isBunker := kr.(keyer.BunkerSigner); isBunker && errors.Is(ctx.Err(), context.DeadlineExceeded) {
						err = fmt.Errorf("timeout waiting for bunker to respond")
					}
					return fmt.Errorf("error signing with provided key: %w", err)
				}
			}

			// print event as json
			var result string
			if c.Bool("envelope") {
				j, _ := json.Marshal(nostr.EventEnvelope{Event: evt})
				result = string(j)
			} else {
				j, _ := easyjson.Marshal(&evt)
				result = string(j)
			}
			stdout(result)

			return publishFlow(ctx, c, kr, evt, relays)
		}

		for stdinEvent := range getJsonsOrBlank() {
			if err := handleEvent(stdinEvent); err != nil {
				ctx = lineProcessingError(ctx, err.Error())
			}
		}

		exitIfLineProcessingError(ctx)
		return nil
	},
}

func publishFlow(ctx context.Context, c *cli.Command, kr nostr.Signer, evt nostr.Event, relays []*nostr.Relay) error {
	doAuth := c.Bool("auth")

	// publish to relays
	successRelays := make([]string, 0, len(relays))
	if len(relays) > 0 {
		os.Stdout.Sync()

		if c.Bool("confirm") {
			relaysStr := make([]string, len(relays))
			for i, r := range relays {
				relaysStr[i] = strings.ToLower(strings.Split(r.URL, "://")[1])
			}
			time.Sleep(time.Millisecond * 10)
			if !askConfirmation("publish to [ " + strings.Join(relaysStr, " ") + " ]? ") {
				return nil
			}
		}

		if supportsDynamicMultilineMagic() {
			// overcomplicated multiline rendering magic
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			urls := make([]string, len(relays))
			lines := make([][][]byte, len(urls))
			flush := func() {
				for _, line := range lines {
					for _, part := range line {
						os.Stderr.Write(part)
					}
					os.Stderr.Write([]byte{'\n'})
				}
			}
			render := func() {
				clearLines(len(lines))
				flush()
			}
			flush()

			logthis := func(relayUrl, s string, args ...any) {
				idx := slices.Index(urls, relayUrl)
				lines[idx] = append(lines[idx], []byte(fmt.Sprintf(s, args...)))
				render()
			}
			colorizethis := func(relayUrl string, colorize func(string, ...any) string) {
				cleanUrl, _ := strings.CutPrefix(relayUrl, "wss://")
				idx := slices.Index(urls, relayUrl)
				lines[idx][0] = []byte(fmt.Sprintf("publishing to %s... ", colorize(cleanUrl)))
				render()
			}

			for i, relay := range relays {
				urls[i] = relay.URL
				lines[i] = make([][]byte, 1, 3)
				colorizethis(relay.URL, color.CyanString)
			}
			render()

			for res := range sys.Pool.PublishMany(ctx, urls, evt) {
				if res.Error == nil {
					colorizethis(res.RelayURL, colors.successf)
					logthis(res.RelayURL, "success.")
					successRelays = append(successRelays, res.RelayURL)
				} else {
					colorizethis(res.RelayURL, colors.errorf)

					// in this case it's likely that the lowest-level error is the one that will be more helpful
					low := unwrapAll(res.Error)

					// hack for some messages such as from relay.westernbtc.com
					msg := strings.ReplaceAll(low.Error(), evt.PubKey.Hex(), "author")

					// do not allow the message to overflow the term window
					msg = clampMessage(msg, 20+len(res.RelayURL))

					logthis(res.RelayURL, msg)
				}
			}
		} else {
			// normal dumb flow
			for i, relay := range relays {
			publish:
				cleanUrl, _ := strings.CutPrefix(relay.URL, "wss://")
				log("publishing to %s... ", color.CyanString(cleanUrl))
				ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()

				if !relay.IsConnected() {
					if new_, err := sys.Pool.EnsureRelay(relay.URL); err == nil {
						relays[i] = new_
						relay = new_
					}
				}

				err := relay.Publish(ctx, evt)
				if err == nil {
					// published fine
					log("success.\n")
					successRelays = append(successRelays, relay.URL)
					continue // continue to next relay
				}

				// error publishing
				if strings.HasPrefix(err.Error(), "msg: auth-required:") && kr != nil && doAuth {
					// if the relay is requesting auth and we can auth, let's do it
					pk, _ := kr.GetPublicKey(ctx)
					npub := nip19.EncodeNpub(pk)
					log("authenticating as %s... ", color.YellowString("%s…%s", npub[0:7], npub[58:]))
					if err := relay.Auth(ctx, kr.SignEvent); err == nil {
						// try to publish again, but this time don't try to auth again
						doAuth = false
						goto publish
					} else {
						log("auth error: %s. ", err)
					}
				}
				log("failed: %s\n", err)
			}
		}

		if len(successRelays) > 0 && c.Bool("nevent") {
			log(nip19.EncodeNevent(evt.ID, successRelays, evt.PubKey) + "\n")
		}
	}

	return nil
}
