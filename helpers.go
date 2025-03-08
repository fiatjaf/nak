package main

import (
	"bufio"
	"context"
	"fmt"
	"iter"
	"math/rand"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/fatih/color"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/sdk"
	"github.com/urfave/cli/v3"
)

var sys *sdk.System

var (
	hintsFilePath   string
	hintsFileExists bool
)

var json = jsoniter.ConfigFastest

const (
	LINE_PROCESSING_ERROR = iota
)

var (
	log        = func(msg string, args ...any) { fmt.Fprintf(color.Error, msg, args...) }
	logverbose = func(msg string, args ...any) {} // by default do nothing
	stdout     = fmt.Println
)

func isPiped() bool {
	stat, _ := os.Stdin.Stat()
	return stat.Mode()&os.ModeCharDevice == 0
}

func getJsonsOrBlank() iter.Seq[string] {
	var curr strings.Builder

	return func(yield func(string) bool) {
		hasStdin := writeStdinLinesOrNothing(func(stdinLine string) bool {
			// we're look for an event, but it may be in multiple lines, so if json parsing fails
			// we'll try the next line until we're successful
			curr.WriteString(stdinLine)
			stdinEvent := curr.String()

			var dummy any
			if err := json.Unmarshal([]byte(stdinEvent), &dummy); err != nil {
				return true
			}

			if !yield(stdinEvent) {
				return false
			}

			curr.Reset()
			return true
		})

		if !hasStdin {
			yield("{}")
		}
	}
}

func getStdinLinesOrBlank() iter.Seq[string] {
	return func(yield func(string) bool) {
		hasStdin := writeStdinLinesOrNothing(func(stdinLine string) bool {
			if !yield(stdinLine) {
				return false
			}
			return true
		})

		if !hasStdin {
			yield("")
		}
	}
}

func getStdinLinesOrArguments(args cli.Args) iter.Seq[string] {
	return getStdinLinesOrArgumentsFromSlice(args.Slice())
}

func getStdinLinesOrArgumentsFromSlice(args []string) iter.Seq[string] {
	// try the first argument
	if len(args) > 0 {
		return slices.Values(args)
	}

	// try the stdin
	return func(yield func(string) bool) {
		writeStdinLinesOrNothing(yield)
	}
}

func writeStdinLinesOrNothing(yield func(string) bool) (hasStdinLines bool) {
	if isPiped() {
		// piped
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 16*1024*1024), 256*1024*1024)
		hasEmittedAtLeastOne := false
		for scanner.Scan() {
			if !yield(strings.TrimSpace(scanner.Text())) {
				return
			}
			hasEmittedAtLeastOne = true
		}
		return hasEmittedAtLeastOne
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
) []*nostr.Relay {
	sys.Pool = nostr.NewSimplePool(context.Background(),
		append(opts,
			nostr.WithEventMiddleware(sys.TrackEventHints),
			nostr.WithPenaltyBox(),
			nostr.WithRelayOptions(
				nostr.WithRequestHeader(http.Header{textproto.CanonicalMIMEHeaderKey("user-agent"): {"nak/s"}}),
			),
		)...,
	)

	relays := make([]*nostr.Relay, 0, len(relayUrls))
relayLoop:
	for _, url := range relayUrls {
		log("connecting to %s... ", url)
		if relay, err := sys.Pool.EnsureRelay(url); err == nil {
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
						return signer(ctx, nostr.RelayEvent{Event: authEvent, Relay: relay})
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
	return relays
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

var colors = struct {
	reset   func(...any) (int, error)
	italic  func(...any) string
	italicf func(string, ...any) string
	bold    func(...any) string
	boldf   func(string, ...any) string
}{
	color.New(color.Reset).Print,
	color.New(color.Italic).Sprint,
	color.New(color.Italic).Sprintf,
	color.New(color.Bold).Sprint,
	color.New(color.Bold).Sprintf,
}
