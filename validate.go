package main

import (
	"context"
	"fmt"

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
			Name:        "schema",
			Usage:       "url to download the YAML schema from, or path to the file",
			Value:       schema.DefaultSchemaURL,
			TakesFile:   true,
			Destination: &schemaURI,
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		sch, err := getSchema()
		if err != nil {
			return err
		}

		validator := schema.NewValidatorFromSchema(sch)

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
			if stdinEvent == "{}" && !isPiped() {
				// blank sentinel from getJsonsOrBlank(), use the arguments instead
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
