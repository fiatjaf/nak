package nostrfs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip10"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/nip22"
	"github.com/nbd-wtf/go-nostr/nip27"
	"github.com/nbd-wtf/go-nostr/nip92"
	sdk "github.com/nbd-wtf/go-nostr/sdk"
)

type EventDir struct {
	fs.Inode
	ctx context.Context
	wd  string
	evt *nostr.Event
}

var _ = (fs.NodeGetattrer)((*EventDir)(nil))

func (e *EventDir) Getattr(_ context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mtime = uint64(e.evt.CreatedAt)
	return fs.OK
}

func FetchAndCreateEventDir(
	ctx context.Context,
	parent fs.InodeEmbedder,
	wd string,
	sys *sdk.System,
	pointer nostr.EventPointer,
) (*fs.Inode, error) {
	event, _, err := sys.FetchSpecificEvent(ctx, pointer, sdk.FetchSpecificEventParameters{
		WithRelays: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch: %w", err)
	}

	return CreateEventDir(ctx, parent, wd, event), nil
}

func CreateEventDir(
	ctx context.Context,
	parent fs.InodeEmbedder,
	wd string,
	event *nostr.Event,
) *fs.Inode {
	h := parent.EmbeddedInode().NewPersistentInode(
		ctx,
		&EventDir{ctx: ctx, wd: wd, evt: event},
		fs.StableAttr{Mode: syscall.S_IFDIR, Ino: hexToUint64(event.ID)},
	)

	npub, _ := nip19.EncodePublicKey(event.PubKey)
	h.AddChild("@author", h.NewPersistentInode(
		ctx,
		&fs.MemSymlink{
			Data: []byte(wd + "/" + npub),
		},
		fs.StableAttr{Mode: syscall.S_IFLNK},
	), true)

	eventj, _ := json.MarshalIndent(event, "", "  ")
	h.AddChild("event.json", h.NewPersistentInode(
		ctx,
		&fs.MemRegularFile{
			Data: eventj,
			Attr: fuse.Attr{
				Mode:  0444,
				Ctime: uint64(event.CreatedAt),
				Mtime: uint64(event.CreatedAt),
				Size:  uint64(len(event.Content)),
			},
		},
		fs.StableAttr{},
	), true)

	h.AddChild("id", h.NewPersistentInode(
		ctx,
		&fs.MemRegularFile{
			Data: []byte(event.ID),
			Attr: fuse.Attr{
				Mode:  0444,
				Ctime: uint64(event.CreatedAt),
				Mtime: uint64(event.CreatedAt),
				Size:  uint64(64),
			},
		},
		fs.StableAttr{},
	), true)

	h.AddChild("content.txt", h.NewPersistentInode(
		ctx,
		&fs.MemRegularFile{
			Data: []byte(event.Content),
			Attr: fuse.Attr{
				Mode:  0444,
				Ctime: uint64(event.CreatedAt),
				Mtime: uint64(event.CreatedAt),
				Size:  uint64(len(event.Content)),
			},
		},
		fs.StableAttr{},
	), true)

	var refsdir *fs.Inode
	i := 0
	for ref := range nip27.ParseReferences(*event) {
		i++
		if refsdir == nil {
			refsdir = h.NewPersistentInode(ctx, &fs.Inode{}, fs.StableAttr{Mode: syscall.S_IFDIR})
			h.AddChild("references", refsdir, true)
		}
		refsdir.AddChild(fmt.Sprintf("ref_%02d", i), refsdir.NewPersistentInode(
			ctx,
			&fs.MemSymlink{
				Data: []byte(wd + "/" + nip19.EncodePointer(ref.Pointer)),
			},
			fs.StableAttr{Mode: syscall.S_IFLNK},
		), true)
	}

	var imagesdir *fs.Inode
	images := nip92.ParseTags(event.Tags)
	for _, imeta := range images {
		if imeta.URL == "" {
			continue
		}
		if imagesdir == nil {
			in := &fs.Inode{}
			imagesdir = h.NewPersistentInode(ctx, in, fs.StableAttr{Mode: syscall.S_IFDIR})
			h.AddChild("images", imagesdir, true)
		}
		imagesdir.AddChild(filepath.Base(imeta.URL), imagesdir.NewPersistentInode(
			ctx,
			&AsyncFile{
				ctx: ctx,
				load: func() ([]byte, nostr.Timestamp) {
					ctx, cancel := context.WithTimeout(ctx, time.Second*20)
					defer cancel()
					r, err := http.NewRequestWithContext(ctx, "GET", imeta.URL, nil)
					if err != nil {
						return nil, 0
					}
					resp, err := http.DefaultClient.Do(r)
					if err != nil {
						return nil, 0
					}
					defer resp.Body.Close()
					if resp.StatusCode >= 300 {
						return nil, 0
					}
					w := &bytes.Buffer{}
					io.Copy(w, resp.Body)
					return w.Bytes(), 0
				},
			},
			fs.StableAttr{},
		), true)
	}

	if event.Kind == 1 {
		if pointer := nip10.GetThreadRoot(event.Tags); pointer != nil {
			nevent := nip19.EncodePointer(*pointer)
			h.AddChild("@root", h.NewPersistentInode(
				ctx,
				&fs.MemSymlink{
					Data: []byte(wd + "/" + nevent),
				},
				fs.StableAttr{Mode: syscall.S_IFLNK},
			), true)
		}
		if pointer := nip10.GetImmediateParent(event.Tags); pointer != nil {
			nevent := nip19.EncodePointer(*pointer)
			h.AddChild("@parent", h.NewPersistentInode(
				ctx,
				&fs.MemSymlink{
					Data: []byte(wd + "/" + nevent),
				},
				fs.StableAttr{Mode: syscall.S_IFLNK},
			), true)
		}
	} else if event.Kind == 1111 {
		if pointer := nip22.GetThreadRoot(event.Tags); pointer != nil {
			if xp, ok := pointer.(nostr.ExternalPointer); ok {
				h.AddChild("@root", h.NewPersistentInode(
					ctx,
					&fs.MemRegularFile{
						Data: []byte(`<!doctype html><meta http-equiv="refresh" content="0; url=` + xp.Thing + `" />`),
					},
					fs.StableAttr{},
				), true)
			} else {
				nevent := nip19.EncodePointer(pointer)
				h.AddChild("@parent", h.NewPersistentInode(
					ctx,
					&fs.MemSymlink{
						Data: []byte(wd + "/" + nevent),
					},
					fs.StableAttr{Mode: syscall.S_IFLNK},
				), true)
			}
		}
		if pointer := nip22.GetImmediateParent(event.Tags); pointer != nil {
			if xp, ok := pointer.(nostr.ExternalPointer); ok {
				h.AddChild("@parent", h.NewPersistentInode(
					ctx,
					&fs.MemRegularFile{
						Data: []byte(`<!doctype html><meta http-equiv="refresh" content="0; url=` + xp.Thing + `" />`),
					},
					fs.StableAttr{},
				), true)
			} else {
				nevent := nip19.EncodePointer(pointer)
				h.AddChild("@parent", h.NewPersistentInode(
					ctx,
					&fs.MemSymlink{
						Data: []byte(wd + "/" + nevent),
					},
					fs.StableAttr{Mode: syscall.S_IFLNK},
				), true)
			}
		}
	}

	return h
}
