package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/keyer"
	"fiatjaf.com/nostr/nip44"
	"github.com/fatih/color"
	"github.com/mailru/easyjson"
	"github.com/urfave/cli/v3"
)

var gift = &cli.Command{
	Name:  "gift",
	Usage: "gift-wraps (or unwraps) an event according to NIP-59",
	Description: `example:
  nak event | nak gift wrap --sec <sec-a> -p <sec-b> | nak gift unwrap --sec <sec-b> --from <pub-a>

a decoupled key (if it has been created or received with "nak dekey" previously) will be used by default.`,
	DisableSliceFlagSeparator: true,
	Flags: append(
		defaultKeyFlags,
		&cli.BoolFlag{
			Name:  "use-direct",
			Usage: "Use the key given to --sec directly even when a decoupled key exists.",
		},
	),
	Commands: []*cli.Command{
		{
			Name: "wrap",
			Flags: []cli.Flag{
				&PubKeyFlag{
					Name:     "recipient-pubkey",
					Aliases:  []string{"p", "tgt", "target", "pubkey", "to"},
					Required: true,
				},
			},
			Usage: "turns an event into a rumor (unsigned) then gift-wraps it to the recipient",
			Description: `example:
  nak event -c 'hello' | nak gift wrap --sec <my-secret-key> -p <target-public-key>`,
			Action: func(ctx context.Context, c *cli.Command) error {
				kr, _, err := gatherKeyerFromArguments(ctx, c)
				if err != nil {
					return err
				}

				// get sender pubkey (ourselves)
				sender, err := kr.GetPublicKey(ctx)
				if err != nil {
					return fmt.Errorf("failed to get sender pubkey: %w", err)
				}

				var cipher nostr.Cipher = kr
				// use decoupled key if it exists
				configPath := c.String("config-path")
				eSec, has, err := getDecoupledEncryptionKey(ctx, configPath, sender)
				if has {
					if err != nil {
						return fmt.Errorf("decoupled encryption key exists, but we failed to get it: %w; call `nak dekey` to attempt a fix or call this again with --use-direct to bypass", err)
					}
					cipher = keyer.NewPlainKeySigner(eSec)
				}

				recipient := getPubKey(c, "recipient-pubkey")

				// read event from stdin
				for eventJSON := range getJsonsOrBlank() {
					if eventJSON == "{}" {
						continue
					}

					var originalEvent nostr.Event
					if err := easyjson.Unmarshal([]byte(eventJSON), &originalEvent); err != nil {
						return fmt.Errorf("invalid event JSON: %w", err)
					}

					// turn into rumor (unsigned event)
					rumor := originalEvent
					rumor.Sig = [64]byte{} // remove signature
					rumor.PubKey = sender
					rumor.ID = rumor.GetID() // compute ID

					// create seal
					rumorJSON, _ := easyjson.Marshal(rumor)
					encryptedRumor, err := cipher.Encrypt(ctx, string(rumorJSON), recipient)
					if err != nil {
						return fmt.Errorf("failed to encrypt rumor: %w", err)
					}
					seal := &nostr.Event{
						Kind:      13,
						Content:   encryptedRumor,
						PubKey:    sender,
						CreatedAt: randomNow(),
						Tags:      nostr.Tags{},
					}
					if err := kr.SignEvent(ctx, seal); err != nil {
						return fmt.Errorf("failed to sign seal: %w", err)
					}

					// create gift wrap
					ephemeral := nostr.Generate()
					sealJSON, _ := easyjson.Marshal(seal)
					convkey, err := nip44.GenerateConversationKey(recipient, ephemeral)
					if err != nil {
						return fmt.Errorf("failed to generate conversation key: %w", err)
					}
					encryptedSeal, err := nip44.Encrypt(string(sealJSON), convkey)
					if err != nil {
						return fmt.Errorf("failed to encrypt seal: %w", err)
					}
					wrap := &nostr.Event{
						Kind:      1059,
						Content:   encryptedSeal,
						CreatedAt: randomNow(),
						Tags:      nostr.Tags{{"p", recipient.Hex()}},
					}
					wrap.Sign(ephemeral)

					// print the gift-wrap
					wrapJSON, err := easyjson.Marshal(wrap)
					if err != nil {
						return fmt.Errorf("failed to marshal gift wrap: %w", err)
					}
					stdout(string(wrapJSON))
				}

				return nil
			},
		},
		{
			Name:  "unwrap",
			Usage: "decrypts a gift-wrap event sent by the sender to us and exposes its internal rumor (unsigned event).",
			Description: `example:
  nak req -p <my-public-key> -k 1059 dmrelay.com | nak gift unwrap --sec <my-secret-key> --from <sender-public-key>`,
			Flags: []cli.Flag{
				&PubKeyFlag{
					Name:     "sender-pubkey",
					Aliases:  []string{"p", "src", "source", "pubkey", "from"},
					Required: true,
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				kr, _, err := gatherKeyerFromArguments(ctx, c)
				if err != nil {
					return err
				}

				// get receiver public key (ourselves)
				receiver, err := kr.GetPublicKey(ctx)
				if err != nil {
					return err
				}

				var cipher nostr.Cipher = kr
				// use decoupled key if it exists
				configPath := c.String("config-path")
				eSec, has, err := getDecoupledEncryptionKey(ctx, configPath, receiver)
				if has {
					if err != nil {
						return fmt.Errorf("decoupled encryption key exists, but we failed to get it: %w; call `nak dekey` to attempt a fix or call this again with --use-direct to bypass", err)
					}
					cipher = keyer.NewPlainKeySigner(eSec)
				}

				sender := getPubKey(c, "sender-pubkey")

				// read gift-wrapped event from stdin
				for wrapJSON := range getJsonsOrBlank() {
					if wrapJSON == "{}" {
						continue
					}

					var wrap nostr.Event
					if err := easyjson.Unmarshal([]byte(wrapJSON), &wrap); err != nil {
						return fmt.Errorf("invalid gift wrap JSON: %w", err)
					}

					if wrap.Kind != 1059 {
						return fmt.Errorf("not a gift wrap event (kind %d)", wrap.Kind)
					}

					ephemeralPubkey := wrap.PubKey

					// decrypt seal
					sealJSON, err := cipher.Decrypt(ctx, wrap.Content, ephemeralPubkey)
					if err != nil {
						return fmt.Errorf("failed to decrypt seal: %w", err)
					}

					var seal nostr.Event
					if err := easyjson.Unmarshal([]byte(sealJSON), &seal); err != nil {
						return fmt.Errorf("invalid seal JSON: %w", err)
					}

					if seal.Kind != 13 {
						return fmt.Errorf("not a seal event (kind %d)", seal.Kind)
					}

					// decrypt rumor
					rumorJSON, err := cipher.Decrypt(ctx, seal.Content, sender)
					if err != nil {
						return fmt.Errorf("failed to decrypt rumor: %w", err)
					}

					var rumor nostr.Event
					if err := easyjson.Unmarshal([]byte(rumorJSON), &rumor); err != nil {
						return fmt.Errorf("invalid rumor JSON: %w", err)
					}

					// output the unwrapped event (rumor)
					stdout(rumorJSON)
				}

				return nil
			},
		},
	},
}

func randomNow() nostr.Timestamp {
	const twoDays = 2 * 24 * 60 * 60
	now := time.Now().Unix()
	randomOffset := rand.Int63n(twoDays)
	return nostr.Timestamp(now - randomOffset)
}

func getDecoupledEncryptionKey(ctx context.Context, configPath string, pubkey nostr.PubKey) (nostr.SecretKey, bool, error) {
	relays := sys.FetchWriteRelays(ctx, pubkey)

	keyAnnouncementResult := sys.Pool.FetchManyReplaceable(ctx, relays, nostr.Filter{
		Kinds:   []nostr.Kind{10044},
		Authors: []nostr.PubKey{pubkey},
	}, nostr.SubscriptionOptions{Label: "nak-nip4e-gift"})
	var eSec nostr.SecretKey
	var ePub nostr.PubKey

	keyAnnouncementEvent, ok := keyAnnouncementResult.Load(nostr.ReplaceableKey{PubKey: pubkey, D: ""})
	if ok {
		// get the pub from the tag
		for _, tag := range keyAnnouncementEvent.Tags {
			if len(tag) >= 2 && tag[0] == "n" {
				ePub, _ = nostr.PubKeyFromHex(tag[1])
				break
			}
		}
		if ePub == nostr.ZeroPK {
			return [32]byte{}, true, fmt.Errorf("got invalid kind:10044 event, no 'n' tag")
		}

		// check if we have the key
		eKeyPath := filepath.Join(configPath, "dekey", "p", pubkey.Hex(), "e", ePub.Hex())
		if data, err := os.ReadFile(eKeyPath); err == nil {
			log(color.GreenString("- and we have it locally already\n"))
			eSec, err = nostr.SecretKeyFromHex(string(data))
			if err != nil {
				return [32]byte{}, true, fmt.Errorf("invalid main key: %w", err)
			}
			if eSec.Public() != ePub {
				return [32]byte{}, true, fmt.Errorf("stored decoupled encryption key is corrupted: %w", err)
			}

			return eSec, true, nil
		}
	}

	return [32]byte{}, false, nil
}
