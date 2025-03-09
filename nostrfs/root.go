package nostrfs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/sdk"
)

type NostrRoot struct {
	sys *sdk.System
	fs.Inode

	rootPubKey string
	signer     nostr.Signer
	ctx        context.Context
}

var _ = (fs.NodeOnAdder)((*NostrRoot)(nil))

func NewNostrRoot(ctx context.Context, sys *sdk.System, user nostr.User) *NostrRoot {
	pubkey, _ := user.GetPublicKey(ctx)
	signer, _ := user.(nostr.Signer)

	return &NostrRoot{
		sys:        sys,
		ctx:        ctx,
		rootPubKey: pubkey,
		signer:     signer,
	}
}

func (r *NostrRoot) OnAdd(context.Context) {
	if r.rootPubKey == "" {
		return
	}

	// add our contacts
	fl := r.sys.FetchFollowList(r.ctx, r.rootPubKey)
	for _, f := range fl.Items {
		pointer := nostr.ProfilePointer{PublicKey: f.Pubkey, Relays: []string{f.Relay}}
		npub, _ := nip19.EncodePublicKey(f.Pubkey)
		r.AddChild(
			npub,
			CreateNpubDir(r.ctx, r.sys, r, pointer),
			true,
		)
	}

	// add ourselves
	npub, _ := nip19.EncodePublicKey(r.rootPubKey)
	if r.GetChild(npub) == nil {
		pointer := nostr.ProfilePointer{PublicKey: r.rootPubKey}
		r.AddChild(
			npub,
			CreateNpubDir(r.ctx, r.sys, r, pointer),
			true,
		)
	}

	// add a link to ourselves
	me := r.NewPersistentInode(r.ctx, &fs.MemSymlink{Data: []byte(npub)}, fs.StableAttr{Mode: syscall.S_IFLNK})
	r.AddChild("@me", me, true)
}

func (r *NostrRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	// check if we already have this npub
	child := r.GetChild(name)
	if child != nil {
		return child, fs.OK
	}

	pointer, err := nip19.ToPointer(name)
	if err != nil {
		return nil, syscall.ENOENT
	}

	switch p := pointer.(type) {
	case nostr.ProfilePointer:
		npubdir := CreateNpubDir(ctx, r.sys, r, p)
		return npubdir, fs.OK
	case nostr.EventPointer:
		eventdir, err := FetchAndCreateEventDir(ctx, r, r.sys, p)
		if err != nil {
			return nil, syscall.ENOENT
		}
		return eventdir, fs.OK
	default:
		return nil, syscall.ENOENT
	}
}
