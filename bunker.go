package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"fiatjaf.com/nostr/nip44"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nip46"
	"github.com/fatih/color"
	"github.com/mdp/qrterminal/v3"
	"github.com/urfave/cli/v3"
)

const PERSISTENCE = "PERSISTENCE"

var bunker = &cli.Command{
	Name:                      "bunker",
	Usage:                     "starts a nip46 signer daemon with the given --sec key",
	ArgsUsage:                 "[relay...]",
	Description:               ``,
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:     "persist",
			Usage:    "whether to read and store authorized keys from and to a config file",
			Category: PERSISTENCE,
		},
		&cli.StringFlag{
			Name:     "profile",
			Value:    "default",
			Usage:    "config file name to use for --persist mode (implies that if provided) -- based on --config-path, i.e. ~/.config/nak/",
			OnlyOnce: true,
			Category: PERSISTENCE,
		},
		&cli.StringFlag{
			Name:        "sec",
			Usage:       "secret key to sign the event, as hex or nsec",
			DefaultText: "the key '1'",
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
		&PubKeySliceFlag{
			Name:    "authorized-keys",
			Aliases: []string{"k"},
			Usage:   "pubkeys for which we will always respond",
		},
		&cli.StringSliceFlag{
			Name:   "relay",
			Usage:  "relays to connect to (can also be provided as naked arguments)",
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:  "qrcode",
			Usage: "display a QR code for the bunker URI",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		// read config from file
		config := struct {
			AuthorizedKeys []nostr.PubKey      `json:"authorized-keys"`
			Secret         plainOrEncryptedKey `json:"sec"`
			Relays         []string            `json:"relays"`
		}{
			AuthorizedKeys: make([]nostr.PubKey, 0, 3),
		}
		baseRelaysUrls := appendUnique(c.Args().Slice(), c.StringSlice("relay")...)
		for i, url := range baseRelaysUrls {
			baseRelaysUrls[i] = nostr.NormalizeURL(url)
		}
		baseAuthorizedKeys := getPubKeySlice(c, "authorized-keys")

		var baseSecret plainOrEncryptedKey
		{
			sec := c.String("sec")
			if c.Bool("prompt-sec") {
				var err error
				sec, err = askPassword("type your secret key as ncryptsec, nsec or hex: ", nil)
				if err != nil {
					return fmt.Errorf("failed to get secret key: %w", err)
				}
			}
			if strings.HasPrefix(sec, "ncryptsec1") {
				baseSecret.Encrypted = &sec
			} else if sec != "" {
				if prefix, ski, err := nip19.Decode(sec); err == nil && prefix == "nsec" {
					sk := ski.(nostr.SecretKey)
					baseSecret.Plain = &sk
				} else if sk, err := nostr.SecretKeyFromHex(sec); err != nil {
					return fmt.Errorf("invalid secret key: %w", err)
				} else {
					baseSecret.Plain = &sk
				}
			}
		}

		// default case: persist() is nil
		var persist func()

		if c.Bool("persist") || c.IsSet("profile") {
			path := filepath.Join(c.String("config-path"), "bunker")
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
			path = filepath.Join(path, c.String("profile"))

			persist = func() {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					log(color.RedString("failed to persist: %w\n"), err)
					os.Exit(4)
				}
				data, err := json.MarshalIndent(config, "", "  ")
				if err != nil {
					log(color.RedString("failed to persist: %w\n"), err)
					os.Exit(4)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					log(color.RedString("failed to persist: %w\n"), err)
					os.Exit(4)
				}
			}

			log(color.YellowString("reading config from %s\n"), path)
			b, err := os.ReadFile(path)
			if err == nil {
				if err := json.Unmarshal(b, &config); err != nil {
					return err
				}
			} else if !os.IsNotExist(err) {
				return err
			}

			for i, url := range config.Relays {
				config.Relays[i] = nostr.NormalizeURL(url)
			}
			config.Relays = appendUnique(config.Relays, baseRelaysUrls...)
			config.AuthorizedKeys = appendUnique(config.AuthorizedKeys, baseAuthorizedKeys...)

			if config.Secret.Plain == nil && config.Secret.Encrypted == nil {
				// we don't have any secret key stored, so just use whatever was given via flags
				config.Secret = baseSecret
			} else if baseSecret.Plain == nil && baseSecret.Encrypted == nil {
				// we didn't provide any keys, so we just use the stored
			} else {
				// we have a secret key stored
				// if we also provided a key we check if they match and fail otherwise
				if !baseSecret.equals(config.Secret) {
					return fmt.Errorf("--sec provided conflicts with stored, you should create a new --profile or omit the --sec flag")
				}
			}
		} else {
			config.Secret = baseSecret
			config.Relays = baseRelaysUrls
			config.AuthorizedKeys = baseAuthorizedKeys
		}

		// if we got here without any keys set (no flags, first time using a profile), use the default
		if config.Secret.Plain == nil && config.Secret.Encrypted == nil {
			sec := os.Getenv("NOSTR_SECRET_KEY")
			if sec == "" {
				sec = defaultKey
			}
			sk, err := nostr.SecretKeyFromHex(sec)
			if err != nil {
				return fmt.Errorf("default key is wrong: %w", err)
			}
			config.Secret.Plain = &sk
		}

		if len(config.Relays) == 0 {
			return fmt.Errorf("no relays given")
		}

		// decrypt key here if necessary
		var sec nostr.SecretKey
		if config.Secret.Plain != nil {
			sec = *config.Secret.Plain
		} else {
			plain, err := promptDecrypt(*config.Secret.Encrypted)
			if err != nil {
				return fmt.Errorf("failed to decrypt: %w", err)
			}
			sec = plain
		}

		if persist != nil {
			persist()
		}

		// try to connect to the relays here
		qs := url.Values{}
		relayURLs := make([]string, 0, len(config.Relays))
		relays := connectToAllRelays(ctx, c, config.Relays, nil, nostr.PoolOptions{})
		if len(relays) == 0 {
			log("failed to connect to any of the given relays.\n")
			os.Exit(3)
		}
		for _, relay := range relays {
			relayURLs = append(relayURLs, relay.URL)
			qs.Add("relay", relay.URL)
		}
		if len(relayURLs) == 0 {
			return fmt.Errorf("not connected to any relays: please specify at least one")
		}

		// other arguments
		authorizedSecrets := c.StringSlice("authorized-secrets")

		// this will be used to auto-authorize the next person who connects who isn't pre-authorized
		// it will be stored
		newSecret := randString(12)

		// static information
		pubkey := sec.Public()
		npub := nip19.EncodeNpub(pubkey)
		signer := nip46.NewStaticKeySigner(sec)
		printLock := sync.Mutex{}
		exitChan := make(chan bool, 1)

		// subscription management
		var events chan nostr.RelayEvent
		var cancelSubscription context.CancelFunc
		subscriptionMutex := sync.Mutex{}

		// Function to create/recreate subscription
		updateSubscription := func() {
			subscriptionMutex.Lock()
			defer subscriptionMutex.Unlock()

			// Cancel existing subscription if it exists
			if cancelSubscription != nil {
				cancelSubscription()
			}

			// Create new context for the subscription
			subCtx, cancel := context.WithCancel(ctx)
			cancelSubscription = cancel

			// Create new subscription with current relay list
			events = sys.Pool.SubscribeMany(subCtx, relayURLs, nostr.Filter{
				Kinds:     []nostr.Kind{nostr.KindNostrConnect},
				Tags:      nostr.TagMap{"p": []string{pubkey.Hex()}},
				Since:     nostr.Now(),
				LimitZero: true,
			}, nostr.SubscriptionOptions{
				Label: "nak-bunker",
			})
		}

		// Initial subscription to relays
		updateSubscription()
		handlerWg := sync.WaitGroup{}

		// == SUBCOMMANDS ==

		// printHelp displays available commands for the bunker interface
		printHelp := func() {
			log("%s\n", color.CyanString("Available Commands:"))
			log("  %s - Show this help message\n", color.GreenString("help, h, ?"))
			log("  %s - Display current bunker information\n", color.GreenString("info, i"))
			log("  %s - Generate and display QR code for the bunker URI\n", color.GreenString("qr"))
			log("  %s - Connect to a remote client using nostrconnect:// URI\n", color.GreenString("connect, c <nostrconnect://uri>"))
			log("  %s - Shutdown the bunker\n", color.GreenString("exit, quit, q"))
			log("\n")
		}

		// Create a function to print QR code on demand
		printQR := func() {
			qs.Set("secret", newSecret)
			bunkerURI := fmt.Sprintf("bunker://%s?%s", pubkey.Hex(), qs.Encode())
			printLock.Lock()
			log("\nQR Code for bunker URI:\n")
			qrterminal.Generate(bunkerURI, qrterminal.L, os.Stdout)
			log("\n\n")
			printLock.Unlock()
		}

		// this function will be called every now and then
		printBunkerInfo := func() {
			qs.Set("secret", newSecret)
			bunkerURI := fmt.Sprintf("bunker://%s?%s", pubkey.Hex(), qs.Encode())

			authorizedKeysStr := ""
			if len(config.AuthorizedKeys) != 0 {
				authorizedKeysStr = "\n  authorized keys:"
				for _, pubkey := range config.AuthorizedKeys {
					authorizedKeysStr += "\n    - " + colors.italic(pubkey.Hex())
				}
			}

			authorizedSecretsStr := ""
			if len(authorizedSecrets) != 0 {
				authorizedSecretsStr = "\n  authorized secrets:\n    - " + colors.italic(strings.Join(authorizedSecrets, "\n    - "))
			}

			preauthorizedFlags := ""
			for _, k := range config.AuthorizedKeys {
				preauthorizedFlags += " -k " + k.Hex()
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

			// only print the restart command if not persisting:
			if persist == nil {
				restartCommand := fmt.Sprintf("nak bunker %s%s %s",
					secretKeyFlag,
					preauthorizedFlags,
					strings.Join(relayURLsPossiblyWithoutSchema, " "),
				)

				log("listening at %v:\n  pubkey: %s \n  npub: %s%s%s\n  to restart: %s\n  bunker: %s\n\n",
					colors.bold(relayURLs),
					colors.bold(pubkey.Hex()),
					colors.bold(npub),
					authorizedKeysStr,
					authorizedSecretsStr,
					color.CyanString(restartCommand),
					colors.bold(bunkerURI),
				)
			} else {
				// otherwise just print the data
				log("listening at %v:\n  pubkey: %s \n  npub: %s%s%s\n  bunker: %s\n\n",
					colors.bold(relayURLs),
					colors.bold(pubkey.Hex()),
					colors.bold(npub),
					authorizedKeysStr,
					authorizedSecretsStr,
					colors.bold(bunkerURI),
				)
			}

			// print QR code if requested
			if c.Bool("qrcode") {
				printQR()
			}
		}

		// handleConnect processes nostrconnect:// URIs for direct connection flow
		handleConnect := func(connectURI string) {
			if !strings.HasPrefix(connectURI, "nostrconnect://") {
				log("Error: URI must start with nostrconnect://\n")
				return
			}

			// Parse the nostrconnect URI
			u, err := url.Parse(connectURI)
			if err != nil {
				log("Error: Invalid nostrconnect URI: %v\n", err)
				return
			}

			// Extract client pubkey from the URI
			clientPubkeyHex := u.Host
			if clientPubkeyHex == "" {
				log("Error: Missing client pubkey in URI\n")
				return
			}

			clientPubkey, err := nostr.PubKeyFromHex(clientPubkeyHex)
			if err != nil {
				log("Error: Invalid client pubkey: %v\n", err)
				return
			}

			// Extract relays from query parameters
			queryParams := u.Query()
			newRelayURLs := queryParams["relay"]
			if len(newRelayURLs) == 0 {
				log("Error: No relays specified in URI\n")
				return
			}

			// Extract secret from query parameters (optional)
			var secret string
			secrets := queryParams["secret"]
			if len(secrets) > 0 {
				secret = secrets[0]
			} else {
				// log error and return if no secret is provided
				log("Error: No secret provided in URI\n")
				return
			}

			log("Parsed nostrconnect URI:\n")
			log("  Client pubkey: %s\n", color.CyanString(clientPubkey.Hex()))
			log("  Relays: %v\n", newRelayURLs)
			log("  Secret: %s\n", color.YellowString(secret))

			// Normalize relay URLs
			for i, relayURL := range newRelayURLs {
				newRelayURLs[i] = nostr.NormalizeURL(relayURL)
			}

			// Update relays in config (add new ones)
			relaysAdded := false
			for _, relayURL := range newRelayURLs {
				if !slices.Contains(config.Relays, relayURL) {
					config.Relays = append(config.Relays, relayURL)
					log("Added new relay: %s\n", color.MagentaString(relayURL))
					relaysAdded = true
				}
			}

			// Add client key to authorized keys
			keyAdded := false
			if !slices.Contains(config.AuthorizedKeys, clientPubkey) {
				config.AuthorizedKeys = append(config.AuthorizedKeys, clientPubkey)
				log("Added client key to authorized keys: %s\n", color.GreenString(clientPubkey.Hex()))
				keyAdded = true
			}

			// Persist config if needed and changes were made
			if persist != nil && (relaysAdded || keyAdded) {
				persist()
				log(color.GreenString("Configuration saved\n"))
			}

			// Connect to new relays if any were added
			if relaysAdded {
				newRelays := connectToAllRelays(ctx, c, newRelayURLs, nil, nostr.PoolOptions{})
				log("Connected to %d new relay(s)\n", len(newRelays))

				// Update the relay URLs list with successfully connected relays
				for _, relay := range newRelays {
					if !slices.Contains(relayURLs, relay.URL) {
						relayURLs = append(relayURLs, relay.URL)
					}
				}
				log("Updated subscription to listen on %d relay(s)\n", len(relayURLs))

				// Cancel and recreate subscription with updated relay list
				updateSubscription()
				log("Subscription updated to include new relays\n")
			}

			// Prepare the response payload
			responsePayload := nip46.Response{
				ID:     secret,
				Result: "ack",
			}

			// Marshal the response payload to JSON
			responseJSON, err := json.MarshalIndent(responsePayload, "", "  ")
			if err != nil {
				log("Error: Failed to marshal response payload: %v\n", err)
				return
			}

			log("Sending connect response with:\n%s\n", string(responseJSON))

			// Encrypt the response using NIP-44
			conversationKey, err := nip44.GenerateConversationKey(clientPubkey, sec)
			if err != nil {
				log("Error: Failed to generate conversation key: %v\n", err)
				return
			}
			encryptedContent, err := nip44.Encrypt(string(responseJSON), conversationKey)
			if err != nil {
				log("Error: Failed to encrypt response content: %v\n", err)
				return
			}

			// Create the kind 24133 event
			eventResponse := nostr.Event{
				Kind:      nostr.KindNostrConnect,
				PubKey:    pubkey,
				Content:   encryptedContent,
				Tags:      nostr.Tags{{"p", clientPubkey.Hex()}},
				CreatedAt: nostr.Now(),
			}

			// Sign the event with the signer
			if err := eventResponse.Sign(sec); err != nil {
				log("Error: Failed to sign connect response: %v\n", err)
				return
			}

			targetRelays := newRelayURLs
			if len(targetRelays) == 0 {
				targetRelays = relayURLs
			}

			log("Sending connect response...\n")
			successCount := atomic.Uint32{}
			handlerWg.Add(len(targetRelays))
			for _, relayURL := range targetRelays {
				go func(relayURL string) {
					if relay, _ := sys.Pool.EnsureRelay(relayURL); relay != nil {
						err := relay.Publish(ctx, eventResponse)
						printLock.Lock()
						if err != nil {
							log("Failed to publish to %s: %v\n", relayURL, err)
						} else {
							log("Published connect response to %s\n", color.GreenString(relayURL))
							successCount.Add(1)
						}
						printLock.Unlock()
						handlerWg.Done()
					}
				}(relayURL)
			}
			handlerWg.Wait()

			if successCount.Load() == 0 {
				log("Error: Failed to publish connect response to any relay\n")
			} else {
				log(color.GreenString("\nConnect response sent successfully to %d relay(s)!\n"), successCount.Load())
			}

			printBunkerInfo()
		}

		// handleBunkerCommand processes user commands in the bunker interface
		handleBunkerCommand := func(command string) {
			parts := strings.Fields(command)
			if len(parts) == 0 {
				return
			}

			switch strings.ToLower(parts[0]) {
			case "help", "h", "?":
				printHelp()
			case "info", "i":
				printBunkerInfo()
			case "qr":
				printQR()
			case "connect", "c":
				if len(parts) < 2 {
					log("Usage: connect <nostrconnect://uri>\n")
					return
				}
				handleConnect(parts[1])
			case "exit", "quit", "q":
				log("Exit command received.\n")
				exitChan <- true
			case "":
				// Ignore empty commands
			default:
				log("Unknown command: %s. Type 'help' for available commands.\n", command)
			}
		}

		// == END OF SUBCOMMANDS ==

		// Print initial bunker information
		printBunkerInfo()

		// just a gimmick
		var cancelPreviousBunkerInfoPrint context.CancelFunc
		_, cancel := context.WithCancel(ctx)
		cancelPreviousBunkerInfoPrint = cancel

		// asking user for authorization
		signer.AuthorizeRequest = func(harmless bool, from nostr.PubKey, secret string) bool {
			if secret == newSecret {
				// store this key
				config.AuthorizedKeys = appendUnique(config.AuthorizedKeys, from)
				// discard this and generate a new secret
				newSecret = randString(12)
				// print bunker info again after this
				go func() {
					time.Sleep(3 * time.Second)
					printBunkerInfo()
				}()

				if persist != nil {
					persist()
				}
			}

			return slices.Contains(config.AuthorizedKeys, from) || slices.Contains(authorizedSecrets, secret)
		}

		// Start command input handler in a separate goroutine
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				command := strings.TrimSpace(scanner.Text())
				handleBunkerCommand(command)
			}
			if err := scanner.Err(); err != nil {
				log("error reading command: %v\n", err)
			}
		}()

		// Print initial command help
		log("%s\nType 'help' for available commands or 'exit' to quit.\n%s\n",
			color.CyanString("--------------- Bunker Command Interface ---------------"),
			color.CyanString("--------------------------------------------------------"))

		for {
			// Check if exit was requested first
			select {
			case <-exitChan:
				log("Shutting down bunker...\n")
				return nil
			case ie := <-events:
				cancelPreviousBunkerInfoPrint() // this prevents us from printing a million bunker info blocks

				// handle the NIP-46 request event
				req, resp, eventResponse, err := signer.HandleRequest(ctx, ie.Event)
				if err != nil {
					log("< failed to handle request from %s: %s\n", ie.Event.PubKey, err.Error())
					continue
				}

				jreq, _ := json.MarshalIndent(req, "", "  ")
				log("- got request from '%s': %s\n", color.New(color.Bold, color.FgBlue).Sprint(ie.Event.PubKey.Hex()), string(jreq))
				jresp, _ := json.MarshalIndent(resp, "", "  ")
				log("~ responding with %s\n", string(jresp))

				handlerWg.Add(len(relayURLs))
				for _, relayURL := range relayURLs {
					go func(relayURL string) {
						if relay, _ := sys.Pool.EnsureRelay(relayURL); relay != nil {
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
						log("\n")
						printBunkerInfo()
					}
				}()
			case <-time.After(100 * time.Millisecond):
				// Continue to check for exit signal even when no events
				continue
			}
		}
	},
}

type plainOrEncryptedKey struct {
	Plain     *nostr.SecretKey
	Encrypted *string
}

func (pe plainOrEncryptedKey) MarshalJSON() ([]byte, error) {
	if pe.Plain != nil {
		res := make([]byte, 66)
		hex.Encode(res[1:], (*pe.Plain)[:])
		res[0] = '"'
		res[65] = '"'
		return res, nil
	} else if pe.Encrypted != nil {
		return json.Marshal(*pe.Encrypted)
	}

	return nil, fmt.Errorf("no key to marshal")
}

func (pe *plainOrEncryptedKey) UnmarshalJSON(buf []byte) error {
	if len(buf) == 66 {
		sk, err := nostr.SecretKeyFromHex(string(buf[1 : 1+64]))
		if err != nil {
			return err
		}
		pe.Plain = &sk
		return nil
	} else if bytes.HasPrefix(buf, []byte("\"nsec")) {
		_, v, err := nip19.Decode(string(buf[1 : len(buf)-1]))
		if err != nil {
			return err
		}
		sk := v.(nostr.SecretKey)
		pe.Plain = &sk
		return nil
	} else if bytes.HasPrefix(buf, []byte("\"ncryptsec1")) {
		ncryptsec := string(buf[1 : len(buf)-1])
		pe.Encrypted = &ncryptsec
		return nil
	}

	return fmt.Errorf("unrecognized key format '%s'", string(buf))
}

func (a plainOrEncryptedKey) equals(b plainOrEncryptedKey) bool {
	if a.Plain == nil && b.Plain != nil {
		return false
	}
	if a.Plain != nil && b.Plain == nil {
		return false
	}
	if a.Plain != nil && b.Plain != nil && *a.Plain != *b.Plain {
		return false
	}

	if a.Encrypted == nil && b.Encrypted != nil {
		return false
	}
	if a.Encrypted != nil && b.Encrypted == nil {
		return false
	}
	if a.Encrypted != nil && b.Encrypted != nil && *a.Encrypted != *b.Encrypted {
		return false
	}

	return true
}
