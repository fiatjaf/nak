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
)

type NpubDir struct {
	fs.Inode
	root    *NostrRoot
	pointer nostr.ProfilePointer
	fetched atomic.Bool
}

func (r *NostrRoot) CreateNpubDir(
	parent fs.InodeEmbedder,
	pointer nostr.ProfilePointer,
	signer nostr.Signer,
) *fs.Inode {
	npubdir := &NpubDir{root: r, pointer: pointer}
	h := parent.EmbeddedInode().NewPersistentInode(
		r.ctx,
		npubdir,
		fs.StableAttr{Mode: syscall.S_IFDIR, Ino: hexToUint64(pointer.PublicKey)},
	)

	relays := r.sys.FetchOutboxRelays(r.ctx, pointer.PublicKey, 2)

	h.AddChild("pubkey", h.NewPersistentInode(
		r.ctx,
		&fs.MemRegularFile{Data: []byte(pointer.PublicKey + "\n"), Attr: fuse.Attr{Mode: 0444}},
		fs.StableAttr{},
	), true)

	go func() {
		pm := r.sys.FetchProfileMetadata(r.ctx, pointer.PublicKey)
		if pm.Event == nil {
			return
		}

		metadataj, _ := json.MarshalIndent(pm, "", "  ")
		h.AddChild(
			"metadata.json",
			h.NewPersistentInode(
				r.ctx,
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

		ctx, cancel := context.WithTimeout(r.ctx, time.Second*20)
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
			r.ctx,
			&ViewDir{
				root: r,
				filter: nostr.Filter{
					Kinds:   []int{1},
					Authors: []string{pointer.PublicKey},
				},
				paginate: true,
				relays:   relays,
				create: func(n *ViewDir, event *nostr.Event) (string, *fs.Inode) {
					return event.ID, r.CreateEventDir(n, event)
				},
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	h.AddChild(
		"comments",
		h.NewPersistentInode(
			r.ctx,
			&ViewDir{
				root: r,
				filter: nostr.Filter{
					Kinds:   []int{1111},
					Authors: []string{pointer.PublicKey},
				},
				paginate: true,
				relays:   relays,
				create: func(n *ViewDir, event *nostr.Event) (string, *fs.Inode) {
					return event.ID, r.CreateEventDir(n, event)
				},
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	h.AddChild(
		"photos",
		h.NewPersistentInode(
			r.ctx,
			&ViewDir{
				root: r,
				filter: nostr.Filter{
					Kinds:   []int{20},
					Authors: []string{pointer.PublicKey},
				},
				paginate: true,
				relays:   relays,
				create: func(n *ViewDir, event *nostr.Event) (string, *fs.Inode) {
					return event.ID, r.CreateEventDir(n, event)
				},
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	h.AddChild(
		"videos",
		h.NewPersistentInode(
			r.ctx,
			&ViewDir{
				root: r,
				filter: nostr.Filter{
					Kinds:   []int{21, 22},
					Authors: []string{pointer.PublicKey},
				},
				paginate: false,
				relays:   relays,
				create: func(n *ViewDir, event *nostr.Event) (string, *fs.Inode) {
					return event.ID, r.CreateEventDir(n, event)
				},
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	h.AddChild(
		"highlights",
		h.NewPersistentInode(
			r.ctx,
			&ViewDir{
				root: r,
				filter: nostr.Filter{
					Kinds:   []int{9802},
					Authors: []string{pointer.PublicKey},
				},
				paginate: false,
				relays:   relays,
				create: func(n *ViewDir, event *nostr.Event) (string, *fs.Inode) {
					return event.ID, r.CreateEventDir(n, event)
				},
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	h.AddChild(
		"articles",
		h.NewPersistentInode(
			r.ctx,
			&ViewDir{
				root: r,
				filter: nostr.Filter{
					Kinds:   []int{30023},
					Authors: []string{pointer.PublicKey},
				},
				paginate: false,
				relays:   relays,
				create: func(n *ViewDir, event *nostr.Event) (string, *fs.Inode) {
					d := event.Tags.GetD()
					if d == "" {
						d = "_"
					}
					return d, r.CreateEntityDir(n, ".md", event)
				},
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	h.AddChild(
		"wiki",
		h.NewPersistentInode(
			r.ctx,
			&ViewDir{
				root: r,
				filter: nostr.Filter{
					Kinds:   []int{30818},
					Authors: []string{pointer.PublicKey},
				},
				paginate: false,
				relays:   relays,
				create: func(n *ViewDir, event *nostr.Event) (string, *fs.Inode) {
					d := event.Tags.GetD()
					if d == "" {
						d = "_"
					}
					return d, r.CreateEntityDir(n, ".adoc", event)
				},
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		),
		true,
	)

	return h
}
