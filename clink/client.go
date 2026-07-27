package clink

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip44"
)

const (
	KindPubRPC     nostr.Kind = 21000
	KindClinkOffer nostr.Kind = 21001
	DefaultTimeout            = 20 * time.Second
)

// Client is the Go CLINK SDK. Pass any *nostr.Pool (e.g. from the CLI's sys.Pool).
type Client struct {
	Pool      *nostr.Pool
	SecretKey nostr.SecretKey
	Timeout   time.Duration
}

func NewClient(pool *nostr.Pool, sk nostr.SecretKey) *Client {
	return &Client{Pool: pool, SecretKey: sk, Timeout: DefaultTimeout}
}

func (c *Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultTimeout
}

func (c *Client) PubKey() nostr.PubKey { return c.SecretKey.Public() }

// Noffer requests a BOLT11 invoice from a noffer (kind 21001).
func (c *Client) Noffer(ctx context.Context, offer OfferPointer, amountSats int64, description string) (string, error) {
	payload := map[string]any{"offer": offer.Offer}
	if amountSats > 0 {
		payload["amount_sats"] = amountSats
	}
	if description != "" {
		if len(description) > 100 {
			return "", fmt.Errorf("description must be <= 100 chars")
		}
		payload["description"] = description
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	ck, err := nip44.GenerateConversationKey(offer.Pubkey, c.SecretKey)
	if err != nil {
		return "", err
	}
	content, err := nip44.Encrypt(string(plaintext), ck)
	if err != nil {
		return "", err
	}

	ourPub := c.PubKey()
	evt := nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      KindClinkOffer,
		Tags: nostr.Tags{
			{"p", offer.Pubkey.Hex()},
			{"clink_version", "1"},
		},
		Content: content,
	}
	if err := evt.Sign(c.SecretKey); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	relays := []string{offer.Relay}
	events := c.Pool.SubscribeMany(ctx, relays, nostr.Filter{
		Kinds:     []nostr.Kind{KindClinkOffer},
		Tags:      nostr.TagMap{"p": []string{ourPub.Hex()}, "e": []string{evt.ID.Hex()}},
		Since:     nostr.Now() - 2,
		LimitZero: true,
	}, nostr.SubscriptionOptions{Label: "clink-noffer"})

	resultCh := make(chan map[string]any, 1)
	errCh := make(chan error, 1)
	go readNofferResponse(events, c.SecretKey, offer.Pubkey, resultCh, errCh)

	if err := c.publish(ctx, relays, evt); err != nil {
		cancel()
		return "", err
	}

	select {
	case res := <-resultCh:
		return parseNofferBolt11(res)
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", fmt.Errorf("timed out waiting for noffer invoice")
	}
}

// Pub is a Lightning.Pub kind-21000 RPC client for one destination.
type Pub struct {
	client *Client
	Dest   nostr.PubKey
	Relays []string
}

func (c *Client) Pub(dest nostr.PubKey, relays []string) *Pub {
	return &Pub{client: c, Dest: dest, Relays: relays}
}

// Call sends a Pub RPC and waits for the matching requestId response.
func (p *Pub) Call(ctx context.Context, rpcName string, body any) (map[string]any, error) {
	c := p.client
	reqID := randomHex(16)
	msg := map[string]any{
		"rpcName":        rpcName,
		"authIdentifier": c.PubKey().Hex(),
		"requestId":      reqID,
	}
	if body != nil {
		msg["body"] = body
	}
	plaintext, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	shared, err := pubSharedSecret(c.SecretKey, p.Dest)
	if err != nil {
		return nil, err
	}
	content, err := encryptPubV1(string(plaintext), shared)
	if err != nil {
		return nil, err
	}

	evt := nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      KindPubRPC,
		Tags:      nostr.Tags{{"p", p.Dest.Hex()}},
		Content:   content,
	}
	if err := evt.Sign(c.SecretKey); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	events := c.Pool.SubscribeMany(ctx, p.Relays, nostr.Filter{
		Kinds:     []nostr.Kind{KindPubRPC},
		Tags:      nostr.TagMap{"p": []string{c.PubKey().Hex()}},
		Since:     nostr.Now() - 2,
		LimitZero: true,
	}, nostr.SubscriptionOptions{Label: "clink-rpc"})

	resultCh := make(chan map[string]any, 1)
	errCh := make(chan error, 1)
	go readPubRPCResponse(events, c.SecretKey, p.Dest, reqID, resultCh, errCh)

	if err := c.publish(ctx, p.Relays, evt); err != nil {
		cancel()
		return nil, err
	}

	select {
	case res := <-resultCh:
		return res, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("timed out waiting for %s", rpcName)
	}
}

func (p *Pub) GetUserInfo(ctx context.Context) (map[string]any, error) {
	res, err := p.Call(ctx, "GetUserInfo", nil)
	if err != nil {
		return nil, err
	}
	return res, StatusOK(res)
}

func (p *Pub) AddUserOffer(ctx context.Context, body map[string]any) (map[string]any, error) {
	res, err := p.Call(ctx, "AddUserOffer", body)
	if err != nil {
		return nil, err
	}
	return res, StatusOK(res)
}

func (p *Pub) GetUserOffer(ctx context.Context, offerID string) (map[string]any, error) {
	res, err := p.Call(ctx, "GetUserOffer", map[string]any{"offer_id": offerID})
	if err != nil {
		return nil, err
	}
	return res, StatusOK(res)
}

func (p *Pub) PayInvoice(ctx context.Context, bolt11 string) (map[string]any, error) {
	res, err := p.Call(ctx, "PayInvoice", map[string]any{"invoice": bolt11, "amount": 0})
	if err != nil {
		return nil, err
	}
	return res, StatusOK(res)
}

func StatusOK(res map[string]any) error {
	status, _ := res["status"].(string)
	if status == "OK" {
		return nil
	}
	reason, _ := res["reason"].(string)
	if reason == "" {
		reason = fmt.Sprint(res)
	}
	return fmt.Errorf("%s", reason)
}

func AsInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}

func (c *Client) publish(ctx context.Context, relays []string, evt nostr.Event) error {
	var last error
	ok := false
	for res := range c.Pool.PublishMany(ctx, relays, evt) {
		if res.Error != nil {
			last = res.Error
			continue
		}
		ok = true
	}
	if ok {
		return nil
	}
	if last == nil {
		last = fmt.Errorf("publish failed")
	}
	return last
}

func readPubRPCResponse(
	events <-chan nostr.RelayEvent,
	sk nostr.SecretKey,
	dest nostr.PubKey,
	reqID string,
	resultCh chan<- map[string]any,
	errCh chan<- error,
) {
	for re := range events {
		if re.PubKey != dest {
			continue
		}
		shared, err := pubSharedSecret(sk, dest)
		if err != nil {
			continue
		}
		plain, err := decryptPubV1(re.Content, shared)
		if err != nil {
			continue
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(plain), &res); err != nil {
			continue
		}
		if fmt.Sprint(res["requestId"]) != reqID {
			continue
		}
		resultCh <- res
		return
	}
	errCh <- fmt.Errorf("subscription closed before response")
}

func readNofferResponse(
	events <-chan nostr.RelayEvent,
	sk nostr.SecretKey,
	dest nostr.PubKey,
	resultCh chan<- map[string]any,
	errCh chan<- error,
) {
	ck, err := nip44.GenerateConversationKey(dest, sk)
	if err != nil {
		errCh <- err
		return
	}
	for re := range events {
		if re.PubKey != dest {
			continue
		}
		plain, err := nip44.Decrypt(re.Content, ck)
		if err != nil {
			continue
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(plain), &res); err != nil {
			continue
		}
		resultCh <- res
		return
	}
	errCh <- fmt.Errorf("subscription closed before noffer response")
}

func parseNofferBolt11(res map[string]any) (string, error) {
	if bolt11, ok := res["bolt11"].(string); ok && bolt11 != "" {
		return bolt11, nil
	}
	if errMsg, ok := res["error"].(string); ok {
		return "", fmt.Errorf("noffer error: %s", errMsg)
	}
	b, _ := json.Marshal(res)
	return "", fmt.Errorf("unexpected noffer response: %s", string(b))
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// EphemeralKey returns a fresh secret key (e.g. for one-shot invoice requests).
func EphemeralKey() (nostr.SecretKey, error) {
	var sk nostr.SecretKey
	_, err := rand.Read(sk[:])
	return sk, err
}
