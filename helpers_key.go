package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/keyer"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nip42"
	"fiatjaf.com/nostr/nip46"
	"fiatjaf.com/nostr/nip49"
	"github.com/chzyer/readline"
	"github.com/denisbrodbeck/machineid"
	"github.com/fatih/color"
	"github.com/mattn/go-tty/v2"
	"github.com/urfave/cli/v3"
)

// the default secret key we will use when "--sec" is not provided
func defaultKey() nostr.SecretKey {
	mid, err := machineid.ID()
	if err != nil {
		k := nostr.SecretKey{}
		k[29] = 'n'
		k[30] = 'a'
		k[31] = 'k'
		return k
	}

	return sha256.Sum256([]byte(mid))
}

func gatherKeyerFromArguments(ctx context.Context, c *cli.Command) (nostr.Keyer, nostr.SecretKey, error) {
	key, bunker, err := gatherSecretKeyOrBunkerFromArguments(ctx, c)
	if err != nil {
		return nil, nostr.SecretKey{}, err
	}

	var kr nostr.Keyer
	if bunker != nil {
		kr = keyer.NewBunkerSignerFromBunkerClient(bunker)
	} else {
		kr = keyer.NewPlainKeySigner(key)
	}

	return kr, key, nil
}

func gatherSecretKeyOrBunkerFromArguments(ctx context.Context, c *cli.Command) (nostr.SecretKey, *nip46.BunkerClient, error) {
	sec := c.String("sec")
	if strings.HasPrefix(sec, "bunker://") {
		// it's a bunker
		bunkerURL := sec
		clientKeyHex := c.String("connect-as")
		var clientKey nostr.SecretKey

		if clientKeyHex != "" {
			var err error
			clientKey, err = nostr.SecretKeyFromHex(clientKeyHex)
			if err != nil {
				return nostr.SecretKey{}, nil, fmt.Errorf("bunker client key '%s' is invalid: %w", clientKeyHex, err)
			}
		} else {
			clientKey = defaultKey()
		}

		logverbose("[nip46]: connecting to %s with client key %s\n", bunkerURL, clientKey.Hex())

		bunker, err := nip46.ConnectBunker(ctx, clientKey, bunkerURL, nil, func(s string) {
			log(color.CyanString("[nip46]: open the following URL: %s"), s)
		})
		if err != nil {
			return nostr.SecretKey{}, nil, fmt.Errorf("failed to connect to %s: %w", bunkerURL, err)
		}

		return nostr.SecretKey{}, bunker, err
	}

	if c.Bool("prompt-sec") {
		var err error
		sec, err = askPassword("type your secret key as ncryptsec, nsec or hex: ", nil)
		if err != nil {
			return nostr.SecretKey{}, nil, fmt.Errorf("failed to get secret key: %w", err)
		}
	}

	if strings.HasPrefix(sec, "ncryptsec1") {
		sk, err := promptDecrypt(sec)
		if err != nil {
			return nostr.SecretKey{}, nil, fmt.Errorf("failed to decrypt: %w", err)
		}
		return sk, nil, nil
	}

	if prefix, ski, err := nip19.Decode(sec); err == nil && prefix == "nsec" {
		return ski.(nostr.SecretKey), nil, nil
	}

	sk, err := nostr.SecretKeyFromHex(sec)
	if err != nil {
		return nostr.SecretKey{}, nil, fmt.Errorf("invalid secret key: %w", err)
	}

	return sk, nil, nil
}

func authSigner(ctx context.Context, c *cli.Command, log func(s string, args ...any), authEvent *nostr.Event) (err error) {
	defer func() {
		if err != nil {
			cleanUrl, _ := strings.CutPrefix(nip42.GetRelayURLFromAuthEvent(*authEvent), "wss://")
			log("%s auth failed: %s\n", colors.errorf(cleanUrl), err)
		}
	}()

	if !c.Bool("auth") && !c.Bool("force-pre-auth") {
		return fmt.Errorf("auth required, but --auth flag not given")
	}

	kr, _, err := gatherKeyerFromArguments(ctx, c)
	if err != nil {
		return err
	}

	pk, _ := kr.GetPublicKey(ctx)
	npub := nip19.EncodeNpub(pk)
	log("authenticating as %s... ", color.YellowString("%s…%s", npub[0:7], npub[58:]))

	return kr.SignEvent(ctx, authEvent)
}

func promptDecrypt(ncryptsec string) (nostr.SecretKey, error) {
	for i := 1; i < 4; i++ {
		var attemptStr string
		if i > 1 {
			attemptStr = fmt.Sprintf(" [%d/3]", i)
		}
		password, err := askPassword("type the password to decrypt your secret key"+attemptStr+": ", nil)
		if err != nil {
			return nostr.SecretKey{}, err
		}
		sec, err := nip49.Decrypt(ncryptsec, password)
		if err != nil {
			continue
		}
		return sec, nil
	}
	return nostr.SecretKey{}, fmt.Errorf("couldn't decrypt private key")
}

func askPassword(msg string, shouldAskAgain func(answer string) bool) (string, error) {
	if isPiped() {
		// use TTY method when stdin is piped
		tty, err := tty.Open()
		if err != nil {
			return "", fmt.Errorf("can't prompt for a secret key when processing data from a pipe on this system (failed to open /dev/tty: %w), try again without --prompt-sec or provide the key via --sec or NOSTR_SECRET_KEY environment variable", err)
		}
		defer tty.Close()
		for {
			// print the prompt to stderr so it's visible to the user
			log(color.YellowString(msg))

			// read password from TTY with masking
			password, err := tty.ReadPassword()
			if err != nil {
				return "", err
			}

			// print newline after password input
			fmt.Fprintln(os.Stderr)

			answer := strings.TrimSpace(string(password))
			if shouldAskAgain != nil && shouldAskAgain(answer) {
				continue
			}
			return answer, nil
		}
	} else {
		// use normal readline method when stdin is not piped
		config := &readline.Config{
			Stdout:                 os.Stderr,
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
