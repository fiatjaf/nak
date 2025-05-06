package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/sdk"
	"github.com/urfave/cli/v3"
)

var publish = &cli.Command{
	Name:  "publish",
	Usage: "publishes a note with content from stdin",
	Description: `reads content from stdin and publishes it as a note, optionally as a reply to another note.

example:
	echo "hello world" | nak publish
	echo "I agree!" | nak publish --reply nevent1...
	echo "tagged post" | nak publish -t t=mytag -t e=someeventid`,
	DisableSliceFlagSeparator: true,
	Flags: append(defaultKeyFlags,
		&cli.StringFlag{
			Name:  "reply",
			Usage: "event id, naddr1 or nevent1 code to reply to",
		},
		&cli.StringSliceFlag{
			Name:    "tag",
			Aliases: []string{"t"},
			Usage:   "sets a tag field on the event, takes a value like -t e=<id> or -t sometag=\"value one;value two;value three\"",
		},
		&NaturalTimeFlag{
			Name:        "created-at",
			Aliases:     []string{"time", "ts"},
			Usage:       "unix timestamp value for the created_at field",
			DefaultText: "now",
			Value:       nostr.Now(),
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
		&cli.BoolFlag{
			Name:     "confirm",
			Usage:    "ask before publishing the event",
			Category: CATEGORY_EXTRAS,
		},
	),
	Action: func(ctx context.Context, c *cli.Command) error {
		content, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read from stdin: %w", err)
		}

		evt := nostr.Event{
			Kind:      1,
			Content:   strings.TrimSpace(string(content)),
			Tags:      make(nostr.Tags, 0, 4),
			CreatedAt: nostr.Now(),
		}

		// handle timestamp flag
		if c.IsSet("created-at") {
			evt.CreatedAt = getNaturalDate(c, "created-at")
		}

		// handle reply flag
		var replyRelays []string
		if replyTo := c.String("reply"); replyTo != "" {
			var replyEvent *nostr.Event

			// try to decode as nevent or naddr first
			if strings.HasPrefix(replyTo, "nevent1") || strings.HasPrefix(replyTo, "naddr1") {
				_, value, err := nip19.Decode(replyTo)
				if err != nil {
					return fmt.Errorf("invalid reply target: %w", err)
				}

				switch pointer := value.(type) {
				case nostr.EventPointer:
					replyEvent, _, err = sys.FetchSpecificEvent(ctx, pointer, sdk.FetchSpecificEventParameters{})
				case nostr.EntityPointer:
					replyEvent, _, err = sys.FetchSpecificEvent(ctx, pointer, sdk.FetchSpecificEventParameters{})
				}
				if err != nil {
					return fmt.Errorf("failed to fetch reply target event: %w", err)
				}
			} else {
				// try as raw event ID
				id, err := nostr.IDFromHex(replyTo)
				if err != nil {
					return fmt.Errorf("invalid event id: %w", err)
				}
				replyEvent, _, err = sys.FetchSpecificEvent(ctx, nostr.EventPointer{ID: id}, sdk.FetchSpecificEventParameters{})
				if err != nil {
					return fmt.Errorf("failed to fetch reply target event: %w", err)
				}
			}

			if replyEvent.Kind != 1 {
				evt.Kind = 1111
			}

			// add reply tags
			evt.Tags = append(evt.Tags,
				nostr.Tag{"e", replyEvent.ID.Hex(), "", "reply"},
				nostr.Tag{"p", replyEvent.PubKey.Hex()},
			)

			replyRelays = sys.FetchInboxRelays(ctx, replyEvent.PubKey, 3)
		}

		// handle other tags -- copied from event.go
		tagFlags := c.StringSlice("tag")
		for _, tagFlag := range tagFlags {
			// tags are in the format key=value
			tagName, tagValue, found := strings.Cut(tagFlag, "=")
			tag := []string{tagName}
			if found {
				// tags may also contain extra elements separated with a ";"
				tagValues := strings.Split(tagValue, ";")
				tag = append(tag, tagValues...)
			}
			evt.Tags = append(evt.Tags, tag)
		}

		// process the content
		targetRelays := sys.PrepareNoteEvent(ctx, &evt)

		// connect to all the relays (like event.go)
		kr, _, err := gatherKeyerFromArguments(ctx, c)
		if err != nil {
			return err
		}
		pk, err := kr.GetPublicKey(ctx)
		if err != nil {
			return fmt.Errorf("failed to get our public key: %w", err)
		}

		relayUrls := sys.FetchWriteRelays(ctx, pk)
		relayUrls = nostr.AppendUnique(relayUrls, targetRelays...)
		relayUrls = nostr.AppendUnique(relayUrls, replyRelays...)
		relayUrls = nostr.AppendUnique(relayUrls, c.Args().Slice()...)
		relays := connectToAllRelays(ctx, c, relayUrls, nil,
			nostr.PoolOptions{
				AuthHandler: func(ctx context.Context, authEvent *nostr.Event) error {
					return authSigner(ctx, c, func(s string, args ...any) {}, authEvent)
				},
			},
		)

		if len(relays) == 0 {
			if len(relayUrls) == 0 {
				return fmt.Errorf("no relays to publish this note to.")
			} else {
				return fmt.Errorf("failed to connect to any of [ %v ].", relayUrls)
			}
		}

		// sign the event
		if err := kr.SignEvent(ctx, &evt); err != nil {
			return fmt.Errorf("error signing event: %w", err)
		}

		// print
		stdout(evt.String())

		// publish (like event.go)
		return publishFlow(ctx, c, kr, evt, relays)
	},
}
