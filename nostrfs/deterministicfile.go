package nostrfs

import (
	"context"
	"syscall"
	"unsafe"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type DeterministicFile struct {
	fs.Inode
	get func() (ctime, mtime uint64, data string)
}

var (
	_ = (fs.NodeOpener)((*DeterministicFile)(nil))
	_ = (fs.NodeReader)((*DeterministicFile)(nil))
	_ = (fs.NodeGetattrer)((*DeterministicFile)(nil))
)

func (r *NostrRoot) NewDeterministicFile(get func() (ctime, mtime uint64, data string)) *DeterministicFile {
	return &DeterministicFile{
		get: get,
	}
}

func (f *DeterministicFile) Open(ctx context.Context, flags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	return nil, fuse.FOPEN_KEEP_CACHE, fs.OK
}

func (f *DeterministicFile) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	var content string
	out.Mode = 0444
	out.Ctime, out.Mtime, content = f.get()
	out.Size = uint64(len(content))
	return fs.OK
}

func (f *DeterministicFile) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	_, _, content := f.get()
	data := unsafe.Slice(unsafe.StringData(content), len(content))

	end := int(off) + len(dest)
	if end > len(data) {
		end = len(data)
	}
	return fuse.ReadResultData(data[off:end]), fs.OK
}
