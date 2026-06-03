package main

import (
	"context"
	"fmt"
	"strconv"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/schema"
	"github.com/urfave/cli/v3"
)

var kindCmd = &cli.Command{
	Name:  "kind",
	Usage: "look up information about a Nostr event kind",
	Description: `takes a kind number or a kind name and prints information about that kind from the registry of kinds schema.

example:
	nak kind 1
	nak kind "text note"
	nak kind "git meta"
	nak kind "fav relays"
	nak kind 30023`,
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
		if c.Args().Len() != 1 {
			return fmt.Errorf("requires exactly one argument: kind number or name")
		}

		input := c.Args().First()

		// resolve input to a kind number
		var k nostr.Kind
		if n, err := strconv.ParseUint(input, 10, 16); err == nil && n >= 0 {
			k = nostr.Kind(n)
		} else {
			resolved, err := stringToKind(input)
			if err != nil {
				return fmt.Errorf("failed to resolve kind: %w", err)
			}
			k = resolved
		}

		sch, err := getSchema()
		if err != nil {
			return err
		}

		ks := sch.Kinds[strconv.Itoa(int(k.Num()))]

		j, _ := json.MarshalIndent(ks, "", "  ")
		stdout(string(j))
		return nil
	},
}
