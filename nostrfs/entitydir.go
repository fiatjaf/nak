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
	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nip27"
	"fiatjaf.com/nostr/nip73"
	"fiatjaf.com/nostr/nip92"
	sdk "fiatjaf.com/nostr/sdk"
	"github.com/fatih/color"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type EntityDir struct {
	fs.Inode
	root *NostrRoot

	publisher *debouncer.Debouncer
	event     *nostr.Event
	updating  struct {
		title       string
		content     string
		publishedAt uint64
	}
}

var (
	_ = (fs.NodeOnAdder)((*EntityDir)(nil))
	_ = (fs.NodeGetattrer)((*EntityDir)(nil))
	_ = (fs.NodeSetattrer)((*EntityDir)(nil))
	_ = (fs.NodeCreater)((*EntityDir)(nil))
	_ = (fs.NodeUnlinker)((*EntityDir)(nil))
)

func (e *EntityDir) Getattr(_ context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Ctime = uint64(e.event.CreatedAt)
	if e.updating.publishedAt != 0 {
		out.Mtime = e.updating.publishedAt
	} else {
		out.Mtime = e.PublishedAt()
	}
	return fs.OK
}

func (e *EntityDir) Create(
	_ context.Context,
	name string,
	flags uint32,
	mode uint32,
	out *fuse.EntryOut,
) (node *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if name == "publish" && e.publisher.IsRunning() {
		// this causes the publish process to be triggered faster
		log := e.root.ctx.Value("log").(func(msg string, args ...any))
		log("publishing now!\n")
		e.publisher.Flush()
		return nil, nil, 0, syscall.ENOTDIR
	}

	return nil, nil, 0, syscall.ENOTSUP
}

func (e *EntityDir) Unlink(ctx context.Context, name string) syscall.Errno {
	switch name {
	case "content" + kindToExtension(e.event.Kind):
		e.updating.content = e.event.Content
		return syscall.ENOTDIR
	case "title":
		e.updating.title = e.Title()
		return syscall.ENOTDIR
	default:
		return syscall.EINTR
	}
}

func (e *EntityDir) Setattr(_ context.Context, _ fs.FileHandle, in *fuse.SetAttrIn, _ *fuse.AttrOut) syscall.Errno {
	e.updating.publishedAt = in.Mtime
	return fs.OK
}

func (e *EntityDir) OnAdd(_ context.Context) {
	log := e.root.ctx.Value("log").(func(msg string, args ...any))

	e.AddChild("@author", e.NewPersistentInode(
		e.root.ctx,
		&fs.MemSymlink{
			Data: []byte(e.root.wd + "/" + nip19.EncodeNpub(e.event.PubKey)),
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

	if e.root.signer == nil || e.root.rootPubKey != e.event.PubKey {
		// read-only
		e.AddChild("title", e.NewPersistentInode(
			e.root.ctx,
			&DeterministicFile{
				get: func() (ctime uint64, mtime uint64, data string) {
					return uint64(e.event.CreatedAt), e.PublishedAt(), e.Title()
				},
			},
			fs.StableAttr{},
		), true)
		e.AddChild("content."+kindToExtension(e.event.Kind), e.NewPersistentInode(
			e.root.ctx,
			&DeterministicFile{
				get: func() (ctime uint64, mtime uint64, data string) {
					return uint64(e.event.CreatedAt), e.PublishedAt(), e.event.Content
				},
			},
			fs.StableAttr{},
		), true)
	} else {
		// writeable
		e.updating.title = e.Title()
		e.updating.publishedAt = e.PublishedAt()
		e.updating.content = e.event.Content

		e.AddChild("title", e.NewPersistentInode(
			e.root.ctx,
			e.root.NewWriteableFile(e.updating.title, uint64(e.event.CreatedAt), e.updating.publishedAt, func(s string) {
				log("title updated")
				e.updating.title = strings.TrimSpace(s)
				e.handleWrite()
			}),
			fs.StableAttr{},
		), true)

		e.AddChild("content."+kindToExtension(e.event.Kind), e.NewPersistentInode(
			e.root.ctx,
			e.root.NewWriteableFile(e.updating.content, uint64(e.event.CreatedAt), e.updating.publishedAt, func(s string) {
				log("content updated")
				e.updating.content = strings.TrimSpace(s)
				e.handleWrite()
			}),
			fs.StableAttr{},
		), true)
	}

	var refsdir *fs.Inode
	i := 0
	for ref := range nip27.Parse(e.event.Content) {
		if _, isExternal := ref.Pointer.(nip73.ExternalPointer); isExternal {
			continue
		}
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

func (e *EntityDir) IsNew() bool {
	return e.event.CreatedAt == 0
}

func (e *EntityDir) PublishedAt() uint64 {
	if tag := e.event.Tags.Find("published_at"); tag != nil {
		publishedAt, _ := strconv.ParseUint(tag[1], 10, 64)
		return publishedAt
	}
	return uint64(e.event.CreatedAt)
}

func (e *EntityDir) Title() string {
	if tag := e.event.Tags.Find("title"); tag != nil {
		return tag[1]
	}
	return ""
}

func (e *EntityDir) handleWrite() {
	log := e.root.ctx.Value("log").(func(msg string, args ...any))
	logverbose := e.root.ctx.Value("logverbose").(func(msg string, args ...any))

	if e.root.opts.AutoPublishArticlesTimeout.Hours() < 24*365 {
		if e.publisher.IsRunning() {
			log(", timer reset")
		}
		log(", publishing the ")
		if e.IsNew() {
			log("new")
		} else {
			log("updated")
		}
		log(" event in %d seconds...\n", int(e.root.opts.AutoPublishArticlesTimeout.Seconds()))
	} else {
		log(".\n")
	}
	if !e.publisher.IsRunning() {
		log("- `touch publish` to publish immediately\n")
		log("- `rm title content." + kindToExtension(e.event.Kind) + "` to erase and cancel the edits\n")
	}

	e.publisher.Call(func() {
		if e.Title() == e.updating.title && e.event.Content == e.updating.content {
			log("not modified, publish canceled.\n")
			return
		}

		evt := nostr.Event{
			Kind:      e.event.Kind,
			Content:   e.updating.content,
			Tags:      make(nostr.Tags, len(e.event.Tags)),
			CreatedAt: nostr.Now(),
		}
		copy(evt.Tags, e.event.Tags) // copy tags because that's the rule
		if e.updating.title != "" {
			if titleTag := evt.Tags.Find("title"); titleTag != nil {
				titleTag[1] = e.updating.title
			} else {
				evt.Tags = append(evt.Tags, nostr.Tag{"title", e.updating.title})
			}
		}

		// "published_at" tag
		publishedAtStr := strconv.FormatUint(e.updating.publishedAt, 10)
		if publishedAtStr != "0" {
			if publishedAtTag := evt.Tags.Find("published_at"); publishedAtTag != nil {
				publishedAtTag[1] = publishedAtStr
			} else {
				evt.Tags = append(evt.Tags, nostr.Tag{"published_at", publishedAtStr})
			}
		}

		// add "p" tags from people mentioned and "q" tags from events mentioned
		for ref := range nip27.Parse(evt.Content) {
			if _, isExternal := ref.Pointer.(nip73.ExternalPointer); isExternal {
				continue
			}

			tag := ref.Pointer.AsTag()
			key := tag[0]
			val := tag[1]
			if key == "e" || key == "a" {
				key = "q"
			}
			if existing := evt.Tags.FindWithValue(key, val); existing == nil {
				evt.Tags = append(evt.Tags, tag)
			}
		}

		// sign and publish
		if err := e.root.signer.SignEvent(e.root.ctx, &evt); err != nil {
			log("failed to sign: '%s'.\n", err)
			return
		}
		logverbose("%s\n", evt)

		relays := e.root.sys.FetchWriteRelays(e.root.ctx, e.root.rootPubKey)
		if len(relays) == 0 {
			relays = e.root.sys.FetchOutboxRelays(e.root.ctx, e.root.rootPubKey, 6)
		}

		log("publishing to %d relays... ", len(relays))
		success := false
		first := true
		for res := range e.root.sys.Pool.PublishMany(e.root.ctx, relays, evt) {
			cleanUrl, _ := strings.CutPrefix(res.RelayURL, "wss://")
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
			e.updating.publishedAt = uint64(evt.CreatedAt) // set this so subsequent edits get the correct value
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

	return r.CreateEntityDir(parent, event), nil
}

func (r *NostrRoot) CreateEntityDir(
	parent fs.InodeEmbedder,
	event *nostr.Event,
) *fs.Inode {
	return parent.EmbeddedInode().NewPersistentInode(
		r.ctx,
		&EntityDir{root: r, event: event, publisher: debouncer.New(r.opts.AutoPublishArticlesTimeout)},
		fs.StableAttr{Mode: syscall.S_IFDIR},
	)
}
