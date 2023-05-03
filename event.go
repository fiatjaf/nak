package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/urfave/cli/v2"
)

const CATEGORY_EVENT_FIELDS = "EVENT FIELDS"

var event = &cli.Command{
	Name:  "event",
	Usage: "generates an encoded event",
	Description: `example usage (for sending directly to a relay with 'nostcat'):
		nak event -k 1 -c hello --envelope | nostcat wss://nos.lol`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "sec",
			Usage:       "secret key to sign the event",
			DefaultText: "the key '1'",
			Value:       "0000000000000000000000000000000000000000000000000000000000000001",
		},
		&cli.BoolFlag{
			Name:  "envelope",
			Usage: "print the event enveloped in a [\"EVENT\", ...] message ready to be sent to a relay",
		},
		&cli.IntFlag{
			Name:        "kind",
			Aliases:     []string{"k"},
			Usage:       "event kind",
			DefaultText: "1",
			Value:       1,
			Category:    CATEGORY_EVENT_FIELDS,
		},
		&cli.StringFlag{
			Name:        "content",
			Aliases:     []string{"c"},
			Usage:       "event content",
			DefaultText: "hello from the nostr army knife",
			Value:       "hello from the nostr army knife",
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
			Value:       "now",
			Category:    CATEGORY_EVENT_FIELDS,
		},
	},
	Action: func(c *cli.Context) error {
		evt := nostr.Event{
			Kind:    c.Int("kind"),
			Content: c.String("content"),
			Tags:    make(nostr.Tags, 0, 3),
		}

		tags := make([][]string, 0, 5)
		for _, tagFlag := range c.StringSlice("tag") {
			spl := strings.Split(tagFlag, "=")
			if len(spl) == 2 && len(spl[0]) == 1 {
				tags = append(tags, spl)
			}
		}
		for _, etag := range c.StringSlice("e") {
			tags = append(tags, []string{"e", etag})
		}
		for _, ptag := range c.StringSlice("p") {
			tags = append(tags, []string{"p", ptag})
		}
		if len(tags) > 0 {
			for _, tag := range tags {
				evt.Tags = append(evt.Tags, tag)
			}
		}

		createdAt := c.String("created-at")
		ts := time.Now()
		if createdAt != "now" {
			if v, err := strconv.ParseInt(createdAt, 10, 64); err != nil {
				return fmt.Errorf("failed to parse timestamp '%s': %w", createdAt, err)
			} else {
				ts = time.Unix(v, 0)
			}
		}
		evt.CreatedAt = nostr.Timestamp(ts.Unix())

		if err := evt.Sign(c.String("sec")); err != nil {
			return fmt.Errorf("error signing with provided key: %w", err)
		}

		var result string
		if c.Bool("envelope") {
			j, _ := json.Marshal([]any{"EVENT", evt})
			result = string(j)
		} else {
			result = evt.String()
		}

		fmt.Println(result)
		return nil
	},
}
