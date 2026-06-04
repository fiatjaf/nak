package main

import (
	"context"

	"fiatjaf.com/nostr/nip19"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

var profile = &cli.Command{
	Name:  "profile",
	Usage: "displays profile information for a given pubkey",
	Description: `fetches and displays profile metadata, relays, and contact count for a given pubkey.

example usage:
  nak profile npub1h8spmtw9m2huyv6v2j2qd5zv956z2zdugl6mgx02f2upffwpm3nqv0j4ps
  nak profile user@example.com`,
	ArgsUsage: "[pubkey]",
	Action: func(ctx context.Context, c *cli.Command) error {
		for pubkeyInput := range getStdinLinesOrArguments(c.Args()) {
			pk, err := parsePubKey(pubkeyInput)
			if err != nil {
				ctx = lineProcessingError(ctx, "invalid pubkey '%s': %s", pubkeyInput, err)
				continue
			}

			pm := sys.FetchProfileMetadata(ctx, pk)

			npub := nip19.EncodeNpub(pk)
			stdout(colors.bold("pubkey (hex):"), pk.Hex())
			stdout(colors.bold("npub:"), color.HiCyanString(npub))

			relayList := sys.FetchRelayList(ctx, pk)
			writeRelays := make([]string, 0, 3)
			for _, rl := range relayList.Items {
				if rl.Outbox {
					writeRelays = append(writeRelays, rl.URL)
					if len(writeRelays) == 3 {
						break
					}
				}
			}
			if len(writeRelays) > 0 {
				nprofile := nip19.EncodeNprofile(pk, writeRelays)
				stdout(colors.bold("profile uri:"), color.HiCyanString("nostr:"+nprofile))
			}

			if pm.Name != "" {
				stdout(colors.bold("name:"), color.HiBlueString(pm.Name))
			}
			if pm.DisplayName != "" {
				stdout(colors.bold("display_name:"), color.HiBlueString(pm.DisplayName))
			}
			if pm.About != "" {
				stdout(colors.bold("about:"), color.HiBlueString(pm.About))
			}
			if pm.Picture != "" {
				stdout(colors.bold("picture:"), color.HiBlueString(pm.Picture))
			}
			if pm.Banner != "" {
				stdout(colors.bold("banner:"), color.HiBlueString(pm.Banner))
			}
			if pm.Website != "" {
				stdout(colors.bold("website:"), color.HiBlueString(pm.Website))
			}
			if pm.NIP05 != "" {
				isValid := pm.NIP05Valid(ctx)
				if isValid {
					stdout(colors.bold("nip05:"), color.HiGreenString(pm.NIP05), color.HiGreenString("(verified)"))
				} else {
					stdout(colors.bold("nip05:"), color.HiRedString(pm.NIP05), color.HiRedString("(not verified)"))
				}
			}
			if pm.LUD16 != "" {
				stdout(colors.bold("lud16:"), color.HiBlueString(pm.LUD16))
			}

			if len(relayList.Items) > 0 {
				stdout(colors.bold("relays:"))
				for _, relay := range relayList.Items {
					access := ""
					if relay.Inbox && relay.Outbox {
						access = "read/write"
					} else if relay.Inbox {
						access = "read"
					} else if relay.Outbox {
						access = "write"
					}
					stdout("  ", color.HiBlueString(relay.URL), color.HiCyanString("(%s)", access))
				}
			}

			followList := sys.FetchFollowList(ctx, pk)
			contactCount := len(followList.Items)
			stdout(colors.bold("follows:"), color.HiCyanString("%d", contactCount))
		}

		exitIfLineProcessingError(ctx)
		return nil
	},
}
