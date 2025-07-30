package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"fiatjaf.com/nostr"
	"github.com/urfave/cli/v3"
)

type nip47Client struct {
	Relay        string
	WalletPubkey nostr.PubKey
	Secret       nostr.SecretKey
}

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
				url := c.String("url")

				client, err := newNip47ClientFromUrl(url)
				if err != nil {
					return fmt.Errorf("failed to parse url: %w", err)
				}

				info, err := client.info(ctx)
				if err != nil {
					return fmt.Errorf("failed to get info: %w", err)
				}

				stdout(info)
				return nil
			},
		},
	},
}

func newNip47ClientFromUrl(nwcUrl string) (*nip47Client, error) {
	if !strings.HasPrefix(nwcUrl, "nostr+walletconnect://") {
		return nil, fmt.Errorf("must start with nostr+walletconnect://")
	}

	parts := strings.SplitN(nwcUrl, "?", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("query not found")
	}

	walletPubkey, err := nostr.PubKeyFromHex(strings.TrimPrefix(parts[0], "nostr+walletconnect://"))
	if err != nil {
		return nil, fmt.Errorf("invalid wallet pubkey: %w", err)
	}

	params, err := url.ParseQuery(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}

	relay := params.Get("relay")
	if relay == "" {
		return nil, fmt.Errorf("relay missing")
	}

	secret := params.Get("secret")
	if secret == "" {
		return nil, fmt.Errorf("secret missing")
	}

	sk, err := nostr.SecretKeyFromHex(secret)
	if err != nil {
		return nil, fmt.Errorf("invalid secret: %w", err)
	}

	return &nip47Client{
		Relay:        relay,
		WalletPubkey: walletPubkey,
		Secret:       sk,
	}, nil
}

// fetch info event (kind 13194)
func (c *nip47Client) info(ctx context.Context) (*nostr.Event, error) {
	r, err := nostr.RelayConnect(ctx, c.Relay, nostr.RelayOptions{})
	if err != nil {
		return nil, err
	}
	defer r.Close()

	sub, err := r.Subscribe(ctx, nostr.Filter{
		Kinds:   []nostr.Kind{13194},
		Authors: []nostr.PubKey{c.WalletPubkey},
	}, nostr.SubscriptionOptions{})
	if err != nil {
		return nil, err
	}
	defer sub.Close()

	for {
		select {
		case ev := <-sub.Events:
			return &ev, nil
		case <-ctx.Done():
			return nil, fmt.Errorf("timed out waiting for info event")
		}
	}
}
