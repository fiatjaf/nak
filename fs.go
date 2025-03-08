package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/colduction/nocopy"
	"github.com/fatih/color"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
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

		root := &NostrRoot{ctx: ctx, rootPubKey: c.String("pubkey")}

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

type NostrRoot struct {
	fs.Inode
	rootPubKey string
	ctx        context.Context
}

var _ = (fs.NodeOnAdder)((*NostrRoot)(nil))

func (r *NostrRoot) OnAdd(context.Context) {
	if r.rootPubKey == "" {
		return
	}

	fl := sys.FetchFollowList(r.ctx, r.rootPubKey)

	for _, f := range fl.Items {
		h := r.NewPersistentInode(
			r.ctx,
			&NpubDir{pointer: nostr.ProfilePointer{PublicKey: f.Pubkey, Relays: []string{f.Relay}}},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		)
		npub, _ := nip19.EncodePublicKey(f.Pubkey)
		r.AddChild(npub, h, true)
	}
}

func (r *NostrRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	// check if we already have this npub
	child := r.GetChild(name)
	if child != nil {
		return child, fs.OK
	}

	// if the name starts with "npub1" or "nprofile1", create a new npub directory
	if strings.HasPrefix(name, "npub1") || strings.HasPrefix(name, "nprofile1") {
		npubdir, err := NewNpubDir(name)
		if err != nil {
			return nil, syscall.ENOENT
		}

		return r.NewPersistentInode(
			ctx,
			npubdir,
			fs.StableAttr{Mode: syscall.S_IFDIR},
		), 0
	}

	return nil, syscall.ENOENT
}

type NpubDir struct {
	fs.Inode
	pointer nostr.ProfilePointer
	ctx     context.Context
	fetched atomic.Bool
}

func NewNpubDir(npub string) (*NpubDir, error) {
	pointer, err := nip19.ToPointer(npub)
	if err != nil {
		return nil, err
	}

	pp, ok := pointer.(nostr.ProfilePointer)
	if !ok {
		return nil, fmt.Errorf("directory must be npub or nprofile")
	}

	return &NpubDir{pointer: pp}, nil
}

var _ = (fs.NodeOpendirer)((*NpubDir)(nil))

func (n *NpubDir) Opendir(ctx context.Context) syscall.Errno {
	if n.fetched.CompareAndSwap(true, true) {
		return fs.OK
	}

	for ie := range sys.Pool.FetchMany(ctx, sys.FetchOutboxRelays(ctx, n.pointer.PublicKey, 2), nostr.Filter{
		Kinds:   []int{1},
		Authors: []string{n.pointer.PublicKey},
	}, nostr.WithLabel("nak-fs-feed")) {
		h := n.NewPersistentInode(
			ctx,
			&EventFile{ctx: ctx, evt: *ie.Event},
			fs.StableAttr{
				Mode: syscall.S_IFREG,
				Ino:  hexToUint64(ie.Event.ID),
			},
		)
		n.AddChild(ie.Event.ID, h, true)
	}

	return fs.OK
}

type EventFile struct {
	fs.Inode
	ctx context.Context
	evt nostr.Event
}

var (
	_ = (fs.NodeOpener)((*EventFile)(nil))
	_ = (fs.NodeGetattrer)((*EventFile)(nil))
)

func (c *EventFile) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0444
	out.Size = uint64(len(c.evt.String()))
	ts := uint64(c.evt.CreatedAt)
	out.Atime = ts
	out.Mtime = ts
	out.Ctime = ts

	return fs.OK
}

func (c *EventFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

func (c *EventFile) Read(
	ctx context.Context,
	fh fs.FileHandle,
	dest []byte,
	off int64,
) (fuse.ReadResult, syscall.Errno) {
	buf := c.evt.String()

	end := int(off) + len(dest)
	if end > len(buf) {
		end = len(c.evt.Content)
	}
	return fuse.ReadResultData(nocopy.StringToByteSlice(c.evt.Content[off:end])), fs.OK
}
