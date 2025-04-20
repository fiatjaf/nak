package nostrfs

import (
	"context"
	"path/filepath"
	"syscall"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip05"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/sdk"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type Options struct {
	AutoPublishNotesTimeout    time.Duration
	AutoPublishArticlesTimeout time.Duration
}

type NostrRoot struct {
	fs.Inode

	ctx        context.Context
	wd         string
	sys        *sdk.System
	rootPubKey nostr.PubKey
	signer     nostr.Signer

	opts Options
}

var _ = (fs.NodeOnAdder)((*NostrRoot)(nil))

func NewNostrRoot(ctx context.Context, sys *sdk.System, user nostr.User, mountpoint string, o Options) *NostrRoot {
	pubkey, _ := user.GetPublicKey(ctx)
	abs, _ := filepath.Abs(mountpoint)

	var signer nostr.Signer
	if user != nil {
		signer, _ = user.(nostr.Signer)
	}

	return &NostrRoot{
		ctx:        ctx,
		sys:        sys,
		rootPubKey: pubkey,
		signer:     signer,
		wd:         abs,

		opts: o,
	}
}

func (r *NostrRoot) OnAdd(_ context.Context) {
	if r.rootPubKey == nostr.ZeroPK {
		return
	}

	go func() {
		time.Sleep(time.Millisecond * 100)

		// add our contacts
		fl := r.sys.FetchFollowList(r.ctx, r.rootPubKey)
		for _, f := range fl.Items {
			pointer := nostr.ProfilePointer{PublicKey: f.Pubkey, Relays: []string{f.Relay}}
			r.AddChild(
				nip19.EncodeNpub(f.Pubkey),
				r.CreateNpubDir(r, pointer, nil),
				true,
			)
		}

		// add ourselves
		npub := nip19.EncodeNpub(r.rootPubKey)
		if r.GetChild(npub) == nil {
			pointer := nostr.ProfilePointer{PublicKey: r.rootPubKey}

			r.AddChild(
				npub,
				r.CreateNpubDir(r, pointer, r.signer),
				true,
			)
		}

		// add a link to ourselves
		r.AddChild("@me", r.NewPersistentInode(
			r.ctx,
			&fs.MemSymlink{Data: []byte(r.wd + "/" + npub)},
			fs.StableAttr{Mode: syscall.S_IFLNK},
		), true)
	}()
}

func (r *NostrRoot) Lookup(_ context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	out.SetEntryTimeout(time.Minute * 5)

	child := r.GetChild(name)
	if child != nil {
		return child, fs.OK
	}

	if pp, err := nip05.QueryIdentifier(r.ctx, name); err == nil {
		return r.NewPersistentInode(
			r.ctx,
			&fs.MemSymlink{Data: []byte(r.wd + "/" + nip19.EncodePointer(*pp))},
			fs.StableAttr{Mode: syscall.S_IFLNK},
		), fs.OK
	}

	pointer, err := nip19.ToPointer(name)
	if err != nil {
		return nil, syscall.ENOENT
	}

	switch p := pointer.(type) {
	case nostr.ProfilePointer:
		npubdir := r.CreateNpubDir(r, p, nil)
		return npubdir, fs.OK
	case nostr.EventPointer:
		eventdir, err := r.FetchAndCreateEventDir(r, p)
		if err != nil {
			return nil, syscall.ENOENT
		}
		return eventdir, fs.OK
	default:
		return nil, syscall.ENOENT
	}
}
