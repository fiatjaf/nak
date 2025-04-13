package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/keyer"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/nip46"
	"github.com/nbd-wtf/go-nostr/nip49"
	"golang.org/x/term"
)

var defaultKeyFlags = []cli.Flag{
	&cli.StringFlag{
		Name:        "sec",
		Usage:       "secret key to sign the event, as nsec, ncryptsec or hex, or a bunker URL, it is more secure to use the environment variable NOSTR_SECRET_KEY than this flag",
		DefaultText: "the key '1'",
		Aliases:     []string{"connect"},
		Category:    CATEGORY_SIGNER,
	},
	&cli.BoolFlag{
		Name:     "prompt-sec",
		Usage:    "prompt the user to paste a hex or nsec with which to sign the event",
		Category: CATEGORY_SIGNER,
	},
	&cli.StringFlag{
		Name:        "connect-as",
		Usage:       "private key to use when communicating with nip46 bunkers",
		DefaultText: "a random key",
		Category:    CATEGORY_SIGNER,
		Sources:     cli.EnvVars("NOSTR_CLIENT_KEY"),
	},
}

func gatherKeyerFromArguments(ctx context.Context, c *cli.Command) (nostr.Keyer, string, error) {
	key, bunker, err := gatherSecretKeyOrBunkerFromArguments(ctx, c)
	if err != nil {
		return nil, "", err
	}

	var kr nostr.Keyer
	if bunker != nil {
		kr = keyer.NewBunkerSignerFromBunkerClient(bunker)
	} else {
		kr, err = keyer.NewPlainKeySigner(key)
	}

	return kr, key, err
}

func gatherSecretKeyOrBunkerFromArguments(ctx context.Context, c *cli.Command) (string, *nip46.BunkerClient, error) {
	var err error

	sec := c.String("sec")
	if strings.HasPrefix(sec, "bunker://") {
		// it's a bunker
		bunkerURL := sec
		clientKey := c.String("connect-as")
		if clientKey != "" {
			clientKey = strings.Repeat("0", 64-len(clientKey)) + clientKey
		} else {
			clientKey = nostr.GeneratePrivateKey()
		}
		bunker, err := nip46.ConnectBunker(ctx, clientKey, bunkerURL, nil, func(s string) {
			log(color.CyanString("[nip46]: open the following URL: %s"), s)
		})
		return "", bunker, err
	}

	// take private from flags, environment variable or default to 1
	if sec == "" {
		if key, ok := os.LookupEnv("NOSTR_SECRET_KEY"); ok {
			sec = key
		} else {
			sec = "0000000000000000000000000000000000000000000000000000000000000001"
		}
	}

	if c.Bool("prompt-sec") {
		sec, err = askPassword("type your secret key as ncryptsec, nsec or hex: ", nil)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get secret key: %w", err)
		}
	}

	if strings.HasPrefix(sec, "ncryptsec1") {
		sec, err = promptDecrypt(sec)
		if err != nil {
			return "", nil, fmt.Errorf("failed to decrypt: %w", err)
		}
	} else if bsec, err := hex.DecodeString(leftPadKey(sec)); err == nil {
		sec = hex.EncodeToString(bsec)
	} else if prefix, hexvalue, err := nip19.Decode(sec); err != nil {
		return "", nil, fmt.Errorf("invalid nsec: %w", err)
	} else if prefix == "nsec" {
		sec = hexvalue.(string)
	}

	if ok := nostr.IsValid32ByteHex(sec); !ok {
		return "", nil, fmt.Errorf("invalid secret key")
	}

	return sec, nil, nil
}

func promptDecrypt(ncryptsec string) (string, error) {
	for i := 1; i < 4; i++ {
		var attemptStr string
		if i > 1 {
			attemptStr = fmt.Sprintf(" [%d/3]", i)
		}
		password, err := askPassword("type the password to decrypt your secret key"+attemptStr+": ", nil)
		if err != nil {
			return "", err
		}
		sec, err := nip49.Decrypt(ncryptsec, password)
		if err != nil {
			continue
		}
		return sec, nil
	}
	return "", fmt.Errorf("couldn't decrypt private key")
}

func askPassword(msg string, shouldAskAgain func(answer string) bool) (string, error) {
	if isPiped() {
		// Use TTY method when stdin is piped
		tty, err := os.Open("/dev/tty")
		if err != nil {
			return "", fmt.Errorf("can't prompt for a secret key when processing data from a pipe on this system (failed to open /dev/tty: %w), try again without --prompt-sec or provide the key via --sec or NOSTR_SECRET_KEY environment variable", err)
		}
		defer tty.Close()

		for {
			// Print the prompt to stderr so it's visible to the user
			fmt.Fprintf(color.Error, color.YellowString(msg))

			// Read password from TTY with masking
			password, err := term.ReadPassword(int(tty.Fd()))
			if err != nil {
				return "", err
			}

			// Print newline after password input
			fmt.Fprintln(color.Error)

			answer := strings.TrimSpace(string(password))
			if shouldAskAgain != nil && shouldAskAgain(answer) {
				continue
			}
			return answer, nil
		}
	} else {
		// Use normal readline method when stdin is not piped
		config := &readline.Config{
			Stdout:                 color.Error,
			Prompt:                 color.YellowString(msg),
			InterruptPrompt:        "^C",
			DisableAutoSaveHistory: true,
			EnableMask:             true,
			MaskRune:               '*',
		}

		rl, err := readline.NewEx(config)
		if err != nil {
			return "", err
		}

		for {
			answer, err := rl.Readline()
			if err != nil {
				return "", err
			}
			answer = strings.TrimSpace(answer)
			if shouldAskAgain != nil && shouldAskAgain(answer) {
				continue
			}
			return answer, err
		}
	}
}
