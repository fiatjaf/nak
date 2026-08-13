package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nip42"
	"fiatjaf.com/nostr/nip46"
	"github.com/charmbracelet/x/ansi"
	"github.com/fatih/color"
	"github.com/mdp/qrterminal/v3"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

const PERSISTENCE = "PERSISTENCE"

type bunkerTerminal struct {
	mu         sync.Mutex
	out        io.Writer
	log        func(string, ...any)
	width      int
	footer     string
	footerRows int
}

func newBunkerTerminal(out io.Writer, log func(string, ...any), width int) *bunkerTerminal {
	return &bunkerTerminal{out: out, log: log, width: width}
}

func (t *bunkerTerminal) Log(msg string, args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.width == 0 || t.footer == "" {
		t.log(msg, args...)
		return
	}

	t.clearFooter()
	text := fmt.Sprintf(msg, args...)
	t.log("%s", text)
	if !strings.HasSuffix(text, "\n") {
		fmt.Fprintln(t.out)
	}
	t.drawFooter()
}

func (t *bunkerTerminal) SetFooter(footer string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	footer = strings.TrimRight(footer, "\n") + "\n"
	if t.width == 0 {
		t.footer = footer
		t.log("%s", footer)
		return
	}

	t.clearFooter()
	t.footer = footer
	t.footerRows = bunkerFooterRows(footer, t.width)
	t.drawFooter()
}

func (t *bunkerTerminal) clearFooter() {
	for range t.footerRows {
		fmt.Fprint(t.out, "\033[1A\033[2K\r")
	}
}

func (t *bunkerTerminal) drawFooter() {
	fmt.Fprint(t.out, t.footer)
}

func bunkerFooterRows(footer string, width int) int {
	rows := 0
	for line := range strings.SplitSeq(strings.TrimSuffix(footer, "\n"), "\n") {
		lineWidth := ansi.StringWidth(line)
		rows += max(1, (lineWidth+width-1)/width)
	}
	return rows
}

func bunkerTerminalWidth() int {
	if runtime.GOOS == "windows" || !term.IsTerminal(int(os.Stderr.Fd())) {
		return 0
	}
	width, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil {
		return 0
	}
	return width
}

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
		baseRelaysUrls := nostr.AppendUnique(c.Args().Slice(), c.StringSlice("relay")...)
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
					log(color.RedString("failed to persist: %s\n"), err)
					os.Exit(4)
				}
				data, err := json.MarshalIndent(config, "", "  ")
				if err != nil {
					log(color.RedString("failed to persist: %s\n"), err)
					os.Exit(4)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					log(color.RedString("failed to persist: %s\n"), err)
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
			config.Relays = nostr.AppendUnique(config.Relays, baseRelaysUrls...)
			for _, bak := range baseAuthorizedKeys {
				if !slices.ContainsFunc(config.Clients, func(c BunkerConfigClient) bool { return c.PubKey == bak }) {
					config.Clients = append(config.Clients, BunkerConfigClient{PubKey: bak})
				}
			}

			if config.Secret.Plain == nil && config.Secret.Encrypted == nil {
				// we don't have any secret key stored, so just use whatever was given via flags (or defaults)
				config.Secret = baseSecret
			} else if !c.IsSet("sec") && !c.IsSet("prompt-sec") {
				// we didn't provide any keys explicitly, so we just use the stored
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
				sec = defaultKey().Hex()
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

		footerWidth := 0
		if c.Count("quiet") == 0 {
			footerWidth = bunkerTerminalWidth()
		}
		terminal := newBunkerTerminal(color.Error, log, footerWidth)
		sys.Pool.RelayOptions.NoticeHandler = func(relay *nostr.Relay, notice string) {
			terminal.Log("NOTICE from %s: '%s'\n", relay.URL, notice)
		}
		sys.Pool.AuthRequiredHandler = func(ctx context.Context, authEvent *nostr.Event) error {
			return authSigner(ctx, c, func(s string, args ...any) {
				if strings.HasPrefix(s, "authenticating as") {
					cleanURL, _ := strings.CutPrefix(nip42.GetRelayURLFromAuthEvent(*authEvent), "wss://")
					s = "authenticating to " + color.CyanString(cleanURL) + " as" + s[len("authenticating as"):]
				}
				terminal.Log(s+"\n", args...)
			}, authEvent)
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
		relays := connectToAllRelays(ctx, c, allRelays)
		if len(relays) == 0 {
			log("failed to connect to any of the given relays.\n")
			os.Exit(3)
		}
		for _, relay := range config.Relays {
			qs.Add("relay", relay)
		}
		if len(relays) == 0 {
			return fmt.Errorf("not connected to any relays: please specify at least one")
		}

		// other arguments
		authorizedSecrets := c.StringSlice("authorized-secrets")

		// this will be used to auto-authorize the next person who connects who isn't pre-authorized
		// it will be stored
		newSecret := randString(12)

		// guards config.Clients and newSecret, which are accessed from the socket
		// goroutine, the per-request handler goroutines and here
		var mu sync.Mutex

		// static information
		pubkey := sec.Public()
		npub := nip19.EncodeNpub(pubkey)

		bunkerInfo := func() (string, string) {
			iqs := make(url.Values)
			maps.Copy(iqs, qs)
			iqs.Set("secret", newSecret)
			bunkerURI := fmt.Sprintf("bunker://%s?%s", pubkey.Hex(), iqs.Encode())

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

			relayURLsPossiblyWithoutSchema := make([]string, len(config.Relays))
			for i, url := range config.Relays {
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

				return fmt.Sprintf("listening at %v:\n  pubkey: %s \n  npub: %s%s%s\n  to restart: %s\n  bunker: %s\n",
					colors.bold(config.Relays),
					colors.bold(pubkey.Hex()),
					colors.bold(npub),
					authorizedKeysStr,
					authorizedSecretsStr,
					color.CyanString(restartCommand),
					colors.bold(bunkerURI),
				), bunkerURI
			} else {
				// otherwise just print the data
				return fmt.Sprintf("listening at %v:\n  pubkey: %s \n  npub: %s%s%s\n  bunker: %s\n",
					colors.bold(config.Relays),
					colors.bold(pubkey.Hex()),
					colors.bold(npub),
					authorizedKeysStr,
					authorizedSecretsStr,
					colors.bold(bunkerURI),
				), bunkerURI
			}
		}

		setBunkerInfo := func() {
			info, _ := bunkerInfo()
			terminal.SetFooter(info)
		}

		info, bunkerURI := bunkerInfo()
		if c.Bool("qrcode") {
			log("QR Code for bunker URI:\n")
			qrterminal.Generate(bunkerURI, qrterminal.L, os.Stdout)
			log("\n\n")
		}
		terminal.SetFooter(info)

		// subscribe to relays
		events := sys.Pool.SubscribeMany(ctx, allRelays, nostr.Filter{
			Kinds:     []nostr.Kind{nostr.KindNostrConnect},
			Tags:      nostr.TagMap{"p": []string{pubkey.Hex()}},
			Since:     nostr.Now(),
			LimitZero: true,
		}, nostr.SubscriptionOptions{Label: "nak-bunker"})

		signer := nip46.NewStaticKeySigner(sec)
		signer.DefaultRelays = config.Relays

		signer.AuthorizeRequest = func(harmless bool, from nostr.PubKey, secret string) bool {
			mu.Lock()

			if slices.ContainsFunc(config.Clients, func(b BunkerConfigClient) bool { return b.PubKey == from }) {
				mu.Unlock()
				return true
			}
			if slices.Contains(authorizedSecrets, secret) {
				// add client to authorized list for subsequent requests
				config.Clients = append(config.Clients, BunkerConfigClient{PubKey: from})
				if persist != nil {
					persist()
				}
				setBunkerInfo()
				mu.Unlock()
				return true
			}

			if secret == newSecret {
				// store this key
				config.Clients = append(config.Clients, BunkerConfigClient{PubKey: from})
				// discard this and generate a new secret
				newSecret = randString(12)

				if persist != nil {
					persist()
				}

				setBunkerInfo()
				mu.Unlock()
				return true
			}

			mu.Unlock()
			return false
		}

		handleBunkerRequest := func(ie nostr.RelayEvent) {
			// handle the NIP-46 request event
			from := ie.Event.PubKey
			req, resp, eventResponse, err := signer.HandleRequest(ctx, ie.Event)
			if err != nil {
				if errors.Is(err, nip46.AlreadyHandled) {
					return
				}

				terminal.Log("< failed to handle request from %s: %s\n", from.Hex(), err.Error())
				return
			}

			jreq, _ := json.MarshalIndent(req, "", "  ")
			terminal.Log("- got request from '%s': %s\n", color.New(color.Bold, color.FgBlue).Sprint(from.Hex()), string(jreq))
			jresp, _ := json.MarshalIndent(resp, "", "  ")
			terminal.Log("~ responding with %s\n", string(jresp))

			// use custom relays if they are defined for this client
			// (normally if the initial connection came from a nostrconnect:// URL)
			relays := config.Relays
			mu.Lock()
			for _, c := range config.Clients {
				if c.PubKey == from && len(c.CustomRelays) > 0 {
					relays = c.CustomRelays
					break
				}
			}
			mu.Unlock()

			for res := range sys.Pool.PublishMany(ctx, relays, eventResponse) {
				if res.Error == nil {
					terminal.Log("* sent response through %s\n", res.Relay.URL)
				} else {
					terminal.Log("* failed to send response through %s: %s\n", res.RelayURL, res.Error)
				}
			}
		}

		// unix socket nostrconnect:// handling
		go func() {
			for uri := range onSocketConnect(ctx, c, terminal.Log) {
				clientPublicKey, err := nostr.PubKeyFromHex(uri.Host)
				if err != nil {
					continue
				}
				terminal.Log("- got nostrconnect:// request from '%s': %s\n", color.New(color.Bold, color.FgBlue).Sprint(clientPublicKey.Hex()), uri.String())

				relays := uri.Query()["relay"]

				// pre-authorize this client since the user has explicitly added it
				mu.Lock()
				clientAdded := false
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
					clientAdded = true
				}

				if persist != nil {
					persist()
				}
				if clientAdded {
					setBunkerInfo()
				}
				mu.Unlock()

				resp, eventResponse, err := signer.HandleNostrConnectURI(ctx, uri)
				if err != nil {
					terminal.Log("* failed to handle: %s\n", err)
					continue
				}

				go func() {
					for event := range sys.Pool.SubscribeMany(ctx, relays, nostr.Filter{
						Kinds:     []nostr.Kind{nostr.KindNostrConnect},
						Tags:      nostr.TagMap{"p": []string{pubkey.Hex()}},
						Since:     nostr.Now(),
						LimitZero: true,
					}, nostr.SubscriptionOptions{Label: "nak-bunker"}) {
						// handle directly instead of forwarding into the main events
						// channel, which is owned (and eventually closed) by the pool
						go handleBunkerRequest(event)
					}
				}()

				time.Sleep(time.Millisecond * 25)
				jresp, _ := json.MarshalIndent(resp, "", "  ")
				terminal.Log("~ responding with %s\n", string(jresp))
				for res := range sys.Pool.PublishMany(ctx, relays, eventResponse) {
					if res.Error == nil {
						terminal.Log("* sent through %s\n", res.Relay.URL)
					} else {
						terminal.Log("* failed to send through %s: %s\n", res.RelayURL, res.Error)
					}
				}
			}
		}()

		for ie := range events {
			go handleBunkerRequest(ie)
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

func onSocketConnect(ctx context.Context, c *cli.Command, log func(string, ...any)) chan *url.URL {
	res := make(chan *url.URL)
	socketPath := getSocketPath(c)

	// ensure directory exists
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		log(color.RedString("failed to create socket directory: %s\n", err))
		return res
	}

	// delete existing socket file if it exists
	if _, err := os.Stat(socketPath); err == nil {
		if err := os.Remove(socketPath); err != nil {
			log(color.RedString("failed to remove existing socket file: %s\n", err))
			return res
		}
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log(color.RedString("failed to listen on unix socket %s: %s\n", socketPath, err))
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
