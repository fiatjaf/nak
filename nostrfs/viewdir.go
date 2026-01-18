package nostrfs

import (
	"context"
	"strings"
	"sync/atomic"
	"syscall"

	"fiatjaf.com/lib/debouncer"
	"fiatjaf.com/nostr"
	"github.com/fatih/color"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type ViewDir struct {
	fs.Inode
	root        *NostrRoot
	fetched     atomic.Bool
	filter      nostr.Filter
	paginate    bool
	relays      []string
	replaceable bool
	createable  bool
	publisher   *debouncer.Debouncer
	publishing  struct {
		note string
	}
}

var (
	_ = (fs.NodeOpendirer)((*ViewDir)(nil))
	_ = (fs.NodeGetattrer)((*ViewDir)(nil))
	_ = (fs.NodeMkdirer)((*ViewDir)(nil))
	_ = (fs.NodeSetattrer)((*ViewDir)(nil))
	_ = (fs.NodeCreater)((*ViewDir)(nil))
	_ = (fs.NodeUnlinker)((*ViewDir)(nil))
)

func (f *ViewDir) Setattr(_ context.Context, _ fs.FileHandle, _ *fuse.SetAttrIn, _ *fuse.AttrOut) syscall.Errno {
	return fs.OK
}

func (n *ViewDir) Create(
	_ context.Context,
	name string,
	flags uint32,
	mode uint32,
	out *fuse.EntryOut,
) (node *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if !n.createable || n.root.rootPubKey != n.filter.Authors[0] {
		return nil, nil, 0, syscall.EPERM
	}
	if n.publisher == nil {
		n.publisher = debouncer.New(n.root.opts.AutoPublishNotesTimeout)
	}
	if n.filter.Kinds[0] != 1 {
		return nil, nil, 0, syscall.ENOTSUP
	}

	switch name {
	case "new":
		log := n.root.ctx.Value("log").(func(msg string, args ...any))

		if n.publisher.IsRunning() {
			log("pending note updated, timer reset.")
		} else {
			log("new note detected")
			if n.root.opts.AutoPublishNotesTimeout.Hours() < 24*365 {
				log(", publishing it in %d seconds...\n", int(n.root.opts.AutoPublishNotesTimeout.Seconds()))
			} else {
				log(".\n")
			}
			log("- `touch publish` to publish immediately\n")
			log("- `rm new` to erase and cancel the publication.\n")
		}

		n.publisher.Call(n.publishNote)

		first := true

		return n.NewPersistentInode(
			n.root.ctx,
			n.root.NewWriteableFile(n.publishing.note, uint64(nostr.Now()), uint64(nostr.Now()), func(s string) {
				if !first {
					log("pending note updated, timer reset.\n")
				}
				first = false
				n.publishing.note = strings.TrimSpace(s)
				n.publisher.Call(n.publishNote)
			}),
			fs.StableAttr{},
		), nil, 0, fs.OK
	case "publish":
		if n.publisher.IsRunning() {
			// this causes the publish process to be triggered faster
			log := n.root.ctx.Value("log").(func(msg string, args ...any))
			log("publishing now!\n")
			n.publisher.Flush()
			return nil, nil, 0, syscall.ENOTDIR
		}
	}

	return nil, nil, 0, syscall.ENOTSUP
}

func (n *ViewDir) Unlink(ctx context.Context, name string) syscall.Errno {
	if !n.createable || n.root.rootPubKey != n.filter.Authors[0] {
		return syscall.EPERM
	}
	if n.publisher == nil {
		n.publisher = debouncer.New(n.root.opts.AutoPublishNotesTimeout)
	}
	if n.filter.Kinds[0] != 1 {
		return syscall.ENOTSUP
	}

	switch name {
	case "new":
		log := n.root.ctx.Value("log").(func(msg string, args ...any))
		log("publishing canceled.\n")
		n.publisher.Stop()
		n.publishing.note = ""
		return fs.OK
	}

	return syscall.ENOTSUP
}

func (n *ViewDir) publishNote() {
	log := n.root.ctx.Value("log").(func(msg string, args ...any))

	log("publishing note...\n")
	evt := nostr.Event{
		Kind:      1,
		CreatedAt: nostr.Now(),
		Content:   n.publishing.note,
		Tags:      make(nostr.Tags, 0, 2),
	}

	// our write relays
	relays := n.root.sys.FetchWriteRelays(n.root.ctx, n.root.rootPubKey)
	if len(relays) == 0 {
		relays = n.root.sys.FetchOutboxRelays(n.root.ctx, n.root.rootPubKey, 6)
	}

	// massage and extract tags from raw text
	targetRelays := n.root.sys.PrepareNoteEvent(n.root.ctx, &evt)
	relays = nostr.AppendUnique(relays, targetRelays...)

	// sign and publish
	if err := n.root.signer.SignEvent(n.root.ctx, &evt); err != nil {
		log("failed to sign: %s\n", err)
		return
	}
	log(evt.String() + "\n")

	log("publishing to %d relays... ", len(relays))
	success := false
	first := true
	for res := range n.root.sys.Pool.PublishMany(n.root.ctx, relays, evt) {
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
		n.RmChild("new")
		n.AddChild(evt.ID.Hex(), n.root.CreateEventDir(n, &evt), true)
		log("event published as %s and updated locally.\n", color.BlueString(evt.ID.Hex()))
	}
}

func (n *ViewDir) Getattr(_ context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	now := nostr.Now()
	if n.filter.Until != 0 {
		now = n.filter.Until
	}
	aMonthAgo := now - 30*24*60*60
	out.Mtime = uint64(aMonthAgo)

	return fs.OK
}

func (n *ViewDir) Opendir(ctx context.Context) syscall.Errno {
	if n.fetched.CompareAndSwap(true, true) {
		return fs.OK
	}

	if n.paginate {
		now := nostr.Now()
		if n.filter.Until != 0 {
			now = n.filter.Until
		}
		aMonthAgo := now - 30*24*60*60
		n.filter.Since = aMonthAgo

		filter := n.filter
		filter.Until = aMonthAgo

		n.AddChild("@previous", n.NewPersistentInode(
			n.root.ctx,
			&ViewDir{
				root:        n.root,
				filter:      filter,
				relays:      n.relays,
				replaceable: n.replaceable,
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		), true)
	}

	if n.replaceable {
		for rkey, evt := range n.root.sys.Pool.FetchManyReplaceable(n.root.ctx, n.relays, n.filter, nostr.SubscriptionOptions{
			Label: "nakfs",
		}).Range {
			name := rkey.D
			if name == "" {
				name = "_"
			}
			if n.GetChild(name) == nil {
				n.AddChild(name, n.root.CreateEntityDir(n, &evt), true)
			}
		}
	} else {
		for ie := range n.root.sys.Pool.FetchMany(n.root.ctx, n.relays, n.filter,
			nostr.SubscriptionOptions{
				Label: "nakfs",
			}) {
			if n.GetChild(ie.Event.ID.Hex()) == nil {
				n.AddChild(ie.Event.ID.Hex(), n.root.CreateEventDir(n, &ie.Event), true)
			}
		}
	}

	return fs.OK
}

func (n *ViewDir) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if !n.createable || n.root.signer == nil || n.root.rootPubKey != n.filter.Authors[0] {
		return nil, syscall.ENOTSUP
	}

	if n.replaceable {
		// create a template event that can later be modified and published as new
		return n.root.CreateEntityDir(n, &nostr.Event{
			PubKey:    n.root.rootPubKey,
			CreatedAt: 0,
			Kind:      n.filter.Kinds[0],
			Tags: nostr.Tags{
				nostr.Tag{"d", name},
			},
		}), fs.OK
	}

	return nil, syscall.ENOTSUP
}
