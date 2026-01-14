package main

import (
	"github.com/winfsp/cgofuse/fuse"
)

type TestFS struct {
	fuse.FileSystemBase
}

func (fs *TestFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	switch path {
	case "/":
		stat.Mode = fuse.S_IFDIR | 0755
		return 0
	case "/static.txt":
		stat.Mode = fuse.S_IFREG | 0644
		stat.Size = 12
		return 0
	case "/dynamic":
		// Simulate dynamic node creation like nostrfs
		stat.Mode = fuse.S_IFLNK | 0777
		stat.Size = 10
		return 0
	default:
		return -fuse.ENOENT
	}
}

func (fs *TestFS) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {
	if path == "/" {
		fill(".", nil, 0)
		fill("..", nil, 0)
		fill("static.txt", nil, 0)
		fill("dynamic", nil, 0)
		return 0
	}
	return -fuse.ENOENT
}

func (fs *TestFS) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	if path == "/static.txt" {
		content := "hello world"
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

func (fs *TestFS) Readlink(path string) (errc int, target string) {
	if path == "/dynamic" {
		return 0, "target"
	}
	return -fuse.ENOENT, ""
}

func main() {
	fs := &TestFS{}
	host := fuse.NewFileSystemHost(fs)
	host.Mount("", []string{"testmount9"})
}
