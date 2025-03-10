package nostrfs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/nip27"
	"github.com/nbd-wtf/go-nostr/nip92"
	sdk "github.com/nbd-wtf/go-nostr/sdk"
)

type EntityDir struct {
	fs.Inode
	ctx context.Context
	wd  string
	evt *nostr.Event
}

var _ = (fs.NodeGetattrer)((*EntityDir)(nil))

func (e *EntityDir) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	publishedAt := uint64(e.evt.CreatedAt)
	out.Ctime = publishedAt

	if tag := e.evt.Tags.Find("published_at"); tag != nil {
		publishedAt, _ = strconv.ParseUint(tag[1], 10, 64)
	}
	out.Mtime = publishedAt

	return fs.OK
}

func FetchAndCreateEntityDir(
	ctx context.Context,
	parent fs.InodeEmbedder,
	wd string,
	extension string,
	sys *sdk.System,
	pointer nostr.EntityPointer,
) (*fs.Inode, error) {
	event, _, err := sys.FetchSpecificEvent(ctx, pointer, sdk.FetchSpecificEventParameters{
		WithRelays: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch: %w", err)
	}

	return CreateEntityDir(ctx, parent, wd, extension, event), nil
}

func CreateEntityDir(
	ctx context.Context,
	parent fs.InodeEmbedder,
	wd string,
	extension string,
	event *nostr.Event,
) *fs.Inode {
	h := parent.EmbeddedInode().NewPersistentInode(
		ctx,
		&EntityDir{ctx: ctx, wd: wd, evt: event},
		fs.StableAttr{Mode: syscall.S_IFDIR, Ino: hexToUint64(event.ID)},
	)

	var publishedAt uint64
	if tag := event.Tags.Find("published_at"); tag != nil {
		publishedAt, _ = strconv.ParseUint(tag[1], 10, 64)
	}

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
				Mtime: uint64(publishedAt),
				Size:  uint64(len(event.Content)),
			},
		},
		fs.StableAttr{},
	), true)

	h.AddChild("identifier", h.NewPersistentInode(
		ctx,
		&fs.MemRegularFile{
			Data: []byte(event.Tags.GetD()),
			Attr: fuse.Attr{
				Mode:  0444,
				Ctime: uint64(event.CreatedAt),
				Mtime: uint64(publishedAt),
				Size:  uint64(len(event.Tags.GetD())),
			},
		},
		fs.StableAttr{},
	), true)

	if tag := event.Tags.Find("title"); tag != nil {
		h.AddChild("title", h.NewPersistentInode(
			ctx,
			&fs.MemRegularFile{
				Data: []byte(tag[1]),
				Attr: fuse.Attr{
					Mode:  0444,
					Ctime: uint64(event.CreatedAt),
					Mtime: uint64(publishedAt),
					Size:  uint64(len(tag[1])),
				},
			},
			fs.StableAttr{},
		), true)
	}

	h.AddChild("content"+extension, h.NewPersistentInode(
		ctx,
		&fs.MemRegularFile{
			Data: []byte(event.Content),
			Attr: fuse.Attr{
				Mode:  0444,
				Ctime: uint64(event.CreatedAt),
				Mtime: uint64(publishedAt),
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
	addImage := func(url string) {
		if imagesdir == nil {
			in := &fs.Inode{}
			imagesdir = h.NewPersistentInode(ctx, in, fs.StableAttr{Mode: syscall.S_IFDIR})
			h.AddChild("images", imagesdir, true)
		}
		imagesdir.AddChild(filepath.Base(url), imagesdir.NewPersistentInode(
			ctx,
			&AsyncFile{
				ctx: ctx,
				load: func() ([]byte, nostr.Timestamp) {
					ctx, cancel := context.WithTimeout(ctx, time.Second*20)
					defer cancel()
					r, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

	images := nip92.ParseTags(event.Tags)
	for _, imeta := range images {
		if imeta.URL == "" {
			continue
		}
		addImage(imeta.URL)
	}

	if tag := event.Tags.Find("image"); tag != nil {
		addImage(tag[1])
	}

	return h
}
