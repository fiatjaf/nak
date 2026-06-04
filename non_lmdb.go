//go:build !linux || riscv64 || arm64

package main

import (
	"fiatjaf.com/nostr/sdk"
	"github.com/urfave/cli/v3"
)

func setupLocalDatabases(c *cli.Command, sys *sdk.System) {
}
