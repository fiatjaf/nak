package main

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/nip49"
	"github.com/urfave/cli/v2"
)

var key = &cli.Command{
	Name:        "key",
	Usage:       "operations on secret keys: generate, derive, encrypt, decrypt.",
	Description: ``,
	Subcommands: []*cli.Command{
		generate,
		public,
		encrypt,
		decrypt,
		combine,
	},
}

var generate = &cli.Command{
	Name:        "generate",
	Usage:       "generates a secret key",
	Description: ``,
	Action: func(c *cli.Context) error {
		sec := nostr.GeneratePrivateKey()
		stdout(sec)
		return nil
	},
}

var public = &cli.Command{
	Name:        "public",
	Usage:       "computes a public key from a secret key",
	Description: ``,
	ArgsUsage:   "[secret]",
	Action: func(c *cli.Context) error {
		for sec := range getSecretKeysFromStdinLinesOrSlice(c, c.Args().Slice()) {
			pubkey, err := nostr.GetPublicKey(sec)
			if err != nil {
				lineProcessingError(c, "failed to derive public key: %s", err)
				continue
			}
			stdout(pubkey)
		}
		return nil
	},
}

var encrypt = &cli.Command{
	Name:        "encrypt",
	Usage:       "encrypts a secret key and prints an ncryptsec code",
	Description: `uses the NIP-49 standard.`,
	ArgsUsage:   "<secret> <password>",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:        "logn",
			Usage:       "the bigger the number the harder it will be to bruteforce the password",
			Value:       16,
			DefaultText: "16",
		},
	},
	Action: func(c *cli.Context) error {
		var content string
		var password string
		switch c.Args().Len() {
		case 1:
			content = ""
			password = c.Args().Get(0)
		case 2:
			content = c.Args().Get(0)
			password = c.Args().Get(1)
		}
		if password == "" {
			return fmt.Errorf("no password given")
		}
		for sec := range getSecretKeysFromStdinLinesOrSlice(c, []string{content}) {
			ncryptsec, err := nip49.Encrypt(sec, password, uint8(c.Int("logn")), 0x02)
			if err != nil {
				lineProcessingError(c, "failed to encrypt: %s", err)
				continue
			}
			stdout(ncryptsec)
		}
		return nil
	},
}

var decrypt = &cli.Command{
	Name:        "decrypt",
	Usage:       "takes an ncrypsec and a password and decrypts it into an nsec",
	Description: `uses the NIP-49 standard.`,
	ArgsUsage:   "<ncryptsec-code> <password>",
	Action: func(c *cli.Context) error {
		var content string
		var password string
		switch c.Args().Len() {
		case 1:
			content = ""
			password = c.Args().Get(0)
		case 2:
			content = c.Args().Get(0)
			password = c.Args().Get(1)
		}
		if password == "" {
			return fmt.Errorf("no password given")
		}
		for ncryptsec := range getStdinLinesOrArgumentsFromSlice([]string{content}) {
			sec, err := nip49.Decrypt(ncryptsec, password)
			if err != nil {
				lineProcessingError(c, "failed to decrypt: %s", err)
				continue
			}
			nsec, _ := nip19.EncodePrivateKey(sec)
			stdout(nsec)
		}
		return nil
	},
}

var combine = &cli.Command{
	Name:        "combine",
	Usage:       "combines two or more pubkeys using musig2",
	Description: `The public keys must have 33 bytes (66 characters hex), with the 02 or 03 prefix. It is common in Nostr to drop that first byte, so you'll have to derive the public keys again from the private keys in order to get it back.`,
	ArgsUsage:   "[pubkey...]",
	Action: func(c *cli.Context) error {
		keys := make([]*btcec.PublicKey, 0, 5)
		for _, pub := range c.Args().Slice() {
			keyb, err := hex.DecodeString(pub)
			if err != nil {
				return fmt.Errorf("error parsing key %s: %w", pub, err)
			}

			pubk, err := btcec.ParsePubKey(keyb)
			if err != nil {
				return fmt.Errorf("error parsing key %s: %w", pub, err)
			}

			keys = append(keys, pubk)
		}

		agg, _, _, err := musig2.AggregateKeys(keys, true)
		if err != nil {
			return err
		}

		fmt.Println(hex.EncodeToString(agg.FinalKey.SerializeCompressed()))
		return nil
	},
}

func getSecretKeysFromStdinLinesOrSlice(c *cli.Context, keys []string) chan string {
	ch := make(chan string)
	go func() {
		for sec := range getStdinLinesOrArgumentsFromSlice(keys) {
			if sec == "" {
				continue
			}
			if strings.HasPrefix(sec, "nsec1") {
				_, data, err := nip19.Decode(sec)
				if err != nil {
					lineProcessingError(c, "invalid nsec code: %s", err)
					continue
				}
				sec = data.(string)
			}
			if !nostr.IsValid32ByteHex(sec) {
				lineProcessingError(c, "invalid hex key")
				continue
			}
			ch <- sec
		}
		close(ch)
	}()
	return ch
}
