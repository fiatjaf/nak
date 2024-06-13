package main

import (
	"fmt"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/urfave/cli/v2"
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
		nak encode nsec <privkey-hex>`,
	Before: func(c *cli.Context) error {
		if c.Args().Len() < 1 {
			return fmt.Errorf("expected more than 1 argument.")
		}
		return nil
	},
	Subcommands: []*cli.Command{
		{
			Name:  "npub",
			Usage: "encode a hex public key into bech32 'npub' format",
			Action: func(c *cli.Context) error {
				for target := range getStdinLinesOrArguments(c.Args()) {
					if ok := nostr.IsValidPublicKey(target); !ok {
						lineProcessingError(c, "invalid public key: %s", target)
						continue
					}

					if npub, err := nip19.EncodePublicKey(target); err == nil {
						stdout(npub)
					} else {
						return err
					}
				}

				exitIfLineProcessingError(c)
				return nil
			},
		},
		{
			Name:  "nsec",
			Usage: "encode a hex private key into bech32 'nsec' format",
			Action: func(c *cli.Context) error {
				for target := range getStdinLinesOrArguments(c.Args()) {
					if ok := nostr.IsValid32ByteHex(target); !ok {
						lineProcessingError(c, "invalid private key: %s", target)
						continue
					}

					if npub, err := nip19.EncodePrivateKey(target); err == nil {
						stdout(npub)
					} else {
						return err
					}
				}

				exitIfLineProcessingError(c)
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
			Action: func(c *cli.Context) error {
				for target := range getStdinLinesOrArguments(c.Args()) {
					if ok := nostr.IsValid32ByteHex(target); !ok {
						lineProcessingError(c, "invalid public key: %s", target)
						continue
					}

					relays := c.StringSlice("relay")
					if err := normalizeAndValidateRelayURLs(relays); err != nil {
						return err
					}

					if npub, err := nip19.EncodeProfile(target, relays); err == nil {
						stdout(npub)
					} else {
						return err
					}
				}

				exitIfLineProcessingError(c)
				return nil
			},
		},
		{
			Name:  "nevent",
			Usage: "generate event codes with optionally attached relay information",
			Flags: []cli.Flag{
				&cli.StringSliceFlag{
					Name:    "relay",
					Aliases: []string{"r"},
					Usage:   "attach relay hints to nevent code",
				},
				&cli.StringFlag{
					Name:    "author",
					Aliases: []string{"a"},
					Usage:   "attach an author pubkey as a hint to the nevent code",
				},
			},
			Action: func(c *cli.Context) error {
				for target := range getStdinLinesOrArguments(c.Args()) {
					if ok := nostr.IsValid32ByteHex(target); !ok {
						lineProcessingError(c, "invalid event id: %s", target)
						continue
					}

					author := c.String("author")
					if author != "" {
						if ok := nostr.IsValidPublicKey(author); !ok {
							return fmt.Errorf("invalid 'author' public key")
						}
					}

					relays := c.StringSlice("relay")
					if err := normalizeAndValidateRelayURLs(relays); err != nil {
						return err
					}

					if npub, err := nip19.EncodeEvent(target, relays, author); err == nil {
						stdout(npub)
					} else {
						return err
					}
				}

				exitIfLineProcessingError(c)
				return nil
			},
		},
		{
			Name:  "naddr",
			Usage: "generate codes for NIP-33 parameterized replaceable events",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "identifier",
					Aliases:  []string{"d"},
					Usage:    "the \"d\" tag identifier of this replaceable event -- can also be read from stdin",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "pubkey",
					Usage:    "pubkey of the naddr author",
					Aliases:  []string{"author", "a", "p"},
					Required: true,
				},
				&cli.Int64Flag{
					Name:     "kind",
					Aliases:  []string{"k"},
					Usage:    "kind of referred replaceable event",
					Required: true,
				},
				&cli.StringSliceFlag{
					Name:    "relay",
					Aliases: []string{"r"},
					Usage:   "attach relay hints to naddr code",
				},
			},
			Action: func(c *cli.Context) error {
				for d := range getStdinLinesOrBlank() {
					pubkey := c.String("pubkey")
					if ok := nostr.IsValidPublicKey(pubkey); !ok {
						return fmt.Errorf("invalid 'pubkey'")
					}

					kind := c.Int("kind")
					if kind < 30000 || kind >= 40000 {
						return fmt.Errorf("kind must be between 30000 and 39999, as per NIP-16, got %d", kind)
					}

					if d == "" {
						d = c.String("identifier")
						if d == "" {
							lineProcessingError(c, "\"d\" tag identifier can't be empty")
							continue
						}
					}

					relays := c.StringSlice("relay")
					if err := normalizeAndValidateRelayURLs(relays); err != nil {
						return err
					}

					if npub, err := nip19.EncodeEntity(pubkey, kind, d, relays); err == nil {
						stdout(npub)
					} else {
						return err
					}
				}

				exitIfLineProcessingError(c)
				return nil
			},
		},
		{
			Name:  "note",
			Usage: "generate note1 event codes (not recommended)",
			Action: func(c *cli.Context) error {
				for target := range getStdinLinesOrArguments(c.Args()) {
					if ok := nostr.IsValid32ByteHex(target); !ok {
						lineProcessingError(c, "invalid event id: %s", target)
						continue
					}

					if note, err := nip19.EncodeNote(target); err == nil {
						stdout(note)
					} else {
						return err
					}
				}

				exitIfLineProcessingError(c)
				return nil
			},
		},
	},
}
