//go:build mangen

package main

import (
	"fmt"
	"os"

	docs "github.com/urfave/cli-docs/v3"
)

// generates the man page and exits before main() runs.
// usage: go run -tags mangen . > nak.1
func init() {
	s, err := docs.ToManWithSection(app, 1)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(s)
	os.Exit(0)
}
