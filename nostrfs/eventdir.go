package nostrfs

import (
	"context"
	"fmt"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/mailru/easyjson"
	"github.com/nbd-wtf/go-nostr"
	sdk "github.com/nbd-wtf/go-nostr/sdk"
)

type EventDir struct {
	fs.Inode
	ctx context.Context
	evt *nostr.Event
}

func FetchAndCreateEventDir(
	ctx context.Context,
	parent fs.InodeEmbedder,
	sys *sdk.System,
	pointer nostr.EventPointer,
) (*fs.Inode, error) {
	event, _, err := sys.FetchSpecificEvent(ctx, pointer, sdk.FetchSpecificEventParameters{
		WithRelays: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch: %w", err)
	}

	return CreateEventDir(ctx, parent, event), nil
}

func CreateEventDir(
	ctx context.Context,
	parent fs.InodeEmbedder,
	event *nostr.Event,
) *fs.Inode {
	h := parent.EmbeddedInode().NewPersistentInode(
		ctx,
		&EventDir{ctx: ctx, evt: event},
		fs.StableAttr{Mode: syscall.S_IFDIR},
	)

	eventj, _ := easyjson.Marshal(event)
	h.AddChild("event.json", h.NewPersistentInode(
		ctx,
		&fs.MemRegularFile{
			Data: eventj,
			Attr: fuse.Attr{Mode: 0444},
		},
		fs.StableAttr{},
	), true)

	h.AddChild("content.txt", h.NewPersistentInode(
		ctx,
		&fs.MemRegularFile{
			Data: []byte(event.Content),
			Attr: fuse.Attr{Mode: 0444},
		},
		fs.StableAttr{},
	), true)

	return h
}
