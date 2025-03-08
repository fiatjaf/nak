package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/fiatjaf/nak/nostrfs"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/keyer"
	"github.com/urfave/cli/v3"
)

var fsCmd = &cli.Command{
	Name:        "fs",
	Usage:       "mount a FUSE filesystem that exposes Nostr events as files.",
	Description: `(experimental)`,
	ArgsUsage:   "<mountpoint>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "pubkey",
			Usage: "public key from where to to prepopulate directories",
			Validator: func(pk string) error {
				if nostr.IsValidPublicKey(pk) {
					return nil
				}
				return fmt.Errorf("invalid public key '%s'", pk)
			},
		},
	},
	DisableSliceFlagSeparator: true,
	Action: func(ctx context.Context, c *cli.Command) error {
		mountpoint := c.Args().First()
		if mountpoint == "" {
			return fmt.Errorf("must be called with a directory path to serve as the mountpoint as an argument")
		}

		root := nostrfs.NewNostrRoot(ctx, sys, keyer.NewReadOnlyUser(c.String("pubkey")))

		// create the server
		log("- mounting at %s... ", color.HiCyanString(mountpoint))
		timeout := time.Second * 120
		server, err := fs.Mount(mountpoint, root, &fs.Options{
			MountOptions: fuse.MountOptions{
				Debug: isVerbose,
				Name:  "nak",
			},
			AttrTimeout:  &timeout,
			EntryTimeout: &timeout,
			Logger:       nostr.DebugLogger,
		})
		if err != nil {
			return fmt.Errorf("mount failed: %w", err)
		}
		log("ok\n")

		// setup signal handling for clean unmount
		ch := make(chan os.Signal, 1)
		chErr := make(chan error)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-ch
			log("- unmounting... ")
			err := server.Unmount()
			if err != nil {
				chErr <- fmt.Errorf("unmount failed: %w", err)
			} else {
				log("ok\n")
				chErr <- nil
			}
		}()

		// serve the filesystem until unmounted
		server.Wait()
		return <-chErr
	},
}
