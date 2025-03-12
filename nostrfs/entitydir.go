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
	"strings"
	"syscall"
	"time"
	"unsafe"

	"fiatjaf.com/lib/debouncer"
	"github.com/fatih/color"
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
	root *NostrRoot

	publisher *debouncer.Debouncer
	extension string
	event     *nostr.Event
	updating  struct {
		title   string
		content string
	}
}

var (
	_ = (fs.NodeOnAdder)((*EntityDir)(nil))
	_ = (fs.NodeGetattrer)((*EntityDir)(nil))
	_ = (fs.NodeCreater)((*EntityDir)(nil))
	_ = (fs.NodeUnlinker)((*EntityDir)(nil))
)

func (e *EntityDir) Getattr(_ context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	publishedAt := uint64(e.event.CreatedAt)
	out.Ctime = publishedAt

	if tag := e.event.Tags.Find("published_at"); tag != nil {
		publishedAt, _ = strconv.ParseUint(tag[1], 10, 64)
	}
	out.Mtime = publishedAt

	return fs.OK
}

func (e *EntityDir) Create(
	_ context.Context,
	name string,
	flags uint32,
	mode uint32,
	out *fuse.EntryOut,
) (node *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if name == "publish" {
		// this causes the publish process to be triggered faster
		e.publisher.Flush()
		return nil, nil, 0, syscall.ENOTDIR
	}

	return nil, nil, 0, syscall.ENOTSUP
}

func (e *EntityDir) Unlink(ctx context.Context, name string) syscall.Errno {
	switch name {
	case "content" + e.extension:
		e.updating.content = e.event.Content
		return syscall.ENOTDIR
	case "title":
		e.updating.title = ""
		if titleTag := e.event.Tags.Find("title"); titleTag != nil {
			e.updating.title = titleTag[1]
		}
		return syscall.ENOTDIR
	default:
		return syscall.EINTR
	}
}

func (e *EntityDir) OnAdd(_ context.Context) {
	log := e.root.ctx.Value("log").(func(msg string, args ...any))
	publishedAt := uint64(e.event.CreatedAt)
	if tag := e.event.Tags.Find("published_at"); tag != nil {
		publishedAt, _ = strconv.ParseUint(tag[1], 10, 64)
	}

	npub, _ := nip19.EncodePublicKey(e.event.PubKey)
	e.AddChild("@author", e.NewPersistentInode(
		e.root.ctx,
		&fs.MemSymlink{
			Data: []byte(e.root.wd + "/" + npub),
		},
		fs.StableAttr{Mode: syscall.S_IFLNK},
	), true)

	e.AddChild("event.json", e.NewPersistentInode(
		e.root.ctx,
		&DeterministicFile{
			get: func() (ctime uint64, mtime uint64, data string) {
				eventj, _ := json.MarshalIndent(e.event, "", "  ")
				return uint64(e.event.CreatedAt),
					uint64(e.event.CreatedAt),
					unsafe.String(unsafe.SliceData(eventj), len(eventj))
			},
		},
		fs.StableAttr{},
	), true)

	e.AddChild("identifier", e.NewPersistentInode(
		e.root.ctx,
		&fs.MemRegularFile{
			Data: []byte(e.event.Tags.GetD()),
			Attr: fuse.Attr{
				Mode:  0444,
				Ctime: uint64(e.event.CreatedAt),
				Mtime: uint64(e.event.CreatedAt),
				Size:  uint64(len(e.event.Tags.GetD())),
			},
		},
		fs.StableAttr{},
	), true)

	if e.root.signer == nil {
		// read-only
		e.AddChild("title", e.NewPersistentInode(
			e.root.ctx,
			&DeterministicFile{
				get: func() (ctime uint64, mtime uint64, data string) {
					var title string
					if tag := e.event.Tags.Find("title"); tag != nil {
						title = tag[1]
					} else {
						title = e.event.Tags.GetD()
					}
					return uint64(e.event.CreatedAt), publishedAt, title
				},
			},
			fs.StableAttr{},
		), true)
		e.AddChild("content."+e.extension, e.NewPersistentInode(
			e.root.ctx,
			&DeterministicFile{
				get: func() (ctime uint64, mtime uint64, data string) {
					return uint64(e.event.CreatedAt), publishedAt, e.event.Content
				},
			},
			fs.StableAttr{},
		), true)
	} else {
		// writeable
		if tag := e.event.Tags.Find("title"); tag != nil {
			e.updating.title = tag[1]
		}
		e.updating.content = e.event.Content

		e.AddChild("title", e.NewPersistentInode(
			e.root.ctx,
			e.root.NewWriteableFile(e.updating.title, uint64(e.event.CreatedAt), publishedAt, func(s string) {
				log("title updated")
				e.updating.title = strings.TrimSpace(s)
				e.handleWrite()
			}),
			fs.StableAttr{},
		), true)

		e.AddChild("content."+e.extension, e.NewPersistentInode(
			e.root.ctx,
			e.root.NewWriteableFile(e.updating.content, uint64(e.event.CreatedAt), publishedAt, func(s string) {
				log("content updated")
				e.updating.content = strings.TrimSpace(s)
				e.handleWrite()
			}),
			fs.StableAttr{},
		), true)
	}

	var refsdir *fs.Inode
	i := 0
	for ref := range nip27.ParseReferences(*e.event) {
		i++
		if refsdir == nil {
			refsdir = e.NewPersistentInode(e.root.ctx, &fs.Inode{}, fs.StableAttr{Mode: syscall.S_IFDIR})
			e.root.AddChild("references", refsdir, true)
		}
		refsdir.AddChild(fmt.Sprintf("ref_%02d", i), refsdir.NewPersistentInode(
			e.root.ctx,
			&fs.MemSymlink{
				Data: []byte(e.root.wd + "/" + nip19.EncodePointer(ref.Pointer)),
			},
			fs.StableAttr{Mode: syscall.S_IFLNK},
		), true)
	}

	var imagesdir *fs.Inode
	addImage := func(url string) {
		if imagesdir == nil {
			in := &fs.Inode{}
			imagesdir = e.NewPersistentInode(e.root.ctx, in, fs.StableAttr{Mode: syscall.S_IFDIR})
			e.AddChild("images", imagesdir, true)
		}
		imagesdir.AddChild(filepath.Base(url), imagesdir.NewPersistentInode(
			e.root.ctx,
			&AsyncFile{
				ctx: e.root.ctx,
				load: func() ([]byte, nostr.Timestamp) {
					ctx, cancel := context.WithTimeout(e.root.ctx, time.Second*20)
					defer cancel()
					r, err := http.NewRequestWithContext(ctx, "GET", url, nil)
					if err != nil {
						log("failed to load image %s: %s\n", url, err)
						return nil, 0
					}
					resp, err := http.DefaultClient.Do(r)
					if err != nil {
						log("failed to load image %s: %s\n", url, err)
						return nil, 0
					}
					defer resp.Body.Close()
					if resp.StatusCode >= 300 {
						log("failed to load image %s: %s\n", url, err)
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

	images := nip92.ParseTags(e.event.Tags)
	for _, imeta := range images {
		if imeta.URL == "" {
			continue
		}
		addImage(imeta.URL)
	}

	if tag := e.event.Tags.Find("image"); tag != nil {
		addImage(tag[1])
	}
}

func (e *EntityDir) handleWrite() {
	log := e.root.ctx.Value("log").(func(msg string, args ...any))

	if e.publisher.IsRunning() {
		log(", timer reset")
	}
	log(", will publish the updated event in 30 seconds...\n")
	if !e.publisher.IsRunning() {
		log("- `touch publish` to publish immediately\n")
		log("- `rm title content." + e.extension + "` to erase and cancel the edits\n")
	}

	e.publisher.Call(func() {
		if currentTitle := e.event.Tags.Find("title"); (currentTitle != nil && currentTitle[1] == e.updating.title) || (currentTitle == nil && e.updating.title == "") && e.updating.content == e.event.Content {
			log("back into the previous state, not publishing.\n")
			return
		}

		evt := nostr.Event{
			Kind:      e.event.Kind,
			Content:   e.updating.content,
			Tags:      make(nostr.Tags, len(e.event.Tags)),
			CreatedAt: nostr.Now(),
		}
		copy(evt.Tags, e.event.Tags)
		if e.updating.title != "" {
			if titleTag := evt.Tags.Find("title"); titleTag != nil {
				titleTag[1] = e.updating.title
			} else {
				evt.Tags = append(evt.Tags, nostr.Tag{"title", e.updating.title})
			}
		}
		if publishedAtTag := evt.Tags.Find("published_at"); publishedAtTag == nil {
			evt.Tags = append(evt.Tags, nostr.Tag{
				"published_at",
				strconv.FormatInt(int64(e.event.CreatedAt), 10),
			})
		}
		for ref := range nip27.ParseReferences(evt) {
			tag := ref.Pointer.AsTag()
			if existing := evt.Tags.FindWithValue(tag[0], tag[1]); existing == nil {
				evt.Tags = append(evt.Tags, tag)
			}
		}
		if err := e.root.signer.SignEvent(e.root.ctx, &evt); err != nil {
			log("failed to sign: '%s'.\n", err)
			return
		}

		relays := e.root.sys.FetchWriteRelays(e.root.ctx, evt.PubKey, 8)
		log("publishing to %d relays... ", len(relays))
		success := false
		first := true
		for res := range e.root.sys.Pool.PublishMany(e.root.ctx, relays, evt) {
			cleanUrl, ok := strings.CutPrefix(res.RelayURL, "wss://")
			if !ok {
				cleanUrl = res.RelayURL
			}

			if !first {
				log(", ")
			}
			first = false

			if res.Error != nil {
				log("%s: %s", color.RedString(cleanUrl), res.Error)
			} else {
				success = true
				log("%s: ok", color.GreenString(cleanUrl))
			}
		}
		log("\n")

		if success {
			e.event = &evt
			log("event updated locally.\n")
		} else {
			log("failed.\n")
		}
	})
}

func (r *NostrRoot) FetchAndCreateEntityDir(
	parent fs.InodeEmbedder,
	extension string,
	pointer nostr.EntityPointer,
) (*fs.Inode, error) {
	event, _, err := r.sys.FetchSpecificEvent(r.ctx, pointer, sdk.FetchSpecificEventParameters{
		WithRelays: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch: %w", err)
	}

	return r.CreateEntityDir(parent, extension, event), nil
}

func (r *NostrRoot) CreateEntityDir(
	parent fs.InodeEmbedder,
	extension string,
	event *nostr.Event,
) *fs.Inode {
	return parent.EmbeddedInode().NewPersistentInode(
		r.ctx,
		&EntityDir{root: r, event: event, publisher: debouncer.New(time.Second * 30), extension: extension},
		fs.StableAttr{Mode: syscall.S_IFDIR, Ino: hexToUint64(event.ID)},
	)
}
