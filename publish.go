package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/sdk"
	"github.com/urfave/cli/v3"
)

var publish = &cli.Command{
	Name:  "publish",
	Usage: "publishes a note with content from stdin or argument",
	Description: `reads content and publishes it as a note, optionally as a reply to another note.
Either pipe content or pass it as the first argument.

example:
	echo "hello world" | nak publish
	nak publish "hello world"
	echo "I agree!" | nak publish --reply nevent1...
	nak publish -t t=mytag -t e=someeventid "tagged post"`,
	DisableSliceFlagSeparator: true,
	Flags: combineFlags([][]cli.Flag{},
		&cli.StringSliceFlag{
			Name:    "relay",
			Aliases: []string{"r"},
			Usage:   "also publish to these relays",
		},
		&cli.StringFlag{
			Name:  "reply",
			Usage: "event id, naddr1 or nevent1 code to reply to",
		},
		&cli.StringFlag{
			Name:  "comment",
			Usage: "event id, naddr1 or nevent1 code to comment on (NIP-22 kind 1111, takes precedence over --reply)",
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
		var content []byte
		if isPiped() {
			var err error
			content, err = io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read from stdin: %w", err)
			}
		} else if firstArg := c.Args().First(); firstArg != "" {
			content = []byte(firstArg)
		} else {
			return fmt.Errorf("no content. pipe or pass content as argument.")
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

		// handle reply/comment flag
		// --comment takes precedence over --reply
		target := c.String("comment")
		isComment := c.IsSet("comment")
		if target == "" {
			target = c.String("reply")
		}
		var replyRelays []string
		if target != "" {
			replyEvent, relayHints, err := sys.FetchSpecificEventFromInput(ctx, target, sdk.FetchSpecificEventParameters{})
			if err != nil {
				return fmt.Errorf("failed to fetch reply target event: %w", err)
			}

			if isComment || replyEvent.Kind != 1 {
				// NIP-22 comment
				evt.Kind = 1111
				buildNIP22Tags(&evt, replyEvent, relayHints)
			} else {
				// NIP-10 reply
				if replyEvent.Kind != 1 {
					evt.Kind = 1111
					evt.Tags = append(evt.Tags, nostr.Tag{"K", fmt.Sprint(replyEvent.Kind)})
				}

				// add reply tags per NIP-10
				rootID := ""
				rootRelay := ""
				for _, tag := range replyEvent.Tags {
					if len(tag) >= 2 && tag[0] == "e" {
						if len(tag) >= 4 && tag[3] == "root" {
							rootID = tag[1]
							if len(tag) >= 3 {
								rootRelay = tag[2]
							}
							break
						}
						if rootID == "" {
							rootID = tag[1]
							if len(tag) >= 3 {
								rootRelay = tag[2]
							}
						}
					}
				}
				if rootID == "" {
					// replying to root event
					evt.Tags = append(evt.Tags,
						nostr.Tag{"e", replyEvent.ID.Hex(), "", "root"},
					)
				} else {
					// replying to a reply
					evt.Tags = append(evt.Tags,
						nostr.Tag{"e", rootID, rootRelay, "root"},
						nostr.Tag{"e", replyEvent.ID.Hex(), "", "reply"},
					)
				}
				evt.Tags = append(evt.Tags,
					nostr.Tag{"p", replyEvent.PubKey.Hex()},
				)
			}

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
		relayUrls = nostr.AppendUnique(relayUrls, c.StringSlice("relay")...)

		relays := connectToAllRelays(ctx, c, relayUrls)

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

func buildNIP22Tags(evt *nostr.Event, target *nostr.Event, relayHints []string) {
	var relayHint string
	if len(relayHints) > 0 {
		relayHint = relayHints[0]
	}

	dtag := func() string {
		for _, tag := range target.Tags {
			if len(tag) >= 2 && tag[0] == "d" {
				return tag[1]
			}
		}
		return ""
	}

	// find root scope in target's uppercase NIP-22 tags
	var rootScopeName, rootScopeValue, rootScopeRelay, rootKind, rootPubkey string
	for _, tag := range target.Tags {
		if len(tag) >= 2 && (tag[0] == "E" || tag[0] == "A" || tag[0] == "I") {
			rootScopeName = tag[0]
			rootScopeValue = tag[1]
			if len(tag) >= 3 {
				rootScopeRelay = tag[2]
			}
			break
		}
	}

	if rootScopeName != "" {
		for _, tag := range target.Tags {
			if len(tag) >= 2 && tag[0] == "K" {
				rootKind = tag[1]
				break
			}
		}
		for _, tag := range target.Tags {
			if len(tag) >= 2 && tag[0] == "P" {
				rootPubkey = tag[1]
				break
			}
		}
	} else {
		if target.Kind.IsAddressable() || target.Kind.IsReplaceable() {
			rootScopeName = "A"
			rootScopeValue = fmt.Sprintf("%d:%s:%s", target.Kind, target.PubKey.Hex(), dtag())
		} else {
			rootScopeName = "E"
			rootScopeValue = target.ID.Hex()
		}
		rootScopeRelay = relayHint
		rootKind = fmt.Sprint(target.Kind)
		rootPubkey = target.PubKey.Hex()
	}

	// root scope tag
	rootScopeTag := nostr.Tag{rootScopeName, rootScopeValue}
	if rootScopeName == "E" && rootPubkey != "" {
		// the pubkey goes in the 4th position, so the relay hint must be
		// present (even if empty) to keep it there
		rootScopeTag = append(rootScopeTag, rootScopeRelay, rootPubkey)
	} else if rootScopeRelay != "" {
		rootScopeTag = append(rootScopeTag, rootScopeRelay)
	}
	evt.Tags = append(evt.Tags, rootScopeTag)

	if rootKind != "" {
		evt.Tags = append(evt.Tags, nostr.Tag{"K", rootKind})
	}
	if rootPubkey != "" && rootScopeName != "I" {
		evt.Tags = append(evt.Tags, nostr.Tag{"P", rootPubkey})
	}

	// parent tags (lowercase) - always reference the target event
	if target.Kind.IsAddressable() || target.Kind.IsReplaceable() {
		aValue := fmt.Sprintf("%d:%s:%s", target.Kind, target.PubKey.Hex(), dtag())
		aTag := nostr.Tag{"a", aValue}
		if relayHint != "" {
			aTag = append(aTag, relayHint)
		}
		evt.Tags = append(evt.Tags, aTag)
	}
	eTag := nostr.Tag{"e", target.ID.Hex()}
	if target.PubKey != (nostr.PubKey{}) {
		// same here: keep the pubkey in the 4th position
		eTag = append(eTag, relayHint, target.PubKey.Hex())
	} else if relayHint != "" {
		eTag = append(eTag, relayHint)
	}
	evt.Tags = append(evt.Tags, eTag)

	evt.Tags = append(evt.Tags, nostr.Tag{"k", fmt.Sprint(target.Kind)})

	pTag := nostr.Tag{"p", target.PubKey.Hex()}
	if relayHint != "" {
		pTag = append(pTag, relayHint)
	}
	evt.Tags = append(evt.Tags, pTag)
}
