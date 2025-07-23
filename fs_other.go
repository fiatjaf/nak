//go:build windows || openbsd

package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

var fsCmd = &cli.Command{
	Name:                      "fs",
	Usage:                     "mount a FUSE filesystem that exposes Nostr events as files.",
	Description:               `doesn't work on Windows and OpenBSD.`,
	DisableSliceFlagSeparator: true,
	Action: func(ctx context.Context, c *cli.Command) error {
		return fmt.Errorf("this doesn't work on Windows and OpenBSD.")
	},
}
