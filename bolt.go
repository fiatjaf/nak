//go:build !nodb && (bolt || !linux || riscv64 || arm64)

package main

import (
	"os"
	"path/filepath"

	"fiatjaf.com/nostr/eventstore/boltdb"
	"fiatjaf.com/nostr/eventstore/nullstore"
	"fiatjaf.com/nostr/sdk"
	"fiatjaf.com/nostr/sdk/hints/bbolth"
	boltkv "fiatjaf.com/nostr/sdk/kvstore/bbolt"
	"github.com/urfave/cli/v3"
)

// setupLocalDatabases uses boltdb instead of lmdb for the local databases.
// it is used by the "nak-b" build (tag 'bolt') and also by the default build
// when lmdb is not an option (non-linux or arm/riscv).
// bbolt is pure go, so it works everywhere, but the databases are kept under a
// separate "bolt" directory so they don't collide with the lmdb ones.
func setupLocalDatabases(c *cli.Command, sys *sdk.System) {
	configPath := c.String("config-path")
	if configPath == "" {
		return
	}

	boltPath := filepath.Join(configPath, "bolt")

	hintsPath := filepath.Join(boltPath, "outbox", "hints.db")
	os.MkdirAll(filepath.Dir(hintsPath), 0755)
	if hdb, err := bbolth.NewBoltHints(hintsPath); err != nil {
		log("failed to create bolt hints db at '%s': %s\n", hintsPath, err)
	} else {
		sys.Hints = hdb
	}

	eventsPath := filepath.Join(boltPath, "events.db")
	os.MkdirAll(filepath.Dir(eventsPath), 0755)
	sys.Store = &boltdb.BoltBackend{Path: eventsPath}
	if err := sys.Store.Init(); err != nil {
		log("failed to create bolt events db at '%s': %s\n", eventsPath, err)
		sys.Store = &nullstore.NullStore{}
	}

	kvPath := filepath.Join(boltPath, "kvstore.db")
	os.MkdirAll(filepath.Dir(kvPath), 0755)
	if kv, err := boltkv.NewStore(kvPath); err != nil {
		log("failed to create bolt kvstore db at '%s': %s\n", kvPath, err)
	} else {
		sys.KVStore = kv
	}
}
