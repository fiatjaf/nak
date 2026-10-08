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

with --nip66 it probes each relay with the given comma-separated checks (open, read, write, nip11 or all) and prints an unsigned NIP-66 kind:30166 relay discovery event instead, which can be signed and published with 'nak event':

		nak relay --nip66=open,read,nip11 nostr.wine relay.damus.io | nak event --sec <key> wss://my.relay

the "write" check (included in "all") publishes an ephemeral event signed by a throwaway key to the relay.
`,
	ArgsUsage:                 "<relay-url>...",
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "nip66",
			Usage: "probe the relay with the given checks (open, read, write, nip11 or all) and print a kind:30166 event",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		var checks nip66Checks
		if c.IsSet("nip66") {
			var err error
			if checks, err = parseNIP66Checks(c.String("nip66")); err != nil {
				return err
			}
		}

		for url := range getStdinLinesOrArguments(c.Args()) {
			if url == "" {
				return fmt.Errorf("specify the <relay-url>")
			}

			if c.IsSet("nip66") {
				evt, err := probeRelayNIP66(ctx, url, checks)
				if err != nil {
					ctx = lineProcessingError(ctx, "failed to probe '%s': %s", url, err)
					continue
				}
				stdout(evt.String())
				continue
			}

			info, err := nip11.Fetch(ctx, url)
			if err != nil {
				ctx = lineProcessingError(ctx, "failed to fetch '%s' information document: %s", url, err)
				continue
			}

			pretty, _ := json.MarshalIndent(info, "", "  ")
			stdout(string(pretty))
		}
		exitIfLineProcessingError(ctx)
		return nil
	},
}
