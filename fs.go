//go:build !windows

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
	Flags: append(defaultKeyFlags,
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
			kr = keyer.NewReadOnlyUser(c.String("pubkey"))
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
		timeout := time.Second * 120
		server, err := fs.Mount(mountpoint, root, &fs.Options{
			MountOptions: fuse.MountOptions{
				Debug:          isVerbose,
				Name:           "nak",
				FsName:         "nak",
				RememberInodes: true,
			},
			AttrTimeout:  &timeout,
			EntryTimeout: &timeout,
			Logger:       nostr.DebugLogger,
		})
		if err != nil {
			return fmt.Errorf("mount failed: %w", err)
		}
		log("ok.\n")

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
