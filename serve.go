package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/eventstore/slicestore"
	"fiatjaf.com/nostr/khatru"
	"fiatjaf.com/nostr/khatru/blossom"
	"fiatjaf.com/nostr/khatru/grasp"
	"github.com/bep/debounce"
	"github.com/fatih/color"
	"github.com/puzpuzpuz/xsync/v3"
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
		&cli.BoolFlag{
			Name:  "negentropy",
			Usage: "enable negentropy syncing",
		},
		&cli.BoolFlag{
			Name:  "grasp",
			Usage: "enable grasp server",
		},
		&cli.StringFlag{
			Name:      "grasp-path",
			Usage:     "where to store the repositories",
			TakesFile: true,
			Hidden:    true,
		},
		&cli.BoolFlag{
			Name:  "blossom",
			Usage: "enable blossom server",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		db := &slicestore.SliceStore{}

		var blobStore *xsync.MapOf[string, []byte]
		var repoDir string

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

		rl.UseEventstore(db, 500)

		if c.Bool("negentropy") {
			rl.Negentropy = true
		}

		started := make(chan bool)
		exited := make(chan error)

		hostname := c.String("hostname")
		port := int(c.Uint("port"))

		var printStatus func()

		if c.Bool("blossom") {
			bs := blossom.New(rl, fmt.Sprintf("http://%s:%d", hostname, port))
			bs.Store = blossom.NewMemoryBlobIndex()

			blobStore = xsync.NewMapOf[string, []byte]()
			bs.StoreBlob = func(ctx context.Context, sha256 string, ext string, body []byte) error {
				blobStore.Store(sha256+ext, body)
				log("    got %s %s\n", color.GreenString("blob stored"), sha256+ext)
				printStatus()
				return nil
			}
			bs.LoadBlob = func(ctx context.Context, sha256 string, ext string) (io.ReadSeeker, *url.URL, error) {
				if body, ok := blobStore.Load(sha256 + ext); ok {
					log("    got %s %s\n", color.BlueString("blob downloaded"), sha256+ext)
					printStatus()
					return bytes.NewReader(body), nil, nil
				}
				return nil, nil, nil
			}
			bs.DeleteBlob = func(ctx context.Context, sha256 string, ext string) error {
				blobStore.Delete(sha256 + ext)
				log("    got %s %s\n", color.RedString("blob deleted"), sha256+ext)
				printStatus()
				return nil
			}
		}

		if c.Bool("grasp") {
			repoDir = c.String("grasp-path")
			if repoDir == "" {
				var err error
				repoDir, err = os.MkdirTemp("", "nak-serve-grasp-repos-")
				if err != nil {
					return fmt.Errorf("failed to create grasp repos directory: %w", err)
				}
			}
			g := grasp.New(rl, repoDir)
			g.OnRead = func(ctx context.Context, pubkey nostr.PubKey, repo string) (reject bool, reason string) {
				log("    got %s %s %s\n", color.CyanString("git read"), pubkey.Hex(), repo)
				printStatus()
				return false, ""
			}
			g.OnWrite = func(ctx context.Context, pubkey nostr.PubKey, repo string) (reject bool, reason string) {
				log("    got %s %s %s\n", color.YellowString("git write"), pubkey.Hex(), repo)
				printStatus()
				return false, ""
			}
		}

		go func() {
			err := rl.Start(hostname, port, started)
			exited <- err
		}()

		// relay logging
		rl.OnRequest = func(ctx context.Context, filter nostr.Filter) (reject bool, msg string) {
			negentropy := ""
			if khatru.IsNegentropySession(ctx) {
				negentropy = color.HiBlueString("negentropy ")
			}

			log("    got %s%s %v\n", negentropy, color.HiYellowString("request"), colors.italic(filter))
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

		totalConnections := atomic.Int32{}
		rl.OnConnect = func(ctx context.Context) {
			totalConnections.Add(1)
			go func() {
				<-ctx.Done()
				totalConnections.Add(-1)
			}()
		}

		d := debounce.New(time.Second * 2)
		printStatus = func() {
			d(func() {
				totalEvents, err := db.CountEvents(nostr.Filter{})
				if err != nil {
					log("failed to count: %s\n", err)
				}
				subs := rl.GetListeningFilters()

				blossomMsg := ""
				if c.Bool("blossom") {
					blobsStored := blobStore.Size()
					blossomMsg = fmt.Sprintf("blobs: %s, ",
						color.HiMagentaString("%d", blobsStored),
					)
				}

				graspMsg := ""
				if c.Bool("grasp") {
					gitAnnounced := 0
					gitStored := 0
					for evt := range db.QueryEvents(nostr.Filter{Kinds: []nostr.Kind{nostr.Kind(30617)}}, 500) {
						gitAnnounced++
						identifier := evt.Tags.GetD()
						if info, err := os.Stat(filepath.Join(repoDir, identifier)); err == nil && info.IsDir() {
							gitStored++
						}
					}
					graspMsg = fmt.Sprintf("git announced: %s, git stored: %s, ",
						color.HiMagentaString("%d", gitAnnounced),
						color.HiMagentaString("%d", gitStored),
					)
				}

				log("  %s events: %s, %s%sconnections: %s, subscriptions: %s\n",
					color.HiMagentaString("•"),
					color.HiMagentaString("%d", totalEvents),
					blossomMsg,
					graspMsg,
					color.HiMagentaString("%d", totalConnections.Load()),
					color.HiMagentaString("%d", len(subs)),
				)
			})
		}

		<-started
		log("%s relay running at %s", color.HiRedString(">"), colors.boldf("ws://%s:%d", hostname, port))
		if c.Bool("grasp") {
			log(" (grasp repos at %s)", repoDir)
		}
		log("\n")

		return <-exited
	},
}
