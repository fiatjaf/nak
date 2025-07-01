package main

import (
	"context"
	"fmt"

	"fiatjaf.com/nostr"
	"github.com/mailru/easyjson"
	"github.com/urfave/cli/v3"
)

var filter = &cli.Command{
	Name:  "filter",
	Usage: "applies an event filter to an event to see if it matches.",
	Description: `
example:
		echo '{"kind": 1, "content": "hello"}' | nak filter -k 1
		nak filter '{"kind": 1, "content": "hello"}' -k 1
		nak filter '{"kind": 1, "content": "hello"}' '{"kinds": [1]}' -k 0
`,
	DisableSliceFlagSeparator: true,
	Flags:                     reqFilterFlags,
	ArgsUsage:                 "[event_json] [base_filter_json]",
	Action: func(ctx context.Context, c *cli.Command) error {
		args := c.Args().Slice()

		var baseFilter nostr.Filter
		var baseEvent nostr.Event

		if len(args) == 2 {
			// two arguments: first is event, second is base filter
			if err := easyjson.Unmarshal([]byte(args[0]), &baseEvent); err != nil {
				return fmt.Errorf("invalid base event: %w", err)
			}
			if err := easyjson.Unmarshal([]byte(args[1]), &baseFilter); err != nil {
				return fmt.Errorf("invalid base filter: %w", err)
			}
		} else if len(args) == 1 {
			if isPiped() {
				// one argument + stdin: argument is base filter
				if err := easyjson.Unmarshal([]byte(args[0]), &baseFilter); err != nil {
					return fmt.Errorf("invalid base filter: %w", err)
				}
			} else {
				// one argument, no stdin: argument is event
				if err := easyjson.Unmarshal([]byte(args[0]), &baseEvent); err != nil {
					return fmt.Errorf("invalid base event: %w", err)
				}
			}
		}

		// apply flags to filter
		if err := applyFlagsToFilter(c, &baseFilter); err != nil {
			return err
		}

		// if there is no stdin we'll still get an empty object here
		for evtj := range getJsonsOrBlank() {
			var evt nostr.Event
			if err := easyjson.Unmarshal([]byte(evtj), &evt); err != nil {
				ctx = lineProcessingError(ctx, "invalid event: %s", err)
				continue
			}

			// merge that with the base event
			if evt.ID == nostr.ZeroID {
				evt.ID = baseEvent.ID
			}
			if evt.PubKey == nostr.ZeroPK {
				evt.PubKey = baseEvent.PubKey
			}
			if evt.Sig == [64]byte{} {
				evt.Sig = baseEvent.Sig
			}
			if evt.Content == "" {
				evt.Content = baseEvent.Content
			}
			if len(evt.Tags) == 0 {
				evt.Tags = baseEvent.Tags
			}
			if evt.CreatedAt == 0 {
				evt.CreatedAt = baseEvent.CreatedAt
			}

			if baseFilter.Matches(evt) {
				stdout(evt)
			} else {
				logverbose("event %s didn't match %s", evt, baseFilter)
			}
		}

		exitIfLineProcessingError(ctx)
		return nil
	},
}
