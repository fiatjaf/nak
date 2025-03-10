package nostrfs

import (
	"context"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip05"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/sdk"
)

type NostrRoot struct {
	fs.Inode

	ctx        context.Context
	wd         string
	sys        *sdk.System
	rootPubKey string
	signer     nostr.Signer
}

var _ = (fs.NodeOnAdder)((*NostrRoot)(nil))

func NewNostrRoot(ctx context.Context, sys *sdk.System, user nostr.User, mountpoint string) *NostrRoot {
	pubkey, _ := user.GetPublicKey(ctx)
	signer, _ := user.(nostr.Signer)
	abs, _ := filepath.Abs(mountpoint)

	return &NostrRoot{
		ctx:        ctx,
		sys:        sys,
		rootPubKey: pubkey,
		signer:     signer,
		wd:         abs,
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
			CreateNpubDir(r.ctx, r.sys, r, r.wd, pointer),
			true,
		)
	}

	// add ourselves
	npub, _ := nip19.EncodePublicKey(r.rootPubKey)
	if r.GetChild(npub) == nil {
		pointer := nostr.ProfilePointer{PublicKey: r.rootPubKey}
		r.AddChild(
			npub,
			CreateNpubDir(r.ctx, r.sys, r, r.wd, pointer),
			true,
		)
	}

	// add a link to ourselves
	r.AddChild("@me", r.NewPersistentInode(
		r.ctx,
		&fs.MemSymlink{Data: []byte(r.wd + "/" + npub)},
		fs.StableAttr{Mode: syscall.S_IFLNK},
	), true)
}

func (r *NostrRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	out.SetEntryTimeout(time.Minute * 5)

	child := r.GetChild(name)
	if child != nil {
		return child, fs.OK
	}

	if pp, err := nip05.QueryIdentifier(ctx, name); err == nil {
		npubdir := CreateNpubDir(ctx, r.sys, r, r.wd, *pp)
		return npubdir, fs.OK
	}

	pointer, err := nip19.ToPointer(name)
	if err != nil {
		return nil, syscall.ENOENT
	}

	switch p := pointer.(type) {
	case nostr.ProfilePointer:
		npubdir := CreateNpubDir(ctx, r.sys, r, r.wd, p)
		return npubdir, fs.OK
	case nostr.EventPointer:
		eventdir, err := FetchAndCreateEventDir(ctx, r, r.wd, r.sys, p)
		if err != nil {
			return nil, syscall.ENOENT
		}
		return eventdir, fs.OK
	default:
		return nil, syscall.ENOENT
	}
}
