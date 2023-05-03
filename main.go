package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/nbd-wtf/go-nostr"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "nak",
		Usage: "the nostr army knife command-line tool",
		Commands: []*cli.Command{
			{
				Name:  "req",
				Usage: "generates an encoded REQ message to be sent to a relay",
				Description: `example usage (with 'nostcat'):
					nak req -k 1 -a 3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d | nostcat wss://nostr-pub.wellorder.net
                `,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:    "author",
						Aliases: []string{"a"},
						Usage:   "only accept events from these authors (pubkey as hex)",
					},
					&cli.StringSliceFlag{
						Name:    "id",
						Aliases: []string{"i"},
						Usage:   "only accept events with these ids (hex)",
					},
					&cli.IntSliceFlag{
						Name:    "kind",
						Aliases: []string{"k"},
						Usage:   "only accept events with these kind numbers",
					},
					&cli.StringSliceFlag{
						Name:    "tag",
						Aliases: []string{"t"},
						Usage:   "takes a tag like -t e=<id>, only accept events with these tags",
					},
					&cli.StringSliceFlag{
						Name:    "event-tag",
						Aliases: []string{"e"},
						Usage:   "shortcut for --tag e=<value>",
					},
					&cli.StringSliceFlag{
						Name:    "pubkey-tag",
						Aliases: []string{"p"},
						Usage:   "shortcut for --tag p=<value>",
					},
					&cli.IntFlag{
						Name:    "since",
						Aliases: []string{"s"},
						Usage:   "only accept events newer than this (unix timestamp)",
					},
					&cli.IntFlag{
						Name:    "until",
						Aliases: []string{"u"},
						Usage:   "only accept events older than this (unix timestamp)",
					},
					&cli.IntFlag{
						Name:    "limit",
						Aliases: []string{"l"},
						Usage:   "only accept up to this number of events",
					},
				},
				Action: func(c *cli.Context) error {
					filter := nostr.Filter{}

					if authors := c.StringSlice("author"); len(authors) > 0 {
						filter.Authors = authors
					}
					if ids := c.StringSlice("id"); len(ids) > 0 {
						filter.IDs = ids
					}
					if kinds := c.IntSlice("kind"); len(kinds) > 0 {
						filter.Kinds = kinds
					}

					tags := make([][]string, 0, 5)
					for _, tagFlag := range c.StringSlice("tag") {
						spl := strings.Split(tagFlag, "=")
						if len(spl) == 2 && len(spl[0]) == 1 {
							tags = append(tags, spl)
						}
					}
					for _, etag := range c.StringSlice("event-tag") {
						tags = append(tags, []string{"e", etag})
					}
					for _, ptag := range c.StringSlice("pubkey-tag") {
						tags = append(tags, []string{"p", ptag})
					}
					if len(tags) > 0 {
						filter.Tags = make(nostr.TagMap)
						for _, tag := range tags {
							if _, ok := filter.Tags[tag[0]]; !ok {
								filter.Tags[tag[0]] = make([]string, 0, 3)
							}
							filter.Tags[tag[0]] = append(filter.Tags[tag[0]], tag[1])
						}
					}

					if since := c.Int("since"); since != 0 {
						ts := nostr.Timestamp(since)
						filter.Since = &ts
					}
					if until := c.Int("until"); until != 0 {
						ts := nostr.Timestamp(until)
						filter.Until = &ts
					}
					if limit := c.Int("limit"); limit != 0 {
						filter.Limit = limit
					}

					fmt.Println(filter.String())
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("failed to run cli: %s", err)
	}
}
