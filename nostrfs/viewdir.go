package nostrfs

import (
	"context"
	"sync/atomic"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/nbd-wtf/go-nostr"
	sdk "github.com/nbd-wtf/go-nostr/sdk"
)

type ViewDir struct {
	fs.Inode
	ctx     context.Context
	sys     *sdk.System
	wd      string
	fetched atomic.Bool
	filter  nostr.Filter
	relays  []string
}

var (
	_ = (fs.NodeOpendirer)((*ViewDir)(nil))
	_ = (fs.NodeGetattrer)((*ViewDir)(nil))
)

func (n *ViewDir) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	now := nostr.Now()
	if n.filter.Until != nil {
		now = *n.filter.Until
	}
	aMonthAgo := now - 30*24*60*60
	out.Mtime = uint64(aMonthAgo)
	return fs.OK
}

func (n *ViewDir) Opendir(ctx context.Context) syscall.Errno {
	if n.fetched.CompareAndSwap(true, true) {
		return fs.OK
	}

	now := nostr.Now()
	if n.filter.Until != nil {
		now = *n.filter.Until
	}
	aMonthAgo := now - 30*24*60*60
	n.filter.Since = &aMonthAgo

	for ie := range n.sys.Pool.FetchMany(ctx, n.relays, n.filter, nostr.WithLabel("nakfs")) {
		e := CreateEventDir(ctx, n, n.wd, ie.Event)
		n.AddChild(ie.Event.ID, e, true)
	}

	filter := n.filter
	filter.Until = &aMonthAgo

	n.AddChild("@previous", n.NewPersistentInode(
		ctx,
		&ViewDir{
			ctx:    n.ctx,
			sys:    n.sys,
			filter: filter,
			wd:     n.wd,
			relays: n.relays,
		},
		fs.StableAttr{Mode: syscall.S_IFDIR},
	), true)

	return fs.OK
}
