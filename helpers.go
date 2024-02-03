package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/nip49"
	"github.com/urfave/cli/v2"
)

const (
	LINE_PROCESSING_ERROR = iota

	BOLD_ON  = "\033[1m"
	BOLD_OFF = "\033[21m"
)

var log = func(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, msg, args...)
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

func getStdinLinesOrFirstArgument(arg string) chan string {
	// try the first argument
	if arg != "" {
		single := make(chan string, 1)
		single <- arg
		close(single)
		return single
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
			scanner.Buffer(make([]byte, 16*1024), 256*1024)
			for scanner.Scan() {
				ch <- strings.TrimSpace(scanner.Text())
			}
			close(ch)
		}()
		return true
	} else {
		// not piped
		return false
	}
}

func validateRelayURLs(wsurls []string) error {
	for _, wsurl := range wsurls {
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
	opts ...nostr.PoolOption,
) (*nostr.SimplePool, []*nostr.Relay) {
	relays := make([]*nostr.Relay, 0, len(relayUrls))
	pool := nostr.NewSimplePool(ctx, opts...)
	for _, url := range relayUrls {
		log("connecting to %s... ", url)
		if relay, err := pool.EnsureRelay(url); err == nil {
			relays = append(relays, relay)
			log("ok.\n")
		} else {
			log(err.Error() + "\n")
		}
	}
	return pool, relays
}

func lineProcessingError(c *cli.Context, msg string, args ...any) {
	c.Context = context.WithValue(c.Context, LINE_PROCESSING_ERROR, true)
	log(msg+"\n", args...)
}

func exitIfLineProcessingError(c *cli.Context) {
	if val := c.Context.Value(LINE_PROCESSING_ERROR); val != nil && val.(bool) {
		os.Exit(123)
	}
}

func gatherSecretKeyFromArguments(c *cli.Context) (string, error) {
	var err error
	sec := c.String("sec")
	if c.Bool("prompt-sec") {
		if isPiped() {
			return "", fmt.Errorf("can't prompt for a secret key when processing data from a pipe, try again without --prompt-sec")
		}
		sec, err = askPassword("type your secret key as ncryptsec, nsec or hex: ", nil)
		if err != nil {
			return "", fmt.Errorf("failed to get secret key: %w", err)
		}
	}
	if strings.HasPrefix(sec, "ncryptsec1") {
		sec, err = promptDecrypt(sec)
		if err != nil {
			return "", fmt.Errorf("failed to decrypt: %w", err)
		}
	} else if bsec, err := hex.DecodeString(strings.Repeat("0", 64-len(sec)) + sec); err == nil {
		sec = hex.EncodeToString(bsec)
	} else if prefix, hexvalue, err := nip19.Decode(sec); err != nil {
		return "", fmt.Errorf("invalid nsec: %w", err)
	} else if prefix == "nsec" {
		sec = hexvalue.(string)
	}

	if ok := nostr.IsValid32ByteHex(sec); !ok {
		return "", fmt.Errorf("invalid secret key")
	}
	return sec, nil
}

func promptDecrypt(ncryptsec1 string) (string, error) {
	for i := 1; i < 4; i++ {
		var attemptStr string
		if i > 1 {
			attemptStr = fmt.Sprintf(" [%d/3]", i)
		}
		password, err := askPassword("type the password to decrypt your secret key"+attemptStr+": ", nil)
		if err != nil {
			return "", err
		}
		sec, err := nip49.Decrypt(ncryptsec1, password)
		if err != nil {
			continue
		}
		return sec, nil
	}
	return "", fmt.Errorf("couldn't decrypt private key")
}

func ask(msg string, defaultValue string, shouldAskAgain func(answer string) bool) (string, error) {
	return _ask(&readline.Config{
		Stdout:                 os.Stderr,
		Prompt:                 color.YellowString(msg),
		InterruptPrompt:        "^C",
		DisableAutoSaveHistory: true,
	}, msg, defaultValue, shouldAskAgain)
}

func askPassword(msg string, shouldAskAgain func(answer string) bool) (string, error) {
	config := &readline.Config{
		Stdout:                 os.Stderr,
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
