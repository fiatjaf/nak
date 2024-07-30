package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/fiatjaf/cli/v3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/nip46"
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
		&cli.StringSliceFlag{
			Name:    "authorized-secrets",
			Aliases: []string{"s"},
			Usage:   "secrets for which we will always respond",
		},
		&cli.StringSliceFlag{
			Name:    "authorized-keys",
			Aliases: []string{"k"},
			Usage:   "pubkeys for which we will always respond",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		// try to connect to the relays here
		qs := url.Values{}
		relayURLs := make([]string, 0, c.Args().Len())
		if relayUrls := c.Args().Slice(); len(relayUrls) > 0 {
			_, relays := connectToAllRelays(ctx, relayUrls, false)
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
		sec, _, err := gatherSecretKeyOrBunkerFromArguments(ctx, c)
		if err != nil {
			return err
		}

		// other arguments
		authorizedKeys := c.StringSlice("authorized-keys")
		authorizedSecrets := c.StringSlice("authorized-secrets")

		// this will be used to auto-authorize the next person who connects who isn't pre-authorized
		// it will be stored
		newSecret := randString(12)

		// static information
		pubkey, err := nostr.GetPublicKey(sec)
		if err != nil {
			return err
		}
		npub, _ := nip19.EncodePublicKey(pubkey)
		bold := color.New(color.Bold).Sprint
		italic := color.New(color.Italic).Sprint

		// this function will be called every now and then
		printBunkerInfo := func() {
			qs.Set("secret", newSecret)
			bunkerURI := fmt.Sprintf("bunker://%s?%s", pubkey, qs.Encode())

			authorizedKeysStr := ""
			if len(authorizedKeys) != 0 {
				authorizedKeysStr = "\n  authorized keys:\n    - " + italic(strings.Join(authorizedKeys, "\n    - "))
			}

			authorizedSecretsStr := ""
			if len(authorizedSecrets) != 0 {
				authorizedSecretsStr = "\n  authorized secrets:\n    - " + italic(strings.Join(authorizedSecrets, "\n    - "))
			}

			preauthorizedFlags := ""
			for _, k := range authorizedKeys {
				preauthorizedFlags += " -k " + k
			}
			for _, s := range authorizedSecrets {
				preauthorizedFlags += " -s " + s
			}

			secretKeyFlag := ""
			if sec := c.String("sec"); sec != "" {
				secretKeyFlag = "--sec " + sec
			}

			relayURLsPossiblyWithoutSchema := make([]string, len(relayURLs))
			for i, url := range relayURLs {
				if strings.HasPrefix(url, "wss://") {
					relayURLsPossiblyWithoutSchema[i] = url[6:]
				} else {
					relayURLsPossiblyWithoutSchema[i] = url
				}
			}

			restartCommand := fmt.Sprintf("nak bunker %s%s %s",
				secretKeyFlag,
				preauthorizedFlags,
				strings.Join(relayURLsPossiblyWithoutSchema, " "),
			)

			log("listening at %v:\n  pubkey: %s \n  npub: %s%s%s\n  to restart: %s\n  bunker: %s\n\n",
				bold(relayURLs),
				bold(pubkey),
				bold(npub),
				authorizedKeysStr,
				authorizedSecretsStr,
				color.CyanString(restartCommand),
				bold(bunkerURI),
			)
		}
		printBunkerInfo()

		// subscribe to relays
		pool := nostr.NewSimplePool(ctx)
		now := nostr.Now()
		events := pool.SubMany(ctx, relayURLs, nostr.Filters{
			{
				Kinds:     []int{nostr.KindNostrConnect},
				Tags:      nostr.TagMap{"p": []string{pubkey}},
				Since:     &now,
				LimitZero: true,
			},
		})

		signer := nip46.NewStaticKeySigner(sec)
		handlerWg := sync.WaitGroup{}
		printLock := sync.Mutex{}

		// just a gimmick
		var cancelPreviousBunkerInfoPrint context.CancelFunc
		_, cancel := context.WithCancel(ctx)
		cancelPreviousBunkerInfoPrint = cancel

		// asking user for authorization
		signer.AuthorizeRequest = func(harmless bool, from string, secret string) bool {
			if secret == newSecret {
				// store this key
				authorizedKeys = append(authorizedKeys, from)
				// discard this and generate a new secret
				newSecret = randString(12)
				// print bunker info again after this
				go func() {
					time.Sleep(3 * time.Second)
					printBunkerInfo()
				}()
			}

			return harmless || slices.Contains(authorizedKeys, from) || slices.Contains(authorizedSecrets, secret)
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
						err := relay.Publish(ctx, eventResponse)
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
				ctx, cancel := context.WithCancel(ctx)
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
