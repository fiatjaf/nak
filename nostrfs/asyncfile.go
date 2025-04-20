package nostrfs

import (
	"context"
	"sync/atomic"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"fiatjaf.com/nostr"
)

type AsyncFile struct {
	fs.Inode
	ctx     context.Context
	fetched atomic.Bool
	data    []byte
	ts      nostr.Timestamp
	load    func() ([]byte, nostr.Timestamp)
}

var (
	_ = (fs.NodeOpener)((*AsyncFile)(nil))
	_ = (fs.NodeGetattrer)((*AsyncFile)(nil))
)

func (af *AsyncFile) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	if af.fetched.CompareAndSwap(false, true) {
		af.data, af.ts = af.load()
	}

	out.Size = uint64(len(af.data))
	out.Mtime = uint64(af.ts)
	return fs.OK
}

func (af *AsyncFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	if af.fetched.CompareAndSwap(false, true) {
		af.data, af.ts = af.load()
	}

	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

func (af *AsyncFile) Read(
	ctx context.Context,
	f fs.FileHandle,
	dest []byte,
	off int64,
) (fuse.ReadResult, syscall.Errno) {
	end := int(off) + len(dest)
	if end > len(af.data) {
		end = len(af.data)
	}
	return fuse.ReadResultData(af.data[off:end]), 0
}
