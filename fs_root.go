//go:build !openbsd

package main

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip05"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/sdk"
	"github.com/winfsp/cgofuse/fuse"
)

type FSOptions struct {
	AutoPublishNotesTimeout    time.Duration
	AutoPublishArticlesTimeout time.Duration
}

type FSRoot struct {
	fuse.FileSystemBase
	ctx        context.Context
	sys        *sdk.System
	rootPubKey nostr.PubKey
	signer     nostr.Signer
	opts       FSOptions
	mountpoint string

	mu           sync.RWMutex
	nodes        map[string]*FSNode // path -> node
	nextIno      uint64
	pendingNotes map[string]*time.Timer // path -> auto-publish timer
}

type FSNode struct {
	ino      uint64
	path     string
	name     string
	isDir    bool
	size     int64
	mode     uint32
	mtime    time.Time
	data     []byte
	children map[string]*FSNode
	loadFunc func() ([]byte, error) // for lazy loading
	loaded   bool
}

var _ fuse.FileSystemInterface = (*FSRoot)(nil)

func NewFSRoot(ctx context.Context, sys interface{}, user interface{}, mountpoint string, o FSOptions) *FSRoot {
	var system *sdk.System
	if sys != nil {
		system = sys.(*sdk.System)
	}

	var pubkey nostr.PubKey
	var signer nostr.Signer

	if user != nil {
		if u, ok := user.(nostr.User); ok {
			pubkey, _ = u.GetPublicKey(ctx)
			signer, _ = user.(nostr.Signer)
		}
	}

	abs, _ := filepath.Abs(mountpoint)

	root := &FSRoot{
		ctx:          ctx,
		sys:          system,
		rootPubKey:   pubkey,
		signer:       signer,
		opts:         o,
		mountpoint:   abs,
		nodes:        make(map[string]*FSNode),
		nextIno:      2, // 1 is reserved for root
		pendingNotes: make(map[string]*time.Timer),
	}

	// initialize root directory
	rootNode := &FSNode{
		ino:      1,
		path:     "/",
		name:     "",
		isDir:    true,
		mode:     fuse.S_IFDIR | 0755,
		mtime:    time.Now(),
		children: make(map[string]*FSNode),
	}
	root.nodes["/"] = rootNode

	// start async initialization
	go root.initialize()

	return root
}

func (r *FSRoot) initialize() {
	if r.rootPubKey == nostr.ZeroPK {
		return
	}

	log := r.getLog()
	time.Sleep(time.Millisecond * 100)

	// fetch follow list
	fl := r.sys.FetchFollowList(r.ctx, r.rootPubKey)
	log("- fetched %d contacts\n", len(fl.Items))

	r.mu.Lock()
	defer r.mu.Unlock()

	// add our contacts
	for _, f := range fl.Items {
		npub := nip19.EncodeNpub(f.Pubkey)
		if _, exists := r.nodes["/"+npub]; !exists {
			r.createNpubDirLocked(npub, f.Pubkey, nil)
		}
	}

	// add ourselves
	npub := nip19.EncodeNpub(r.rootPubKey)
	if _, exists := r.nodes["/"+npub]; !exists {
		r.createNpubDirLocked(npub, r.rootPubKey, r.signer)
	}

	// add @me symlink (for now, just create a text file pointing to our npub)
	meNode := &FSNode{
		ino:   r.nextIno,
		path:  "/@me",
		name:  "@me",
		isDir: false,
		mode:  fuse.S_IFREG | 0444,
		mtime: time.Now(),
		data:  []byte(npub + "\n"),
		size:  int64(len(npub) + 1),
	}
	r.nextIno++
	r.nodes["/@me"] = meNode
	r.nodes["/"].children["@me"] = meNode
}

func (r *FSRoot) fetchMetadata(dirPath string, pubkey nostr.PubKey) {
	pm := r.sys.FetchProfileMetadata(r.ctx, pubkey)
	if pm.Event == nil {
		return
	}

	// use the content field which contains the actual profile JSON
	metadataJ := []byte(pm.Event.Content)

	r.mu.Lock()
	defer r.mu.Unlock()

	metadataNode := &FSNode{
		ino:   r.nextIno,
		path:  dirPath + "/metadata.json",
		name:  "metadata.json",
		isDir: false,
		mode:  fuse.S_IFREG | 0444,
		mtime: time.Unix(int64(pm.Event.CreatedAt), 0),
		data:  metadataJ,
		size:  int64(len(metadataJ)),
	}
	r.nextIno++
	r.nodes[dirPath+"/metadata.json"] = metadataNode
	if dir, ok := r.nodes[dirPath]; ok {
		dir.children["metadata.json"] = metadataNode
	}
}

func (r *FSRoot) fetchProfilePicture(dirPath string, pubkey nostr.PubKey) {
	pm := r.sys.FetchProfileMetadata(r.ctx, pubkey)
	if pm.Event == nil || pm.Picture == "" {
		return
	}

	// download picture
	ctx, cancel := context.WithTimeout(r.ctx, time.Second*20)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", pm.Picture, nil)
	if err != nil {
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return
	}

	// read image data
	imageData := make([]byte, 0, 1024*1024) // 1MB initial capacity
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			imageData = append(imageData, buf[:n]...)
		}
		if err != nil {
			break
		}
		if len(imageData) > 10*1024*1024 { // 10MB max
			break
		}
	}

	if len(imageData) == 0 {
		return
	}

	// detect file extension from content-type or URL
	ext := "png"
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		switch ct {
		case "image/jpeg":
			ext = "jpg"
		case "image/png":
			ext = "png"
		case "image/gif":
			ext = "gif"
		case "image/webp":
			ext = "webp"
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	picturePath := dirPath + "/picture." + ext
	pictureNode := &FSNode{
		ino:   r.nextIno,
		path:  picturePath,
		name:  "picture." + ext,
		isDir: false,
		mode:  fuse.S_IFREG | 0444,
		mtime: time.Unix(int64(pm.Event.CreatedAt), 0),
		data:  imageData,
		size:  int64(len(imageData)),
	}
	r.nextIno++
	r.nodes[picturePath] = pictureNode
	if dir, ok := r.nodes[dirPath]; ok {
		dir.children["picture."+ext] = pictureNode
	}
}

func (r *FSRoot) fetchEvents(dirPath string, filter nostr.Filter) {
	ctx, cancel := context.WithTimeout(r.ctx, time.Second*10)
	defer cancel()

	// get relays for authors
	var relays []string
	if len(filter.Authors) > 0 {
		relays = r.sys.FetchOutboxRelays(ctx, filter.Authors[0], 3)
	}
	if len(relays) == 0 {
		relays = []string{"wss://relay.damus.io", "wss://nos.lol"}
	}

	log := r.getLog()
	log("- fetching events for %s from %v\n", dirPath, relays)

	// fetch events
	events := make([]*nostr.Event, 0)
	for ie := range r.sys.Pool.FetchMany(ctx, relays, filter, nostr.SubscriptionOptions{
		Label: "nak-fs",
	}) {
		// make a copy to avoid pointer issues with loop variable
		evt := ie.Event
		events = append(events, &evt)
		if len(events) >= int(filter.Limit) {
			break
		}
	}

	log("- fetched %d events for %s\n", len(events), dirPath)

	r.mu.Lock()
	defer r.mu.Unlock()

	dir, ok := r.nodes[dirPath]
	if !ok {
		return
	}

	// track oldest timestamp for pagination
	var oldestTimestamp nostr.Timestamp
	if len(events) > 0 {
		oldestTimestamp = events[len(events)-1].CreatedAt
	}

	for _, evt := range events {
		// create filename based on event
		filename := r.eventToFilename(evt)
		filePath := dirPath + "/" + filename

		if _, exists := r.nodes[filePath]; exists {
			continue
		}

		content := evt.Content
		if len(content) == 0 {
			content = "(empty)"
		}

		fileNode := &FSNode{
			ino:   r.nextIno,
			path:  filePath,
			name:  filename,
			isDir: false,
			mode:  fuse.S_IFREG | 0644,
			mtime: time.Unix(int64(evt.CreatedAt), 0),
			data:  []byte(content),
			size:  int64(len(content)),
		}
		r.nextIno++
		r.nodes[filePath] = fileNode
		dir.children[filename] = fileNode
	}

	// add "more" file for pagination if we got a full page
	if len(events) >= int(filter.Limit) {
		moreFile := &FSNode{
			ino:   r.nextIno,
			path:  dirPath + "/.more",
			name:  ".more",
			isDir: false,
			mode:  fuse.S_IFREG | 0444,
			mtime: time.Now(),
			data:  []byte(fmt.Sprintf("Read this file to load more events (until: %d)\n", oldestTimestamp)),
			size:  int64(len(fmt.Sprintf("Read this file to load more events (until: %d)\n", oldestTimestamp))),
			loadFunc: func() ([]byte, error) {
				// when .more is read, fetch next page
				newFilter := filter
				newFilter.Until = oldestTimestamp
				go r.fetchEvents(dirPath, newFilter)
				return []byte("Loading more events...\n"), nil
			},
		}
		r.nextIno++
		r.nodes[dirPath+"/.more"] = moreFile
		dir.children[".more"] = moreFile
	}
}

func (r *FSRoot) eventToFilename(evt *nostr.Event) string {
	// use event ID first 8 chars + extension based on kind
	ext := kindToExtension(evt.Kind)

	// get hex representation of event ID
	// evt.ID.String() may return format like ":1234abcd" so use Hex() or remove colons
	idHex := evt.ID.Hex()
	if len(idHex) > 8 {
		idHex = idHex[:8]
	}

	// for articles, try to use title
	if evt.Kind == 30023 || evt.Kind == 30818 {
		for _, tag := range evt.Tags {
			if len(tag) >= 2 && tag[0] == "title" {
				titleStr := tag[1]
				if titleStr != "" {
					// sanitize title for filename
					name := strings.Map(func(r rune) rune {
						if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
							return '-'
						}
						return r
					}, titleStr)
					if len(name) > 50 {
						name = name[:50]
					}
					return fmt.Sprintf("%s-%s.%s", name, idHex, ext)
				}
			}
		}
	}

	return fmt.Sprintf("%s.%s", idHex, ext)
}

func (r *FSRoot) getLog() func(string, ...interface{}) {
	if log := r.ctx.Value("log"); log != nil {
		return log.(func(string, ...interface{}))
	}
	return func(string, ...interface{}) {}
}

func (r *FSRoot) getNode(path string) *FSNode {
	originalPath := path

	// normalize path
	if path == "" {
		path = "/"
	}

	// convert Windows backslashes to forward slashes
	path = strings.ReplaceAll(path, "\\", "/")

	// ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// remove trailing slash except for root
	if path != "/" && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}

	// debug logging
	if r.ctx.Value("logverbose") != nil {
		logv := r.ctx.Value("logverbose").(func(string, ...interface{}))
		logv("getNode: original='%s' normalized='%s'\n", originalPath, path)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	node := r.nodes[path]

	// debug: if not found, show similar paths
	if node == nil && r.ctx.Value("logverbose") != nil {
		logv := r.ctx.Value("logverbose").(func(string, ...interface{}))
		logv("getNode: NOT FOUND '%s'\n", path)
		basename := filepath.Base(path)
		logv("getNode: searching for similar (basename='%s'):\n", basename)
		count := 0
		for p := range r.nodes {
			if strings.Contains(p, basename) {
				logv("  - '%s'\n", p)
				count++
				if count >= 5 {
					break
				}
			}
		}
	}

	return node
}

func (r *FSRoot) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	node := r.getNode(path)

	// if node doesn't exist, try dynamic lookup
	// but skip for special files starting with @ or .
	if node == nil {
		basename := filepath.Base(path)
		if !strings.HasPrefix(basename, "@") && !strings.HasPrefix(basename, ".") {
			if r.dynamicLookup(path) {
				node = r.getNode(path)
			}
		}
	}

	if node == nil {
		return -fuse.ENOENT
	}

	stat.Ino = node.ino
	stat.Mode = node.mode
	stat.Size = node.size
	stat.Mtim = fuse.NewTimespec(node.mtime)
	stat.Atim = stat.Mtim
	stat.Ctim = stat.Mtim

	return 0
}

// dynamicLookup tries to create nodes on-demand for npub/note/nevent paths
func (r *FSRoot) dynamicLookup(path string) bool {
	// normalize path
	path = strings.ReplaceAll(path, "\\", "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// get the first component after root
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 0 {
		return false
	}

	name := parts[0]

	// try to decode as nostr pointer
	pointer, err := nip19.ToPointer(name)
	if err != nil {
		// try NIP-05
		if strings.Contains(name, "@") && !strings.HasPrefix(name, "@") {
			ctx, cancel := context.WithTimeout(r.ctx, time.Second*5)
			defer cancel()
			if pp, err := nip05.QueryIdentifier(ctx, name); err == nil {
				pointer = pp
			} else {
				return false
			}
		} else {
			return false
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// check if already exists
	if _, exists := r.nodes["/"+name]; exists {
		return true
	}

	switch p := pointer.(type) {
	case nostr.ProfilePointer:
		// create npub directory dynamically
		r.createNpubDirLocked(name, p.PublicKey, nil)
		return true

	case nostr.EventPointer:
		// create event directory dynamically
		return r.createEventDirLocked(name, p)

	default:
		return false
	}
}

func (r *FSRoot) createNpubDirLocked(npub string, pubkey nostr.PubKey, signer nostr.Signer) {
	dirPath := "/" + npub

	// check if already exists
	if _, exists := r.nodes[dirPath]; exists {
		return
	}

	dirNode := &FSNode{
		ino:      r.nextIno,
		path:     dirPath,
		name:     npub,
		isDir:    true,
		mode:     fuse.S_IFDIR | 0755,
		mtime:    time.Now(),
		children: make(map[string]*FSNode),
	}
	r.nextIno++
	r.nodes[dirPath] = dirNode
	r.nodes["/"].children[npub] = dirNode

	// add pubkey file
	pubkeyData := []byte(pubkey.Hex() + "\n")
	pubkeyNode := &FSNode{
		ino:   r.nextIno,
		path:  dirPath + "/pubkey",
		name:  "pubkey",
		isDir: false,
		mode:  fuse.S_IFREG | 0444,
		mtime: time.Now(),
		data:  pubkeyData,
		size:  int64(len(pubkeyData)),
	}
	r.nextIno++
	r.nodes[dirPath+"/pubkey"] = pubkeyNode
	dirNode.children["pubkey"] = pubkeyNode

	// fetch metadata asynchronously
	go r.fetchMetadata(dirPath, pubkey)

	// add notes directory
	r.createViewDirLocked(dirPath, "notes", nostr.Filter{
		Kinds:   []nostr.Kind{1},
		Authors: []nostr.PubKey{pubkey},
		Limit:   50,
	})

	// add articles directory
	r.createViewDirLocked(dirPath, "articles", nostr.Filter{
		Kinds:   []nostr.Kind{30023},
		Authors: []nostr.PubKey{pubkey},
		Limit:   50,
	})

	// add comments directory
	r.createViewDirLocked(dirPath, "comments", nostr.Filter{
		Kinds:   []nostr.Kind{1111},
		Authors: []nostr.PubKey{pubkey},
		Limit:   50,
	})

	// add highlights directory
	r.createViewDirLocked(dirPath, "highlights", nostr.Filter{
		Kinds:   []nostr.Kind{9802},
		Authors: []nostr.PubKey{pubkey},
		Limit:   50,
	})

	// add photos directory
	r.createViewDirLocked(dirPath, "photos", nostr.Filter{
		Kinds:   []nostr.Kind{20},
		Authors: []nostr.PubKey{pubkey},
		Limit:   50,
	})

	// add videos directory
	r.createViewDirLocked(dirPath, "videos", nostr.Filter{
		Kinds:   []nostr.Kind{21, 22},
		Authors: []nostr.PubKey{pubkey},
		Limit:   50,
	})

	// add wikis directory
	r.createViewDirLocked(dirPath, "wikis", nostr.Filter{
		Kinds:   []nostr.Kind{30818},
		Authors: []nostr.PubKey{pubkey},
		Limit:   50,
	})

	// fetch profile picture asynchronously
	go r.fetchProfilePicture(dirPath, pubkey)
}

func (r *FSRoot) createViewDirLocked(parentPath, name string, filter nostr.Filter) {
	dirPath := parentPath + "/" + name

	// check if already exists
	if _, exists := r.nodes[dirPath]; exists {
		return
	}

	dirNode := &FSNode{
		ino:      r.nextIno,
		path:     dirPath,
		name:     name,
		isDir:    true,
		mode:     fuse.S_IFDIR | 0755,
		mtime:    time.Now(),
		children: make(map[string]*FSNode),
	}
	r.nextIno++

	r.nodes[dirPath] = dirNode
	if parent, ok := r.nodes[parentPath]; ok {
		parent.children[name] = dirNode
	}

	// fetch events asynchronously
	go r.fetchEvents(dirPath, filter)
}

func (r *FSRoot) createEventDirLocked(name string, pointer nostr.EventPointer) bool {
	dirPath := "/" + name

	// fetch the event
	ctx, cancel := context.WithTimeout(r.ctx, time.Second*10)
	defer cancel()

	var relays []string
	if len(pointer.Relays) > 0 {
		relays = pointer.Relays
	} else {
		relays = []string{"wss://relay.damus.io", "wss://nos.lol"}
	}

	filter := nostr.Filter{IDs: []nostr.ID{pointer.ID}}

	var evt *nostr.Event
	for ie := range r.sys.Pool.FetchMany(ctx, relays, filter, nostr.SubscriptionOptions{
		Label: "nak-fs-event",
	}) {
		// make a copy to avoid pointer issues
		evtCopy := ie.Event
		evt = &evtCopy
		break
	}

	if evt == nil {
		return false
	}

	// create event directory
	dirNode := &FSNode{
		ino:      r.nextIno,
		path:     dirPath,
		name:     name,
		isDir:    true,
		mode:     fuse.S_IFDIR | 0755,
		mtime:    time.Unix(int64(evt.CreatedAt), 0),
		children: make(map[string]*FSNode),
	}
	r.nextIno++
	r.nodes[dirPath] = dirNode
	r.nodes["/"].children[name] = dirNode

	// add content file
	ext := kindToExtension(evt.Kind)
	contentPath := dirPath + "/content." + ext
	contentNode := &FSNode{
		ino:   r.nextIno,
		path:  contentPath,
		name:  "content." + ext,
		isDir: false,
		mode:  fuse.S_IFREG | 0644,
		mtime: time.Unix(int64(evt.CreatedAt), 0),
		data:  []byte(evt.Content),
		size:  int64(len(evt.Content)),
	}
	r.nextIno++
	r.nodes[contentPath] = contentNode
	dirNode.children["content."+ext] = contentNode

	// add event.json
	eventJSON, _ := stdjson.MarshalIndent(evt, "", "  ")
	eventJSONPath := dirPath + "/event.json"
	eventJSONNode := &FSNode{
		ino:   r.nextIno,
		path:  eventJSONPath,
		name:  "event.json",
		isDir: false,
		mode:  fuse.S_IFREG | 0444,
		mtime: time.Unix(int64(evt.CreatedAt), 0),
		data:  eventJSON,
		size:  int64(len(eventJSON)),
	}
	r.nextIno++
	r.nodes[eventJSONPath] = eventJSONNode
	dirNode.children["event.json"] = eventJSONNode

	return true
}

func (r *FSRoot) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64,
) int {
	node := r.getNode(path)
	if node == nil || !node.isDir {
		return -fuse.ENOENT
	}

	fill(".", nil, 0)
	fill("..", nil, 0)

	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, child := range node.children {
		stat := &fuse.Stat_t{
			Ino:  child.ino,
			Mode: child.mode,
			Size: child.size,
			Mtim: fuse.NewTimespec(child.mtime),
		}
		if !fill(name, stat, 0) {
			break
		}
	}

	return 0
}

func (r *FSRoot) Open(path string, flags int) (int, uint64) {
	// log the open attempt
	if r.ctx.Value("logverbose") != nil {
		logv := r.ctx.Value("logverbose").(func(string, ...interface{}))
		logv("Open: path='%s' flags=%d\n", path, flags)
	}

	node := r.getNode(path)
	if node == nil {
		return -fuse.ENOENT, ^uint64(0)
	}
	if node.isDir {
		return -fuse.EISDIR, ^uint64(0)
	}

	// load data if needed
	if node.loadFunc != nil && !node.loaded {
		r.mu.Lock()
		if !node.loaded {
			if data, err := node.loadFunc(); err == nil {
				node.data = data
				node.size = int64(len(data))
				node.loaded = true
			}
		}
		r.mu.Unlock()
	}

	return 0, node.ino
}

func (r *FSRoot) Read(path string, buff []byte, ofst int64, fh uint64) int {
	node := r.getNode(path)
	if node == nil || node.isDir {
		return -fuse.ENOENT
	}

	if ofst >= node.size {
		return 0
	}

	endofst := ofst + int64(len(buff))
	if endofst > node.size {
		endofst = node.size
	}

	n := copy(buff, node.data[ofst:endofst])
	return n
}

func (r *FSRoot) Opendir(path string) (int, uint64) {
	node := r.getNode(path)
	if node == nil {
		return -fuse.ENOENT, ^uint64(0)
	}
	if !node.isDir {
		return -fuse.ENOTDIR, ^uint64(0)
	}
	return 0, node.ino
}

func (r *FSRoot) Release(path string, fh uint64) int {
	return 0
}

func (r *FSRoot) Releasedir(path string, fh uint64) int {
	return 0
}

// Create creates a new file
func (r *FSRoot) Create(path string, flags int, mode uint32) (int, uint64) {
	// parse path
	path = strings.ReplaceAll(path, "\\", "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	dir := filepath.Dir(path)
	name := filepath.Base(path)

	r.mu.Lock()
	defer r.mu.Unlock()

	// check if parent directory exists
	parent, ok := r.nodes[dir]
	if !ok || !parent.isDir {
		return -fuse.ENOENT, ^uint64(0)
	}

	// check if file already exists
	if _, exists := r.nodes[path]; exists {
		return -fuse.EEXIST, ^uint64(0)
	}

	// create new file node
	fileNode := &FSNode{
		ino:   r.nextIno,
		path:  path,
		name:  name,
		isDir: false,
		mode:  fuse.S_IFREG | 0644,
		mtime: time.Now(),
		data:  []byte{},
		size:  0,
	}
	r.nextIno++

	r.nodes[path] = fileNode
	parent.children[name] = fileNode

	return 0, fileNode.ino
}

// Truncate truncates a file
func (r *FSRoot) Truncate(path string, size int64, fh uint64) int {
	node := r.getNode(path)
	if node == nil {
		return -fuse.ENOENT
	}
	if node.isDir {
		return -fuse.EISDIR
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if size == 0 {
		node.data = []byte{}
	} else if size < int64(len(node.data)) {
		node.data = node.data[:size]
	} else {
		// extend with zeros
		newData := make([]byte, size)
		copy(newData, node.data)
		node.data = newData
	}
	node.size = size
	node.mtime = time.Now()

	return 0
}

// Write writes data to a file
func (r *FSRoot) Write(path string, buff []byte, ofst int64, fh uint64) int {
	node := r.getNode(path)
	if node == nil {
		return -fuse.ENOENT
	}
	if node.isDir {
		return -fuse.EISDIR
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	endofst := ofst + int64(len(buff))

	// extend data if necessary
	if endofst > int64(len(node.data)) {
		newData := make([]byte, endofst)
		copy(newData, node.data)
		node.data = newData
	}

	n := copy(node.data[ofst:], buff)
	node.size = int64(len(node.data))
	node.mtime = time.Now()

	// check if this is a note that should be auto-published
	if r.signer != nil && strings.Contains(path, "/notes/") && !strings.HasPrefix(filepath.Base(path), ".") {
		// cancel existing timer if any
		if timer, exists := r.pendingNotes[path]; exists {
			timer.Stop()
		}

		// schedule auto-publish
		timeout := r.opts.AutoPublishNotesTimeout
		if timeout > 0 && timeout < time.Hour*24*365 {
			r.pendingNotes[path] = time.AfterFunc(timeout, func() {
				r.publishNote(path)
			})
		}
	}

	return n
}

func (r *FSRoot) publishNote(path string) {
	r.mu.Lock()
	node, ok := r.nodes[path]
	if !ok {
		r.mu.Unlock()
		return
	}

	content := string(node.data)
	r.mu.Unlock()

	if r.signer == nil {
		return
	}

	log := r.getLog()
	log("- auto-publishing note from %s\n", path)

	// create and sign event
	evt := &nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      1,
		Tags:      nostr.Tags{},
		Content:   content,
	}

	if err := r.signer.SignEvent(r.ctx, evt); err != nil {
		log("- failed to sign note: %v\n", err)
		return
	}

	// publish to relays
	ctx, cancel := context.WithTimeout(r.ctx, time.Second*10)
	defer cancel()

	relays := r.sys.FetchOutboxRelays(ctx, r.rootPubKey, 3)
	if len(relays) == 0 {
		relays = []string{"wss://relay.damus.io", "wss://nos.lol"}
	}

	for _, url := range relays {
		relay, err := r.sys.Pool.EnsureRelay(url)
		if err != nil {
			continue
		}
		relay.Publish(ctx, *evt)
	}

	log("- published note %s to %d relays\n", evt.ID.Hex()[:8], len(relays))

	// update filename to include event ID
	r.mu.Lock()
	defer r.mu.Unlock()

	dir := filepath.Dir(path)
	oldName := filepath.Base(path)
	ext := filepath.Ext(oldName)
	newName := evt.ID.Hex()[:8] + ext
	newPath := dir + "/" + newName

	// rename node
	if _, exists := r.nodes[newPath]; !exists {
		node.path = newPath
		node.name = newName
		r.nodes[newPath] = node
		delete(r.nodes, path)

		if parent, ok := r.nodes[dir]; ok {
			delete(parent.children, oldName)
			parent.children[newName] = node
		}
	}

	delete(r.pendingNotes, path)
}

// Unlink deletes a file
func (r *FSRoot) Unlink(path string) int {
	path = strings.ReplaceAll(path, "\\", "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	dir := filepath.Dir(path)
	name := filepath.Base(path)

	r.mu.Lock()
	defer r.mu.Unlock()

	// check if file exists
	node, ok := r.nodes[path]
	if !ok {
		return -fuse.ENOENT
	}
	if node.isDir {
		return -fuse.EISDIR
	}

	// remove from parent
	if parent, ok := r.nodes[dir]; ok {
		delete(parent.children, name)
	}

	// remove from nodes map
	delete(r.nodes, path)

	return 0
}

// Mkdir creates a new directory
func (r *FSRoot) Mkdir(path string, mode uint32) int {
	path = strings.ReplaceAll(path, "\\", "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	dir := filepath.Dir(path)
	name := filepath.Base(path)

	r.mu.Lock()
	defer r.mu.Unlock()

	// check if parent directory exists
	parent, ok := r.nodes[dir]
	if !ok || !parent.isDir {
		return -fuse.ENOENT
	}

	// check if directory already exists
	if _, exists := r.nodes[path]; exists {
		return -fuse.EEXIST
	}

	// create new directory node
	dirNode := &FSNode{
		ino:      r.nextIno,
		path:     path,
		name:     name,
		isDir:    true,
		mode:     fuse.S_IFDIR | 0755,
		mtime:    time.Now(),
		children: make(map[string]*FSNode),
	}
	r.nextIno++

	r.nodes[path] = dirNode
	parent.children[name] = dirNode

	return 0
}

// Rmdir removes a directory
func (r *FSRoot) Rmdir(path string) int {
	path = strings.ReplaceAll(path, "\\", "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	if path == "/" {
		return -fuse.EACCES
	}

	dir := filepath.Dir(path)
	name := filepath.Base(path)

	r.mu.Lock()
	defer r.mu.Unlock()

	// check if directory exists
	node, ok := r.nodes[path]
	if !ok {
		return -fuse.ENOENT
	}
	if !node.isDir {
		return -fuse.ENOTDIR
	}

	// check if directory is empty
	if len(node.children) > 0 {
		return -fuse.ENOTEMPTY
	}

	// remove from parent
	if parent, ok := r.nodes[dir]; ok {
		delete(parent.children, name)
	}

	// remove from nodes map
	delete(r.nodes, path)

	return 0
}

// Utimens updates file timestamps
func (r *FSRoot) Utimens(path string, tmsp []fuse.Timespec) int {
	node := r.getNode(path)
	if node == nil {
		return -fuse.ENOENT
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(tmsp) > 1 {
		node.mtime = time.Unix(tmsp[1].Sec, int64(tmsp[1].Nsec))
	}

	return 0
}

func kindToExtension(kind nostr.Kind) string {
	switch kind {
	case 30023:
		return "md"
	case 30818:
		return "djot"
	default:
		return "txt"
	}
}
