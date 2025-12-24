package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nip44"
	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

var dekey = &cli.Command{
	Name:                      "dekey",
	Usage:                     "handles NIP-4E decoupled encryption keys",
	Description:               "maybe this picture will explain better than I can do here for now: https://cdn.azzamo.net/89c543d261ad0d665c1dea78f91e527c2e39e7fe503b440265a3c47e63c9139f.png",
	DisableSliceFlagSeparator: true,
	Flags: append(defaultKeyFlags,
		&cli.StringFlag{
			Name:  "device",
			Usage: "name of this device that will be published and displayed on other clients",
			Value: func() string {
				if hostname, err := os.Hostname(); err == nil {
					return "nak@" + hostname
				}
				return "nak@unknown"
			}(),
		},
		&cli.BoolFlag{
			Name:  "rotate",
			Usage: "force the creation of a new encryption key, effectively invalidating any previous ones",
		},
		&cli.BoolFlag{
			Name:    "authorize-all",
			Aliases: []string{"yolo"},
			Usage:   "do not ask for confirmation, just automatically send the encryption key to all devices that exist",
		},
		&cli.BoolFlag{
			Name:  "reject-all",
			Usage: "do not ask for confirmation, just not send the encryption key to any device",
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
		deviceName := c.String("device")

		log("handling device key for %s as %s\n",
			color.YellowString(deviceName),
			color.CyanString(nip19.EncodeNpub(userPub)),
		)
		// check if we already have a local-device secret key
		deviceKeyPath := filepath.Join(configPath, "dekey", "device-key")
		var deviceSec nostr.SecretKey
		if data, err := os.ReadFile(deviceKeyPath); err == nil {
			log(color.GreenString("found existing device key\n"))
			deviceSec, err = nostr.SecretKeyFromHex(string(data))
			if err != nil {
				return fmt.Errorf("invalid device key in %s: %w", deviceKeyPath, err)
			}
		} else {
			log(color.YellowString("generating new device key\n"))
			// create one
			deviceSec = nostr.Generate()
			os.MkdirAll(filepath.Dir(deviceKeyPath), 0700)
			if err := os.WriteFile(deviceKeyPath, []byte(deviceSec.Hex()), 0600); err != nil {
				return fmt.Errorf("failed to write device key: %w", err)
			}
			log(color.GreenString("device key generated and stored\n"))
		}
		devicePub := deviceSec.Public()

		// get relays for the user
		log("fetching write relays for %s\n", color.CyanString(nip19.EncodeNpub(userPub)))
		relays := sys.FetchWriteRelays(ctx, userPub)
		relayList := connectToAllRelays(ctx, c, relays, nil, nostr.PoolOptions{})
		if len(relayList) == 0 {
			return fmt.Errorf("no relays to use")
		}

		// check for kind:10044
		log("- checking for user encryption key (kind:10044)\n")
		keyAnnouncementResult := sys.Pool.FetchManyReplaceable(ctx, relays, nostr.Filter{
			Kinds:   []nostr.Kind{10044},
			Authors: []nostr.PubKey{userPub},
		}, nostr.SubscriptionOptions{Label: "nak-nip4e"})
		var eSec nostr.SecretKey
		var ePub nostr.PubKey

		var generateNewEncryptionKey bool
		keyAnnouncementEvent, ok := keyAnnouncementResult.Load(nostr.ReplaceableKey{PubKey: userPub, D: ""})
		if !ok {
			log("- no user encryption key found, generating new one\n")
			generateNewEncryptionKey = true
		} else {
			// get the pub from the tag
			for _, tag := range keyAnnouncementEvent.Tags {
				if len(tag) >= 2 && tag[0] == "n" {
					ePub, _ = nostr.PubKeyFromHex(tag[1])
					break
				}
			}
			if ePub == nostr.ZeroPK {
				return fmt.Errorf("got invalid kind:10044 event, no 'n' tag")
			}

			log(". an encryption public key already exists: %s\n", color.CyanString(ePub.Hex()))
			if c.Bool("rotate") {
				log(color.GreenString("rotating it by generating a new one\n"))
				generateNewEncryptionKey = true
			}
		}

		if generateNewEncryptionKey {
			// generate main secret key
			eSec = nostr.Generate()
			ePub = eSec.Public()

			// store it
			eKeyPath := filepath.Join(configPath, "dekey", "p", userPub.Hex(), "e", ePub.Hex())
			os.MkdirAll(filepath.Dir(eKeyPath), 0700)
			if err := os.WriteFile(eKeyPath, []byte(eSec.Hex()), 0600); err != nil {
				return fmt.Errorf("failed to write user encryption key: %w", err)
			}
			log("user encryption key generated and stored, public key: %s\n", color.CyanString(ePub.Hex()))

			// publish kind:10044
			log("publishing user encryption public key (kind:10044)\n")
			evt10044 := nostr.Event{
				Kind:      10044,
				Content:   "",
				CreatedAt: nostr.Now(),
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
			// check if we have the key
			eKeyPath := filepath.Join(configPath, "dekey", "p", userPub.Hex(), "e", ePub.Hex())
			if data, err := os.ReadFile(eKeyPath); err == nil {
				log(color.GreenString("- and we have it locally already\n"))
				eSec, err = nostr.SecretKeyFromHex(string(data))
				if err != nil {
					return fmt.Errorf("invalid main key: %w", err)
				}
				if eSec.Public() != ePub {
					return fmt.Errorf("stored user encryption key is corrupted: %w", err)
				}
			} else {
				log("- encryption key not found locally, attempting to fetch the key from other devices\n")

				// check if our kind:4454 is already published
				log("- checking for existing device announcement (kind:4454)\n")
				ourDeviceAnnouncementEvents := make([]nostr.Event, 0, 1)
				for evt := range sys.Pool.FetchMany(ctx, relays, nostr.Filter{
					Kinds:   []nostr.Kind{4454},
					Authors: []nostr.PubKey{userPub},
					Tags: nostr.TagMap{
						"P": []string{devicePub.Hex()},
					},
					Limit: 1,
				}, nostr.SubscriptionOptions{Label: "nak-nip4e"}) {
					ourDeviceAnnouncementEvents = append(ourDeviceAnnouncementEvents, evt.Event)
				}
				if len(ourDeviceAnnouncementEvents) == 0 {
					log(". no device announcement found, publishing kind:4454 for %s\n", color.YellowString(deviceName))
					// publish kind:4454
					evt := nostr.Event{
						Kind:      4454,
						Content:   "",
						CreatedAt: nostr.Now(),
						Tags: nostr.Tags{
							{"client", deviceName},
							{"P", devicePub.Hex()},
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
					log(color.GreenString(". device announcement published\n"))
					ourDeviceAnnouncementEvents = append(ourDeviceAnnouncementEvents, evt)
				} else {
					log(color.GreenString(". device already registered\n"))
				}

				// see if some other device has shared the key with us from kind:4455
				for eKeyMsg := range sys.Pool.FetchMany(ctx, relays, nostr.Filter{
					Kinds: []nostr.Kind{4455},
					Tags: nostr.TagMap{
						"p": []string{devicePub.Hex()},
					},
					Since: keyAnnouncementEvent.CreatedAt + 1,
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
						log(color.GreenString("successfully decrypted encryption key from another device\n"))
						// store it
						os.MkdirAll(filepath.Dir(eKeyPath), 0700)
						os.WriteFile(eKeyPath, []byte(eSecHex), 0600)

						// delete our 4454 if we had one, since we received the key
						if len(ourDeviceAnnouncementEvents) > 0 {
							log("deleting our device announcement (kind:4454) since we received the encryption key\n")
							deletion4454 := nostr.Event{
								CreatedAt: nostr.Now(),
								Kind:      5,
								Tags: nostr.Tags{
									{"e", ourDeviceAnnouncementEvents[0].ID.Hex()},
								},
							}
							if err := kr.SignEvent(ctx, &deletion4454); err != nil {
								log(color.RedString("failed to sign 4454 deletion: %v\n"), err)
							} else if err := publishFlow(ctx, c, kr, deletion4454, relayList); err != nil {
								log(color.RedString("failed to publish 4454 deletion: %v\n"), err)
							} else {
								log(color.GreenString("- device announcement deleted\n"))
							}
						}

						// delete the 4455 we just decrypted
						log("deleting the key message (kind:4455) we just decrypted\n")
						deletion4455 := nostr.Event{
							CreatedAt: nostr.Now(),
							Kind:      5,
							Tags: nostr.Tags{
								{"e", eKeyMsg.ID.Hex()},
							},
						}
						if err := kr.SignEvent(ctx, &deletion4455); err != nil {
							log(color.RedString("failed to sign 4455 deletion: %v\n"), err)
						} else if err := publishFlow(ctx, c, kr, deletion4455, relayList); err != nil {
							log(color.RedString("failed to publish 4455 deletion: %v\n"), err)
						} else {
							log(color.GreenString("- key message deleted\n"))
						}

						break
					}
				}
			}
		}

		if eSec == [32]byte{} {
			log("encryption secret key not available, must be sent from another device to %s first\n",
				color.YellowString(deviceName))
			return nil
		}
		log(color.GreenString("- encryption key ready\n"))

		// now we have mainSec, check for other kind:4454 events newer than the 10044
		log("- checking for other devices and key messages so we can send the key\n")
		keyMsgs := make([]string, 0, 5)
		for keyOrDeviceEvt := range sys.Pool.FetchMany(ctx, relays, nostr.Filter{
			Kinds:   []nostr.Kind{4454, 4455},
			Authors: []nostr.PubKey{userPub},
			Since:   keyAnnouncementEvent.CreatedAt + 1,
		}, nostr.SubscriptionOptions{Label: "nak-nip4e"}) {
			if keyOrDeviceEvt.Kind == 4455 {
				// got key event
				keyEvent := keyOrDeviceEvt

				// assume a key msg will always come before its associated devicemsg
				// so just store them here:
				pubkeyTag := keyEvent.Tags.Find("p")
				if pubkeyTag == nil {
					continue
				}
				keyMsgs = append(keyMsgs, pubkeyTag[1])
			} else if keyOrDeviceEvt.Kind == 4454 {
				// device event
				deviceEvt := keyOrDeviceEvt

				// skip ourselves
				if deviceEvt.Tags.FindWithValue("P", devicePub.Hex()) != nil {
					continue
				}

				// if there is a clock skew (current time is earlier than the time of this device's announcement) skip it
				if nostr.Now() < deviceEvt.CreatedAt {
					continue
				}

				// if this already has a corresponding keyMsg then skip it
				pubkeyTag := deviceEvt.Tags.Find("P")
				if pubkeyTag == nil {
					continue
				}

				if slices.Contains(keyMsgs, pubkeyTag[1]) {
					continue
				}

				deviceTag := deviceEvt.Tags.Find("client")
				if deviceTag == nil {
					continue
				}

				// here we know we're dealing with a deviceMsg without a corresponding keyMsg
				// so we have to build a keyMsg for them
				theirDevice, err := nostr.PubKeyFromHex(pubkeyTag[1])
				if err != nil {
					continue
				}

				if c.Bool("authorize-all") {
					// will proceed
				} else if c.Bool("reject-all") {
					log("  - skipping %s\n", color.YellowString(deviceTag[1]))
					continue
				} else {
					var proceed bool
					if err := survey.AskOne(&survey.Confirm{
						Message: fmt.Sprintf("share encryption key with %s"+colors.bold("?"),
							color.YellowString(deviceTag[1])),
					}, &proceed); err != nil {
						return err
					}
					if proceed {
						// will proceed
					} else {
						// won't proceed
						var deleteDevice bool
						if err := survey.AskOne(&survey.Confirm{
							Message: fmt.Sprintf("  delete %s"+colors.bold("'s announcement?"), color.YellowString(deviceTag[1])),
						}, &deleteDevice); err != nil {
							return err
						}

						if deleteDevice {
							log("  - deleting %s\n", color.YellowString(deviceTag[1]))
							deletion := nostr.Event{
								CreatedAt: nostr.Now(),
								Kind:      5,
								Tags: nostr.Tags{
									{"e", deviceEvt.ID.Hex()},
								},
							}
							if err := kr.SignEvent(ctx, &deletion); err != nil {
								return fmt.Errorf("failed to sign deletion '%s': %w", deletion.GetID().Hex(), err)
							}
							if err := publishFlow(ctx, c, kr, deletion, relayList); err != nil {
								return fmt.Errorf("publish flow failed: %w", err)
							}
						} else {
							log("  - skipped\n")
						}

						continue
					}
				}

				log("- sending encryption key to new device %s\n", color.YellowString(deviceTag[1]))
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

				if err := publishFlow(ctx, c, kr, evt4455, relayList); err != nil {
					log(color.RedString("failed to publish key message: %v\n"), err)
				} else {
					log("  - encryption key sent to %s\n", color.GreenString(deviceTag[1]))
				}
			}
		}

		stdout(ePub.Hex())
		return nil
	},
}
