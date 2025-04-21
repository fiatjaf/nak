package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nip49"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/urfave/cli/v3"
)

var key = &cli.Command{
	Name:                      "key",
	Usage:                     "operations on secret keys: generate, derive, encrypt, decrypt",
	Description:               ``,
	DisableSliceFlagSeparator: true,
	Commands: []*cli.Command{
		generate,
		public,
		encryptKey,
		decryptKey,
		combine,
	},
}

var generate = &cli.Command{
	Name:                      "generate",
	Usage:                     "generates a secret key",
	Description:               ``,
	DisableSliceFlagSeparator: true,
	Action: func(ctx context.Context, c *cli.Command) error {
		sec := nostr.Generate()
		stdout(sec.Hex())
		return nil
	},
}

var public = &cli.Command{
	Name:                      "public",
	Usage:                     "computes a public key from a secret key",
	Description:               ``,
	ArgsUsage:                 "[secret]",
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "with-parity",
			Usage: "output 33 bytes instead of 32, the first one being either '02' or '03', a prefix indicating whether this pubkey is even or odd.",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		for sk := range getSecretKeysFromStdinLinesOrSlice(ctx, c, c.Args().Slice()) {
			_, pk := btcec.PrivKeyFromBytes(sk[:])

			if c.Bool("with-parity") {
				stdout(hex.EncodeToString(pk.SerializeCompressed()))
			} else {
				stdout(hex.EncodeToString(pk.SerializeCompressed()[1:]))
			}
		}
		return nil
	},
}

var encryptKey = &cli.Command{
	Name:                      "encrypt",
	Usage:                     "encrypts a secret key and prints an ncryptsec code",
	Description:               `uses the nip49 standard.`,
	ArgsUsage:                 "<secret> <password>",
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:        "logn",
			Usage:       "the bigger the number the harder it will be to bruteforce the password",
			Value:       16,
			DefaultText: "16",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		keys := make([]string, 0, 1)
		var password string
		switch c.Args().Len() {
		case 1:
			password = c.Args().Get(0)
		case 2:
			keys = append(keys, c.Args().Get(0))
			password = c.Args().Get(1)
		}
		if password == "" {
			return fmt.Errorf("no password given")
		}
		for sec := range getSecretKeysFromStdinLinesOrSlice(ctx, c, keys) {
			ncryptsec, err := nip49.Encrypt(sec, password, uint8(c.Int("logn")), 0x02)
			if err != nil {
				ctx = lineProcessingError(ctx, "failed to encrypt: %s", err)
				continue
			}
			stdout(ncryptsec)
		}
		return nil
	},
}

var decryptKey = &cli.Command{
	Name:                      "decrypt",
	Usage:                     "takes an ncrypsec and a password and decrypts it into an nsec",
	Description:               `uses the nip49 standard.`,
	ArgsUsage:                 "<ncryptsec-code> <password>",
	DisableSliceFlagSeparator: true,
	Action: func(ctx context.Context, c *cli.Command) error {
		var ncryptsec string
		var password string
		switch c.Args().Len() {
		case 2:
			ncryptsec = c.Args().Get(0)
			password = c.Args().Get(1)
			if password == "" {
				return fmt.Errorf("no password given")
			}
			sk, err := nip49.Decrypt(ncryptsec, password)
			if err != nil {
				return fmt.Errorf("failed to decrypt: %s", err)
			}
			stdout(sk.Hex())
			return nil
		case 1:
			if arg := c.Args().Get(0); strings.HasPrefix(arg, "ncryptsec1") {
				ncryptsec = arg
				if sk, err := promptDecrypt(ncryptsec); err != nil {
					return err
				} else {
					stdout(sk.Hex())
					return nil
				}
			} else {
				password = c.Args().Get(0)
				for ncryptsec := range getStdinLinesOrArgumentsFromSlice([]string{ncryptsec}) {
					sk, err := nip49.Decrypt(ncryptsec, password)
					if err != nil {
						ctx = lineProcessingError(ctx, "failed to decrypt: %s", err)
						continue
					}
					stdout(sk.Hex())
				}
				return nil
			}
		default:
			return fmt.Errorf("invalid number of arguments")
		}
	},
}

var combine = &cli.Command{
	Name:  "combine",
	Usage: "combines two or more pubkeys using musig2",
	Description: `The public keys must have 33 bytes (66 characters hex), with the 02 or 03 prefix. It is common in Nostr to drop that first byte, so you'll have to derive the public keys again from the private keys in order to get it back.

However, if the intent is to check if two existing Nostr pubkeys match a given combined pubkey, then it might be sufficient to calculate the combined key for all the possible combinations of pubkeys in the input.`,
	ArgsUsage:                 "[pubkey...]",
	DisableSliceFlagSeparator: true,
	Action: func(ctx context.Context, c *cli.Command) error {
		type Combination struct {
			Variants []string `json:"input_variants"`
			Output   struct {
				XOnly   string `json:"x_only"`
				Variant string `json:"variant"`
			} `json:"combined_key"`
		}

		type Result struct {
			Keys         []string      `json:"keys"`
			Combinations []Combination `json:"combinations"`
		}

		result := Result{}

		result.Keys = c.Args().Slice()
		keyGroups := make([][]*btcec.PublicKey, 0, len(result.Keys))

		for i, keyhex := range result.Keys {
			keyb, err := hex.DecodeString(keyhex)
			if err != nil {
				return fmt.Errorf("error parsing key %s: %w", keyhex, err)
			}

			if len(keyb) == 32 /* we'll use both the 02 and the 03 prefix versions */ {
				group := make([]*btcec.PublicKey, 2)
				for i, prefix := range []byte{0x02, 0x03} {
					pubk, err := btcec.ParsePubKey(append([]byte{prefix}, keyb...))
					if err != nil {
						log("error parsing key %s: %s", keyhex, err)
						continue
					}
					group[i] = pubk
				}
				keyGroups = append(keyGroups, group)
			} else /* assume it's 33 */ {
				pubk, err := btcec.ParsePubKey(keyb)
				if err != nil {
					return fmt.Errorf("error parsing key %s: %w", keyhex, err)
				}
				keyGroups = append(keyGroups, []*btcec.PublicKey{pubk})

				// remove the leading byte from the output just so it is all uniform
				result.Keys[i] = result.Keys[i][2:]
			}
		}

		result.Combinations = make([]Combination, 0, 16)

		var fn func(prepend int, curr []int)
		fn = func(prepend int, curr []int) {
			curr = append([]int{prepend}, curr...)
			if len(curr) == len(keyGroups) {
				combi := Combination{
					Variants: make([]string, len(keyGroups)),
				}

				combining := make([]*btcec.PublicKey, len(keyGroups))
				for g, altKeys := range keyGroups {
					altKey := altKeys[curr[g]]
					variant := secp256k1.PubKeyFormatCompressedEven
					if altKey.Y().Bit(0) == 1 {
						variant = secp256k1.PubKeyFormatCompressedOdd
					}
					combi.Variants[g] = hex.EncodeToString([]byte{variant})
					combining[g] = altKey
				}

				agg, _, _, err := musig2.AggregateKeys(combining, true)
				if err != nil {
					log("error aggregating: %s", err)
					return
				}

				serialized := agg.FinalKey.SerializeCompressed()
				combi.Output.XOnly = hex.EncodeToString(serialized[1:])
				combi.Output.Variant = hex.EncodeToString(serialized[0:1])
				result.Combinations = append(result.Combinations, combi)
				return
			}

			fn(0, curr)
			if len(keyGroups[len(keyGroups)-len(curr)-1]) > 1 {
				fn(1, curr)
			}
		}

		fn(0, nil)
		if len(keyGroups[len(keyGroups)-1]) > 1 {
			fn(1, nil)
		}

		res, _ := json.MarshalIndent(result, "", "  ")
		stdout(string(res))

		return nil
	},
}

func getSecretKeysFromStdinLinesOrSlice(ctx context.Context, _ *cli.Command, keys []string) chan nostr.SecretKey {
	ch := make(chan nostr.SecretKey)
	go func() {
		for sec := range getStdinLinesOrArgumentsFromSlice(keys) {
			if sec == "" {
				continue
			}

			var sk nostr.SecretKey
			if strings.HasPrefix(sec, "nsec1") {
				_, data, err := nip19.Decode(sec)
				if err != nil {
					ctx = lineProcessingError(ctx, "invalid nsec code: %s", err)
					continue
				}
				sk = data.(nostr.SecretKey)
			}

			sk, err := nostr.SecretKeyFromHex(sec)
			if err != nil {
				ctx = lineProcessingError(ctx, "invalid hex key: %s", err)
				continue
			}

			ch <- sk
		}
		close(ch)
	}()
	return ch
}
