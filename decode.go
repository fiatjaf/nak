package main

import (
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	sdk "github.com/nbd-wtf/nostr-sdk"
	"github.com/urfave/cli/v2"
)

var decode = &cli.Command{
	Name:  "decode",
	Usage: "decodes nip19, nip21, nip05 or hex entities",
	Description: `example usage:
		nak decode npub1uescmd5krhrmj9rcura833xpke5eqzvcz5nxjw74ufeewf2sscxq4g7chm
		nak decode nevent1qqs29yet5tp0qq5xu5qgkeehkzqh5qu46739axzezcxpj4tjlkx9j7gpr4mhxue69uhkummnw3ez6ur4vgh8wetvd3hhyer9wghxuet5sh59ud
		nak decode nprofile1qqsrhuxx8l9ex335q7he0f09aej04zpazpl0ne2cgukyawd24mayt8gpz4mhxue69uhk2er9dchxummnw3ezumrpdejqz8thwden5te0dehhxarj94c82c3wwajkcmr0wfjx2u3wdejhgqgcwaehxw309aex2mrp0yhxummnw3exzarf9e3k7mgnp0sh5
		nak decode nsec1jrmyhtjhgd9yqalps8hf9mayvd58852gtz66m7tqpacjedkp6kxq4dyxsr`,
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
	Action: func(c *cli.Context) error {
		for input := range getStdinLinesOrFirstArgument(c.Args().First()) {
			if strings.HasPrefix(input, "nostr:") {
				input = input[6:]
			}

			var decodeResult DecodeResult
			if b, err := hex.DecodeString(input); err == nil {
				if len(b) == 64 {
					decodeResult.HexResult.PossibleTypes = []string{"sig"}
					decodeResult.HexResult.Signature = hex.EncodeToString(b)
				} else if len(b) == 32 {
					decodeResult.HexResult.PossibleTypes = []string{"pubkey", "private_key", "event_id"}
					decodeResult.HexResult.ID = hex.EncodeToString(b)
					decodeResult.HexResult.PrivateKey = hex.EncodeToString(b)
					decodeResult.HexResult.PublicKey = hex.EncodeToString(b)
				} else {
					lineProcessingError(c, "hex string with invalid number of bytes: %d", len(b))
					continue
				}
			} else if evp := sdk.InputToEventPointer(input); evp != nil {
				decodeResult = DecodeResult{EventPointer: evp}
			} else if pp := sdk.InputToProfile(c.Context, input); pp != nil {
				decodeResult = DecodeResult{ProfilePointer: pp}
			} else if prefix, value, err := nip19.Decode(input); err == nil && prefix == "naddr" {
				ep := value.(nostr.EntityPointer)
				decodeResult = DecodeResult{EntityPointer: &ep}
			} else if prefix, value, err := nip19.Decode(input); err == nil && prefix == "nsec" {
				decodeResult.PrivateKey.PrivateKey = value.(string)
				decodeResult.PrivateKey.PublicKey, _ = nostr.GetPublicKey(value.(string))
			} else {
				lineProcessingError(c, "couldn't decode input '%s': %s", input, err)
				continue
			}

			stdout(decodeResult.JSON())

		}

		exitIfLineProcessingError(c)
		return nil
	},
}

type DecodeResult struct {
	*nostr.EventPointer
	*nostr.ProfilePointer
	*nostr.EntityPointer
	HexResult struct {
		PossibleTypes []string `json:"possible_types"`
		PublicKey     string   `json:"pubkey,omitempty"`
		ID            string   `json:"event_id,omitempty"`
		PrivateKey    string   `json:"private_key,omitempty"`
		Signature     string   `json:"sig,omitempty"`
	}
	PrivateKey struct {
		nostr.ProfilePointer
		PrivateKey string `json:"private_key"`
	}
}

func (d DecodeResult) JSON() string {
	var j []byte
	if d.EventPointer != nil {
		j, _ = json.MarshalIndent(d.EventPointer, "", "  ")
	} else if d.ProfilePointer != nil {
		j, _ = json.MarshalIndent(d.ProfilePointer, "", "  ")
	} else if d.EntityPointer != nil {
		j, _ = json.MarshalIndent(d.EntityPointer, "", "  ")
	} else if len(d.HexResult.PossibleTypes) > 0 {
		j, _ = json.MarshalIndent(d.HexResult, "", "  ")
	} else if d.PrivateKey.PrivateKey != "" {
		j, _ = json.MarshalIndent(d.PrivateKey, "", "  ")
	}
	return string(j)
}
