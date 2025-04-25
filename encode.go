package main

import (
	"context"
	"fmt"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"github.com/urfave/cli/v3"
)

var encode = &cli.Command{
	Name:  "encode",
	Usage: "encodes notes and other stuff to nip19 entities",
	Description: `example usage:
		nak encode npub <pubkey-hex>
		nak encode nprofile <pubkey-hex>
		nak encode nprofile --relay <relay-url> <pubkey-hex>
		nak encode nevent <event-id>
		nak encode nevent --author <pubkey-hex> --relay <relay-url> --relay <other-relay> <event-id>
		nak encode nsec <privkey-hex>
		echo '{"pubkey":"7b225d32d3edb978dba1adfd9440105646babbabbda181ea383f74ba53c3be19","relays":["wss://nada.zero"]}' | nak encode
		echo '{
		  "id":"7b225d32d3edb978dba1adfd9440105646babbabbda181ea383f74ba53c3be19"
		  "relays":["wss://nada.zero"],
		  "author":"ebb6ff85430705651b311ed51328767078fd790b14f02d22efba68d5513376bc"
		} | nak encode`,
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "relay",
			Aliases: []string{"r"},
			Usage:   "attach relay hints to naddr code",
		},
	},
	DisableSliceFlagSeparator: true,
	Action: func(ctx context.Context, c *cli.Command) error {
		if c.Args().Len() != 0 {
			return nil
		}

		relays := c.StringSlice("relay")
		if err := normalizeAndValidateRelayURLs(relays); err != nil {
			return err
		}

		hasStdin := false
		for jsonStr := range getJsonsOrBlank() {
			if jsonStr == "{}" {
				hasStdin = false
				continue
			} else {
				hasStdin = true
			}

			var eventPtr nostr.EventPointer
			if err := json.Unmarshal([]byte(jsonStr), &eventPtr); err == nil && eventPtr.ID != nostr.ZeroID {
				stdout(nip19.EncodeNevent(eventPtr.ID, appendUnique(relays, eventPtr.Relays...), eventPtr.Author))
				continue
			}

			var profilePtr nostr.ProfilePointer
			if err := json.Unmarshal([]byte(jsonStr), &profilePtr); err == nil && profilePtr.PublicKey != nostr.ZeroPK {
				stdout(nip19.EncodeNprofile(profilePtr.PublicKey, appendUnique(relays, profilePtr.Relays...)))
				continue
			}

			var entityPtr nostr.EntityPointer
			if err := json.Unmarshal([]byte(jsonStr), &entityPtr); err == nil && entityPtr.PublicKey != nostr.ZeroPK {
				stdout(nip19.EncodeNaddr(entityPtr.PublicKey, entityPtr.Kind, entityPtr.Identifier, appendUnique(relays, entityPtr.Relays...)))
				continue
			}

			ctx = lineProcessingError(ctx, "couldn't decode JSON '%s'", jsonStr)
		}

		if !hasStdin {
			return nil
		}

		exitIfLineProcessingError(ctx)
		return nil
	},
	Commands: []*cli.Command{
		{
			Name:                      "npub",
			Usage:                     "encode a hex public key into bech32 'npub' format",
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				for target := range getStdinLinesOrArguments(c.Args()) {
					pk, err := nostr.PubKeyFromHexCheap(target)
					if err != nil {
						ctx = lineProcessingError(ctx, "invalid public key '%s': %w", target, err)
						continue
					}

					stdout(nip19.EncodeNpub(pk))
				}

				exitIfLineProcessingError(ctx)
				return nil
			},
		},
		{
			Name:                      "nsec",
			Usage:                     "encode a hex private key into bech32 'nsec' format",
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				for target := range getStdinLinesOrArguments(c.Args()) {
					sk, err := nostr.SecretKeyFromHex(target)
					if err != nil {
						ctx = lineProcessingError(ctx, "invalid private key '%s': %w", target, err)
						continue
					}

					stdout(nip19.EncodeNsec(sk))
				}

				exitIfLineProcessingError(ctx)
				return nil
			},
		},
		{
			Name:  "nprofile",
			Usage: "generate profile codes with attached relay information",
			Flags: []cli.Flag{
				&cli.StringSliceFlag{
					Name:    "relay",
					Aliases: []string{"r"},
					Usage:   "attach relay hints to nprofile code",
				},
			},
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				for target := range getStdinLinesOrArguments(c.Args()) {
					pk, err := nostr.PubKeyFromHexCheap(target)
					if err != nil {
						ctx = lineProcessingError(ctx, "invalid public key '%s': %w", target, err)
						continue
					}

					relays := c.StringSlice("relay")
					if err := normalizeAndValidateRelayURLs(relays); err != nil {
						return err
					}

					stdout(nip19.EncodeNprofile(pk, relays))
				}

				exitIfLineProcessingError(ctx)
				return nil
			},
		},
		{
			Name:  "nevent",
			Usage: "generate event codes with optionally attached relay information",
			Flags: []cli.Flag{
				&PubKeyFlag{
					Name:    "author",
					Aliases: []string{"a"},
					Usage:   "attach an author pubkey as a hint to the nevent code",
				},
			},
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				for target := range getStdinLinesOrArguments(c.Args()) {
					id, err := nostr.IDFromHex(target)
					if err != nil {
						ctx = lineProcessingError(ctx, "invalid event id: %s", target)
						continue
					}

					author := getPubKey(c, "author")
					relays := c.StringSlice("relay")
					if err := normalizeAndValidateRelayURLs(relays); err != nil {
						return err
					}

					stdout(nip19.EncodeNevent(id, relays, author))
				}

				exitIfLineProcessingError(ctx)
				return nil
			},
		},
		{
			Name:  "naddr",
			Usage: "generate codes for addressable events",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "identifier",
					Aliases:  []string{"d"},
					Usage:    "the \"d\" tag identifier of this replaceable event -- can also be read from stdin",
					Required: true,
				},
				&PubKeyFlag{
					Name:     "pubkey",
					Usage:    "pubkey of the naddr author",
					Aliases:  []string{"author", "a", "p"},
					Required: true,
				},
				&cli.IntFlag{
					Name:     "kind",
					Aliases:  []string{"k"},
					Usage:    "kind of referred replaceable event",
					Required: true,
				},
			},
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				for d := range getStdinLinesOrBlank() {
					pubkey := getPubKey(c, "pubkey")

					kind := c.Int("kind")
					if kind < 30000 || kind >= 40000 {
						return fmt.Errorf("kind must be between 30000 and 39999, got %d", kind)
					}

					if d == "" {
						d = c.String("identifier")
						if d == "" {
							ctx = lineProcessingError(ctx, "\"d\" tag identifier can't be empty")
							continue
						}
					}

					relays := c.StringSlice("relay")
					if err := normalizeAndValidateRelayURLs(relays); err != nil {
						return err
					}

					stdout(nip19.EncodeNaddr(pubkey, nostr.Kind(kind), d, relays))
				}

				exitIfLineProcessingError(ctx)
				return nil
			},
		},
	},
}
