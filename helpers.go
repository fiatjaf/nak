package main

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
)

const (
	LINE_PROCESSING_ERROR = iota
)

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
		return single
	}

	// try the stdin
	multi := make(chan string)
	writeStdinLinesOrNothing(multi)
	return multi
}

func writeStdinLinesOrNothing(ch chan string) (hasStdinLines bool) {
	if stat, _ := os.Stdin.Stat(); stat.Mode()&os.ModeCharDevice == 0 {
		// piped
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
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

func lineProcessingError(c *cli.Context, msg string, args ...any) {
	c.Context = context.WithValue(c.Context, LINE_PROCESSING_ERROR, true)
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
}

func exitIfLineProcessingError(c *cli.Context) {
	if val := c.Context.Value(LINE_PROCESSING_ERROR); val != nil && val.(bool) {
		os.Exit(123)
	}
}
