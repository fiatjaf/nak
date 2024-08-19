package main

import (
	"context"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/fiatjaf/cli/v3"
	"github.com/fiatjaf/eventstore/slicestore"
	"github.com/fiatjaf/khatru"
)

var serve = &cli.Command{
	Name:                      "serve",
	Usage:                     "starts an in-memory relay for testing purposes",
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.UintFlag{
			Name:  "port",
			Usage: "port where to listen for connections",
		},
		&cli.StringFlag{
			Name:        "events",
			Usage:       "file containing the initial batch of events that will be served by the relay as newline-separated JSON (jsonl)",
			DefaultText: "the relay will start empty",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		db := slicestore.SliceStore{}

        reader :=
		if path := c.String("events"); path != "" {
			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to file at '%s': %w", path, err)
			}
		} else if isPiped() {
		}

		rl := khatru.NewRelay()
		rl.QueryEvents = append(rl.QueryEvents, db.QueryEvents)
		rl.CountEvents = append(rl.CountEvents, db.CountEvents)
		rl.DeleteEvent = append(rl.DeleteEvent, db.DeleteEvent)
		rl.StoreEvent = append(rl.StoreEvent, db.SaveEvent)

		started := make(chan bool)
		exited := make(chan bool)

		go func() {
			err := rl.Start("127.0.0.1", int(c.Uint("port")), started)
			if err != nil {
				log("error: %s", err)
			}
			exited <- true
		}()

		bold := color.New(color.Bold).Sprint
		italic := color.New(color.Italic).Sprint

		<-started
		log()
	},
}
