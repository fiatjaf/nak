package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nipb7/blossom"
	"github.com/AlecAivazis/survey/v2"
	"github.com/avast/apkparser"
	"github.com/avast/apkverifier"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

const (
	zapstoreRelay         = "wss://relay.zapstore.dev"
	zapstoreBlossomServer = "https://cdn.zapstore.dev"
	apkMimeType           = "application/vnd.android.package-archive"
)

var software = &cli.Command{
	Name:                      "software",
	Usage:                     "nip82 software applications, releases and assets",
	DisableSliceFlagSeparator: true,
	Commands: []*cli.Command{
		{
			Name:  "publish",
			Usage: "publishes an android app release (application, release and asset events) from a zapstore.yaml-like file",
			Description: `reads metadata from the given yaml file (fields compatible with zapstore.yaml: release_source, name, summary, description, tags, license, website, repository, icon, images, release_notes, supported_nips, min_allowed_version, min_allowed_version_code, plus relays and blossom_servers), applies flags on top of it, then publishes:
  - the kind 32267 application event, if it doesn't exist or differs from what is already published;
  - a kind 30063 release event;
  - a kind 3063 asset event for the apk.

the apk (release_source or --apk) must be a URL or a path on disk. local files (apk, icon, images) are uploaded to the blossom servers.

nothing is uploaded or published unless relays (and blossom servers, when there are local files) are specified. relative paths are resolved from the directory of the yaml file.

with --interactive all fields are prompted for, with the current values as defaults, and the file is updated with the answers.`,
			ArgsUsage: "<zapstore.yaml>",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "interactive",
					Aliases: []string{"i"},
					Usage:   "prompt for metadata interactively and save the answers to the file",
				},
				&cli.BoolFlag{
					Name:    "yes",
					Aliases: []string{"y"},
					Usage:   "publish without asking for confirmation",
				},
				&cli.StringFlag{
					Name:  "apk",
					Usage: "apk URL or local path (overrides release_source)",
				},
				&cli.StringFlag{Name: "name", Usage: "application name"},
				&cli.StringFlag{Name: "summary", Usage: "short description, no markdown"},
				&cli.StringFlag{Name: "description", Usage: "full description, markdown allowed"},
				&cli.StringSliceFlag{Name: "tag", Aliases: []string{"t"}, Usage: "application tags (can be used multiple times)"},
				&cli.StringFlag{Name: "license", Usage: "SPDX license identifier"},
				&cli.StringFlag{Name: "website", Usage: "website URL"},
				&cli.StringFlag{Name: "repository", Usage: "source code repository URL or nip34 naddr"},
				&cli.StringFlag{Name: "icon", Usage: "icon URL or local path"},
				&cli.StringSliceFlag{Name: "image", Usage: "screenshot URL or local path (can be used multiple times)"},
				&cli.StringFlag{Name: "release-notes", Usage: "release notes text or path to a file containing them"},
				&cli.StringSliceFlag{Name: "supported-nip", Usage: "nips supported by this app (can be used multiple times)"},
				&cli.StringFlag{Name: "min-allowed-version", Usage: "minimum version still allowed to run"},
				&cli.IntFlag{Name: "min-allowed-version-code", Usage: "minimum version code still allowed to run"},
				&cli.StringSliceFlag{Name: "relay", Aliases: []string{"r"}, Usage: "relays to publish to (can be used multiple times)"},
				&cli.StringSliceFlag{Name: "blossom-server", Usage: "blossom servers to upload local files to (can be used multiple times)"},
				&cli.StringFlag{Name: "channel", Usage: "release channel", Value: "main"},
				&cli.StringFlag{Name: "variant", Usage: "asset variant name, i.e. \"fdroid\""},
				&cli.StringFlag{Name: "commit", Usage: "commit used to build the apk"},
			},
			Action: softwarePublish,
		},
		{
			Name:                      "apps",
			Usage:                     "list applications published by an author or found on a relay",
			ArgsUsage:                 "[<author>|<relay>]",
			DisableSliceFlagSeparator: true,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "author",
					Aliases: []string{"a"},
					Usage:   "author pubkey (npub, nprofile, hex or nip05)",
				},
				&cli.StringSliceFlag{
					Name:    "relay",
					Aliases: []string{"r"},
					Usage:   "relay to fetch from (can be used multiple times)",
				},
				&cli.IntFlag{
					Name:    "limit",
					Aliases: []string{"l"},
					Usage:   "maximum number of applications to fetch",
					Value:   100,
				},
			},
			Action: softwareListApps,
		},
		{
			Name:                      "releases",
			Usage:                     "list releases and their assets for an application",
			ArgsUsage:                 "<naddr|d-tag>",
			DisableSliceFlagSeparator: true,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "author",
					Aliases: []string{"a"},
					Usage:   "author pubkey used to disambiguate a bare d tag",
				},
				&cli.StringSliceFlag{
					Name:    "relay",
					Aliases: []string{"r"},
					Usage:   "relay to fetch from (can be used multiple times)",
				},
				&cli.IntFlag{
					Name:    "limit",
					Aliases: []string{"l"},
					Usage:   "maximum number of releases to fetch",
					Value:   100,
				},
			},
			Action: softwareListReleases,
		},
	},
}

type softwareConfig struct {
	ReleaseSource         any      `yaml:"release_source,omitempty"`
	Name                  string   `yaml:"name,omitempty"`
	Summary               string   `yaml:"summary,omitempty"`
	Description           string   `yaml:"description,omitempty"`
	Tags                  []string `yaml:"tags,omitempty"`
	License               string   `yaml:"license,omitempty"`
	Website               string   `yaml:"website,omitempty"`
	Repository            string   `yaml:"repository,omitempty"`
	Icon                  string   `yaml:"icon,omitempty"`
	Images                []string `yaml:"images,omitempty"`
	ReleaseNotes          string   `yaml:"release_notes,omitempty"`
	SupportedNIPs         []string `yaml:"supported_nips,omitempty"`
	MinAllowedVersion     string   `yaml:"min_allowed_version,omitempty"`
	MinAllowedVersionCode int64    `yaml:"min_allowed_version_code,omitempty"`
	Relays                []string `yaml:"relays,omitempty"`
	BlossomServers        []string `yaml:"blossom_servers,omitempty"`

	// legacy zsp field, migrated into blossom_servers
	BlossomServer string `yaml:"blossom_server,omitempty"`
}

func (cfg softwareConfig) hasLocalFiles() bool {
	return (cfg.apkSource() != "" && !isURL(cfg.apkSource())) ||
		(cfg.Icon != "" && !isURL(cfg.Icon)) ||
		slices.ContainsFunc(cfg.Images, func(img string) bool { return !isURL(img) })
}

func (cfg softwareConfig) apkSource() string {
	// zsp also accepts objects here (github sources etc.), we only handle plain strings
	s, _ := cfg.ReleaseSource.(string)
	return s
}

type apkInfo struct {
	PackageID       string
	VersionName     string
	VersionCode     int64
	MinSDK          int64
	TargetSDK       int64
	Label           string
	Platforms       []string
	CertFingerprint string
	SHA256          string
	Size            int64
}

func softwarePublish(ctx context.Context, c *cli.Command) error {
	configPath := c.Args().First()
	if configPath == "" {
		return fmt.Errorf("missing path to yaml file")
	}
	baseDir := filepath.Dir(configPath)
	interactive := c.Bool("interactive")

	// read the file, keeping the raw node so we can write it back without losing unknown fields
	var cfg softwareConfig
	var doc yaml.Node
	if data, err := os.ReadFile(configPath); err == nil {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("failed to parse %s: %w", configPath, err)
		}
		if err := doc.Decode(&cfg); err != nil {
			return fmt.Errorf("failed to parse %s: %w", configPath, err)
		}
	} else if !os.IsNotExist(err) || !interactive {
		return fmt.Errorf("failed to read %s: %w", configPath, err)
	}

	// apply flags on top
	relPath := func(p string) string {
		// paths given as flags are relative to the cwd, but in the file they are relative to it
		if p == "" || isURL(p) {
			return p
		}
		if abs, err := filepath.Abs(p); err == nil {
			if absBase, err := filepath.Abs(baseDir); err == nil {
				if rel, err := filepath.Rel(absBase, abs); err == nil {
					return rel
				}
			}
		}
		return p
	}
	if v := c.String("apk"); v != "" {
		cfg.ReleaseSource = relPath(v)
	}
	if v := c.String("name"); v != "" {
		cfg.Name = v
	}
	if v := c.String("summary"); v != "" {
		cfg.Summary = v
	}
	if v := c.String("description"); v != "" {
		cfg.Description = v
	}
	if v := c.StringSlice("tag"); len(v) > 0 {
		cfg.Tags = v
	}
	if v := c.String("license"); v != "" {
		cfg.License = v
	}
	if v := c.String("website"); v != "" {
		cfg.Website = v
	}
	if v := c.String("repository"); v != "" {
		cfg.Repository = v
	}
	if v := c.String("icon"); v != "" {
		cfg.Icon = relPath(v)
	}
	if v := c.StringSlice("image"); len(v) > 0 {
		cfg.Images = make([]string, len(v))
		for i, img := range v {
			cfg.Images[i] = relPath(img)
		}
	}
	if v := c.String("release-notes"); v != "" {
		if _, err := os.Stat(v); err == nil {
			v = relPath(v)
		}
		cfg.ReleaseNotes = v
	}
	if v := c.StringSlice("supported-nip"); len(v) > 0 {
		cfg.SupportedNIPs = v
	}
	if v := c.String("min-allowed-version"); v != "" {
		cfg.MinAllowedVersion = v
	}
	if v := c.Int("min-allowed-version-code"); v != 0 {
		cfg.MinAllowedVersionCode = int64(v)
	}
	if v := c.StringSlice("relay"); len(v) > 0 {
		cfg.Relays = v
	}
	if cfg.BlossomServer != "" {
		cfg.BlossomServers = nostr.AppendUnique(cfg.BlossomServers, cfg.BlossomServer)
		cfg.BlossomServer = ""
	}
	if v := c.StringSlice("blossom-server"); len(v) > 0 {
		cfg.BlossomServers = v
	}
	for i, r := range cfg.Relays {
		cfg.Relays[i] = nostr.NormalizeURL(r)
	}
	for i, b := range cfg.BlossomServers {
		cfg.BlossomServers[i] = normalizeBlossomServer(b)
	}

	if !interactive {
		if len(cfg.Relays) == 0 {
			log("%s\n", color.YellowString("warning: no relays specified, nothing will be published"))
		}
		if cfg.hasLocalFiles() && len(cfg.BlossomServers) == 0 {
			log("%s\n", color.YellowString("warning: no blossom servers specified for uploading local files, nothing will be published"))
		}
	}

	kr, _, err := gatherKeyerFromArguments(ctx, c)
	if err != nil {
		return err
	}
	pubkey, err := kr.GetPublicKey(ctx)
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}
	writeRelays := sys.FetchWriteRelays(ctx, pubkey)

	// the apk comes first since we need its package id and the defaults it gives us
	if interactive {
		source := cfg.apkSource()
		if err := survey.AskOne(&survey.Input{
			Message: "apk (URL or path)",
			Default: source,
		}, &source, survey.WithValidator(survey.Required)); err != nil {
			return err
		}
		cfg.ReleaseSource = source
	}
	source := cfg.apkSource()
	if source == "" {
		return fmt.Errorf("no apk specified (use release_source in the file, --apk or --interactive)")
	}

	apkPath := source
	if isURL(source) {
		log("downloading %s... ", color.CyanString(source))
		tmp, err := downloadToTempFile(ctx, source)
		if err != nil {
			log("\n")
			return err
		}
		defer os.Remove(tmp)
		log("done.\n")
		apkPath = tmp
	} else {
		apkPath = resolveConfigPath(baseDir, source)
	}

	apk, err := parseAPK(apkPath)
	if err != nil {
		return fmt.Errorf("failed to read apk: %w", err)
	}
	log("apk: %s %s (%d) for %s\n", color.CyanString(apk.PackageID), apk.VersionName, apk.VersionCode,
		strings.Join(apk.Platforms, ", "))

	// fetch existing application event so we can use it for defaults and to see if it needs updating
	log("searching for existing application event... ")
	var existing *nostr.Event
	results := sys.Pool.FetchManyReplaceable(ctx,
		nostr.AppendUnique(append(slices.Clone(cfg.Relays), zapstoreRelay), writeRelays...),
		nostr.Filter{
			Kinds:   []nostr.Kind{32267},
			Authors: []nostr.PubKey{pubkey},
			Tags:    nostr.TagMap{"d": []string{apk.PackageID}},
		}, nostr.SubscriptionOptions{Label: "nak-software"})
	results.Range(func(_ nostr.ReplaceableKey, evt nostr.Event) bool {
		existing = &evt
		return false
	})
	if existing != nil {
		log("found one from %s.\n", existing.CreatedAt.Time().Format("2006-01-02"))

		// fill what is missing from the file with what is already published
		tagValue := func(k string) string {
			if t := existing.Tags.Find(k); t != nil {
				return t[1]
			}
			return ""
		}
		tagValues := func(k string) []string {
			vals := []string{}
			for t := range existing.Tags.FindAll(k) {
				vals = append(vals, t[1])
			}
			return vals
		}
		cfg.Name = orDefault(cfg.Name, tagValue("name"))
		cfg.Summary = orDefault(cfg.Summary, tagValue("summary"))
		cfg.Description = orDefault(cfg.Description, existing.Content)
		cfg.License = orDefault(cfg.License, tagValue("license"))
		cfg.Website = orDefault(cfg.Website, tagValue("url"))
		cfg.Repository = orDefault(cfg.Repository, tagValue("repository"))
		cfg.Icon = orDefault(cfg.Icon, tagValue("icon"))
		if len(cfg.Tags) == 0 {
			cfg.Tags = tagValues("t")
		}
		if len(cfg.Images) == 0 {
			cfg.Images = tagValues("image")
		}
		if cfg.Repository == "" {
			if t := existing.Tags.Find("a"); t != nil {
				if ptr, err := nostr.EntityPointerFromTag(t); err == nil {
					cfg.Repository = nip19.EncodeNaddr(ptr.PublicKey, ptr.Kind, ptr.Identifier, ptr.Relays)
				}
			}
		}
	} else {
		log("none found.\n")
	}
	if cfg.Name == "" && !strings.HasPrefix(apk.Label, "@") {
		cfg.Name = apk.Label
	}

	if interactive {
		for _, q := range []struct {
			message string
			target  *string
		}{
			{"name", &cfg.Name},
			{"summary", &cfg.Summary},
		} {
			if err := survey.AskOne(&survey.Input{Message: q.message, Default: *q.target}, q.target); err != nil {
				return err
			}
		}

		editDescription := cfg.Description == ""
		if err := survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("edit description (%q)?", clampWithEllipsis(firstLine(cfg.Description), 40)),
			Default: editDescription,
		}, &editDescription); err != nil {
			return err
		}
		if editDescription {
			description, err := editWithDefaultEditor("nak-software-description.md", cfg.Description, true)
			if err != nil {
				return err
			}
			cfg.Description = strings.TrimSpace(description)
		}

		tags := strings.Join(cfg.Tags, " ")
		if err := survey.AskOne(&survey.Input{Message: "tags (space-separated)", Default: tags}, &tags); err != nil {
			return err
		}
		cfg.Tags = strings.Fields(tags)

		for _, q := range []struct {
			message string
			target  *string
		}{
			{"license (SPDX)", &cfg.License},
			{"website", &cfg.Website},
			{"repository (URL or naddr)", &cfg.Repository},
			{"icon (URL or path)", &cfg.Icon},
		} {
			if err := survey.AskOne(&survey.Input{Message: q.message, Default: *q.target}, q.target); err != nil {
				return err
			}
		}

		images, err := promptForStringList("images", cfg.Images, nil, nil, nil)
		if err != nil {
			return err
		}
		cfg.Images = images

		for _, q := range []struct {
			message string
			target  *string
		}{
			{"release notes (text or path)", &cfg.ReleaseNotes},
			{"min allowed version", &cfg.MinAllowedVersion},
		} {
			if err := survey.AskOne(&survey.Input{Message: q.message, Default: *q.target}, q.target); err != nil {
				return err
			}
		}

		nips := strings.Join(cfg.SupportedNIPs, " ")
		if err := survey.AskOne(&survey.Input{Message: "supported nips (space-separated)", Default: nips}, &nips); err != nil {
			return err
		}
		cfg.SupportedNIPs = strings.Fields(nips)

		// only ask for blossom servers if we have something to upload
		if cfg.hasLocalFiles() {
			suggestions := []string{zapstoreBlossomServer}
			for _, server := range sys.FetchBlossomServerList(ctx, pubkey).Items {
				suggestions = nostr.AppendUnique(suggestions, normalizeBlossomServer(server.Value()))
			}
			defaultServers := cfg.BlossomServers
			if len(defaultServers) == 0 {
				defaultServers = suggestions
			}
			servers, err := promptForStringList("blossom servers", defaultServers, suggestions, normalizeBlossomServer, nil)
			if err != nil {
				return err
			}
			cfg.BlossomServers = servers
			if len(cfg.BlossomServers) == 0 {
				log("%s\n", color.YellowString("warning: no blossom servers specified for uploading local files, nothing will be published"))
			}
		}

		defaultRelays := cfg.Relays
		suggestions := append([]string{zapstoreRelay}, writeRelays...)
		if len(defaultRelays) == 0 {
			defaultRelays = suggestions
		}
		relays, err := promptForStringList("relays", defaultRelays, suggestions, nostr.NormalizeURL, nostr.IsValidRelayURL)
		if err != nil {
			return err
		}
		cfg.Relays = relays
		if len(cfg.Relays) == 0 {
			log("%s\n", color.YellowString("warning: no relays specified, nothing will be published"))
		}

		log("\n")

		if err := writeSoftwareConfig(configPath, &doc, cfg); err != nil {
			return err
		}
		log("saved %s\n", color.GreenString(configPath))
	}

	relays := cfg.Relays

	if cfg.Name == "" {
		return fmt.Errorf("missing app name")
	}

	// local files must be uploaded to blossom, here we just figure out what their URLs will be
	// (icons and images point to the first server, the apk gets one url for each)
	uploads := []string{}
	blobURL := func(ref string) (string, error) {
		if isURL(ref) {
			return ref, nil
		}
		path := resolveConfigPath(baseDir, ref)
		hash, err := sha256File(path)
		if err != nil {
			return "", err
		}
		uploads = append(uploads, path)
		if len(cfg.BlossomServers) == 0 {
			return "", nil
		}
		return cfg.BlossomServers[0] + "/" + hash + strings.ToLower(filepath.Ext(path)), nil
	}

	var iconURL string
	if cfg.Icon != "" {
		if iconURL, err = blobURL(cfg.Icon); err != nil {
			return fmt.Errorf("icon: %w", err)
		}
	}
	imageURLs := make([]string, 0, len(cfg.Images))
	for _, img := range cfg.Images {
		imageURL, err := blobURL(img)
		if err != nil {
			return fmt.Errorf("image: %w", err)
		}
		if imageURL != "" {
			imageURLs = append(imageURLs, imageURL)
		}
	}
	apkURLs := []string{source}
	if !isURL(source) {
		uploads = append(uploads, apkPath)
		apkURLs = make([]string, len(cfg.BlossomServers))
		for i, server := range cfg.BlossomServers {
			apkURLs[i] = server + "/" + apk.SHA256 + ".apk"
		}
	}

	releaseNotes := cfg.ReleaseNotes
	if releaseNotes != "" {
		if data, err := os.ReadFile(resolveConfigPath(baseDir, releaseNotes)); err == nil {
			releaseNotes = string(data)
		}
	}

	version := apk.VersionName
	if version == "" {
		version = strconv.FormatInt(apk.VersionCode, 10)
	}

	// application event
	app := nostr.Event{
		Kind:      32267,
		CreatedAt: nostr.Now(),
		Content:   cfg.Description,
		Tags: nostr.Tags{
			{"d", apk.PackageID},
			{"name", cfg.Name},
		},
	}
	if cfg.Summary != "" {
		app.Tags = append(app.Tags, nostr.Tag{"summary", cfg.Summary})
	}
	if iconURL != "" {
		app.Tags = append(app.Tags, nostr.Tag{"icon", iconURL})
	}
	for _, img := range imageURLs {
		app.Tags = append(app.Tags, nostr.Tag{"image", img})
	}
	for _, t := range cfg.Tags {
		app.Tags = append(app.Tags, nostr.Tag{"t", t})
	}
	if cfg.Website != "" {
		app.Tags = append(app.Tags, nostr.Tag{"url", cfg.Website})
	}
	if strings.HasPrefix(cfg.Repository, "naddr1") {
		prefix, value, err := nip19.Decode(cfg.Repository)
		if err != nil || prefix != "naddr" {
			return fmt.Errorf("invalid repository naddr: %w", err)
		}
		ptr := value.(nostr.EntityPointer)
		tag := nostr.Tag{"a", ptr.AsTagReference()}
		if len(ptr.Relays) > 0 {
			tag = append(tag, ptr.Relays[0])
		}
		app.Tags = append(app.Tags, tag)
	} else if cfg.Repository != "" {
		app.Tags = append(app.Tags, nostr.Tag{"repository", cfg.Repository})
	}
	if cfg.License != "" {
		app.Tags = append(app.Tags, nostr.Tag{"license", cfg.License})
	}
	// the app must list all platforms for which there are assets, including those from previous releases
	platforms := slices.Clone(apk.Platforms)
	if existing != nil {
		for t := range existing.Tags.FindAll("f") {
			platforms = nostr.AppendUnique(platforms, t[1])
		}
		// keep tags we don't manage here (p, zap, h etc)
		managed := []string{"d", "name", "summary", "icon", "image", "t", "url", "repository", "a", "f", "license"}
		for _, t := range existing.Tags {
			if !slices.Contains(managed, t[0]) {
				app.Tags = append(app.Tags, t)
			}
		}
	}
	for _, f := range platforms {
		app.Tags = append(app.Tags, nostr.Tag{"f", f})
	}
	appChanged := existing == nil || !sameEventContents(*existing, app)

	// asset event
	asset := nostr.Event{
		Kind:      3063,
		CreatedAt: nostr.Now(),
		Tags: nostr.Tags{
			{"i", apk.PackageID},
			{"x", apk.SHA256},
			{"version", version},
			{"m", apkMimeType},
		},
	}
	for _, f := range apk.Platforms {
		asset.Tags = append(asset.Tags, nostr.Tag{"f", f})
	}
	for _, url := range apkURLs {
		asset.Tags = append(asset.Tags, nostr.Tag{"url", url})
	}
	asset.Tags = append(asset.Tags, nostr.Tag{"size", strconv.FormatInt(apk.Size, 10)})
	if apk.MinSDK > 0 {
		asset.Tags = append(asset.Tags, nostr.Tag{"min_platform_version", strconv.FormatInt(apk.MinSDK, 10)})
	}
	if apk.TargetSDK > 0 {
		asset.Tags = append(asset.Tags, nostr.Tag{"target_platform_version", strconv.FormatInt(apk.TargetSDK, 10)})
	}
	for _, nip := range cfg.SupportedNIPs {
		asset.Tags = append(asset.Tags, nostr.Tag{"supported_nip", nip})
	}
	if v := c.String("variant"); v != "" {
		asset.Tags = append(asset.Tags, nostr.Tag{"variant", v})
	}
	asset.Tags = append(asset.Tags, nostr.Tag{"filename", filepath.Base(strings.SplitN(source, "?", 2)[0])})
	if v := c.String("commit"); v != "" {
		asset.Tags = append(asset.Tags, nostr.Tag{"commit", v})
	}
	if cfg.MinAllowedVersion != "" {
		asset.Tags = append(asset.Tags, nostr.Tag{"min_allowed_version", cfg.MinAllowedVersion})
	}
	asset.Tags = append(asset.Tags, nostr.Tag{"version_code", strconv.FormatInt(apk.VersionCode, 10)})
	if cfg.MinAllowedVersionCode > 0 {
		asset.Tags = append(asset.Tags, nostr.Tag{"min_allowed_version_code", strconv.FormatInt(cfg.MinAllowedVersionCode, 10)})
	}
	asset.Tags = append(asset.Tags, nostr.Tag{"apk_certificate_hash", apk.CertFingerprint})

	if err := kr.SignEvent(ctx, &asset); err != nil {
		return fmt.Errorf("failed to sign asset event: %w", err)
	}

	// release event
	release := nostr.Event{
		Kind:      30063,
		CreatedAt: nostr.Now(),
		Content:   releaseNotes,
		Tags: nostr.Tags{
			{"i", apk.PackageID},
			{"version", version},
			{"d", apk.PackageID + "@" + version},
			{"c", c.String("channel")},
		},
	}
	for _, f := range apk.Platforms {
		release.Tags = append(release.Tags, nostr.Tag{"f", f})
	}
	if len(relays) > 0 {
		release.Tags = append(release.Tags, nostr.Tag{"e", asset.ID.Hex(), relays[0]})
	} else {
		release.Tags = append(release.Tags, nostr.Tag{"e", asset.ID.Hex()})
	}

	if err := kr.SignEvent(ctx, &release); err != nil {
		return fmt.Errorf("failed to sign release event: %w", err)
	}
	if appChanged {
		if err := kr.SignEvent(ctx, &app); err != nil {
			return fmt.Errorf("failed to sign application event: %w", err)
		}
	}

	// show everything and confirm
	toPublish := []nostr.Event{}
	if appChanged {
		toPublish = append(toPublish, app)
	}
	toPublish = append(toPublish, release, asset)
	for _, evt := range toPublish {
		stdout(prettyEventWithTagsInline(evt))
	}
	if !appChanged {
		log("application event is unchanged, will not be republished.\n")
	}
	if len(uploads) > 0 {
		log("files to upload: %s\n", strings.Join(uploads, " "))
		log("blossom servers: %s\n", strings.Join(cfg.BlossomServers, " "))
	}
	log("relays: %s\n", strings.Join(relays, " "))

	if len(relays) == 0 {
		return fmt.Errorf("no relays specified, nothing was published")
	}
	if len(uploads) > 0 && len(cfg.BlossomServers) == 0 {
		return fmt.Errorf("no blossom servers specified for uploading local files, nothing was published")
	}

	if !c.Bool("yes") && !askConfirmation("publish these events? [y/n] ") {
		return fmt.Errorf("aborted")
	}

	for _, server := range cfg.BlossomServers {
		if len(uploads) == 0 {
			break
		}
		client := blossom.NewClient(server, kr)
		for _, path := range uploads {
			if _, err := client.UploadFilePath(ctx, path); err != nil {
				return fmt.Errorf("failed to upload %s to %s: %w", path, server, err)
			}
			log("> uploaded %s to %s\n", color.GreenString(path), color.CyanString(server))
		}
	}

	for _, evt := range toPublish {
		log("publishing kind %d...\n", evt.Kind)
		if err := publishGitEventToRepoRelays(ctx, evt, relays); err != nil {
			return err
		}
	}

	return nil
}

func softwareListApps(ctx context.Context, c *cli.Command) error {
	if c.Args().Len() > 1 {
		return fmt.Errorf("expected at most one argument (author or relay)")
	}

	relays := slices.Clone(c.StringSlice("relay"))
	if err := normalizeAndValidateRelayURLs(relays); err != nil {
		return err
	}

	authorTarget := strings.TrimSpace(c.String("author"))
	if arg := strings.TrimSpace(c.Args().First()); arg != "" {
		if softwareTargetIsRelay(arg) {
			relay := nostr.NormalizeURL(arg)
			if err := normalizeAndValidateRelayURLs([]string{relay}); err != nil {
				return err
			}
			relays = nostr.AppendUnique(relays, relay)
		} else if authorTarget != "" {
			return fmt.Errorf("author given both as an argument and with --author")
		} else {
			authorTarget = arg
		}
	}

	limit := int(c.Int("limit"))
	if limit <= 0 {
		limit = 100
	}
	filter := nostr.Filter{
		Kinds: []nostr.Kind{32267},
		Limit: limit,
	}

	if authorTarget != "" {
		pk, err := parsePubKeyForCommand(ctx, c, authorTarget)
		if err != nil {
			return fmt.Errorf("invalid author '%s': %w", authorTarget, err)
		}
		relays = softwareAppReadRelays(ctx, pk, relays)
		filter.Authors = []nostr.PubKey{pk}
	}

	if len(relays) == 0 {
		return fmt.Errorf("expected an author or a relay")
	}

	apps := make([]nostr.Event, 0, limit)
	sys.Pool.FetchManyReplaceable(ctx, relays, filter,
		nostr.SubscriptionOptions{Label: "nak-software-apps"}).Range(func(_ nostr.ReplaceableKey, evt nostr.Event) bool {
		apps = append(apps, evt)
		return true
	})

	if len(apps) == 0 {
		log("no applications found.\n")
		return nil
	}

	slices.SortFunc(apps, func(a, b nostr.Event) int {
		an := softwareTagValue(a, "name")
		if an == "" {
			an = softwareTagValue(a, "d")
		}
		bn := softwareTagValue(b, "name")
		if bn == "" {
			bn = softwareTagValue(b, "d")
		}
		if cmp := strings.Compare(strings.ToLower(an), strings.ToLower(bn)); cmp != 0 {
			return cmp
		}
		return strings.Compare(softwareTagValue(a, "d"), softwareTagValue(b, "d"))
	})

	for _, app := range apps {
		d := softwareTagValue(app, "d")
		name := softwareTagValue(app, "name")
		if name == "" {
			name = d
		}
		naddr := nip19.EncodeNaddr(app.PubKey, 32267, d, nil)

		line := fmt.Sprintf("%s  %s (%s)", color.CyanString(naddr), color.GreenString(name), d)
		if platforms := softwareTagValues(app, "f"); len(platforms) > 0 {
			line += "  [" + strings.Join(platforms, ", ") + "]"
		}
		if summary := softwareTagValue(app, "summary"); summary != "" {
			line += "  " + clampWithEllipsis(singleLine(summary), 100)
		}
		stdout(line)
	}

	return nil
}

func softwareListReleases(ctx context.Context, c *cli.Command) error {
	if c.Args().Len() != 1 {
		return fmt.Errorf("expected exactly one argument (application naddr or d tag)")
	}

	relays := slices.Clone(c.StringSlice("relay"))
	if err := normalizeAndValidateRelayURLs(relays); err != nil {
		return err
	}

	app, relays, err := softwareResolveApp(ctx, c, c.Args().First(), strings.TrimSpace(c.String("author")), relays)
	if err != nil {
		return err
	}
	appID := softwareTagValue(*app, "d")

	limit := int(c.Int("limit"))
	if limit <= 0 {
		limit = 100
	}

	releases := make([]nostr.Event, 0, limit)
	sys.Pool.FetchManyReplaceable(ctx, relays, nostr.Filter{
		Kinds:   []nostr.Kind{30063},
		Authors: []nostr.PubKey{app.PubKey},
		Tags:    nostr.TagMap{"i": []string{appID}},
		Limit:   limit,
	}, nostr.SubscriptionOptions{Label: "nak-software-releases"}).Range(
		func(_ nostr.ReplaceableKey, evt nostr.Event) bool {
			releases = append(releases, evt)
			return true
		})

	name := softwareTagValue(*app, "name")
	if name == "" {
		name = appID
	}
	stdout(fmt.Sprintf("%s %s (%s)", colors.bold("application:"), color.GreenString(name), appID))
	if len(releases) == 0 {
		stdout(color.YellowString("no releases found."))
		return nil
	}

	slices.SortFunc(releases, func(a, b nostr.Event) int {
		if a.CreatedAt != b.CreatedAt {
			if a.CreatedAt > b.CreatedAt {
				return -1
			}
			return 1
		}
		return strings.Compare(softwareTagValue(b, "version"), softwareTagValue(a, "version"))
	})

	// the release e tags are the authoritative list of assets, so fetch them by id
	assetIDs := make([]nostr.ID, 0, len(releases))
	assetRelays := slices.Clone(relays)
	for _, release := range releases {
		for tag := range release.Tags.FindAll("e") {
			if len(tag) < 2 {
				continue
			}
			id, err := nostr.IDFromHex(tag[1])
			if err != nil {
				continue
			}
			if !slices.Contains(assetIDs, id) {
				assetIDs = append(assetIDs, id)
			}
			if len(tag) >= 3 {
				relay := nostr.NormalizeURL(tag[2])
				if nostr.IsValidRelayURL(relay) {
					assetRelays = nostr.AppendUnique(assetRelays, relay)
				}
			}
		}
	}
	assets := softwareFetchAssets(ctx, assetRelays, assetIDs)

	for _, release := range releases {
		softwarePrintRelease(release, assets)
	}

	return nil
}

func softwareResolveApp(
	ctx context.Context,
	c *cli.Command,
	target string,
	authorTarget string,
	relayHints []string,
) (*nostr.Event, []string, error) {
	target = strings.TrimSpace(strings.TrimPrefix(target, "nostr:"))
	if target == "" {
		return nil, nil, fmt.Errorf("missing application naddr or d tag")
	}

	if prefix, value, err := nip19.Decode(target); err == nil {
		if prefix != "naddr" {
			return nil, nil, fmt.Errorf("expected an application naddr or d tag, got %s", prefix)
		}
		ptr := value.(nostr.EntityPointer)
		if ptr.Kind != 0 && ptr.Kind != 32267 {
			return nil, nil, fmt.Errorf("naddr points to kind %d, expected 32267", ptr.Kind)
		}
		if ptr.Identifier == "" {
			return nil, nil, fmt.Errorf("naddr has no application identifier")
		}

		relays := nostr.AppendUnique(slices.Clone(relayHints), ptr.Relays...)
		relays = softwareAppReadRelays(ctx, ptr.PublicKey, relays)
		app, err := softwareFetchApp(ctx, ptr.PublicKey, ptr.Identifier, relays)
		if err != nil {
			return nil, nil, err
		}
		return app, relays, nil
	} else if strings.HasPrefix(target, "naddr1") {
		return nil, nil, fmt.Errorf("invalid application naddr: %w", err)
	}

	// a bare d tag is ambiguous across authors, so search by it and complain if
	// more than one application is found
	relays := slices.Clone(relayHints)
	filter := nostr.Filter{
		Kinds: []nostr.Kind{32267},
		Tags:  nostr.TagMap{"d": []string{target}},
		Limit: 10,
	}
	if authorTarget != "" {
		pk, err := parsePubKeyForCommand(ctx, c, authorTarget)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid author '%s': %w", authorTarget, err)
		}
		relays = softwareAppReadRelays(ctx, pk, relays)
		filter.Authors = []nostr.PubKey{pk}
	} else {
		relays = nostr.AppendUnique(relays, zapstoreRelay)
	}
	if len(relays) == 0 {
		return nil, nil, fmt.Errorf("no relays to search for application '%s'", target)
	}

	apps := make([]nostr.Event, 0, 4)
	sys.Pool.FetchManyReplaceable(ctx, relays, filter,
		nostr.SubscriptionOptions{Label: "nak-software-app"}).Range(func(_ nostr.ReplaceableKey, evt nostr.Event) bool {
		apps = append(apps, evt)
		return true
	})

	switch len(apps) {
	case 0:
		return nil, nil, fmt.Errorf("application '%s' not found", target)
	case 1:
		return &apps[0], relays, nil
	default:
		naddrs := make([]string, len(apps))
		for i, app := range apps {
			naddrs[i] = nip19.EncodeNaddr(app.PubKey, 32267, softwareTagValue(app, "d"), nil)
		}
		slices.Sort(naddrs)
		return nil, nil, fmt.Errorf("multiple applications found for '%s', use an naddr or --author: %s",
			target, strings.Join(naddrs, " "))
	}
}

func softwareFetchApp(ctx context.Context, pk nostr.PubKey, d string, relays []string) (*nostr.Event, error) {
	evt, ok := sys.Pool.FetchManyReplaceable(ctx, relays, nostr.Filter{
		Kinds:   []nostr.Kind{32267},
		Authors: []nostr.PubKey{pk},
		Tags:    nostr.TagMap{"d": []string{d}},
	}, nostr.SubscriptionOptions{Label: "nak-software-app"}).Load(nostr.ReplaceableKey{PubKey: pk, D: d})
	if !ok {
		return nil, fmt.Errorf("application %s not found", nip19.EncodeNaddr(pk, 32267, d, relays))
	}
	return &evt, nil
}

// softwareAppReadRelays returns the relays where an author's applications are
// likely to be, including the read relays from their kind 10067 app catalog.
func softwareAppReadRelays(ctx context.Context, pk nostr.PubKey, hints []string) []string {
	relays := nostr.AppendUnique(slices.Clone(hints), sys.FetchOutboxRelays(ctx, pk, 3)...)
	relays = nostr.AppendUnique(relays, zapstoreRelay)

	catalog, ok := sys.Pool.FetchManyReplaceable(ctx, relays, nostr.Filter{
		Kinds:   []nostr.Kind{10067},
		Authors: []nostr.PubKey{pk},
	}, nostr.SubscriptionOptions{Label: "nak-software-catalog"}).Load(nostr.ReplaceableKey{PubKey: pk})
	if ok {
		for tag := range catalog.Tags.FindAll("r") {
			if len(tag) < 2 {
				continue
			}
			if len(tag) >= 3 && tag[2] != "read" {
				continue
			}
			relay := nostr.NormalizeURL(tag[1])
			if nostr.IsValidRelayURL(relay) {
				relays = nostr.AppendUnique(relays, relay)
			}
		}
	}

	return relays
}

func softwareFetchAssets(ctx context.Context, relays []string, ids []nostr.ID) map[nostr.ID]nostr.Event {
	assets := make(map[nostr.ID]nostr.Event, len(ids))
	for start := 0; start < len(ids); start += 100 {
		end := min(start+100, len(ids))
		for ie := range sys.Pool.FetchMany(ctx, relays, nostr.Filter{
			Kinds: []nostr.Kind{3063},
			IDs:   ids[start:end],
		}, nostr.SubscriptionOptions{Label: "nak-software-assets"}) {
			assets[ie.Event.ID] = ie.Event
		}
	}
	return assets
}

func softwarePrintRelease(release nostr.Event, assets map[nostr.ID]nostr.Event) {
	version := softwareTagValue(release, "version")
	if version == "" {
		version = softwareTagValue(release, "d")
	}

	parts := []string{colors.bold(version)}
	if channel := softwareTagValue(release, "c"); channel != "" {
		parts = append(parts, color.CyanString(channel))
	}
	parts = append(parts, release.CreatedAt.Time().Format("2006-01-02"))
	if notes := clampWithEllipsis(singleLine(release.Content), 100); notes != "" {
		parts = append(parts, notes)
	}
	stdout(strings.Join(parts, "  "))

	for tag := range release.Tags.FindAll("e") {
		if len(tag) < 2 {
			continue
		}
		id, err := nostr.IDFromHex(tag[1])
		if err != nil {
			continue
		}
		asset, ok := assets[id]
		if !ok {
			stdout(fmt.Sprintf("  - %s", color.YellowString("missing asset %s", tag[1])))
			continue
		}
		softwarePrintAsset(asset)
	}
}

func softwarePrintAsset(asset nostr.Event) {
	label := softwareTagValue(asset, "variant")
	if label == "" {
		label = softwareTagValue(asset, "filename")
	}
	if label == "" {
		label = asset.ID.Hex()[:8]
	}

	parts := []string{color.GreenString(label)}
	if mime := softwareTagValue(asset, "m"); mime != "" {
		parts = append(parts, mime)
	}
	if size := softwareFormatSize(softwareTagValue(asset, "size")); size != "" {
		parts = append(parts, size)
	}
	if platforms := softwareTagValues(asset, "f"); len(platforms) > 0 {
		parts = append(parts, "["+strings.Join(platforms, ", ")+"]")
	}
	stdout(fmt.Sprintf("  - %s", strings.Join(parts, "  ")))

	for _, url := range softwareTagValues(asset, "url") {
		stdout(fmt.Sprintf("      %s", color.CyanString(url)))
	}

	details := []string{}
	if hash := softwareTagValue(asset, "x"); hash != "" {
		details = append(details, "sha256 "+hash)
	}
	if versionCode := softwareTagValue(asset, "version_code"); versionCode != "" {
		details = append(details, "version_code "+versionCode)
	}
	if cert := softwareTagValue(asset, "apk_certificate_hash"); cert != "" {
		details = append(details, "cert "+cert)
	}
	if len(details) > 0 {
		stdout(fmt.Sprintf("      %s", strings.Join(details, "  ")))
	}
}

func softwareTagValue(evt nostr.Event, name string) string {
	if tag := evt.Tags.Find(name); len(tag) >= 2 {
		return tag[1]
	}
	return ""
}

func softwareTagValues(evt nostr.Event, name string) []string {
	values := []string{}
	for tag := range evt.Tags.FindAll(name) {
		if len(tag) >= 2 {
			values = append(values, tag[1])
		}
	}
	return values
}

func softwareFormatSize(value string) string {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 {
		return value
	}
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(1024), 0
	for n/div >= 1024 && exp < 5 {
		div *= 1024
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func softwareTargetIsRelay(target string) bool {
	return strings.HasPrefix(target, "ws://") || strings.HasPrefix(target, "wss://")
}

func normalizeBlossomServer(s string) string {
	s = strings.TrimSpace(s)
	if !isURL(s) {
		s = "https://" + s
	}
	return strings.TrimSuffix(s, "/")
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

func orDefault(value, def string) string {
	if value != "" {
		return value
	}
	return def
}

func resolveConfigPath(baseDir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(baseDir, p)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func downloadToTempFile(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("failed to download %s: status %d", url, resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "nak-*.apk")
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("failed to download %s: %w", url, err)
	}
	return tmp.Name(), nil
}

func prettyEventWithTagsInline(evt nostr.Event) string {
	tags := make([]string, len(evt.Tags))
	for i, t := range evt.Tags {
		j, _ := json.Marshal(t)
		tags[i] = "    " + string(j)
	}
	content, _ := json.Marshal(evt.Content)
	return fmt.Sprintf("{\n  \"kind\": %d,\n  \"id\": \"%s\",\n  \"pubkey\": \"%s\",\n  \"created_at\": %d,\n  \"tags\": [\n%s\n  ],\n  \"content\": %s,\n  \"sig\": \"%s\"\n}",
		evt.Kind, evt.ID.Hex(), evt.PubKey.Hex(), evt.CreatedAt, strings.Join(tags, ",\n"), content, hex.EncodeToString(evt.Sig[:]))
}

// sameEventContents compares content and tags ignoring tag order
func sameEventContents(a, b nostr.Event) bool {
	if a.Content != b.Content || len(a.Tags) != len(b.Tags) {
		return false
	}
	serialize := func(tags nostr.Tags) []string {
		s := make([]string, len(tags))
		for i, t := range tags {
			j, _ := json.Marshal(t)
			s[i] = string(j)
		}
		slices.Sort(s)
		return s
	}
	return slices.Equal(serialize(a.Tags), serialize(b.Tags))
}

type apkManifestCollector struct{ info *apkInfo }

func (m apkManifestCollector) Flush() error { return nil }

func (m apkManifestCollector) EncodeToken(token xml.Token) error {
	start, ok := token.(xml.StartElement)
	if !ok {
		return nil
	}
	attr := func(name string) string {
		for _, a := range start.Attr {
			if a.Name.Local == name {
				return a.Value
			}
		}
		return ""
	}
	parseInt := func(s string) int64 {
		v, _ := strconv.ParseInt(s, 0, 64)
		return v
	}

	switch start.Name.Local {
	case "manifest":
		m.info.PackageID = attr("package")
		m.info.VersionName = attr("versionName")
		m.info.VersionCode = parseInt(attr("versionCode"))
	case "uses-sdk":
		m.info.MinSDK = parseInt(attr("minSdkVersion"))
		m.info.TargetSDK = parseInt(attr("targetSdkVersion"))
	case "application":
		m.info.Label = attr("label")
	}
	return nil
}

func parseAPK(path string) (*apkInfo, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	info := &apkInfo{Size: stat.Size()}

	if info.SHA256, err = sha256File(path); err != nil {
		return nil, err
	}

	zipErr, _, manifestErr := apkparser.ParseApk(path, apkManifestCollector{info})
	if zipErr != nil {
		return nil, zipErr
	}
	if manifestErr != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", manifestErr)
	}
	if info.PackageID == "" {
		return nil, fmt.Errorf("manifest has no package id")
	}

	res, err := apkverifier.Verify(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to verify signature: %w", err)
	}
	_, cert := apkverifier.PickBestApkCert(res.SignerCerts)
	if cert == nil {
		return nil, fmt.Errorf("no signing certificate found")
	}
	fingerprint := sha256.Sum256(cert.Raw)
	info.CertFingerprint = hex.EncodeToString(fingerprint[:])

	// native libraries determine the architectures, an apk without any runs everywhere
	zr, err := apkparser.OpenZip(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	for name := range zr.File {
		parts := strings.Split(name, "/")
		if len(parts) >= 3 && parts[0] == "lib" && parts[1] != "" {
			info.Platforms = nostr.AppendUnique(info.Platforms, "android-"+parts[1])
		}
	}
	if len(info.Platforms) == 0 {
		info.Platforms = []string{"android-arm64-v8a", "android-armeabi-v7a", "android-x86", "android-x86_64"}
	}
	slices.Sort(info.Platforms)

	// apkparser may leave the label as a resource reference, so resolve it (following aliases)
	if strings.HasPrefix(info.Label, "@") {
		resourceID, err := strconv.ParseUint(info.Label[1:], 16, 32)
		info.Label = ""
		if err == nil {
			if arsc, ok := zr.File["resources.arsc"]; ok {
				if data, err := arsc.ReadAll(64 << 20); err == nil {
					if table, err := apkparser.ParseResourceTable(bytes.NewReader(data)); err == nil {
						for range 16 {
							entry, err := table.GetResourceEntry(uint32(resourceID))
							if err != nil {
								break
							}
							value, err := entry.GetValue().Data()
							if err != nil {
								break
							}
							if s, ok := value.(string); ok {
								info.Label = s
								break
							} else if ref, ok := value.(uint32); ok {
								resourceID = uint64(ref)
							} else {
								break
							}
						}
					}
				}
			}
		}
	}

	return info, nil
}

// writeSoftwareConfig updates the fields we know about in the yaml document, leaving everything else untouched
func writeSoftwareConfig(path string, doc *yaml.Node, cfg softwareConfig) error {
	if doc.Kind == 0 {
		doc.Kind = yaml.DocumentNode
	}
	if len(doc.Content) == 0 {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	mapping := doc.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return fmt.Errorf("%s is not a yaml mapping", path)
	}

	v := reflect.ValueOf(cfg)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		key := strings.Split(t.Field(i).Tag.Get("yaml"), ",")[0]
		field := v.Field(i)
		empty := field.IsZero() || (field.Kind() == reflect.Slice && field.Len() == 0)

		idx := -1
		for j := 0; j+1 < len(mapping.Content); j += 2 {
			if mapping.Content[j].Value == key {
				idx = j
				break
			}
		}

		if empty {
			if idx != -1 {
				mapping.Content = slices.Delete(mapping.Content, idx, idx+2)
			}
			continue
		}

		var value yaml.Node
		if err := value.Encode(field.Interface()); err != nil {
			return fmt.Errorf("failed to encode %s: %w", key, err)
		}
		if idx != -1 {
			value.LineComment = mapping.Content[idx+1].LineComment
			mapping.Content[idx+1] = &value
		} else {
			mapping.Content = append(mapping.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &value)
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}
