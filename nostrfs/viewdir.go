package nostrfs

import (
	"context"
	"sync/atomic"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/nbd-wtf/go-nostr"
)

type ViewDir struct {
	fs.Inode
	root     *NostrRoot
	fetched  atomic.Bool
	filter   nostr.Filter
	paginate bool
	relays   []string
	create   func(*ViewDir, *nostr.Event) (string, *fs.Inode)
}

var (
	_ = (fs.NodeOpendirer)((*ViewDir)(nil))
	_ = (fs.NodeGetattrer)((*ViewDir)(nil))
)

func (n *ViewDir) Getattr(_ context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	now := nostr.Now()
	if n.filter.Until != nil {
		now = *n.filter.Until
	}
	aMonthAgo := now - 30*24*60*60
	out.Mtime = uint64(aMonthAgo)

	return fs.OK
}

func (n *ViewDir) Opendir(_ context.Context) syscall.Errno {
	if n.fetched.CompareAndSwap(true, true) {
		return fs.OK
	}

	if n.paginate {
		now := nostr.Now()
		if n.filter.Until != nil {
			now = *n.filter.Until
		}
		aMonthAgo := now - 30*24*60*60
		n.filter.Since = &aMonthAgo

		for ie := range n.root.sys.Pool.FetchMany(n.root.ctx, n.relays, n.filter, nostr.WithLabel("nakfs")) {
			basename, inode := n.create(n, ie.Event)
			n.AddChild(basename, inode, true)
		}

		filter := n.filter
		filter.Until = &aMonthAgo

		n.AddChild("@previous", n.NewPersistentInode(
			n.root.ctx,
			&ViewDir{
				root:   n.root,
				filter: filter,
				relays: n.relays,
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		), true)
	} else {
		for ie := range n.root.sys.Pool.FetchMany(n.root.ctx, n.relays, n.filter, nostr.WithLabel("nakfs")) {
			basename, inode := n.create(n, ie.Event)
			n.AddChild(basename, inode, true)
		}
	}

	return fs.OK
}
