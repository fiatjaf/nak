package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"iter"
	"math/rand"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nip42"
	"fiatjaf.com/nostr/sdk"
	"github.com/chzyer/readline"
	"github.com/fatih/color"
	jsoniter "github.com/json-iterator/go"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-tty"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

var sys *sdk.System

var json = jsoniter.ConfigFastest

const (
	LINE_PROCESSING_ERROR = iota
)

var (
	log        = func(msg string, args ...any) { fmt.Fprintf(color.Error, msg, args...) }
	logverbose = func(msg string, args ...any) {} // by default do nothing
	stdout     = func(args ...any) { fmt.Fprintln(color.Output, args...) }
)

func isPiped() bool {
	stat, _ := os.Stdin.Stat()
	return stat.Mode()&os.ModeCharDevice == 0
}

func getJsonsOrBlank() iter.Seq[string] {
	var curr strings.Builder

	var finalJsonErr error
	return func(yield func(string) bool) {
		hasStdin := writeStdinLinesOrNothing(func(stdinLine string) bool {
			// we're look for an event, but it may be in multiple lines, so if json parsing fails
			// we'll try the next line until we're successful
			curr.WriteString(stdinLine)
			stdinEvent := curr.String()

			var dummy any
			if err := json.Unmarshal([]byte(stdinEvent), &dummy); err != nil {
				finalJsonErr = err
				return true
			}
			finalJsonErr = nil

			if !yield(stdinEvent) {
				return false
			}

			curr.Reset()
			return true
		})

		if !hasStdin {
			yield("{}")
		}

		if finalJsonErr != nil {
			log(color.YellowString("stdin json parse error: %s", finalJsonErr))
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
	c *cli.Command,
	relayUrls []string,
	preAuthSigner func(ctx context.Context, c *cli.Command, log func(s string, args ...any), authEvent *nostr.Event) (err error), // if this exists we will force preauth
	opts nostr.PoolOptions,
) []*nostr.Relay {
	// first pass to check if these are valid relay URLs
	for _, url := range relayUrls {
		if !nostr.IsValidRelayURL(nostr.NormalizeURL(url)) {
			log("invalid relay URL: %s\n", url)
			os.Exit(4)
		}
	}

	opts.EventMiddleware = sys.TrackEventHints
	opts.PenaltyBox = true
	opts.RelayOptions = nostr.RelayOptions{
		RequestHeader: http.Header{textproto.CanonicalMIMEHeaderKey("user-agent"): {"nak/s"}},
	}
	sys.Pool = nostr.NewPool(opts)

	relays := make([]*nostr.Relay, 0, len(relayUrls))

	if supportsDynamicMultilineMagic() {
		// overcomplicated multiline rendering magic
		lines := make([][][]byte, len(relayUrls))
		flush := func() {
			for _, line := range lines {
				for _, part := range line {
					os.Stderr.Write(part)
				}
				os.Stderr.Write([]byte{'\n'})
			}
		}
		render := func() {
			clearLines(len(lines))
			flush()
		}
		flush()

		wg := sync.WaitGroup{}
		wg.Add(len(relayUrls))
		for i, url := range relayUrls {
			lines[i] = make([][]byte, 1, 2)
			logthis := func(s string, args ...any) {
				lines[i] = append(lines[i], []byte(fmt.Sprintf(s, args...)))
				render()
			}
			colorizepreamble := func(c func(string, ...any) string) {
				lines[i][0] = []byte(fmt.Sprintf("%s... ", c(url)))
			}
			colorizepreamble(color.CyanString)

			go func() {
				relay := connectToSingleRelay(ctx, c, url, preAuthSigner, colorizepreamble, logthis)
				if relay != nil {
					relays = append(relays, relay)
				}
				wg.Done()
			}()
		}
		wg.Wait()
	} else {
		// simple flow
		for _, url := range relayUrls {
			log("connecting to %s... ", color.CyanString(url))
			relay := connectToSingleRelay(ctx, c, url, preAuthSigner, nil, log)
			if relay != nil {
				relays = append(relays, relay)
			}
			log("\n")
		}
	}

	return relays
}

func connectToSingleRelay(
	ctx context.Context,
	c *cli.Command,
	url string,
	preAuthSigner func(ctx context.Context, c *cli.Command, log func(s string, args ...any), authEvent *nostr.Event) (err error),
	colorizepreamble func(c func(string, ...any) string),
	logthis func(s string, args ...any),
) *nostr.Relay {
	if relay, err := sys.Pool.EnsureRelay(url); err == nil {
		if preAuthSigner != nil {
			if colorizepreamble != nil {
				colorizepreamble(color.YellowString)
			}
			logthis("waiting for auth challenge... ")
			time.Sleep(time.Millisecond * 200)

			for range 5 {
				if err := relay.Auth(ctx, func(ctx context.Context, authEvent *nostr.Event) error {
					challengeTag := authEvent.Tags.Find("challenge")
					if challengeTag[1] == "" {
						return fmt.Errorf("auth not received yet *****") // what a giant hack
					}
					return preAuthSigner(ctx, c, logthis, authEvent)
				}); err == nil {
					// auth succeeded
					goto preauthSuccess
				} else {
					// auth failed
					if strings.HasSuffix(err.Error(), "auth not received yet *****") {
						// it failed because we didn't receive the challenge yet, so keep waiting
						time.Sleep(time.Second)
						continue
					} else {
						// it failed for some other reason, so skip this relay
						if colorizepreamble != nil {
							colorizepreamble(colors.errorf)
						}
						logthis(err.Error())
						return nil
					}
				}
			}
			if colorizepreamble != nil {
				colorizepreamble(colors.errorf)
			}
			logthis("failed to get an AUTH challenge in enough time.")
			return nil
		}

	preauthSuccess:
		if colorizepreamble != nil {
			colorizepreamble(colors.successf)
		}
		logthis("ok.")
		return relay
	} else {
		if colorizepreamble != nil {
			colorizepreamble(colors.errorf)
		}

		// if we're here that means we've failed to connect, this may be a huge message
		// but we're likely to only be interested in the lowest level error (although we can leave space)
		logthis(clampError(err, len(url)+12))
		return nil
	}
}

func clearLines(lineCount int) {
	for i := 0; i < lineCount; i++ {
		os.Stderr.Write([]byte("\033[0A\033[2K\r"))
	}
}

func supportsDynamicMultilineMagic() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	if !term.IsTerminal(0) {
		return false
	}

	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return false
	}

	width, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil {
		return false
	}
	if width < 110 {
		return false
	}

	return true
}

func authSigner(ctx context.Context, c *cli.Command, log func(s string, args ...any), authEvent *nostr.Event) (err error) {
	defer func() {
		if err != nil {
			cleanUrl, _ := strings.CutPrefix(nip42.GetRelayURLFromAuthEvent(*authEvent), "wss://")
			log("%s auth failed: %s", colors.errorf(cleanUrl), err)
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

func unwrapAll(err error) error {
	low := err
	for n := low; n != nil; n = errors.Unwrap(low) {
		low = n
	}
	return low
}

func clampMessage(msg string, prefixAlreadyPrinted int) string {
	termSize, _, _ := term.GetSize(int(os.Stderr.Fd()))

	prf := "expected handshake response status code 101 but got "
	if len(msg) > len(prf) && msg[0:len(prf)] == prf {
		msg = "status " + msg[len(prf):]
	}

	if len(msg) > termSize-prefixAlreadyPrinted && prefixAlreadyPrinted+1 < termSize {
		msg = msg[0:termSize-prefixAlreadyPrinted-1] + "…"
	}

	return msg
}

func clampError(err error, prefixAlreadyPrinted int) string {
	termSize, _, _ := term.GetSize(0)
	msg := err.Error()
	if len(msg) > termSize-prefixAlreadyPrinted {
		err = unwrapAll(err)
		msg = clampMessage(err.Error(), prefixAlreadyPrinted)
	}
	return msg
}

func appendUnique[A comparable](list []A, newEls ...A) []A {
ex:
	for _, newEl := range newEls {
		for _, el := range list {
			if el == newEl {
				continue ex
			}
		}
		list = append(list, newEl)
	}
	return list
}

func askConfirmation(msg string) bool {
	if isPiped() {
		tty, err := tty.Open()
		if err != nil {
			return false
		}
		defer tty.Close()

		log(color.YellowString(msg))
		answer, err := tty.ReadString()
		if err != nil {
			return false
		}

		// print newline after password input
		fmt.Fprintln(os.Stderr)

		answer = strings.TrimSpace(string(answer))
		return answer == "y" || answer == "yes"
	} else {
		config := &readline.Config{
			Stdout:                 color.Error,
			Prompt:                 color.YellowString(msg),
			InterruptPrompt:        "^C",
			DisableAutoSaveHistory: true,
			EnableMask:             false,
			MaskRune:               '*',
		}

		rl, err := readline.NewEx(config)
		if err != nil {
			return false
		}

		answer, err := rl.Readline()
		if err != nil {
			return false
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	}
}

var colors = struct {
	reset    func(...any) (int, error)
	italic   func(...any) string
	italicf  func(string, ...any) string
	bold     func(...any) string
	boldf    func(string, ...any) string
	error    func(...any) string
	errorf   func(string, ...any) string
	success  func(...any) string
	successf func(string, ...any) string
}{
	color.New(color.Reset).Print,
	color.New(color.Italic).Sprint,
	color.New(color.Italic).Sprintf,
	color.New(color.Bold).Sprint,
	color.New(color.Bold).Sprintf,
	color.New(color.Bold, color.FgHiRed).Sprint,
	color.New(color.Bold, color.FgHiRed).Sprintf,
	color.New(color.Bold, color.FgHiGreen).Sprint,
	color.New(color.Bold, color.FgHiGreen).Sprintf,
}
