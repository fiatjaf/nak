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
	Flags:                     defaultKeyFlags,
	Commands: []*cli.Command{
		{
			Name: "wrap",
			Flags: []cli.Flag{
				&PubKeyFlag{
					Name:     "recipient-pubkey",
					Aliases:  []string{"p", "tgt", "target", "pubkey", "to"},
					Required: true,
				},
				&cli.BoolFlag{
					Name:  "use-our-identity-key",
					Usage: "Encrypt with the key given to --sec directly even when a decoupled key exists for the sender.",
				},
				&cli.BoolFlag{
					Name:  "use-their-identity-key",
					Usage: "Encrypt to the public key given as --recipient-pubkey directly even when a decoupled key exists for the receiver.",
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

				var using bool

				var cipher nostr.Cipher = kr
				// use decoupled key if it exists
				using = false
				if !c.Bool("use-our-identity-key") {
					configPath := c.String("config-path")
					eSec, has, err := getDecoupledEncryptionSecretKey(ctx, configPath, sender)
					if has {
						if err != nil {
							return fmt.Errorf("our decoupled encryption key exists, but we failed to get it: %w; call `nak dekey` to attempt a fix or call this again with --encrypt-with-our-identity-key to bypass", err)
						}
						cipher = keyer.NewPlainKeySigner(eSec)
						log("- using our decoupled encryption key %s\n", color.CyanString(eSec.Public().Hex()))
						using = true
					}
				}
				if !using {
					log("- using our identity key %s\n", color.CyanString(sender.Hex()))
				}

				recipient := getPubKey(c, "recipient-pubkey")
				using = false
				if !c.Bool("use-their-identity-key") {
					if theirEPub, exists := getDecoupledEncryptionPublicKey(ctx, recipient); exists {
						recipient = theirEPub
						using = true
						log("- using their decoupled encryption public key %s\n", color.CyanString(theirEPub.Hex()))
					}
				}
				if !using {
					log("- using their identity public key %s\n", color.CyanString(recipient.Hex()))
				}

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
  nak req -p <my-public-key> -k 1059 dmrelay.com | nak gift unwrap --sec <my-secret-key>`,
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

				ciphers := []nostr.Cipher{kr}
				// use decoupled key if it exists
				configPath := c.String("config-path")
				eSec, has, err := getDecoupledEncryptionSecretKey(ctx, configPath, receiver)
				if has {
					if err != nil {
						return fmt.Errorf("our decoupled encryption key exists, but we failed to get it: %w; call `nak dekey` to attempt a fix or call this again with --use-direct to bypass", err)
					}
					ciphers = append(ciphers, kr)
					ciphers[0] = keyer.NewPlainKeySigner(eSec) // pub decoupled key first
				}

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

					// decrypt seal (in the process also find out if they encrypted it to our identity key or to our decoupled key)
					var cipher nostr.Cipher
					var seal nostr.Event

					// try both the receiver identity key and decoupled key
					err = nil
					for c, potentialCipher := range ciphers {
						switch c {
						case 0:
							log("- trying the receiver's decoupled encryption key %s\n", color.CyanString(eSec.Public().Hex()))
						case 1:
							log("- trying the receiver's identity key %s\n", color.CyanString(receiver.Hex()))
						}

						sealj, thisErr := potentialCipher.Decrypt(ctx, wrap.Content, wrap.PubKey)
						if thisErr != nil {
							err = thisErr
							continue
						}
						if thisErr := easyjson.Unmarshal([]byte(sealj), &seal); thisErr != nil {
							err = fmt.Errorf("invalid seal JSON: %w", thisErr)
							continue
						}

						cipher = potentialCipher
						break
					}
					if seal.ID == nostr.ZeroID {
						// if both ciphers failed above we'll reach here
						return fmt.Errorf("failed to decrypt seal: %w", err)
					}

					if seal.Kind != 13 {
						return fmt.Errorf("not a seal event (kind %d)", seal.Kind)
					}

					senderEncryptionPublicKeys := []nostr.PubKey{seal.PubKey}
					if theirEPub, exists := getDecoupledEncryptionPublicKey(ctx, seal.PubKey); exists {
						senderEncryptionPublicKeys = append(senderEncryptionPublicKeys, seal.PubKey)
						senderEncryptionPublicKeys[0] = theirEPub // put decoupled key first
					}

					// decrypt rumor (at this point we know what cipher is the one they encrypted to)
					// (but we don't know if they have encrypted with their identity key or their decoupled key, so try both)
					var rumor nostr.Event
					err = nil
					for s, senderEncryptionPublicKey := range senderEncryptionPublicKeys {
						switch s {
						case 0:
							log("- trying the sender's decoupled encryption public key %s\n", color.CyanString(senderEncryptionPublicKey.Hex()))
						case 1:
							log("- trying the sender's identity public key %s\n", color.CyanString(senderEncryptionPublicKey.Hex()))
						}

						rumorj, thisErr := cipher.Decrypt(ctx, seal.Content, senderEncryptionPublicKey)
						if thisErr != nil {
							err = fmt.Errorf("failed to decrypt rumor: %w", thisErr)
							continue
						}
						if thisErr := easyjson.Unmarshal([]byte(rumorj), &rumor); thisErr != nil {
							err = fmt.Errorf("invalid rumor JSON: %w", thisErr)
							continue
						}

						break
					}

					if rumor.ID == nostr.ZeroID {
						return fmt.Errorf("failed to decrypt rumor: %w", err)
					}

					// output the unwrapped event (rumor)
					stdout(rumor.String())
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

func getDecoupledEncryptionSecretKey(ctx context.Context, configPath string, pubkey nostr.PubKey) (nostr.SecretKey, bool, error) {
	relays := sys.FetchWriteRelays(ctx, pubkey)

	keyAnnouncementResult := sys.Pool.FetchManyReplaceable(ctx, relays, nostr.Filter{
		Kinds:   []nostr.Kind{10044},
		Authors: []nostr.PubKey{pubkey},
	}, nostr.SubscriptionOptions{Label: "nak-nip4e-gift"})

	keyAnnouncementEvent, ok := keyAnnouncementResult.Load(nostr.ReplaceableKey{PubKey: pubkey, D: ""})
	if ok {
		var ePub nostr.PubKey

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
			eSec, err := nostr.SecretKeyFromHex(string(data))
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

func getDecoupledEncryptionPublicKey(ctx context.Context, pubkey nostr.PubKey) (nostr.PubKey, bool) {
	relays := sys.FetchWriteRelays(ctx, pubkey)

	keyAnnouncementResult := sys.Pool.FetchManyReplaceable(ctx, relays, nostr.Filter{
		Kinds:   []nostr.Kind{10044},
		Authors: []nostr.PubKey{pubkey},
	}, nostr.SubscriptionOptions{Label: "nak-nip4e-gift"})

	keyAnnouncementEvent, ok := keyAnnouncementResult.Load(nostr.ReplaceableKey{PubKey: pubkey, D: ""})
	if ok {
		var ePub nostr.PubKey

		// get the pub from the tag
		for _, tag := range keyAnnouncementEvent.Tags {
			if len(tag) >= 2 && tag[0] == "n" {
				ePub, _ = nostr.PubKeyFromHex(tag[1])
				break
			}
		}
		if ePub == nostr.ZeroPK {
			return nostr.ZeroPK, false
		}

		return ePub, true
	}

	return nostr.ZeroPK, false
}
