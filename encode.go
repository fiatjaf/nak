package main

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"iter"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"github.com/urfave/cli/v3"
)

func parseProfilePointerInput(target string) (nostr.ProfilePointer, bool) {
	var profilePtr nostr.ProfilePointer
	if err := stdjson.Unmarshal([]byte(target), &profilePtr); err != nil || profilePtr.PublicKey == nostr.ZeroPK {
		return nostr.ProfilePointer{}, false
	}
	return profilePtr, true
}

func parseEventPointerInput(target string) (nostr.EventPointer, bool) {
	var eventPtr nostr.EventPointer
	if err := stdjson.Unmarshal([]byte(target), &eventPtr); err != nil || eventPtr.ID == nostr.ZeroID {
		return nostr.EventPointer{}, false
	}
	return eventPtr, true
}

func parseEntityPointerInput(target string) (nostr.EntityPointer, bool) {
	var entityPtr nostr.EntityPointer
	if err := stdjson.Unmarshal([]byte(target), &entityPtr); err != nil || entityPtr.PublicKey == nostr.ZeroPK || entityPtr.Kind == 0 {
		return nostr.EntityPointer{}, false
	}
	return entityPtr, true
}

func getEncodeSubcommandInput(args cli.Args, allowBlank bool) iter.Seq[string] {
	if args.Len() > 0 {
		return func(yield func(string) bool) {
			for _, arg := range args.Slice() {
				if !yield(arg) {
					return
				}
			}
		}
	}

	return func(yield func(string) bool) {
		for jsonStr := range getJsonsOrBlank() {
			if jsonStr == "{}" {
				if allowBlank {
					yield("")
				}
				return
			}

			if !yield(jsonStr) {
				return
			}
		}
	}
}

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
		  "id":"7b225d32d3edb978dba1adfd9440105646babbabbda181ea383f74ba53c3be19",
		  "relays":["wss://nada.zero"],
		  "author":"ebb6ff85430705651b311ed51328767078fd790b14f02d22efba68d5513376bc"
		} | nak encode`,
	DisableSliceFlagSeparator: true,
	Action: func(ctx context.Context, c *cli.Command) error {
		if c.Args().Len() != 0 {
			switch c.Args().First() {
			case "naddr", "nevent", "npub", "nprofile", "nsec":
				return nil
			}

			return fmt.Errorf("unknown encode target '%s'", c.Args().First())
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

			if eventPtr, ok := parseEventPointerInput(jsonStr); ok {
				stdout(nip19.EncodeNevent(eventPtr.ID, nostr.AppendUnique(relays, eventPtr.Relays...), eventPtr.Author))
				continue
			}

			if entityPtr, ok := parseEntityPointerInput(jsonStr); ok {
				stdout(nip19.EncodeNaddr(entityPtr.PublicKey, entityPtr.Kind, entityPtr.Identifier, nostr.AppendUnique(relays, entityPtr.Relays...)))
				continue
			}

			if profilePtr, ok := parseProfilePointerInput(jsonStr); ok {
				stdout(nip19.EncodeNprofile(profilePtr.PublicKey, nostr.AppendUnique(relays, profilePtr.Relays...)))
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
						ctx = lineProcessingError(ctx, "invalid public key '%s': %s", target, err)
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
						ctx = lineProcessingError(ctx, "invalid private key '%s': %s", target, err)
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
					Usage:   "attach relay hints to the code",
				},
				&BoolIntFlag{
					Name:  "outbox",
					Usage: "automatically appends outbox relays to the code",
					Value: 3,
				},
			},
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				for target := range getEncodeSubcommandInput(c.Args(), false) {
					relays := c.StringSlice("relay")
					pk := nostr.ZeroPK

					if profilePtr, ok := parseProfilePointerInput(target); ok {
						pk = profilePtr.PublicKey
						relays = nostr.AppendUnique(relays, profilePtr.Relays...)
					} else {
						var err error
						pk, err = nostr.PubKeyFromHexCheap(target)
						if err != nil {
							ctx = lineProcessingError(ctx, "invalid public key '%s': %s", target, err)
							continue
						}
					}

					if getBoolInt(c, "outbox") > 0 {
						for _, r := range sys.FetchOutboxRelays(ctx, pk, int(getBoolInt(c, "outbox"))) {
							relays = nostr.AppendUnique(relays, r)
						}
					}

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
				&cli.StringSliceFlag{
					Name:    "relay",
					Aliases: []string{"r"},
					Usage:   "attach relay hints to the code",
				},
				&BoolIntFlag{
					Name:  "outbox",
					Usage: "automatically appends outbox relays to the code",
					Value: 3,
				},
			},
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				for target := range getEncodeSubcommandInput(c.Args(), false) {
					id := nostr.ZeroID
					author := getPubKey(c, "author")
					relays := c.StringSlice("relay")

					if eventPtr, ok := parseEventPointerInput(target); ok {
						id = eventPtr.ID
						relays = nostr.AppendUnique(relays, eventPtr.Relays...)
						if author == nostr.ZeroPK {
							author = eventPtr.Author
						}
					} else {
						var err error
						id, err = parseEventID(target)
						if err != nil {
							ctx = lineProcessingError(ctx, "invalid event id: %s", target)
							continue
						}
					}

					if getBoolInt(c, "outbox") > 0 && author != nostr.ZeroPK {
						for _, r := range sys.FetchOutboxRelays(ctx, author, int(getBoolInt(c, "outbox"))) {
							relays = nostr.AppendUnique(relays, r)
						}
					}

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
					Name:    "identifier",
					Aliases: []string{"d"},
					Usage:   "the \"d\" tag identifier of this replaceable event -- can also be read from stdin",
				},
				&PubKeyFlag{
					Name:    "pubkey",
					Usage:   "pubkey of the naddr author",
					Aliases: []string{"author", "a", "p"},
				},
				&cli.IntFlag{
					Name:    "kind",
					Aliases: []string{"k"},
					Usage:   "kind of referred replaceable event",
				},
				&cli.StringSliceFlag{
					Name:    "relay",
					Aliases: []string{"r"},
					Usage:   "attach relay hints to the code",
				},
				&BoolIntFlag{
					Name:  "outbox",
					Usage: "automatically appends outbox relays to the code",
					Value: 3,
				},
			},
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				for target := range getEncodeSubcommandInput(c.Args(), true) {
					pubkey := getPubKey(c, "pubkey")
					kind := nostr.Kind(c.Int("kind"))
					d := c.String("identifier")
					relays := c.StringSlice("relay")

					if entityPtr, ok := parseEntityPointerInput(target); ok {
						relays = nostr.AppendUnique(relays, entityPtr.Relays...)
						if pubkey == nostr.ZeroPK {
							pubkey = entityPtr.PublicKey
						}
						if kind == 0 {
							kind = entityPtr.Kind
						}
						if !c.IsSet("identifier") {
							d = entityPtr.Identifier
						}
					} else if target != "" {
						d = target
					}

					if pubkey == nostr.ZeroPK {
						ctx = lineProcessingError(ctx, "pubkey must be set")
						continue
					}

					if kind == 0 {
						ctx = lineProcessingError(ctx, "kind must be set")
						continue
					}

					if kind.IsAddressable() {
						if d == "" {
							ctx = lineProcessingError(ctx, "\"d\" tag identifier must be set for addressable events")
							continue
						}
					} else if kind.IsReplaceable() {
						if d != "" {
							ctx = lineProcessingError(ctx, "\"d\" tag identifier must not be set for replaceable events")
							continue
						}
					} else {
						ctx = lineProcessingError(ctx, "can only encode addressable events")
						continue
					}

					if getBoolInt(c, "outbox") > 0 {
						for _, r := range sys.FetchOutboxRelays(ctx, pubkey, int(getBoolInt(c, "outbox"))) {
							relays = nostr.AppendUnique(relays, r)
						}
					}

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
