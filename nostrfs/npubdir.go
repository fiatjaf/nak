package nostrfs

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"syscall"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"github.com/fatih/color"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/liamg/magic"
)

type NpubDir struct {
	fs.Inode
	root    *NostrRoot
	pointer nostr.ProfilePointer
	fetched atomic.Bool
}

var _ = (fs.NodeOnAdder)((*NpubDir)(nil))

func (r *NostrRoot) CreateNpubDir(
	parent fs.InodeEmbedder,
	pointer nostr.ProfilePointer,
	signer nostr.Signer,
) *fs.Inode {
	npubdir := &NpubDir{root: r, pointer: pointer}
	return parent.EmbeddedInode().NewPersistentInode(
		r.ctx,
		npubdir,
		fs.StableAttr{Mode: syscall.S_IFDIR, Ino: binary.BigEndian.Uint64(pointer.PublicKey[8:16])},
	)
}

func (h *NpubDir) OnAdd(_ context.Context) {
	log := h.root.ctx.Value("log").(func(msg string, args ...any))

	relays := h.root.sys.FetchOutboxRelays(h.root.ctx, h.pointer.PublicKey, 2)
	log("- adding folder for %s with relays %s\n",
		color.HiYellowString(nip19.EncodePointer(h.pointer)), color.HiGreenString("%v", relays))

	h.AddChild("pubkey", h.NewPersistentInode(
		h.root.ctx,
		&fs.MemRegularFile{Data: []byte(h.pointer.PublicKey.Hex() + "\n"), Attr: fuse.Attr{Mode: 0444}},
		fs.StableAttr{},
	), true)

	go func() {
		pm := h.root.sys.FetchProfileMetadata(h.root.ctx, h.pointer.PublicKey)
		if pm.Event == nil {
			return
		}

		metadataj, _ := json.MarshalIndent(pm, "", "  ")
		h.AddChild(
			"metadata.json",
			h.NewPersistentInode(
				h.root.ctx,
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

		ctx, cancel := context.WithTimeout(h.root.ctx, time.Second*20)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, "GET", pm.Picture, nil)
		if err == nil {
			resp, err := http.DefaultClient.Do(req)
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

	if h.GetChild("notes") == nil {
		h.AddChild(
			"notes",
			h.NewPersistentInode(
				h.root.ctx,
				&ViewDir{
					root: h.root,
					filter: nostr.Filter{
						Kinds:   []nostr.Kind{1},
						Authors: []nostr.PubKey{h.pointer.PublicKey},
					},
					paginate:    true,
					relays:      relays,
					replaceable: false,
					createable:  true,
				},
				fs.StableAttr{Mode: syscall.S_IFDIR},
			),
			true,
		)
	}

	if h.GetChild("comments") == nil {
		h.AddChild(
			"comments",
			h.NewPersistentInode(
				h.root.ctx,
				&ViewDir{
					root: h.root,
					filter: nostr.Filter{
						Kinds:   []nostr.Kind{1111},
						Authors: []nostr.PubKey{h.pointer.PublicKey},
					},
					paginate:    true,
					relays:      relays,
					replaceable: false,
				},
				fs.StableAttr{Mode: syscall.S_IFDIR},
			),
			true,
		)
	}

	if h.GetChild("photos") == nil {
		h.AddChild(
			"photos",
			h.NewPersistentInode(
				h.root.ctx,
				&ViewDir{
					root: h.root,
					filter: nostr.Filter{
						Kinds:   []nostr.Kind{20},
						Authors: []nostr.PubKey{h.pointer.PublicKey},
					},
					paginate:    true,
					relays:      relays,
					replaceable: false,
				},
				fs.StableAttr{Mode: syscall.S_IFDIR},
			),
			true,
		)
	}

	if h.GetChild("videos") == nil {
		h.AddChild(
			"videos",
			h.NewPersistentInode(
				h.root.ctx,
				&ViewDir{
					root: h.root,
					filter: nostr.Filter{
						Kinds:   []nostr.Kind{21, 22},
						Authors: []nostr.PubKey{h.pointer.PublicKey},
					},
					paginate:    false,
					relays:      relays,
					replaceable: false,
				},
				fs.StableAttr{Mode: syscall.S_IFDIR},
			),
			true,
		)
	}

	if h.GetChild("highlights") == nil {
		h.AddChild(
			"highlights",
			h.NewPersistentInode(
				h.root.ctx,
				&ViewDir{
					root: h.root,
					filter: nostr.Filter{
						Kinds:   []nostr.Kind{9802},
						Authors: []nostr.PubKey{h.pointer.PublicKey},
					},
					paginate:    false,
					relays:      relays,
					replaceable: false,
				},
				fs.StableAttr{Mode: syscall.S_IFDIR},
			),
			true,
		)
	}

	if h.GetChild("articles") == nil {
		h.AddChild(
			"articles",
			h.NewPersistentInode(
				h.root.ctx,
				&ViewDir{
					root: h.root,
					filter: nostr.Filter{
						Kinds:   []nostr.Kind{30023},
						Authors: []nostr.PubKey{h.pointer.PublicKey},
					},
					paginate:    false,
					relays:      relays,
					replaceable: true,
					createable:  true,
				},
				fs.StableAttr{Mode: syscall.S_IFDIR},
			),
			true,
		)
	}

	if h.GetChild("wiki") == nil {
		h.AddChild(
			"wiki",
			h.NewPersistentInode(
				h.root.ctx,
				&ViewDir{
					root: h.root,
					filter: nostr.Filter{
						Kinds:   []nostr.Kind{30818},
						Authors: []nostr.PubKey{h.pointer.PublicKey},
					},
					paginate:    false,
					relays:      relays,
					replaceable: true,
					createable:  true,
				},
				fs.StableAttr{Mode: syscall.S_IFDIR},
			),
			true,
		)
	}
}
