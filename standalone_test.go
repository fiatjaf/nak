package main

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"fiatjaf.com/nostr"
	"github.com/winfsp/cgofuse/fuse"
)

type nodeInfo struct {
	path     string
	isDir    bool
	children map[string]*nodeInfo
	data     []byte
	stat     fuse.Stat_t
}

type NakFS struct {
	fuse.FileSystemBase
	lock    sync.Mutex
	ino     uint64
	openmap map[uint64]*nodeInfo
}

func NewNakFS() *NakFS {
	fs := &NakFS{}
	fs.ino = 1
	fs.openmap = make(map[uint64]*nodeInfo)

	// Initialize root
	tmsp := fuse.Now()
	rootNode := &nodeInfo{
		path:     "/",
		isDir:    true,
		children: make(map[string]*nodeInfo),
		stat: fuse.Stat_t{
			Ino:      fs.ino,
			Mode:     fuse.S_IFDIR | 00777,
			Nlink:    2,
			Uid:      0,
			Gid:      0,
			Size:     0,
			Atim:     tmsp,
			Mtim:     tmsp,
			Ctim:     tmsp,
			Birthtim: tmsp,
		},
	}
	fs.openmap[fs.ino] = rootNode
	fs.ino++

	return fs
}

func (fs *NakFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	node := fs.getNode(path, fh)
	if node == nil {
		return -fuse.ENOENT
	}
	*stat = node.stat
	return 0
}

func (fs *NakFS) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	node := fs.getNode(path, fh)
	if node == nil || !node.isDir {
		return -fuse.ENOENT
	}

	fill(".", &node.stat, 0)
	fill("..", nil, 0)
	for name, child := range node.children {
		fill(name, &child.stat, 0)
	}
	return 0
}

func (fs *NakFS) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	node := fs.getNode(path, fh)
	if node == nil || node.isDir {
		return -fuse.ENOENT
	}

	endofst := ofst + int64(len(buff))
	if endofst > node.stat.Size {
		endofst = node.stat.Size
	}
	if endofst < ofst {
		return 0
	}
	n = copy(buff, node.data[ofst:endofst])
	node.stat.Atim = fuse.Now()
	return n
}

func (fs *NakFS) Open(path string, flags int) (errc int, fh uint64) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	node := fs.getNode(path, ^uint64(0))
	if node == nil {
		return -fuse.ENOENT, ^uint64(0)
	}

	if node.isDir && (flags&fuse.O_ACCMODE) != fuse.O_RDONLY {
		return -fuse.EISDIR, ^uint64(0)
	}

	fh = node.stat.Ino
	return 0, fh
}

func (fs *NakFS) Opendir(path string) (errc int, fh uint64) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	node := fs.getNode(path, ^uint64(0))
	if node == nil {
		return -fuse.ENOENT, ^uint64(0)
	}

	if !node.isDir {
		return -fuse.ENOTDIR, ^uint64(0)
	}

	fh = node.stat.Ino
	return 0, fh
}

func (fs *NakFS) Release(path string, fh uint64) (errc int) {
	return 0
}

func (fs *NakFS) Releasedir(path string, fh uint64) (errc int) {
	return 0
}

func (fs *NakFS) getNode(path string, fh uint64) *nodeInfo {
	if fh != ^uint64(0) {
		if node, ok := fs.openmap[fh]; ok {
			return node
		}
		return nil
	}

	// Simple path resolution
	node := fs.openmap[1] // root node
	if path == "/" || path == "" {
		return node
	}

	// For testing, just return nil for any other path
	return nil
}

func main() {
	fs := NewNakFS()
	host := fuse.NewFileSystemHost(fs)
	host.Mount("", []string{"testmount7"})
}
