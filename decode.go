package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/sdk"
	"github.com/urfave/cli/v2"
)

var decode = &cli.Command{
	Name:  "decode",
	Usage: "decodes nip19, nip21, nip05 or hex entities",
	Description: `example usage:
		nak decode
		nak decode
		nak decode
		nak decode`,
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
		args := c.Args()
		if args.Len() != 1 {
			return fmt.Errorf("invalid number of arguments, need just one")
		}
		input := args.First()

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
				return fmt.Errorf("hex string with invalid number of bytes: %d", len(b))
			}
		} else if evp := sdk.InputToEventPointer(input); evp != nil {
			decodeResult = DecodeResult{EventPointer: evp}
		} else if pp := sdk.InputToProfile(input); pp != nil {
			decodeResult = DecodeResult{ProfilePointer: pp}
		} else if prefix, value, err := nip19.Decode(input); err == nil && prefix == "naddr" {
			ep := value.(nostr.EntityPointer)
			decodeResult = DecodeResult{EntityPointer: &ep}
		} else if prefix, value, err := nip19.Decode(input); err == nil && prefix == "nsec" {
			decodeResult.PrivateKey.PrivateKey = value.(string)
			decodeResult.PrivateKey.PublicKey, _ = nostr.GetPublicKey(value.(string))
		} else {
			return fmt.Errorf("couldn't decode input")
		}

		fmt.Println(decodeResult.JSON())
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
