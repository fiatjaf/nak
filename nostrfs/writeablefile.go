package nostrfs

import (
	"context"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type WriteableFile struct {
	fs.Inode
	root    *NostrRoot
	mu      sync.Mutex
	data    []byte
	attr    fuse.Attr
	onWrite func(string)
}

var (
	_ = (fs.NodeOpener)((*WriteableFile)(nil))
	_ = (fs.NodeReader)((*WriteableFile)(nil))
	_ = (fs.NodeWriter)((*WriteableFile)(nil))
	_ = (fs.NodeGetattrer)((*WriteableFile)(nil))
	_ = (fs.NodeSetattrer)((*WriteableFile)(nil))
	_ = (fs.NodeFlusher)((*WriteableFile)(nil))
)

func (r *NostrRoot) NewWriteableFile(data string, ctime, mtime uint64, onWrite func(string)) *WriteableFile {
	return &WriteableFile{
		root: r,
		data: []byte(data),
		attr: fuse.Attr{
			Mode:  0666,
			Ctime: ctime,
			Mtime: mtime,
			Size:  uint64(len(data)),
		},
		onWrite: onWrite,
	}
}

func (f *WriteableFile) Open(ctx context.Context, flags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	return nil, fuse.FOPEN_KEEP_CACHE, fs.OK
}

func (f *WriteableFile) Write(ctx context.Context, fh fs.FileHandle, data []byte, off int64) (uint32, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()

	offset := int(off)
	end := offset + len(data)
	if len(f.data) < end {
		newData := make([]byte, offset+len(data))
		copy(newData, f.data)
		f.data = newData
	}
	copy(f.data[offset:], data)
	f.data = f.data[0:end]

	f.onWrite(string(f.data))
	return uint32(len(data)), fs.OK
}

func (f *WriteableFile) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	out.Attr = f.attr
	out.Attr.Size = uint64(len(f.data))
	return fs.OK
}

func (f *WriteableFile) Setattr(_ context.Context, _ fs.FileHandle, in *fuse.SetAttrIn, _ *fuse.AttrOut) syscall.Errno {
	f.attr.Mtime = in.Mtime
	f.attr.Atime = in.Atime
	f.attr.Ctime = in.Ctime
	return fs.OK
}

func (f *WriteableFile) Flush(ctx context.Context, fh fs.FileHandle) syscall.Errno {
	return fs.OK
}

func (f *WriteableFile) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	end := int(off) + len(dest)
	if end > len(f.data) {
		end = len(f.data)
	}
	return fuse.ReadResultData(f.data[off:end]), fs.OK
}
