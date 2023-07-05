package main

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

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
		if c.Args().First() == "naddr" {
			// validation will be done on the specific handler
			return nil
		}

		if c.Args().Len() < 2 {
			return fmt.Errorf("expected more than 2 arguments.")
		}
		target := c.Args().Get(c.Args().Len() - 1)
		if target == "--help" {
			return nil
		}

		return validate32BytesHex(target)
	},
	Subcommands: []*cli.Command{
		{
			Name:  "npub",
			Usage: "encode a hex private key into bech32 'npub' format",
			Action: func(c *cli.Context) error {
				if npub, err := nip19.EncodePublicKey(c.Args().First()); err == nil {
					fmt.Println(npub)
					return nil
				} else {
					return err
				}
			},
		},
		{
			Name:  "nsec",
			Usage: "encode a hex private key into bech32 'nsec' format",
			Action: func(c *cli.Context) error {
				if npub, err := nip19.EncodePrivateKey(c.Args().First()); err == nil {
					fmt.Println(npub)
					return nil
				} else {
					return err
				}
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
				relays := c.StringSlice("relay")
				if err := validateRelayURLs(relays); err != nil {
					return err
				}

				if npub, err := nip19.EncodeProfile(c.Args().First(), relays); err == nil {
					fmt.Println(npub)
					return nil
				} else {
					return err
				}
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
					Name:  "author",
					Usage: "attach an author pubkey as a hint to the nevent code",
				},
			},
			Action: func(c *cli.Context) error {
				author := c.String("author")
				if err := validate32BytesHex(author); err != nil {
					return err
				}

				relays := c.StringSlice("relay")
				if err := validateRelayURLs(relays); err != nil {
					return err
				}

				if npub, err := nip19.EncodeEvent(c.Args().First(), relays, author); err == nil {
					fmt.Println(npub)
					return nil
				} else {
					return err
				}
			},
		},
		{
			Name:  "naddr",
			Usage: "generate codes for NIP-33 parameterized replaceable events",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "identifier",
					Aliases:  []string{"d"},
					Usage:    "the \"d\" tag identifier of this replaceable event",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "pubkey",
					Usage:    "pubkey of the naddr author",
					Aliases:  []string{"p"},
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
				pubkey := c.String("pubkey")
				if err := validate32BytesHex(pubkey); err != nil {
					return err
				}

				kind := c.Int("kind")
				if kind < 30000 || kind >= 40000 {
					return fmt.Errorf("kind must be between 30000 and 39999, as per NIP-16, got %d", kind)
				}

				d := c.String("identifier")
				if d == "" {
					return fmt.Errorf("\"d\" tag identifier can't be empty")
				}

				relays := c.StringSlice("relay")
				if err := validateRelayURLs(relays); err != nil {
					return err
				}

				if npub, err := nip19.EncodeEntity(pubkey, kind, d, relays); err == nil {
					fmt.Println(npub)
					return nil
				} else {
					return err
				}
			},
		},
	},
}

func validate32BytesHex(target string) error {
	if _, err := hex.DecodeString(target); err != nil {
		return fmt.Errorf("target '%s' is not valid hex: %s", target, err)
	}
	if len(target) != 64 {
		return fmt.Errorf("expected '%s' to be 64 characters (32 bytes), got %d", target, len(target))
	}
	if strings.ToLower(target) != target {
		return fmt.Errorf("expected target to be all lowercase hex. try again with '%s'", strings.ToLower(target))
	}

	return nil
}

func validateRelayURLs(wsurls []string) error {
	for _, wsurl := range wsurls {
		u, err := url.Parse(wsurl)
		if err != nil {
			return fmt.Errorf("invalid relay url '%s': %s", wsurl, err)
		}

		if u.Scheme != "ws" && u.Scheme != "wss" {
			return fmt.Errorf("relay url must use wss:// or ws:// schemes, got '%s'", wsurl)
		}

		if u.Host == "" {
			return fmt.Errorf("relay url '%s' is missing the hostname", wsurl)
		}
	}

	return nil
}
