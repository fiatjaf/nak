package main

import (
	"github.com/winfsp/cgofuse/fuse"
	"os"
)

type MinimalFS struct {
	fuse.FileSystemBase
}

func (fs *MinimalFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	switch path {
	case "/":
		stat.Mode = fuse.S_IFDIR | 0755
		return 0
	case "/profile.json":
		stat.Mode = fuse.S_IFREG | 0644
		stat.Size = 27
		return 0
	default:
		return -fuse.ENOENT
	}
}

func (fs *MinimalFS) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {
	if path == "/" {
		fill(".", nil, 0)
		fill("..", nil, 0)
		fill("profile.json", nil, 0)
		return 0
	}
	return -fuse.ENOENT
}

func (fs *MinimalFS) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	if path == "/profile.json" {
		content := `{"public_key": "test_key"}`
		if ofst < int64(len(content)) {
			end := ofst + int64(len(buff))
			if end > int64(len(content)) {
				end = int64(len(content))
			}
			return copy(buff, content[ofst:end])
		}
	}
	return 0
}

func main() {
	fs := &MinimalFS{}
	host := fuse.NewFileSystemHost(fs)
	host.Mount("", os.Args[1:])
}
