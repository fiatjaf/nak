package main

import (
	"fmt"
	"strings"

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
		for sec := range getSecretKeyFromStdinLinesOrFirstArgument(c, c.Args().First()) {
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
		for sec := range getSecretKeyFromStdinLinesOrFirstArgument(c, content) {
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
		for ncryptsec := range getStdinLinesOrFirstArgument(content) {
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

func getSecretKeyFromStdinLinesOrFirstArgument(c *cli.Context, content string) chan string {
	ch := make(chan string)
	go func() {
		for sec := range getStdinLinesOrFirstArgument(content) {
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
				lineProcessingError(c, "invalid hex secret key")
				continue
			}
			ch <- sec
		}
		close(ch)
	}()
	return ch
}
