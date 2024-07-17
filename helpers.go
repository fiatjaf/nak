package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/nip46"
	"github.com/nbd-wtf/go-nostr/nip49"
	"github.com/urfave/cli/v3"
)

const (
	LINE_PROCESSING_ERROR = iota
)

var log = func(msg string, args ...any) {
	fmt.Fprintf(color.Error, msg, args...)
}

var stdout = fmt.Println

func isPiped() bool {
	stat, _ := os.Stdin.Stat()
	return stat.Mode()&os.ModeCharDevice == 0
}

func getStdinLinesOrBlank() chan string {
	multi := make(chan string)
	if hasStdinLines := writeStdinLinesOrNothing(multi); !hasStdinLines {
		single := make(chan string, 1)
		single <- ""
		close(single)
		return single
	} else {
		return multi
	}
}

func getStdinLinesOrArguments(args cli.Args) chan string {
	return getStdinLinesOrArgumentsFromSlice(args.Slice())
}

func getStdinLinesOrArgumentsFromSlice(args []string) chan string {
	// try the first argument
	if len(args) > 0 {
		argsCh := make(chan string, 1)
		go func() {
			for _, arg := range args {
				argsCh <- arg
			}
			close(argsCh)
		}()
		return argsCh
	}

	// try the stdin
	multi := make(chan string)
	writeStdinLinesOrNothing(multi)
	return multi
}

func writeStdinLinesOrNothing(ch chan string) (hasStdinLines bool) {
	if isPiped() {
		// piped
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Buffer(make([]byte, 16*1024*1024), 256*1024*1024)
			hasEmittedAtLeastOne := false
			for scanner.Scan() {
				ch <- strings.TrimSpace(scanner.Text())
				hasEmittedAtLeastOne = true
			}
			if !hasEmittedAtLeastOne {
				ch <- ""
			}
			close(ch)
		}()
		return true
	} else {
		// not piped
		return false
	}
}

func normalizeAndValidateRelayURLs(wsurls []string) error {
	for i, wsurl := range wsurls {
		wsurl = nostr.NormalizeURL(wsurl)
		wsurls[i] = wsurl

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

func connectToAllRelays(
	ctx context.Context,
	relayUrls []string,
	forcePreAuth bool,
	opts ...nostr.PoolOption,
) (*nostr.SimplePool, []*nostr.Relay) {
	relays := make([]*nostr.Relay, 0, len(relayUrls))
	pool := nostr.NewSimplePool(ctx, opts...)
relayLoop:
	for _, url := range relayUrls {
		log("connecting to %s... ", url)
		if relay, err := pool.EnsureRelay(url); err == nil {
			if forcePreAuth {
				log("waiting for auth challenge... ")
				signer := opts[0].(nostr.WithAuthHandler)
				time.Sleep(time.Millisecond * 200)
			challengeWaitLoop:
				for {
					// beginhack
					// here starts the biggest and ugliest hack of this codebase
					if err := relay.Auth(ctx, func(authEvent *nostr.Event) error {
						challengeTag := authEvent.Tags.GetFirst([]string{"challenge", ""})
						if (*challengeTag)[1] == "" {
							return fmt.Errorf("auth not received yet *****")
						}
						return signer(authEvent)
					}); err == nil {
						// auth succeeded
						break challengeWaitLoop
					} else {
						// auth failed
						if strings.HasSuffix(err.Error(), "auth not received yet *****") {
							// it failed because we didn't receive the challenge yet, so keep waiting
							time.Sleep(time.Second)
							continue challengeWaitLoop
						} else {
							// it failed for some other reason, so skip this relay
							log(err.Error() + "\n")
							continue relayLoop
						}
					}
					// endhack
				}
			}

			relays = append(relays, relay)
			log("ok.\n")
		} else {
			log(err.Error() + "\n")
		}
	}
	return pool, relays
}

func lineProcessingError(ctx context.Context, msg string, args ...any) context.Context {
	log(msg+"\n", args...)
	return context.WithValue(ctx, LINE_PROCESSING_ERROR, true)
}

func exitIfLineProcessingError(ctx context.Context) {
	if val := ctx.Value(LINE_PROCESSING_ERROR); val != nil && val.(bool) {
		os.Exit(123)
	}
}

func gatherSecretKeyOrBunkerFromArguments(ctx context.Context, c *cli.Command) (string, *nip46.BunkerClient, error) {
	var err error

	if bunkerURL := c.String("connect"); bunkerURL != "" {
		clientKey := c.String("connect-as")
		if clientKey != "" {
			clientKey = strings.Repeat("0", 64-len(clientKey)) + clientKey
		} else {
			clientKey = nostr.GeneratePrivateKey()
		}
		bunker, err := nip46.ConnectBunker(ctx, clientKey, bunkerURL, nil, func(s string) {
			fmt.Fprintf(color.Error, color.CyanString("[nip46]: open the following URL: %s"), s)
		})
		return "", bunker, err
	}
	sec := c.String("sec")
	if c.Bool("prompt-sec") {
		if isPiped() {
			return "", nil, fmt.Errorf("can't prompt for a secret key when processing data from a pipe, try again without --prompt-sec")
		}
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
	config := &readline.Config{
		Stdout:                 color.Error,
		Prompt:                 color.YellowString(msg),
		InterruptPrompt:        "^C",
		DisableAutoSaveHistory: true,
		EnableMask:             true,
		MaskRune:               '*',
	}
	return _ask(config, msg, "", shouldAskAgain)
}

func _ask(config *readline.Config, msg string, defaultValue string, shouldAskAgain func(answer string) bool) (string, error) {
	rl, err := readline.NewEx(config)
	if err != nil {
		return "", err
	}

	rl.WriteStdin([]byte(defaultValue))
	for {
		answer, err := rl.Readline()
		if err != nil {
			return "", err
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if shouldAskAgain != nil && shouldAskAgain(answer) {
			continue
		}
		return answer, err
	}
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func leftPadKey(k string) string {
	return strings.Repeat("0", 64-len(k)) + k
}
