package main

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/eventstore/slicestore"
	"fiatjaf.com/nostr/khatru"
	"github.com/bep/debounce"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

var serve = &cli.Command{
	Name:                      "serve",
	Usage:                     "starts an in-memory relay for testing purposes",
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "hostname",
			Usage: "hostname where to listen for connections",
			Value: "localhost",
		},
		&cli.UintFlag{
			Name:  "port",
			Usage: "port where to listen for connections",
			Value: 10547,
		},
		&cli.StringFlag{
			Name:        "events",
			Usage:       "file containing the initial batch of events that will be served by the relay as newline-separated JSON (jsonl)",
			DefaultText: "the relay will start empty",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		db := &slicestore.SliceStore{MaxLimit: math.MaxInt}

		var scanner *bufio.Scanner
		if path := c.String("events"); path != "" {
			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to file at '%s': %w", path, err)
			}
			scanner = bufio.NewScanner(f)
		} else if isPiped() {
			scanner = bufio.NewScanner(os.Stdin)
		}

		if scanner != nil {
			scanner.Buffer(make([]byte, 16*1024*1024), 256*1024*1024)
			i := 0
			for scanner.Scan() {
				var evt nostr.Event
				if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
					return fmt.Errorf("invalid event received at line %d: %s (`%s`)", i, err, scanner.Text())
				}
				db.SaveEvent(evt)
				i++
			}
		}

		rl := khatru.NewRelay()

		rl.Info.Name = "nak serve"
		rl.Info.Description = "a local relay for testing, debugging and development."
		rl.Info.Software = "https://github.com/fiatjaf/nak"
		rl.Info.Version = version

		rl.UseEventstore(db)

		started := make(chan bool)
		exited := make(chan error)

		hostname := c.String("hostname")
		port := int(c.Uint("port"))

		go func() {
			err := rl.Start(hostname, port, started)
			exited <- err
		}()

		var printStatus func()

		// relay logging
		rl.OnRequest = func(ctx context.Context, filter nostr.Filter) (reject bool, msg string) {
			log("    got %s %v\n", color.HiYellowString("request"), colors.italic(filter))
			printStatus()
			return false, ""
		}

		rl.OnCount = func(ctx context.Context, filter nostr.Filter) (reject bool, msg string) {
			log("    got %s %v\n", color.HiCyanString("count request"), colors.italic(filter))
			printStatus()
			return false, ""
		}

		rl.OnEvent = func(ctx context.Context, event nostr.Event) (reject bool, msg string) {
			log("    got %s %v\n", color.BlueString("event"), colors.italic(event))
			printStatus()
			return false, ""
		}

		d := debounce.New(time.Second * 2)
		printStatus = func() {
			d(func() {
				totalEvents := 0
				for range db.QueryEvents(nostr.Filter{}) {
					totalEvents++
				}
				subs := rl.GetListeningFilters()

				log("  %s events stored: %s, subscriptions opened: %s\n", color.HiMagentaString("•"), color.HiMagentaString("%d", totalEvents), color.HiMagentaString("%d", len(subs)))
			})
		}

		<-started
		log("%s relay running at %s\n", color.HiRedString(">"), colors.boldf("ws://%s:%d", hostname, port))

		return <-exited
	},
}
