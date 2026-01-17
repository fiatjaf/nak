package main

import (
	"context"
	"encoding/hex"
	stdjson "encoding/json"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip05"
	"fiatjaf.com/nostr/nip19"
	"github.com/urfave/cli/v3"
)

var decode = &cli.Command{
	Name:  "decode",
	Usage: "decodes nip19, nip21, nip05 or hex entities",
	Description: `example usage:
		nak decode npub1uescmd5krhrmj9rcura833xpke5eqzvcz5nxjw74ufeewf2sscxq4g7chm
		nak decode nevent1qqs29yet5tp0qq5xu5qgkeehkzqh5qu46739axzezcxpj4tjlkx9j7gpr4mhxue69uhkummnw3ez6ur4vgh8wetvd3hhyer9wghxuet5sh59ud
		nak decode nprofile1qqsrhuxx8l9ex335q7he0f09aej04zpazpl0ne2cgukyawd24mayt8gpz4mhxue69uhk2er9dchxummnw3ezumrpdejqz8thwden5te0dehhxarj94c82c3wwajkcmr0wfjx2u3wdejhgqgcwaehxw309aex2mrp0yhxummnw3exzarf9e3k7mgnp0sh5
		nak decode nsec1jrmyhtjhgd9yqalps8hf9mayvd58852gtz66m7tqpacjedkp6kxq4dyxsr`,
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "id",
			Aliases: []string{"e"},
			Usage:   "return just the event id, if applicable",
		},
		&cli.BoolFlag{
			Name:    "pubkey",
			Aliases: []string{"p"},
			Usage:   "return just the pubkey, if applicable",
		},
	},
	ArgsUsage: "<npub | nprofile | nip05 | nevent | naddr | nsec>",
	Action: func(ctx context.Context, c *cli.Command) error {
		for input := range getStdinLinesOrArguments(c.Args()) {
			if strings.HasPrefix(input, "nostr:") {
				input = input[6:]
			}

			_, data, err := nip19.Decode(input)
			if err == nil {
				switch v := data.(type) {
				case nostr.SecretKey:
					stdout(v.Hex())
					continue
				case nostr.PubKey:
					stdout(v.Hex())
					continue
				case [32]byte:
					stdout(hex.EncodeToString(v[:]))
					continue
				case nostr.EventPointer:
					if c.Bool("id") {
						stdout(v.ID.Hex())
						continue
					}
					out, _ := stdjson.MarshalIndent(v, "", "  ")
					stdout(string(out))
					continue
				case nostr.ProfilePointer:
					if c.Bool("pubkey") {
						stdout(v.PublicKey.Hex())
						continue
					}
					out, _ := stdjson.MarshalIndent(v, "", "  ")
					stdout(string(out))
					continue
				case nostr.EntityPointer:
					out, _ := stdjson.MarshalIndent(v, "", "  ")
					stdout(string(out))
					continue
				}
			}

			pp, _ := nip05.QueryIdentifier(ctx, input)
			if pp != nil {
				if c.Bool("pubkey") {
					stdout(pp.PublicKey.Hex())
					continue
				}
				out, _ := stdjson.MarshalIndent(pp, "", "  ")
				stdout(string(out))
				continue
			}

			ctx = lineProcessingError(ctx, "couldn't decode input '%s'", input)
		}

		exitIfLineProcessingError(ctx)
		return nil
	},
}
