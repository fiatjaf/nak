package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip44"
	"github.com/urfave/cli/v3"
)

var dekey = &cli.Command{
	Name:                      "dekey",
	Usage:                     "handles NIP-4E decoupled encryption keys",
	Description:               "maybe this picture will explain better than I can do here for now: https://cdn.azzamo.net/89c543d261ad0d665c1dea78f91e527c2e39e7fe503b440265a3c47e63c9139f.png",
	DisableSliceFlagSeparator: true,
	Flags: append(defaultKeyFlags,
		&cli.StringFlag{
			Name:  "device-name",
			Usage: "name of this device that will be published and displayed on other clients",
			Value: func() string {
				if hostname, err := os.Hostname(); err == nil {
					return "nak@" + hostname
				}
				return "nak@unknown"
			}(),
		},
	),
	Action: func(ctx context.Context, c *cli.Command) error {
		kr, _, err := gatherKeyerFromArguments(ctx, c)
		if err != nil {
			return err
		}

		userPub, err := kr.GetPublicKey(ctx)
		if err != nil {
			return fmt.Errorf("failed to get user public key: %w", err)
		}

		configPath := c.String("config-path")
		deviceName := c.String("device-name")

		// check if we already have a local-device secret key
		deviceKeyPath := filepath.Join(configPath, "dekey", "device-key")
		var deviceSec nostr.SecretKey
		if data, err := os.ReadFile(deviceKeyPath); err == nil {
			deviceSec, err = nostr.SecretKeyFromHex(string(data))
			if err != nil {
				return fmt.Errorf("invalid device key in %s: %w", deviceKeyPath, err)
			}
		} else {
			// create one
			deviceSec = nostr.Generate()
			os.MkdirAll(filepath.Dir(deviceKeyPath), 0700)
			if err := os.WriteFile(deviceKeyPath, []byte(deviceSec.Hex()), 0600); err != nil {
				return fmt.Errorf("failed to write device key: %w", err)
			}
		}
		devicePub := deviceSec.Public()

		// get relays for the user
		relays := sys.FetchWriteRelays(ctx, userPub)
		relayList := connectToAllRelays(ctx, c, relays, nil, nostr.PoolOptions{})
		if len(relayList) == 0 {
			return fmt.Errorf("no relays to use")
		}

		// check if kind:4454 is already published
		events := sys.Pool.FetchMany(ctx, relays, nostr.Filter{
			Kinds:   []nostr.Kind{4454},
			Authors: []nostr.PubKey{userPub},
			Tags: nostr.TagMap{
				"pubkey": []string{devicePub.Hex()},
			},
		}, nostr.SubscriptionOptions{Label: "nak-nip4e"})
		if len(events) == 0 {
			// publish kind:4454
			evt := nostr.Event{
				Kind:      4454,
				Content:   "",
				CreatedAt: nostr.Now(),
				Tags: nostr.Tags{
					{"client", deviceName},
					{"pubkey", devicePub.Hex()},
				},
			}

			// sign with main key
			if err := kr.SignEvent(ctx, &evt); err != nil {
				return fmt.Errorf("failed to sign device event: %w", err)
			}

			// publish
			if err := publishFlow(ctx, c, kr, evt, relayList); err != nil {
				return err
			}
		}

		// check for kind:10044
		userKeyEventDate := nostr.Now()
		userKeyResult := sys.Pool.FetchManyReplaceable(ctx, relays, nostr.Filter{
			Kinds:   []nostr.Kind{10044},
			Authors: []nostr.PubKey{userPub},
		}, nostr.SubscriptionOptions{Label: "nak-nip4e"})
		var eSec nostr.SecretKey
		var ePub nostr.PubKey
		if userKeyEvent, ok := userKeyResult.Load(nostr.ReplaceableKey{PubKey: userPub, D: ""}); !ok {
			// generate main secret key
			eSec = nostr.Generate()
			ePub := eSec.Public()

			// store it
			eKeyPath := filepath.Join(configPath, "dekey", "e", ePub.Hex())
			os.MkdirAll(filepath.Dir(eKeyPath), 0700)
			if err := os.WriteFile(eKeyPath, []byte(eSec.Hex()), 0600); err != nil {
				return fmt.Errorf("failed to write user encryption key: %w", err)
			}

			// publish kind:10044
			evt10044 := nostr.Event{
				Kind:      10044,
				Content:   "",
				CreatedAt: userKeyEventDate,
				Tags: nostr.Tags{
					{"n", ePub.Hex()},
				},
			}
			if err := kr.SignEvent(ctx, &evt10044); err != nil {
				return fmt.Errorf("failed to sign kind:10044: %w", err)
			}

			if err := publishFlow(ctx, c, kr, evt10044, relayList); err != nil {
				return err
			}
		} else {
			userKeyEventDate = userKeyEvent.CreatedAt

			// get the pub from the tag
			for _, tag := range userKeyEvent.Tags {
				if len(tag) >= 2 && tag[0] == "n" {
					ePub, _ = nostr.PubKeyFromHex(tag[1])
					break
				}
			}
			if ePub == nostr.ZeroPK {
				return fmt.Errorf("invalid kind:10044 event, no 'n' tag")
			}

			// check if we have the key
			eKeyPath := filepath.Join(configPath, "dekey", "e", ePub.Hex())
			if data, err := os.ReadFile(eKeyPath); err == nil {
				eSec, err = nostr.SecretKeyFromHex(string(data))
				if err != nil {
					return fmt.Errorf("invalid main key: %w", err)
				}
				if eSec.Public() != ePub {
					return fmt.Errorf("stored user encryption key is corrupted: %w", err)
				}
			} else {
				// try to decrypt from kind:4455
				for eKeyMsg := range sys.Pool.FetchMany(ctx, relays, nostr.Filter{
					Kinds: []nostr.Kind{4455},
					Tags: nostr.TagMap{
						"p": []string{devicePub.Hex()},
					},
				}, nostr.SubscriptionOptions{Label: "nak-nip4e"}) {
					var senderPub nostr.PubKey
					for _, tag := range eKeyMsg.Tags {
						if len(tag) >= 2 && tag[0] == "P" {
							senderPub, _ = nostr.PubKeyFromHex(tag[1])
							break
						}
					}
					if senderPub == nostr.ZeroPK {
						continue
					}
					ss, err := nip44.GenerateConversationKey(senderPub, deviceSec)
					if err != nil {
						continue
					}
					eSecHex, err := nip44.Decrypt(eKeyMsg.Content, ss)
					if err != nil {
						continue
					}
					eSec, err = nostr.SecretKeyFromHex(eSecHex)
					if err != nil {
						continue
					}
					// check if it matches mainPub
					if eSec.Public() == ePub {
						// store it
						os.MkdirAll(filepath.Dir(eKeyPath), 0700)
						os.WriteFile(eKeyPath, []byte(eSecHex), 0600)
						break
					}
				}
			}
		}

		if eSec == [32]byte{} {
			log("main secret key not available, must authorize on another device\n")
			return nil
		}

		// now we have mainSec, check for other kind:4454 events newer than the 10044
		keyMsgs := make([]string, 0, 5)
		for keyOrDeviceEvt := range sys.Pool.FetchMany(ctx, relays, nostr.Filter{
			Kinds:   []nostr.Kind{4454, 4455},
			Authors: []nostr.PubKey{userPub},
			Since:   userKeyEventDate,
		}, nostr.SubscriptionOptions{Label: "nak-nip4e"}) {
			if keyOrDeviceEvt.Kind == 4455 {
				// key event

				// skip ourselves
				if keyOrDeviceEvt.Tags.FindWithValue("p", devicePub.Hex()) != nil {
					continue
				}

				// assume a key msg will always come before its associated devicemsg
				// so just store them here:
				pubkeyTag := keyOrDeviceEvt.Tags.Find("p")
				if pubkeyTag == nil {
					continue
				}
				keyMsgs = append(keyMsgs, pubkeyTag[1])
			} else if keyOrDeviceEvt.Kind == 4454 {
				// device event

				// skip ourselves
				if keyOrDeviceEvt.Tags.FindWithValue("pubkey", devicePub.Hex()) != nil {
					continue
				}

				// if this already has a corresponding keyMsg then skip it
				pubkeyTag := keyOrDeviceEvt.Tags.Find("pubkey")
				if pubkeyTag == nil {
					continue
				}
				if slices.Contains(keyMsgs, pubkeyTag[1]) {
					continue
				}

				// here we know we're dealing with a deviceMsg without a corresponding keyMsg
				// so we have to build a keyMsg for them
				theirDevice, err := nostr.PubKeyFromHex(pubkeyTag[1])
				if err != nil {
					continue
				}

				ss, err := nip44.GenerateConversationKey(theirDevice, deviceSec)
				if err != nil {
					continue
				}
				ciphertext, err := nip44.Encrypt(eSec.Hex(), ss)
				if err != nil {
					continue
				}

				evt4455 := nostr.Event{
					Kind:      4455,
					Content:   ciphertext,
					CreatedAt: nostr.Now(),
					Tags: nostr.Tags{
						{"p", theirDevice.Hex()},
						{"P", devicePub.Hex()},
					},
				}
				if err := kr.SignEvent(ctx, &evt4455); err != nil {
					continue
				}

				publishFlow(ctx, c, kr, evt4455, relayList)
			}
		}

		return nil
	},
}
