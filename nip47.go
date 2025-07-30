package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"fiatjaf.com/nostr"
	"github.com/urfave/cli/v3"
)

var nwc = &cli.Command{
	Name:                      "nwc",
	Usage:                     "nip47 stuff",
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "url",
			Usage:    "url that starts with nostr+walletconnect://",
			Required: true,
		},
	},
	Commands: []*cli.Command{
		{
			Name:  "info",
			Usage: "get info event (kind 13194)",
			Action: func(ctx context.Context, c *cli.Command) error {
				walletURL := c.String("url")

				relay, walletPubkey, _, err := parseWalletConnectURL(walletURL)
				if err != nil {
					return fmt.Errorf("failed to parse url: %w", err)
				}

				r, err := nostr.RelayConnect(ctx, relay, nostr.RelayOptions{})
				if err != nil {
					return fmt.Errorf("failed to connect to relay %s: %w", relay, err)
				}
				defer r.Close()

				sub, err := r.Subscribe(ctx, nostr.Filter{
					Kinds:   []nostr.Kind{13194},
					Authors: []nostr.PubKey{walletPubkey},
				}, nostr.SubscriptionOptions{})
				if err != nil {
					return fmt.Errorf("failed to subscribe: %w", err)
				}
				defer sub.Close()

				for {
					select {
					case ev := <-sub.Events:
						stdout(ev)
						return nil
					case <-ctx.Done():
						return fmt.Errorf("timed out waiting for info event")
					}
				}
			},
		},
	},
}

func parseWalletConnectURL(walletURL string) (relay string, walletPubkey nostr.PubKey, secret string, err error) {
	if !strings.HasPrefix(walletURL, "nostr+walletconnect://") {
		return "", nostr.PubKey{}, "", fmt.Errorf("must start with nostr+walletconnect://")
	}

	parts := strings.SplitN(walletURL, "?", 2)
	if len(parts) != 2 {
		return "", nostr.PubKey{}, "", fmt.Errorf("query not found")
	}

	walletPubkey, err = nostr.PubKeyFromHex(strings.TrimPrefix(parts[0], "nostr+walletconnect://"))
	if err != nil {
		return "", nostr.PubKey{}, "", fmt.Errorf("invalid wallet pubkey: %w", err)
	}

	params, err := url.ParseQuery(parts[1])
	if err != nil {
		return "", nostr.PubKey{}, "", fmt.Errorf("invalid query: %w", err)
	}

	relay = params.Get("relay")
	if relay == "" {
		return "", nostr.PubKey{}, "", fmt.Errorf("relay missing")
	}

	secret = params.Get("secret")
	if secret == "" {
		return "", nostr.PubKey{}, "", fmt.Errorf("secret missing")
	}

	return relay, walletPubkey, secret, nil
}
