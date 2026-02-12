package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip45"
	"fiatjaf.com/nostr/nip45/hyperloglog"
	"github.com/mailru/easyjson"
	"github.com/urfave/cli/v3"
)

var count = &cli.Command{
	Name:                      "count",
	Usage:                     "generates encoded COUNT messages and optionally use them to talk to relays",
	Description:               `like 'nak req', but does a "COUNT" call instead. Will attempt to perform HyperLogLog aggregation if more than one relay is specified.`,
	DisableSliceFlagSeparator: true,
	Flags:                     reqFilterFlags,
	ArgsUsage:                 "[relay...]",
	Action: func(ctx context.Context, c *cli.Command) error {
		biggerUrlSize := 0
		relayUrls := c.Args().Slice()
		if len(relayUrls) > 0 {
			relays := connectToAllRelays(ctx, c, relayUrls, nil, nostr.PoolOptions{})
			if len(relays) == 0 {
				log("failed to connect to any of the given relays.\n")
				os.Exit(3)
			}
			relayUrls = make([]string, len(relays))
			for i, relay := range relays {
				relayUrls[i] = relay.URL
				if len(relay.URL) > biggerUrlSize {
					biggerUrlSize = len(relay.URL)
				}
			}
		}

		// go line by line from stdin or run once with input from flags
		for stdinFilter := range getJsonsOrBlank() {
			filter := nostr.Filter{}
			if stdinFilter != "" {
				if err := easyjson.Unmarshal([]byte(stdinFilter), &filter); err != nil {
					ctx = lineProcessingError(ctx, "invalid filter '%s' received from stdin: %s", stdinFilter, err)
					continue
				}
			}

			if err := applyFlagsToFilter(c, &filter); err != nil {
				return err
			}

			successes := 0
			if len(relayUrls) > 0 {
				var hll *hyperloglog.HyperLogLog
				if offset := nip45.HyperLogLogEventPubkeyOffsetForFilter(filter); offset != -1 && len(relayUrls) > 1 {
					hll = hyperloglog.New(offset)
				}
				for _, relayUrl := range relayUrls {
					relay, _ := sys.Pool.EnsureRelay(relayUrl)
					count, hllRegisters, err := relay.Count(ctx, filter, nostr.SubscriptionOptions{
						Label: "nak-count",
					})
					fmt.Fprintf(os.Stderr, "%s%s: ", strings.Repeat(" ", biggerUrlSize-len(relayUrl)), relayUrl)

					if err != nil {
						fmt.Fprintf(os.Stderr, "error: %s\n", err)
						continue
					}

					var hasHLLStr string
					if hll != nil && len(hllRegisters) == 256 {
						hll.MergeRegisters(hllRegisters)
						hasHLLStr = " (hll)"
					}

					fmt.Fprintf(os.Stderr, "%d%s\n", count, hasHLLStr)
					successes++
				}
				if successes == 0 {
					return fmt.Errorf("all relays have failed")
				} else if hll != nil {
					fmt.Fprintf(os.Stderr, "HyperLogLog sum: %d\n", hll.Count())
				}
			} else {
				// no relays given, will just print the filter
				var result string
				j, _ := json.Marshal([]any{"COUNT", "nak", filter})
				result = string(j)
				stdout(result)
			}
		}

		exitIfLineProcessingError(ctx)
		return nil
	},
}
