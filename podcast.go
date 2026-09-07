package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip05"
	"fiatjaf.com/nostr/nip19"
	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

type podcastInfo struct {
	PubKey   nostr.PubKey
	Title    string
	Image    string
	Relays   []string
	Metadata nostr.Event
}

var podcast = &cli.Command{
	Name:  "podcast",
	Usage: "play podcasts from Nostr",
	Commands: []*cli.Command{
		{
			Name:      "play",
			Usage:     "play latest episode from podcast or specific episode",
			ArgsUsage: "<pubkey|nip05|npub|nprofile|nevent> [episode-id-prefix]",
			Flags: []cli.Flag{
				&cli.StringSliceFlag{
					Name:    "relay",
					Aliases: []string{"r"},
					Usage:   "also use these relays to fetch from",
				},
				&cli.StringFlag{
					Name:  "player",
					Usage: "player command to use, defaults to auto-detect",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				// a second argument is the id prefix of an episode, as printed by
				// "podcast list"; otherwise the latest episode is played
				if c.Args().Len() == 2 {
					relays := c.StringSlice("relay")
					if err := normalizeAndValidateRelayURLs(relays); err != nil {
						return err
					}

					pk, extraRelays, err := resolvePodcastPubKeyTarget(ctx, c.Args().Get(0))
					if err != nil {
						return fmt.Errorf("invalid podcast target %q: %w", c.Args().Get(0), err)
					}

					episode, err := fetchPodcastEpisodeByPrefix(ctx,
						pk, c.Args().Get(1), nostr.AppendUnique(relays, extraRelays...), 500)
					if err != nil {
						return err
					}

					return playPodcastEpisode(ctx, c.String("player"), episode)
				}

				for target := range getStdinLinesOrArguments(c.Args()) {
					if target == "" {
						return fmt.Errorf("missing podcast or episode target")
					}

					relays := c.StringSlice("relay")
					if err := normalizeAndValidateRelayURLs(relays); err != nil {
						return err
					}

					episode, err := resolvePodcastEpisode(ctx, target, relays)
					if err != nil {
						return err
					}

					if err := playPodcastEpisode(ctx, c.String("player"), episode); err != nil {
						return err
					}
				}

				return nil
			},
		},
		{
			Name:      "info",
			Usage:     "display podcast metadata",
			ArgsUsage: "<pubkey|nip05|npub|nprofile>",
			Flags: []cli.Flag{
				&cli.StringSliceFlag{
					Name:    "relay",
					Aliases: []string{"r"},
					Usage:   "also use these relays to fetch from",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				for target := range getStdinLinesOrArguments(c.Args()) {
					if target == "" {
						return fmt.Errorf("missing podcast target")
					}

					relays := c.StringSlice("relay")
					if err := normalizeAndValidateRelayURLs(relays); err != nil {
						return err
					}

					podcasts, err := resolvePodcastsForPubkey(ctx, target, relays)
					if err != nil {
						return err
					}

					for _, p := range podcasts {
						printPodcastInfo(p)
					}
				}

				return nil
			},
		},
		{
			Name:      "list",
			Usage:     "list podcast episodes",
			ArgsUsage: "<pubkey|nip05|npub|nprofile>",
			Flags: []cli.Flag{
				&cli.StringSliceFlag{
					Name:    "relay",
					Aliases: []string{"r"},
					Usage:   "also use these relays to fetch from",
				},
				&cli.IntFlag{
					Name:    "limit",
					Aliases: []string{"l"},
					Usage:   "maximum number of episodes to fetch",
					Value:   50,
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				for target := range getStdinLinesOrArguments(c.Args()) {
					if target == "" {
						return fmt.Errorf("missing podcast target")
					}

					relays := c.StringSlice("relay")
					if err := normalizeAndValidateRelayURLs(relays); err != nil {
						return err
					}

					podcasts, err := resolvePodcastsForPubkey(ctx, target, relays)
					if err != nil {
						return err
					}

					for _, p := range podcasts {
						if err := printPodcastEpisodeList(ctx, p, int(c.Int("limit"))); err != nil {
							return err
						}
					}
				}

				return nil
			},
		},
	},
}

func resolvePodcastEpisode(ctx context.Context, target string, relayHints []string) (*nostr.Event, error) {
	target = strings.TrimSpace(strings.TrimPrefix(target, "nostr:"))

	if prefix, value, err := nip19.Decode(target); err == nil {
		switch prefix {
		case "nevent":
			pointer := value.(nostr.EventPointer)
			return fetchSpecificPodcastEpisode(ctx, pointer, relayHints)
		case "note":
			return fetchSpecificPodcastEpisode(ctx, value.(nostr.EventPointer), relayHints)
		}
	}

	if pk, extraRelays, err := resolvePodcastPubKeyTarget(ctx, target); err == nil {
		return fetchLatestEpisodeForPodcastSelection(ctx, pk, nostr.AppendUnique(relayHints, extraRelays...))
	}

	return nil, fmt.Errorf("invalid podcast target %q", target)
}

func resolvePodcastPubKeyTarget(ctx context.Context, target string) (nostr.PubKey, []string, error) {
	if strings.TrimSpace(target) == "" {
		return nostr.ZeroPK, nil, fmt.Errorf("missing pubkey target")
	}

	if strings.HasPrefix(target, "nostr:") {
		target = strings.TrimPrefix(target, "nostr:")
	}

	if prefix, value, err := nip19.Decode(target); err == nil {
		switch prefix {
		case "npub":
			return value.(nostr.PubKey), nil, nil
		case "nprofile":
			profile := value.(nostr.ProfilePointer)
			return profile.PublicKey, profile.Relays, nil
		}
	}

	if strings.Contains(target, "@") {
		pp, err := nip05.QueryIdentifier(ctx, target)
		if err != nil {
			return nostr.ZeroPK, nil, err
		}
		return pp.PublicKey, pp.Relays, nil
	}

	pk, err := nostr.PubKeyFromHex(target)
	if err != nil {
		return nostr.ZeroPK, nil, err
	}
	return pk, nil, nil
}

func fetchSpecificPodcastEpisode(ctx context.Context, pointer nostr.EventPointer, relayHints []string) (*nostr.Event, error) {
	relays := slices.Clone(relayHints)
	relays = nostr.AppendUnique(relays, pointer.Relays...)
	if pointer.Author != nostr.ZeroPK {
		relays = nostr.AppendUnique(relays, sys.FetchOutboxRelays(ctx, pointer.Author, 3)...)
	}
	if len(relays) == 0 {
		return nil, fmt.Errorf("no relay hints found for episode")
	}

	res := sys.Pool.QuerySingle(ctx, relays, nostr.Filter{IDs: []nostr.ID{pointer.ID}}, nostr.SubscriptionOptions{Label: "nak-podcast"})
	if res == nil {
		return nil, fmt.Errorf("podcast episode not found")
	}
	if res.Event.Kind != 54 {
		return nil, fmt.Errorf("event is not podcast episode: expected kind 54, got %d", res.Event.Kind)
	}

	return &res.Event, nil
}

func fetchLatestEpisodeForPodcastSelection(ctx context.Context, pk nostr.PubKey, relayHints []string) (*nostr.Event, error) {
	chosen, err := selectPodcast(ctx, pk, relayHints)
	if err != nil {
		return nil, err
	}

	episode, err := fetchLatestPodcastEpisode(ctx, chosen)
	if err != nil {
		return nil, err
	}

	return episode, nil
}

// selectPodcast finds the podcasts published or listed by the given pubkey,
// asking which one is meant when there is more than one.
func selectPodcast(ctx context.Context, pk nostr.PubKey, relayHints []string) (podcastInfo, error) {
	choices, err := discoverPodcasts(ctx, pk, relayHints)
	if err != nil {
		return podcastInfo{}, err
	}

	chosen := choices[0]
	if len(choices) > 1 {
		labels := make([]string, len(choices))
		for i, item := range choices {
			labels[i] = podcastLabel(item)
		}

		var selected string
		if err := survey.AskOne(&survey.Select{
			Message:  "choose podcast",
			Options:  labels,
			PageSize: 12,
		}, &selected); err != nil {
			return podcastInfo{}, err
		}

		for i, label := range labels {
			if label == selected {
				chosen = choices[i]
				break
			}
		}
	}

	return chosen, nil
}

// fetchPodcastEpisodeByPrefix finds one episode of the podcast by the id prefix
// shown by "podcast list".
func fetchPodcastEpisodeByPrefix(ctx context.Context, pk nostr.PubKey, prefix string, relayHints []string, limit int) (*nostr.Event, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return nil, fmt.Errorf("missing episode id prefix")
	}

	chosen, err := selectPodcast(ctx, pk, relayHints)
	if err != nil {
		return nil, err
	}

	episodes, err := fetchPodcastEpisodes(ctx, chosen, limit)
	if err != nil {
		return nil, err
	}

	var matched *nostr.Event
	count := 0
	for _, ep := range episodes {
		if strings.HasPrefix(ep.ID.Hex(), prefix) {
			matched = &ep
			count++
		}
	}

	switch {
	case count == 0:
		return nil, fmt.Errorf("no episode found with id prefix '%s'", prefix)
	case count > 1:
		return nil, fmt.Errorf("id prefix '%s' is ambiguous", prefix)
	}

	return matched, nil
}

func discoverPodcasts(ctx context.Context, pk nostr.PubKey, relayHints []string) ([]podcastInfo, error) {
	choices := make([]podcastInfo, 0, 4)
	seen := make(map[nostr.PubKey]struct{}, 4)

	if direct, ok := fetchPodcastMetadata(ctx, pk, relayHints); ok {
		choices = append(choices, direct)
		seen[pk] = struct{}{}
	}

	authoredRelays := nostr.AppendUnique(slices.Clone(relayHints), sys.FetchOutboxRelays(ctx, pk, 3)...)
	for _, candidatePubKey := range fetchAuthoredPodcastPubKeys(ctx, pk, authoredRelays) {
		if _, ok := seen[candidatePubKey]; ok {
			continue
		}
		meta, ok := fetchPodcastMetadata(ctx, candidatePubKey, relayHints)
		if !ok {
			continue
		}
		if !podcastClaimsAuthor(meta.Metadata, pk) {
			continue
		}
		choices = append(choices, meta)
		seen[candidatePubKey] = struct{}{}
	}

	if len(choices) == 0 {
		return nil, fmt.Errorf("no podcasts found for %s", nip19.EncodeNpub(pk))
	}

	return choices, nil
}

func fetchPodcastMetadata(ctx context.Context, pk nostr.PubKey, relayHints []string) (podcastInfo, bool) {
	relays := nostr.AppendUnique(slices.Clone(relayHints), sys.FetchOutboxRelays(ctx, pk, 3)...)
	if len(relays) == 0 {
		return podcastInfo{}, false
	}

	res := sys.Pool.FetchManyReplaceable(ctx, relays, nostr.Filter{
		Kinds:   []nostr.Kind{10154},
		Authors: []nostr.PubKey{pk},
	}, nostr.SubscriptionOptions{Label: "nak-podcast"})

	evt, ok := res.Load(nostr.ReplaceableKey{PubKey: pk, D: ""})
	if !ok {
		return podcastInfo{}, false
	}

	return podcastInfo{
		PubKey:   pk,
		Title:    podcastTagValue(evt, "title"),
		Image:    podcastTagValue(evt, "image"),
		Relays:   relays,
		Metadata: evt,
	}, true
}

func fetchAuthoredPodcastPubKeys(ctx context.Context, pk nostr.PubKey, relays []string) []nostr.PubKey {
	if len(relays) == 0 {
		return nil
	}

	found := make([]nostr.PubKey, 0, 4)
	seen := make(map[nostr.PubKey]struct{}, 4)
	for _, kind := range []nostr.Kind{10064, 10164} {
		res := sys.Pool.FetchManyReplaceable(ctx, relays, nostr.Filter{
			Kinds:   []nostr.Kind{kind},
			Authors: []nostr.PubKey{pk},
		}, nostr.SubscriptionOptions{Label: "nak-podcast"})

		evt, ok := res.Load(nostr.ReplaceableKey{PubKey: pk, D: ""})
		if !ok {
			continue
		}

		for _, tag := range evt.Tags {
			if len(tag) < 2 || tag[0] != "p" {
				continue
			}
			podcastPK, err := nostr.PubKeyFromHex(tag[1])
			if err != nil {
				continue
			}
			if _, ok := seen[podcastPK]; ok {
				continue
			}
			seen[podcastPK] = struct{}{}
			found = append(found, podcastPK)
		}
	}

	return found
}

func podcastClaimsAuthor(metadata nostr.Event, author nostr.PubKey) bool {
	for _, tag := range metadata.Tags {
		if len(tag) >= 2 && tag[0] == "p" && tag[1] == author.Hex() {
			return true
		}
	}
	return false
}

func fetchLatestPodcastEpisode(ctx context.Context, podcast podcastInfo) (*nostr.Event, error) {
	relays := podcast.Relays
	if len(relays) == 0 {
		relays = sys.FetchOutboxRelays(ctx, podcast.PubKey, 3)
	}
	if len(relays) == 0 {
		return nil, fmt.Errorf("no relay hints found for podcast %s", nip19.EncodeNpub(podcast.PubKey))
	}

	var latest *nostr.Event
	for ie := range sys.Pool.FetchMany(ctx, relays, nostr.Filter{
		Kinds:   []nostr.Kind{54},
		Authors: []nostr.PubKey{podcast.PubKey},
		Limit:   20,
	}, nostr.SubscriptionOptions{Label: "nak-podcast"}) {
		if latest == nil || ie.Event.CreatedAt > latest.CreatedAt {
			event := ie.Event
			latest = &event
		}
	}

	if latest == nil {
		return nil, fmt.Errorf("no episodes found for %s", podcastLabel(podcast))
	}

	return latest, nil
}

func playPodcastEpisode(ctx context.Context, player string, episode *nostr.Event) error {
	audioURL := firstPodcastAudioURL(*episode)
	if audioURL == "" {
		return fmt.Errorf("podcast episode has no audio tag")
	}

	title := podcastTagValue(*episode, "title")
	if title == "" {
		title = episode.ID.Hex()
	}

	stdout(colors.bold("playing:"), color.HiBlueString(title))
	stdout(colors.bold("audio:"), color.HiCyanString(audioURL))

	cmd, err := buildPodcastPlayerCommand(player, audioURL)
	if err != nil {
		return err
	}
	cmd.Stdin = nil
	cmd.Stdout = color.Output
	cmd.Stderr = color.Error

	return runPodcastPlayer(ctx, cmd)
}

// errPodcastInterrupted is returned when the user stops playback with ctrl-c.
var errPodcastInterrupted = errors.New("interrupted")

// runPodcastPlayer runs the player and makes sure it goes away when we do.
// on windows a ctrl-c doesn't necessarily reach the player (mpv.exe is a gui
// subsystem binary and never installs a console handler), so instead of relying
// on the console sending it the event we catch the interrupt ourselves and kill
// the player.
func runPodcastPlayer(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return err
	case <-interrupt:
	case <-ctx.Done():
	}

	killPodcastPlayer(cmd)

	// give the player a moment to die so it doesn't keep writing to our terminal
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}

	return errPodcastInterrupted
}

func killPodcastPlayer(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}

	if runtime.GOOS == "windows" {
		// players tend to spawn helper processes, so kill the whole tree
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		if err := kill.Run(); err == nil {
			return
		}
	}

	cmd.Process.Kill()
}

func buildPodcastPlayerCommand(player string, audioURL string) (*exec.Cmd, error) {
	if strings.TrimSpace(player) != "" {
		parts := strings.Fields(player)
		if len(parts) == 0 {
			return nil, fmt.Errorf("invalid player command %q", player)
		}
		return exec.Command(parts[0], append(parts[1:], audioURL)...), nil
	}

	for _, candidate := range podcastPlayerCandidates() {
		if _, err := exec.LookPath(candidate.name); err == nil {
			return exec.Command(candidate.name, append(candidate.args, audioURL)...), nil
		}
	}

	return nil, fmt.Errorf("no media player found; install mpv, vlc, ffplay or pass --player")
}

func podcastPlayerCandidates() []struct {
	name string
	args []string
} {
	if runtime.GOOS == "darwin" {
		return []struct {
			name string
			args []string
		}{
			{"mpv", nil},
			{"vlc", nil},
			{"ffplay", []string{"-nodisp", "-autoexit"}},
			{"open", nil},
		}
	}

	if runtime.GOOS == "windows" {
		return []struct {
			name string
			args []string
		}{
			// no .exe here on purpose: LookPath honours PATHEXT, so mpv.com,
			// the console wrapper that handles ctrl-c, is preferred when present
			{"mpv", nil},
			{"vlc", nil},
			{"ffplay", []string{"-nodisp", "-autoexit"}},
			{"cmd", []string{"/c", "start"}},
		}
	}

	return []struct {
		name string
		args []string
	}{
		{"mpv", nil},
		{"vlc", nil},
		{"ffplay", []string{"-nodisp", "-autoexit"}},
		{"xdg-open", nil},
	}
}

func firstPodcastAudioURL(evt nostr.Event) string {
	for _, tag := range evt.Tags {
		if len(tag) >= 2 && tag[0] == "audio" {
			return tag[1]
		}
	}
	return ""
}

// singleLine joins the lines of s with spaces, dropping the blank ones.
func singleLine(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return r == '\n' || r == '\r'
	}), " ")
}

func podcastTagValue(evt nostr.Event, name string) string {
	if tag := evt.Tags.Find(name); len(tag) >= 2 {
		return tag[1]
	}
	return ""
}

func podcastLabel(info podcastInfo) string {
	title := strings.TrimSpace(info.Title)
	if title == "" {
		title = nip19.EncodeNpub(info.PubKey)
	} else {
		title += " (" + nip19.EncodeNpub(info.PubKey) + ")"
	}
	return title
}

func resolvePodcastsForPubkey(ctx context.Context, target string, relayHints []string) ([]podcastInfo, error) {
	pk, extraRelays, err := resolvePodcastPubKeyTarget(ctx, target)
	if err != nil {
		return nil, err
	}

	allRelays := nostr.AppendUnique(relayHints, extraRelays...)
	choices, err := discoverPodcasts(ctx, pk, allRelays)
	if err != nil {
		return nil, err
	}

	if len(choices) > 1 {
		labels := make([]string, len(choices))
		for i, item := range choices {
			labels[i] = podcastLabel(item)
		}

		var selected string
		if err := survey.AskOne(&survey.Select{
			Message:  "choose podcast",
			Options:  labels,
			PageSize: 12,
		}, &selected); err != nil {
			return nil, err
		}

		filtered := make([]podcastInfo, 0, 1)
		for i, label := range labels {
			if label == selected {
				filtered = append(filtered, choices[i])
				break
			}
		}
		return filtered, nil
	}

	return choices, nil
}

func printPodcastInfo(p podcastInfo) {
	stdout(colors.bold("title:"), color.HiBlueString(p.Title))
	stdout(colors.bold("pubkey:"), color.HiCyanString(nip19.EncodeNpub(p.PubKey)))

	if desc := podcastTagValue(p.Metadata, "description"); desc != "" {
		stdout(colors.bold("description:"), desc)
	}
	if image := podcastTagValue(p.Metadata, "image"); image != "" {
		stdout(colors.bold("image:"), color.HiCyanString(image))
	}
	if website := podcastTagValue(p.Metadata, "website"); website != "" {
		stdout(colors.bold("website:"), color.HiCyanString(website))
	}

	authors := make([]string, 0, 2)
	for _, tag := range p.Metadata.Tags {
		if len(tag) >= 2 && tag[0] == "p" {
			pk, err := nostr.PubKeyFromHex(tag[1])
			if err != nil {
				continue
			}
			author := nip19.EncodeNpub(pk)
			if len(tag) >= 3 && tag[2] != "" {
				author += " " + tag[2]
			}
			authors = append(authors, author)
		}
	}
	if len(authors) > 0 {
		stdout(colors.bold("authors:"), strings.Join(authors, ", "))
	}

	stdout("")
}

func printPodcastEpisodeList(ctx context.Context, p podcastInfo, limit int) error {
	episodes, err := fetchPodcastEpisodes(ctx, p, limit)
	if err != nil {
		return err
	}

	if len(episodes) == 0 {
		stdout(color.YellowString("no episodes found for %s", p.Title))
		return nil
	}

	stdout(color.CyanString("episodes for %s:", p.Title))
	for _, ep := range episodes {
		title := podcastTagValue(ep, "title")
		if title == "" {
			title = "<untitled>"
		}
		id := ep.ID.Hex()[:8]
		date := ep.CreatedAt.Time().Format("2006-01-02")
		// episode descriptions are several paragraphs long, so fold them into
		// the single line each episode gets here
		desc := clampWithEllipsis(singleLine(ep.Content), 100)
		stdout(fmt.Sprintf("%s  %s  %s  %s", color.CyanString(id), date, title, desc))
	}

	return nil
}

func fetchPodcastEpisodes(ctx context.Context, p podcastInfo, limit int) ([]nostr.Event, error) {
	relays := p.Relays
	if len(relays) == 0 {
		relays = sys.FetchOutboxRelays(ctx, p.PubKey, 3)
	}
	if len(relays) == 0 {
		return nil, fmt.Errorf("no relay hints found for podcast %s", nip19.EncodeNpub(p.PubKey))
	}

	episodes := make([]nostr.Event, 0, limit)
	for ie := range sys.Pool.FetchMany(ctx, relays, nostr.Filter{
		Kinds:   []nostr.Kind{54},
		Authors: []nostr.PubKey{p.PubKey},
		Limit:   limit,
	}, nostr.SubscriptionOptions{Label: "nak-podcast-list"}) {
		episodes = append(episodes, ie.Event)
	}

	return episodes, nil
}
