package main

import (
	"context"
	"fmt"

	"fiatjaf.com/nostr/nip11"
	"github.com/urfave/cli/v3"
)

var relay = &cli.Command{
	Name:  "relay",
	Usage: "gets the relay information document for the given relay, as JSON",
	Description: `
		nak relay nostr.wine
`,
	ArgsUsage:                 "<relay-url>",
	DisableSliceFlagSeparator: true,
	Action: func(ctx context.Context, c *cli.Command) error {
		for url := range getStdinLinesOrArguments(c.Args()) {
			if url == "" {
				return fmt.Errorf("specify the <relay-url>")
			}

			info, err := nip11.Fetch(ctx, url)
			if err != nil {
				ctx = lineProcessingError(ctx, "failed to fetch '%s' information document: %w", url, err)
				continue
			}

			pretty, _ := json.MarshalIndent(info, "", "  ")
			stdout(string(pretty))
		}
		return nil
	},
}
