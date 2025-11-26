package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/urfave/cli/v3"
)

var nip = &cli.Command{
	Name:  "nip",
	Usage: "get the description of a NIP from its number",
	Description: `fetches the NIPs README from GitHub and parses it to find the description of the given NIP number.
	
example:
	nak nip 1
	nak nip list`,
	ArgsUsage: "<NIP number>",
	Commands: []*cli.Command{
		{
			Name:  "list",
			Usage: "list all NIPs",
			Action: func(ctx context.Context, c *cli.Command) error {
				return iterateNips(func(nip, desc string) {
					stdout(nip + ": " + desc)
				})
			},
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		reqNum := c.Args().First()
		if reqNum == "" {
			return fmt.Errorf("missing NIP number")
		}

		normalize := func(s string) string {
			s = strings.ToLower(s)
			s = strings.TrimPrefix(s, "nip-")
			s = strings.TrimLeft(s, "0")
			if s == "" {
				s = "0"
			}
			return s
		}

		reqNum = normalize(reqNum)

		found := false
		err := iterateNips(func(nip, desc string) {
			nipNum := normalize(nip)

			if nipNum == reqNum {
				stdout(strings.TrimSpace(desc))
				found = true
			}
		})

		if err != nil {
			return err
		}

		if !found {
			return fmt.Errorf("NIP-%s not found", strings.ToUpper(reqNum))
		}
		return nil
	},
}

func iterateNips(cb func(nip, desc string)) error {
	resp, err := http.Get("https://raw.githubusercontent.com/nostr-protocol/nips/master/README.md")
	if err != nil {
		return fmt.Errorf("failed to fetch NIPs README: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read NIPs README: %w", err)
	}

	lines := strings.SplitSeq(string(body), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [NIP-") {
			continue
		}

		start := strings.Index(line, "[")
		end := strings.Index(line, "]")
		if start == -1 || end == -1 || end < start {
			continue
		}

		content := line[start+1 : end]

		parts := strings.SplitN(content, ":", 2)
		if len(parts) != 2 {
			continue
		}

		nipPart := parts[0]
		descPart := parts[1]

		cb(nipPart, strings.TrimSpace(descPart))
	}
	return nil
}
