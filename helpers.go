package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/bgentry/speakeasy"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
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

func getStdinLinesOrFirstArgument(c *cli.Context) chan string {
	// try the first argument
	target := c.Args().First()
	if target != "" {
		single := make(chan string, 1)
		single <- target
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

func validate32BytesHex(target string) error {
	if _, err := hex.DecodeString(target); err != nil {
		return fmt.Errorf("target '%s' is not valid hex: %s", target, err)
	}
	if len(target) != 64 {
		return fmt.Errorf("expected '%s' to be 64 characters (32 bytes), got %d", target, len(target))
	}
	if strings.ToLower(target) != target {
		return fmt.Errorf("expected target to be all lowercase hex. try again with '%s'", strings.ToLower(target))
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
	sec := c.String("sec")
	if c.Bool("prompt-sec") {
		if isPiped() {
			return "", fmt.Errorf("can't prompt for a secret key when processing data from a pipe, try again without --prompt-sec")
		}
		var err error
		sec, err = speakeasy.FAsk(os.Stderr, "type your secret key as nsec or hex: ")
		if err != nil {
			return "", fmt.Errorf("failed to get secret key: %w", err)
		}
	}
	if strings.HasPrefix(sec, "nsec1") {
		_, hex, err := nip19.Decode(sec)
		if err != nil {
			return "", fmt.Errorf("invalid nsec: %w", err)
		}
		sec = hex.(string)
	}
	if len(sec) > 64 {
		return "", fmt.Errorf("invalid secret key: too large")
	}
	sec = strings.Repeat("0", 64-len(sec)) + sec // left-pad
	if err := validate32BytesHex(sec); err != nil {
		return "", fmt.Errorf("invalid secret key")
	}

	return sec, nil
}
