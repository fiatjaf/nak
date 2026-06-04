package main

import (
	"context"
	"fmt"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/schema"
	"github.com/urfave/cli/v3"
)

var validateCmd = &cli.Command{
	Name:  "validate",
	Usage: "validates events against the provided RoK YAML schema",
	Description: `takes a URL to a YAML schema in the same format as that of https://github.com/nostr-protocol/registry-of-kinds (defaults to that one) and validates the event tags and content against it, according to its kind.

example:
nak event -k 1 -p not_a_pubkey | nak validate
>> schema validation failed: tag[0][1]: invalid pubkey value 'not_a_pubkey' at tag 'p', index 1: pubkey should be 64-char hex, got 'not_a_pubkey'
`,
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:      "schema",
			Usage:     "url to download the YAML schema from, or path to the file",
			Value:     "https://raw.githubusercontent.com/nostr-protocol/registry-of-kinds/refs/heads/master/schema.yaml",
			TakesFile: true,
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		var validator schema.Validator

		if schemaURL := c.String("schema"); strings.HasPrefix(schemaURL, "http") {
			var err error
			validator, err = schema.NewValidatorFromURL(schemaURL)
			if err != nil {
				return fmt.Errorf("failed to instantiate validator from '%s': %w", schemaURL, err)
			}
		} else {
			var err error
			validator, err = schema.NewValidatorFromFile(schemaURL)
			if err != nil {
				return fmt.Errorf("failed to instantiate validator from %s: %w", schemaURL, err)
			}
		}

		handleEvent := func(stdinEvent string) error {
			evt := nostr.Event{}
			if err := json.Unmarshal([]byte(stdinEvent), &evt); err != nil {
				return fmt.Errorf("invalid event JSON: %w", err)
			}

			if err := validator.ValidateEvent(evt); err != nil {
				return fmt.Errorf("schema validation failed: %w", err)
			}

			stdout(evt)

			return nil
		}

		for stdinEvent := range getJsonsOrBlank() {
			if stdinEvent == "" {
				for _, arg := range c.Args().Slice() {
					if err := handleEvent(arg); err != nil {
						ctx = lineProcessingError(ctx, "%s", err)
					}
				}
				continue
			}

			if err := handleEvent(stdinEvent); err != nil {
				ctx = lineProcessingError(ctx, "%s", err)
			}
		}

		exitIfLineProcessingError(ctx)
		return nil
	},
}
