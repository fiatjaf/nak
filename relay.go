package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nbd-wtf/go-nostr/nip11"
	"github.com/urfave/cli/v2"
)

var relay = &cli.Command{
	Name:  "relay",
	Usage: "gets the relay information document for the given relay, as JSON",
	Description: `example:
		nak relay nostr.wine`,
	ArgsUsage: "<relay-url>",
	Action: func(c *cli.Context) error {
		for url := range getStdinLinesOrArguments(c.Args()) {
			if url == "" {
				return fmt.Errorf("specify the <relay-url>")
			}

			if !strings.HasPrefix(url, "wss://") && !strings.HasPrefix(url, "ws://") {
				url = "wss://" + url
			}

			info, err := nip11.Fetch(c.Context, url)
			if err != nil {
				lineProcessingError(c, "failed to fetch '%s' information document: %w", url, err)
				continue
			}

			pretty, _ := json.MarshalIndent(info, "", "  ")
			stdout(string(pretty))
		}
		return nil
	},
}
