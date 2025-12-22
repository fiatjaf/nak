//go:build linux && !riscv64 && !arm64

package main

import (
	"os"
	"path/filepath"

	"fiatjaf.com/nostr/eventstore/lmdb"
	"fiatjaf.com/nostr/eventstore/nullstore"
	"fiatjaf.com/nostr/sdk"
	"fiatjaf.com/nostr/sdk/hints/lmdbh"
	lmdbkv "fiatjaf.com/nostr/sdk/kvstore/lmdb"
	"github.com/urfave/cli/v3"
)

func setupLocalDatabases(c *cli.Command, sys *sdk.System) {
	configPath := c.String("config-path")
	if configPath != "" {
		hintsPath := filepath.Join(configPath, "outbox/hints")
		os.MkdirAll(hintsPath, 0755)
		_, err := lmdbh.NewLMDBHints(hintsPath)
		if err != nil {
			log("failed to create lmdb hints db at '%s': %s\n", hintsPath, err)
		}

		eventsPath := filepath.Join(configPath, "events")
		os.MkdirAll(eventsPath, 0755)
		sys.Store = &lmdb.LMDBBackend{Path: eventsPath}
		if err := sys.Store.Init(); err != nil {
			log("failed to create boltdb events db at '%s': %s\n", eventsPath, err)
			sys.Store = &nullstore.NullStore{}
		}

		kvPath := filepath.Join(configPath, "kvstore")
		os.MkdirAll(kvPath, 0755)
		if kv, err := lmdbkv.NewStore(kvPath); err != nil {
			log("failed to create boltdb kvstore db at '%s': %s\n", kvPath, err)
		} else {
			sys.KVStore = kv
		}
	}
}
