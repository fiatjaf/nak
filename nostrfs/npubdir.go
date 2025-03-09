package nostrfs

import (
	"context"
	"sync/atomic"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
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

func CreateNpubDir(ctx context.Context, sys *sdk.System, parent fs.InodeEmbedder, pointer nostr.ProfilePointer) *fs.Inode {
	npubdir := &NpubDir{ctx: ctx, sys: sys, pointer: pointer}
	return parent.EmbeddedInode().NewPersistentInode(
		ctx,
		npubdir,
		fs.StableAttr{Mode: syscall.S_IFDIR, Ino: hexToUint64(pointer.PublicKey)},
	)
}

var _ = (fs.NodeOpendirer)((*NpubDir)(nil))

func (n *NpubDir) Opendir(ctx context.Context) syscall.Errno {
	if n.fetched.CompareAndSwap(true, true) {
		return fs.OK
	}

	for ie := range n.sys.Pool.FetchMany(ctx, n.sys.FetchOutboxRelays(ctx, n.pointer.PublicKey, 2), nostr.Filter{
		Kinds:   []int{1},
		Authors: []string{n.pointer.PublicKey},
	}, nostr.WithLabel("nak-fs-feed")) {
		e := CreateEventDir(ctx, n, ie.Event)
		n.AddChild(ie.Event.ID, e, true)
	}

	return fs.OK
}
