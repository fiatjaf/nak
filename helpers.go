package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
)

const (
	LINE_PROCESSING_ERROR = iota
)

func getStdinLinesOrBlank() chan string {
	ch := make(chan string)
	go func() {
		if stat, _ := os.Stdin.Stat(); stat.Mode()&os.ModeCharDevice == 0 {
			// piped
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				ch <- scanner.Text()
			}
		} else {
			// not piped
			ch <- ""
		}
		close(ch)
	}()
	return ch
}

func getStdinOrFirstArgument(c *cli.Context) string {
	// try the first argument
	target := c.Args().First()
	if target != "" {
		return target
	}

	// try the stdin
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		read := bytes.NewBuffer(make([]byte, 0, 1000))
		_, err := io.Copy(read, os.Stdin)
		if err == nil {
			return strings.TrimSpace(read.String())
		}
	}
	return ""
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
