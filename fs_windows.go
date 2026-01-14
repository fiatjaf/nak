//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/keyer"
	"github.com/fatih/color"
	"github.com/fiatjaf/nak/nostrfs"
	"github.com/urfave/cli/v3"
	"github.com/winfsp/cgofuse/fuse"
)

var fsCmd = &cli.Command{
	Name:        "fs",
	Usage:       "mount a FUSE filesystem that exposes Nostr events as files.",
	Description: `(experimental)`,
	ArgsUsage:   "<mountpoint>",
	Flags: append(defaultKeyFlags,
		&PubKeyFlag{
			Name:  "pubkey",
			Usage: "public key from where to to prepopulate directories",
		},
		&cli.DurationFlag{
			Name:  "auto-publish-notes",
			Usage: "delay after which new notes will be auto-published, set to -1 to not publish.",
			Value: time.Second * 30,
		},
		&cli.DurationFlag{
			Name:        "auto-publish-articles",
			Usage:       "delay after which edited articles will be auto-published.",
			Value:       time.Hour * 24 * 365 * 2,
			DefaultText: "basically infinite",
		},
	),
	DisableSliceFlagSeparator: true,
	Action: func(ctx context.Context, c *cli.Command) error {
		mountpoint := c.Args().First()
		if mountpoint == "" {
			return fmt.Errorf("must be called with a directory path to serve as the mountpoint as an argument")
		}

		var kr nostr.User
		if signer, _, err := gatherKeyerFromArguments(ctx, c); err == nil {
			kr = signer
		} else {
			kr = keyer.NewReadOnlyUser(getPubKey(c, "pubkey"))
		}

		apnt := c.Duration("auto-publish-notes")
		if apnt < 0 {
			apnt = time.Hour * 24 * 365 * 3
		}
		apat := c.Duration("auto-publish-articles")
		if apat < 0 {
			apat = time.Hour * 24 * 365 * 3
		}

		root := nostrfs.NewNostrRoot(
			context.WithValue(
				context.WithValue(
					ctx,
					"log", log,
				),
				"logverbose", logverbose,
			),
			sys,
			kr,
			mountpoint,
			nostrfs.Options{
				AutoPublishNotesTimeout:    apnt,
				AutoPublishArticlesTimeout: apat,
			},
		)

		// create the server
		log("- mounting at %s... ", color.HiCyanString(mountpoint))

		// Create cgofuse host
		host := fuse.NewFileSystemHost(root)
		host.SetCapReaddirPlus(true)
		host.SetUseIno(true)

		// Mount the filesystem - Windows/WinFsp version
		// Based on rclone cmount implementation
		mountArgs := []string{
			"-o", "uid=-1",
			"-o", "gid=-1",
			"--FileSystemName=nak",
		}
		
		// Check if mountpoint is a drive letter or directory
		isDriveLetter := len(mountpoint) == 2 && mountpoint[1] == ':'
		
		if !isDriveLetter {
			// WinFsp primarily supports drive letters on Windows
			// Directory mounting may not work reliably
			log("WARNING: directory mounting may not work on Windows (WinFsp limitation)\n")
			log("         consider using a drive letter instead (e.g., 'nak fs Z:')\n")
			
			// For directory mounts, follow rclone's approach:
			// 1. Check that mountpoint doesn't already exist
			if _, err := os.Stat(mountpoint); err == nil {
				return fmt.Errorf("mountpoint path already exists: %s (must not exist before mounting)", mountpoint)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("failed to check mountpoint: %w", err)
			}
			
			// 2. Check that parent directory exists
			parent := filepath.Join(mountpoint, "..")
			if _, err := os.Stat(parent); err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("parent of mountpoint directory does not exist: %s", parent)
				}
				return fmt.Errorf("failed to check parent directory: %w", err)
			}
			
			// 3. Use network mode for directory mounts
			mountArgs = append(mountArgs, "--VolumePrefix=\\nak\\"+filepath.Base(mountpoint))
		}
		
		if isVerbose {
			mountArgs = append(mountArgs, "-o", "debug")
		}
		mountArgs = append(mountArgs, mountpoint)

		log("ok.\n")

		// Mount in main thread like hellofs
		if !host.Mount("", mountArgs) {
			return fmt.Errorf("failed to mount filesystem")
		}
		return nil
	},
}
