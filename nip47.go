package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/keyer"
	"fiatjaf.com/nostr/nip44"
	"github.com/urfave/cli/v3"
)

type nip47Client struct {
	Relay        string
	WalletPubkey nostr.PubKey
	Secret       nostr.SecretKey
}

type nip47Request struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}
type nip47Response struct {
	ResultType string                 `json:"result_type"`
	Result     map[string]interface{} `json:"result"`
	Error      *nip47Error            `json:"error"`
}

func (r nip47Response) String() string {
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf("error marshaling response: %v", err)
	}
	return string(b)
}

type nip47Error struct {
	Code    int    `json:"error"`
	Message string `json:"message"`
}

// encryption tag to negotiate upgrading to NIP44
// only supports nip44_v2 because nip04 is deprecated
// https://github.com/nostr-protocol/nips/pull/1780
type EncryptionTag string

const (
	EncryptionTagNip44V2 EncryptionTag = "nip44_v2"
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
		{
			Name:  "get_info",
			Usage: "get info for this connection",
			Action: func(ctx context.Context, c *cli.Command) error {
				url := c.String("url")

				client, err := newNip47ClientFromUrl(url)
				if err != nil {
					return fmt.Errorf("failed to parse url: %w", err)
				}

				name := "get_info"
				result, err := client.method(ctx, &nip47Request{
					Method: name,
					Params: map[string]interface{}{},
				})
				if err != nil {
					return fmt.Errorf("%s failed: %w", name, err)
				}

				stdout(result)
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

func (c *nip47Client) method(ctx context.Context, req *nip47Request) (*nip47Response, error) {
	command, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal json: %w", err)
	}

	sharedSecret, err := nip44.GenerateConversationKey(c.WalletPubkey, c.Secret)
	if err != nil {
		return nil, fmt.Errorf("failed to compute shared secret: %w", err)
	}

	encrypted, err := nip44.Encrypt(string(command), sharedSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt content: %w", err)
	}

	reqEvent := nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      23194,
		Content:   encrypted,
		Tags: nostr.Tags{
			{"p", c.WalletPubkey.Hex()},
			// the wallet service might not support nip44_v2 or not know
			// about this tag at all so it might respond with nip04 instead.
			{"encryption", string(EncryptionTagNip44V2)},
		},
	}

	if err := keyer.NewPlainKeySigner(c.Secret).SignEvent(ctx, &reqEvent); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}

	r, err := nostr.RelayConnect(ctx, c.Relay, nostr.RelayOptions{})
	if err != nil {
		return nil, err
	}
	defer r.Close()

	if err := r.Publish(ctx, reqEvent); err != nil {
		return nil, err
	}

	sub, err := r.Subscribe(ctx, nostr.Filter{
		Kinds:   []nostr.Kind{23195},
		Authors: []nostr.PubKey{c.WalletPubkey},
		Tags: nostr.TagMap{
			"e": []string{reqEvent.ID.Hex()},
		},
	}, nostr.SubscriptionOptions{})
	if err != nil {
		return nil, err
	}
	defer sub.Close()

	var resEvent nostr.Event
	select {
	case resEvent = <-sub.Events:
		break
	case <-ctx.Done():
		return nil, fmt.Errorf("timed out waiting for response")
	}

	// content might be nip04 for old wallet services
	decrypted, err := nip44.Decrypt(resEvent.Content, sharedSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt response: %w", err)
	}

	var res nip47Response
	if err := json.Unmarshal([]byte(decrypted), &res); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return &res, nil
}
