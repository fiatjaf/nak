package nostrfs

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
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
				relays: relays,
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
				relays: relays,
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
				relays: relays,
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
				relays: relays,
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
				relays: relays,
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	h.AddChild(
		"metadata.json",
		h.NewPersistentInode(
			ctx,
			&AsyncFile{
				ctx: ctx,
				load: func() ([]byte, nostr.Timestamp) {
					pm := sys.FetchProfileMetadata(ctx, pointer.PublicKey)
					jsonb, _ := json.MarshalIndent(pm.Event, "", "  ")
					var ts nostr.Timestamp
					if pm.Event != nil {
						ts = pm.Event.CreatedAt
					}
					return jsonb, ts
				},
			},
			fs.StableAttr{},
		),
		true,
	)

	return h
}
