package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
		config := BunkerConfig{}
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
				// convert from deprecated field
				if len(config.AuthorizedKeys) > 0 {
					config.Clients = make([]BunkerConfigClient, len(config.AuthorizedKeys))
					for i := range config.AuthorizedKeys {
						config.Clients[i] = BunkerConfigClient{PubKey: config.AuthorizedKeys[i]}
					}
					config.AuthorizedKeys = nil
					persist()
				}
			} else if !os.IsNotExist(err) {
				return err
			}

			for i, url := range config.Relays {
				config.Relays[i] = nostr.NormalizeURL(url)
			}
			config.Relays = appendUnique(config.Relays, baseRelaysUrls...)
			for _, bak := range baseAuthorizedKeys {
				if !slices.ContainsFunc(config.Clients, func(c BunkerConfigClient) bool { return c.PubKey == bak }) {
					config.Clients = append(config.Clients, BunkerConfigClient{PubKey: bak})
				}
			}

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
			for _, bak := range baseAuthorizedKeys {
				config.Clients = append(config.Clients, BunkerConfigClient{PubKey: bak})
			}
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
		allRelays := make([]string, len(config.Relays), len(config.Relays)+5)
		copy(allRelays, config.Relays)
		for _, c := range config.Clients {
			for _, url := range c.CustomRelays {
				if !slices.ContainsFunc(allRelays, func(u string) bool { return u == url }) {
					allRelays = append(allRelays, url)
				}
			}
		}
		relayURLs := make([]string, 0, len(allRelays))
		relays := connectToAllRelays(ctx, c, allRelays, nil, nostr.PoolOptions{})
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

		// printQR generates and prints the QR code for the bunker URI
		printQR := func() {
			qs.Set("secret", newSecret)
			bunkerURI := fmt.Sprintf("bunker://%s?%s", pubkey.Hex(), qs.Encode())
			log("\nQR Code for bunker URI:\n")
			qrterminal.Generate(bunkerURI, qrterminal.L, os.Stdout)
			log("\n\n")
		}

		// this function will be called every now and then
		printBunkerInfo := func() {
			qs.Set("secret", newSecret)
			bunkerURI := fmt.Sprintf("bunker://%s?%s", pubkey.Hex(), qs.Encode())

			authorizedKeysStr := ""
			if len(config.Clients) != 0 {
				authorizedKeysStr = "\n  authorized clients:"
				for _, c := range config.Clients {
					authorizedKeysStr += "\n    - " + colors.italic(c.PubKey.Hex())
					name := ""
					if c.Name != "" {
						name = c.Name
						if c.URL != "" {
							name += " " + colors.underline(c.URL)
						}
					} else if c.URL != "" {
						name = colors.underline(c.URL)
					}
					if name != "" {
						authorizedKeysStr += " (" + name + ")"
					}
				}
			}

			authorizedSecretsStr := ""
			if len(authorizedSecrets) != 0 {
				authorizedSecretsStr = "\n  authorized secrets:\n    - " + colors.italic(strings.Join(authorizedSecrets, "\n    - "))
			}

			preauthorizedFlags := ""
			for _, c := range config.Clients {
				preauthorizedFlags += " -k " + c.PubKey.Hex()
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
		printBunkerInfo()

		// subscribe to relays
		events := sys.Pool.SubscribeMany(ctx, relayURLs, nostr.Filter{
			Kinds:     []nostr.Kind{nostr.KindNostrConnect},
			Tags:      nostr.TagMap{"p": []string{pubkey.Hex()}},
			Since:     nostr.Now(),
			LimitZero: true,
		}, nostr.SubscriptionOptions{Label: "nak-bunker"})

		signer := nip46.NewStaticKeySigner(sec)
		signer.DefaultRelays = config.Relays

		// common help to handle nostrconnect:// URIs
		handleNostrConnect := func(uri *url.URL) {
			clientPublicKey, err := nostr.PubKeyFromHex(uri.Host)
			if err != nil {
				log("* invalid nostrconnect:// URI: %s\n", err)
				return
			}
			log("- got nostrconnect:// request from '%s': %s\n", color.New(color.Bold, color.FgBlue).Sprint(clientPublicKey.Hex()), uri.String())

			relays := uri.Query()["relay"]

			// pre-authorize this client since the user has explicitly added it
			if !slices.ContainsFunc(config.Clients, func(c BunkerConfigClient) bool {
				return c.PubKey == clientPublicKey
			}) {
				config.Clients = append(config.Clients, BunkerConfigClient{
					PubKey:       clientPublicKey,
					Name:         uri.Query().Get("name"),
					URL:          uri.Query().Get("url"),
					Icon:         uri.Query().Get("icon"),
					CustomRelays: relays,
				})
			}

			if persist != nil {
				persist()
			}

			resp, eventResponse, err := signer.HandleNostrConnectURI(ctx, uri)
			if err != nil {
				log("* failed to handle: %s\n", err)
				return
			}

			// compute new custom relays to avoid duplicate subscriptions
			newCustomRelays := make([]string, 0, len(relays))
			for _, r := range relays {
				if !slices.Contains(allRelays, r) {
					newCustomRelays = append(newCustomRelays, r)
					allRelays = append(allRelays, r)
				}
			}

			if len(newCustomRelays) > 0 {
				log("subscribing to %d new relays: %s\n", len(newCustomRelays), strings.Join(newCustomRelays, ","))
				go func() {
					for event := range sys.Pool.SubscribeMany(ctx, relays, nostr.Filter{
						Kinds:     []nostr.Kind{nostr.KindNostrConnect},
						Tags:      nostr.TagMap{"p": []string{pubkey.Hex()}},
						Since:     nostr.Now(),
						LimitZero: true,
					}, nostr.SubscriptionOptions{Label: "nak-bunker"}) {
						events <- event
					}
				}()

				time.Sleep(time.Millisecond * 25)
			}

			jresp, _ := json.MarshalIndent(resp, "", "  ")
			log("~ responding with %s\n", string(jresp))
			for res := range sys.Pool.PublishMany(ctx, relays, eventResponse) {
				if res.Error == nil {
					log("* sent through %s\n", res.Relay.URL)
				} else {
					log("* failed to send through %s: %s\n", res.RelayURL, res.Error)
				}
			}
		}

		// unix socket nostrconnect:// handling
		go func() {
			for uri := range onSocketConnect(ctx, c) {
				handleNostrConnect(uri)
			}
		}()

		// just a gimmick
		var cancelPreviousBunkerInfoPrint context.CancelFunc
		_, cancel := context.WithCancel(ctx)
		cancelPreviousBunkerInfoPrint = cancel

		signer.AuthorizeRequest = func(harmless bool, from nostr.PubKey, secret string) bool {
			if slices.ContainsFunc(config.Clients, func(b BunkerConfigClient) bool { return b.PubKey == from }) {
				return true
			}
			if slices.Contains(authorizedSecrets, secret) {
				// add client to authorized list for subsequent requests
				if !slices.ContainsFunc(config.Clients, func(c BunkerConfigClient) bool { return c.PubKey == from }) {
					config.Clients = append(config.Clients, BunkerConfigClient{PubKey: from})
					if persist != nil {
						persist()
					}
				}
				return true
			}

			if secret == newSecret {
				// store this key
				config.Clients = append(config.Clients, BunkerConfigClient{PubKey: from})
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

				return true
			}

			return false
		}

		// == SUBCOMMANDS ==

		exitChan := make(chan bool, 1)

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

		// handleConnectCommand processes nostrconnect:// URIs for interactive connection flow
		handleConnectCommand := func(connectURI string) {
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

			handleNostrConnect(u)
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
				handleConnectCommand(parts[1])
			case "exit", "quit", "q":
				log("Exit command received.\n")
				exitChan <- true
			case "":
				// Ignore empty commands
			default:
				log("Unknown command: %s. Type 'help' for available commands.\n", command)
			}
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

		// == END OF SUBCOMMANDS ==

		for {
			// Check if exit was requested first
			select {
			case <-exitChan:
				log("Shutting down bunker...\n")
				return nil
			case ie := <-events:
				cancelPreviousBunkerInfoPrint() // this prevents us from printing a million bunker info blocks

				// handle the NIP-46 request event
				from := ie.Event.PubKey
				req, resp, eventResponse, err := signer.HandleRequest(ctx, ie.Event)
				if err != nil {
					if errors.Is(err, nip46.AlreadyHandled) {
					continue
				}

				log("< failed to handle request from %s: %s\n", from.Hex(), err.Error())
					continue
				}

				jreq, _ := json.MarshalIndent(req, "", "  ")
				log("- got request from '%s': %s\n", color.New(color.Bold, color.FgBlue).Sprint(from.Hex()), string(jreq))
				jresp, _ := json.MarshalIndent(resp, "", "  ")
				log("~ responding with %s\n", string(jresp))

				// use custom relays if they are defined for this client
				// (normally if the initial connection came from a nostrconnect:// URL)
				relays := relayURLs
				for _, c := range config.Clients {
					if c.PubKey == from && len(c.CustomRelays) > 0 {
						relays = c.CustomRelays
						break
					}
				}

				for res := range sys.Pool.PublishMany(ctx, relays, eventResponse) {
					if res.Error == nil {
						log("* sent response through %s\n", res.Relay.URL)
					} else {
						log("* failed to send response through %s: %s\n", res.RelayURL, res.Error)
					}
				}

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

		return nil
	},
	Commands: []*cli.Command{
		{
			Name:      "connect",
			Usage:     "use the client-initiated NostrConnect flow of NIP46",
			ArgsUsage: "<nostrconnect-uri>",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "profile",
					Usage: "profile name of the bunker to connect to",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				if c.Args().Len() != 1 {
					return fmt.Errorf("must be called with a nostrconnect://... uri")
				}

				if err := sendToSocket(c, c.Args().First()); err != nil {
					return fmt.Errorf("failed to connect to running bunker: %w", err)
				}

				return nil
			},
		},
	},
}

type BunkerConfig struct {
	Clients []BunkerConfigClient `json:"clients"`
	Secret  plainOrEncryptedKey  `json:"sec"`
	Relays  []string             `json:"relays"`

	// deprecated
	AuthorizedKeys []nostr.PubKey `json:"authorized-keys,omitempty"`
}

type BunkerConfigClient struct {
	PubKey       nostr.PubKey `json:"pubkey"`
	Name         string       `json:"name,omitempty"`
	URL          string       `json:"url,omitempty"`
	Icon         string       `json:"icon,omitempty"`
	CustomRelays []string     `json:"custom_relays,omitempty"`
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

func getSocketPath(c *cli.Command) string {
	profile := "default"
	if c.IsSet("profile") {
		profile = c.String("profile")
	}
	return filepath.Join(c.String("config-path"), "bunkerconn", profile)
}

func onSocketConnect(ctx context.Context, c *cli.Command) chan *url.URL {
	res := make(chan *url.URL)
	socketPath := getSocketPath(c)

	// ensure directory exists
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		log(color.RedString("failed to create socket directory: %w\n", err))
		return res
	}

	// delete existing socket file if it exists
	if _, err := os.Stat(socketPath); err == nil {
		if err := os.Remove(socketPath); err != nil {
			log(color.RedString("failed to remove existing socket file: %w\n", err))
			return res
		}
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log(color.RedString("failed to listen on unix socket %s: %w\n", socketPath, err))
		return res
	}

	go func() {
		defer listener.Close()
		defer os.Remove(socketPath) // cleanup socket file on exit

		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}

			go func(conn net.Conn) {
				defer conn.Close()
				buf := make([]byte, 4096)

				for {
					conn.SetReadDeadline(time.Now().Add(5 * time.Second))
					n, err := conn.Read(buf)
					if err != nil {
						break
					}

					uri, err := url.Parse(string(buf[:n]))
					if err == nil && uri.Scheme == "nostrconnect" {
						res <- uri
					}
				}
			}(conn)
		}
	}()

	return res
}

func sendToSocket(c *cli.Command, value string) error {
	socketPath := getSocketPath(c)

	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to bunker unix socket at %s: %w", socketPath, err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte(value))
	if err != nil {
		return fmt.Errorf("failed to send uri to bunker: %w", err)
	}
	return nil
}
