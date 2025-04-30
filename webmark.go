package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/mailru/easyjson"
	"github.com/nbd-wtf/go-nostr"
	"github.com/urfave/cli/v3"
)

var webmark = &cli.Command{
	Name:  "webmark",
	Usage: "publishes a web bookmark event (kind:39701)",
	Description: `create a web bookmark (kind: 39701) with optional title, tags, and published_at timestamp.

To publish the bookmark to relays, provide the relay URLs as additional arguments after the URL and content.

examples:
  # Basic usage with just URL and content
  nak webmark https://example.com/blog/post -c "A great blog post about Nostr"

  # With title and tags
  nak webmark --title "Example Blog" --tag nostr --tag blog https://example.com/blog/post -c "A great blog post about Nostr"

  # With published_at timestamp
  nak webmark --published-at 1738863000 https://example.com/blog/post -c "A great blog post about Nostr"

  # With all options
  nak webmark --title "Example Blog" --tag nostr --tag blog --published-at 1738863000 https://example.com/blog/post -c "A great blog post about Nostr"

  # Publish to specific relays
  nak webmark --title "Example Blog" --tag nostr --tag blog https://example.com/blog/post -c "A great blog post about Nostr" wss://relay1.example.com wss://relay2.example.com`,
	Flags: append(defaultKeyFlags,
		&cli.StringFlag{
			Name:     "title",
			Usage:    "title of the web bookmark",
			Category: CATEGORY_EVENT_FIELDS,
		},
		&cli.StringSliceFlag{
			Name:     "tag",
			Usage:    "add a topic tag (can be used multiple times)",
			Category: CATEGORY_EVENT_FIELDS,
		},
		&cli.StringFlag{
			Name:     "published-at",
			Usage:    "timestamp in unix seconds for when the web bookmark was first published",
			Category: CATEGORY_EVENT_FIELDS,
		},
		&cli.StringFlag{
			Name:     "content",
			Aliases:  []string{"c"},
			Usage:    "content/description of the web bookmark",
			Category: CATEGORY_EVENT_FIELDS,
		},
	),
	Action: func(ctx context.Context, c *cli.Command) error {
		if c.Args().Len() < 1 {
			return fmt.Errorf("URL is required")
		}

		// Parse and normalize URL
		rawURL := c.Args().Get(0)
		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			return fmt.Errorf("invalid URL: %w", err)
		}

		// Normalize URL for d tag by removing scheme and query/fragment
		dTag := parsedURL.Host + parsedURL.Path
		if parsedURL.Path == "" {
			dTag += "/"
		}

		// Get content from -c flag
		content := c.String("content")

		// Create event
		evt := nostr.Event{
			Kind:      39701,
			CreatedAt: nostr.Now(),
			Content:   content,
			Tags: nostr.Tags{
				{"d", dTag},
			},
		}

		// Add title if provided
		if title := c.String("title"); title != "" {
			evt.Tags = append(evt.Tags, nostr.Tag{"title", title})
		}

		// Add topic tags
		for _, tag := range c.StringSlice("tag") {
			evt.Tags = append(evt.Tags, nostr.Tag{"t", tag})
		}

		// Add published_at tag if provided
		if publishedAt := c.String("published-at"); publishedAt != "" {
			evt.Tags = append(evt.Tags, nostr.Tag{"published_at", publishedAt})
		}

		// Get keyer for signing
		keyer, _, err := gatherKeyerFromArguments(ctx, c)
		if err != nil {
			return fmt.Errorf("failed to get keyer: %w", err)
		}

		// Sign event
		if err := keyer.SignEvent(ctx, &evt); err != nil {
			return fmt.Errorf("failed to sign event: %w", err)
		}

		// Print event
		if isVerbose {
			fmt.Fprintf(os.Stderr, "event %s:\n", evt.ID)
			easyjson.MarshalToWriter(evt, os.Stdout)
		} else {
			easyjson.MarshalToWriter(evt, os.Stdout)
		}

		// Publish to relays if provided
		if c.Args().Len() > 2 {
			relayUrls := c.Args().Slice()[2:]
			relays := connectToAllRelays(ctx, c, relayUrls, nil,
				nostr.WithAuthHandler(func(ctx context.Context, authEvent nostr.RelayEvent) error {
					return authSigner(ctx, c, func(s string, args ...any) {}, authEvent)
				}),
			)
			if len(relays) == 0 {
				log("failed to connect to any of the given relays.\n")
				os.Exit(3)
			}

			for _, relay := range relays {
				err := relay.Publish(ctx, evt)
				if err != nil {
					log("failed to publish to %s: %s\n", relay.URL, err)
					continue
				}
			}
		}

		return nil
	},
} 