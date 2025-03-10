package nostrfs

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/liamg/magic"
	"github.com/nbd-wtf/go-nostr"
	sdk "github.com/nbd-wtf/go-nostr/sdk"
)

type NpubDir struct {
	sys *sdk.System
	fs.Inode
	pointer nostr.ProfilePointer
	ctx     context.Context
	fetched atomic.Bool
}

func CreateNpubDir(
	ctx context.Context,
	sys *sdk.System,
	parent fs.InodeEmbedder,
	wd string,
	pointer nostr.ProfilePointer,
) *fs.Inode {
	npubdir := &NpubDir{ctx: ctx, sys: sys, pointer: pointer}
	h := parent.EmbeddedInode().NewPersistentInode(
		ctx,
		npubdir,
		fs.StableAttr{Mode: syscall.S_IFDIR, Ino: hexToUint64(pointer.PublicKey)},
	)

	relays := sys.FetchOutboxRelays(ctx, pointer.PublicKey, 2)

	h.AddChild("pubkey", h.NewPersistentInode(
		ctx,
		&fs.MemRegularFile{Data: []byte(pointer.PublicKey + "\n"), Attr: fuse.Attr{Mode: 0444}},
		fs.StableAttr{},
	), true)

	go func() {
		pm := sys.FetchProfileMetadata(ctx, pointer.PublicKey)
		if pm.Event == nil {
			return
		}

		metadataj, _ := json.MarshalIndent(pm, "", "  ")
		h.AddChild(
			"metadata.json",
			h.NewPersistentInode(
				ctx,
				&fs.MemRegularFile{
					Data: metadataj,
					Attr: fuse.Attr{
						Mtime: uint64(pm.Event.CreatedAt),
						Mode:  0444,
					},
				},
				fs.StableAttr{},
			),
			true,
		)

		ctx, cancel := context.WithTimeout(ctx, time.Second*20)
		defer cancel()
		r, err := http.NewRequestWithContext(ctx, "GET", pm.Picture, nil)
		if err == nil {
			resp, err := http.DefaultClient.Do(r)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode < 300 {
					b := &bytes.Buffer{}
					io.Copy(b, resp.Body)

					ext := "png"
					if ft, err := magic.Lookup(b.Bytes()); err == nil {
						ext = ft.Extension
					}

					h.AddChild("picture."+ext, h.NewPersistentInode(
						ctx,
						&fs.MemRegularFile{
							Data: b.Bytes(),
							Attr: fuse.Attr{
								Mtime: uint64(pm.Event.CreatedAt),
								Mode:  0444,
							},
						},
						fs.StableAttr{},
					), true)
				}
			}
		}
	}()

	h.AddChild(
		"notes",
		h.NewPersistentInode(
			ctx,
			&ViewDir{
				ctx: ctx,
				sys: sys,
				wd:  wd,
				filter: nostr.Filter{
					Kinds:   []int{1},
					Authors: []string{pointer.PublicKey},
				},
				paginate: true,
				relays:   relays,
				create: func(ctx context.Context, n *ViewDir, event *nostr.Event) (string, *fs.Inode) {
					return event.ID, CreateEventDir(ctx, n, n.wd, event)
				},
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	h.AddChild(
		"comments",
		h.NewPersistentInode(
			ctx,
			&ViewDir{
				ctx: ctx,
				sys: sys,
				wd:  wd,
				filter: nostr.Filter{
					Kinds:   []int{1111},
					Authors: []string{pointer.PublicKey},
				},
				paginate: true,
				relays:   relays,
				create: func(ctx context.Context, n *ViewDir, event *nostr.Event) (string, *fs.Inode) {
					return event.ID, CreateEventDir(ctx, n, n.wd, event)
				},
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	h.AddChild(
		"pictures",
		h.NewPersistentInode(
			ctx,
			&ViewDir{
				ctx: ctx,
				sys: sys,
				wd:  wd,
				filter: nostr.Filter{
					Kinds:   []int{20},
					Authors: []string{pointer.PublicKey},
				},
				paginate: true,
				relays:   relays,
				create: func(ctx context.Context, n *ViewDir, event *nostr.Event) (string, *fs.Inode) {
					return event.ID, CreateEventDir(ctx, n, n.wd, event)
				},
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	h.AddChild(
		"videos",
		h.NewPersistentInode(
			ctx,
			&ViewDir{
				ctx: ctx,
				sys: sys,
				wd:  wd,
				filter: nostr.Filter{
					Kinds:   []int{21, 22},
					Authors: []string{pointer.PublicKey},
				},
				paginate: true,
				relays:   relays,
				create: func(ctx context.Context, n *ViewDir, event *nostr.Event) (string, *fs.Inode) {
					return event.ID, CreateEventDir(ctx, n, n.wd, event)
				},
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	h.AddChild(
		"highlights",
		h.NewPersistentInode(
			ctx,
			&ViewDir{
				ctx: ctx,
				sys: sys,
				wd:  wd,
				filter: nostr.Filter{
					Kinds:   []int{9802},
					Authors: []string{pointer.PublicKey},
				},
				paginate: true,
				relays:   relays,
				create: func(ctx context.Context, n *ViewDir, event *nostr.Event) (string, *fs.Inode) {
					return event.ID, CreateEventDir(ctx, n, n.wd, event)
				},
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	h.AddChild(
		"articles",
		h.NewPersistentInode(
			ctx,
			&ViewDir{
				ctx: ctx,
				sys: sys,
				wd:  wd,
				filter: nostr.Filter{
					Kinds:   []int{30023},
					Authors: []string{pointer.PublicKey},
				},
				paginate: false,
				relays:   relays,
				create: func(ctx context.Context, n *ViewDir, event *nostr.Event) (string, *fs.Inode) {
					d := event.Tags.GetD()
					if d == "" {
						d = "_"
					}
					return d, CreateEntityDir(ctx, n, n.wd, ".md", event)
				},
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	h.AddChild(
		"wiki",
		h.NewPersistentInode(
			ctx,
			&ViewDir{
				ctx: ctx,
				sys: sys,
				wd:  wd,
				filter: nostr.Filter{
					Kinds:   []int{30818},
					Authors: []string{pointer.PublicKey},
				},
				paginate: false,
				relays:   relays,
				create: func(ctx context.Context, n *ViewDir, event *nostr.Event) (string, *fs.Inode) {
					d := event.Tags.GetD()
					if d == "" {
						d = "_"
					}
					return d, CreateEntityDir(ctx, n, n.wd, ".adoc", event)
				},
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	return h
}
