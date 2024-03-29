package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/nip46"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/slices"
)

var bunker = &cli.Command{
	Name:        "bunker",
	Usage:       "starts a NIP-46 signer daemon with the given --sec key",
	ArgsUsage:   "[relay...]",
	Description: ``,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "sec",
			Usage:       "secret key to sign the event, as hex or nsec",
			DefaultText: "the key '1'",
			Value:       "0000000000000000000000000000000000000000000000000000000000000001",
		},
		&cli.BoolFlag{
			Name:  "prompt-sec",
			Usage: "prompt the user to paste a hex or nsec with which to sign the event",
		},
		&cli.BoolFlag{
			Name:    "yes",
			Aliases: []string{"y"},
			Usage:   "always respond to any NIP-46 requests from anyone",
		},
	},
	Action: func(c *cli.Context) error {
		// try to connect to the relays here
		qs := url.Values{}
		relayURLs := make([]string, 0, c.Args().Len())
		if relayUrls := c.Args().Slice(); len(relayUrls) > 0 {
			_, relays := connectToAllRelays(c.Context, relayUrls)
			if len(relays) == 0 {
				log("failed to connect to any of the given relays.\n")
				os.Exit(3)
			}
			for _, relay := range relays {
				relayURLs = append(relayURLs, relay.URL)
				qs.Add("relay", relay.URL)
			}
		}
		if len(relayURLs) == 0 {
			return fmt.Errorf("not connected to any relays: please specify at least one")
		}

		// gather the secret key
		sec, _, err := gatherSecretKeyOrBunkerFromArguments(c)
		if err != nil {
			return err
		}
		pubkey, err := nostr.GetPublicKey(sec)
		if err != nil {
			return err
		}
		npub, _ := nip19.EncodePublicKey(pubkey)
		bunkerURI := fmt.Sprintf("bunker://%s?%s", pubkey, qs.Encode())
		bold := color.New(color.Bold).Sprint

		printBunkerInfo := func() {
			log("listening at %v:\n  pubkey: %s \n  npub: %s\n  bunker: %s\n\n",
				bold(relayURLs),
				bold(pubkey),
				bold(npub),
				bold(bunkerURI),
			)
		}
		printBunkerInfo()

		alwaysYes := c.Bool("yes")

		// subscribe to relays
		pool := nostr.NewSimplePool(c.Context)
		events := pool.SubMany(c.Context, relayURLs, nostr.Filters{
			{
				Kinds: []int{24133},
				Tags:  nostr.TagMap{"p": []string{pubkey}},
			},
		})

		signer := nip46.NewStaticKeySigner(sec)
		handlerWg := sync.WaitGroup{}
		printLock := sync.Mutex{}

		// just a gimmick
		var cancelPreviousBunkerInfoPrint context.CancelFunc
		_, cancel := context.WithCancel(c.Context)
		cancelPreviousBunkerInfoPrint = cancel

		// asking user for authorization
		signer.AuthorizeRequest = func(harmless bool, from string) bool {
			return alwaysYes || harmless || askProceed(from)
		}

		for ie := range events {
			cancelPreviousBunkerInfoPrint() // this prevents us from printing a million bunker info blocks

			// handle the NIP-46 request event
			req, resp, eventResponse, err := signer.HandleRequest(ie.Event)
			if err != nil {
				log("< failed to handle request from %s: %s\n", ie.Event.PubKey, err.Error())
				continue
			}

			jreq, _ := json.MarshalIndent(req, "  ", "  ")
			log("- got request from '%s': %s\n", color.New(color.Bold, color.FgBlue).Sprint(ie.Event.PubKey), string(jreq))
			jresp, _ := json.MarshalIndent(resp, "  ", "  ")
			log("~ responding with %s\n", string(jresp))

			handlerWg.Add(len(relayURLs))
			for _, relayURL := range relayURLs {
				go func(relayURL string) {
					if relay, _ := pool.EnsureRelay(relayURL); relay != nil {
						err := relay.Publish(c.Context, eventResponse)
						printLock.Lock()
						if err == nil {
							log("* sent response through %s\n", relay.URL)
						} else {
							log("* failed to send response: %s\n", err)
						}
						printLock.Unlock()
						handlerWg.Done()
					}
				}(relayURL)
			}
			handlerWg.Wait()

			// just after handling one request we trigger this
			go func() {
				ctx, cancel := context.WithCancel(c.Context)
				defer cancel()
				cancelPreviousBunkerInfoPrint = cancel
				// the idea is that we will print the bunker URL again so it is easier to copy-paste by users
				// but we will only do if the bunker is inactive for more than 5 minutes
				select {
				case <-ctx.Done():
				case <-time.After(time.Minute * 5):
					fmt.Fprintf(os.Stderr, "\n")
					printBunkerInfo()
				}
			}()
		}

		return nil
	},
}

var allowedSources = make([]string, 0, 2)

func askProceed(source string) bool {
	if slices.Contains(allowedSources, source) {
		return true
	}

	fmt.Fprintf(os.Stderr, "request from %s:\n", color.New(color.Bold, color.FgBlue).Sprint(source))
	res, err := ask("  proceed to fulfill this request? (yes/no/always from this) (y/n/a): ", "",
		func(answer string) bool {
			if answer != "y" && answer != "n" && answer != "a" {
				return true
			}
			return false
		})
	if err != nil {
		return false
	}
	switch res {
	case "n":
		return false
	case "y":
		return true
	case "a":
		allowedSources = append(allowedSources, source)
		return true
	}

	return false
}
