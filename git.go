package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nip34"
	"fiatjaf.com/nostr/nip34/gitnaturalapi"
	"fiatjaf.com/nostr/nip34/grasp"
	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

var git = &cli.Command{
	Name:  "git",
	Usage: "nip34 and grasp-related operations",
	Description: `this implements versions of common git commands, like 'clone', 'fetch', 'pull' and 'push', but differently from the normal git commands these never take a remote name, the remote is assumed to what is defined by nip34 events and specified in the (automatically hidden) nip34.json file.

aside from those, there is also:
  - 'nak git init' for setting up nip34 repository metadata; and
  - 'nak git sync' for getting the latest metadata update from nostr relays (called automatically by other commands)
`,
	Commands: []*cli.Command{
		{
			Name:  "init",
			Usage: "initialize a nip34 repository configuration",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "interactive",
					Aliases: []string{"i"},
					Usage:   "prompt for repository details interactively",
				},
				&cli.BoolFlag{
					Name:    "force",
					Aliases: []string{"f"},
					Usage:   "overwrite existing nip34.json file",
				},
				&cli.StringFlag{
					Name:  "identifier",
					Usage: "unique identifier for the repository",
				},
				&cli.StringFlag{
					Name:  "name",
					Usage: "repository name",
				},
				&cli.StringFlag{
					Name:  "description",
					Usage: "repository description",
				},
				&cli.StringFlag{
					Name:  "owner",
					Usage: "owner public key",
				},
				&cli.StringSliceFlag{
					Name:  "grasp-servers",
					Usage: "grasp servers (can be used multiple times)",
				},
				&cli.StringSliceFlag{
					Name:  "relays",
					Usage: "relay URLs to publish to (can be used multiple times)",
				},
				&cli.StringFlag{
					Name:  "earliest-unique-commit",
					Usage: "earliest unique commit of the repository",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				// check if current directory is a git repository
				cmd := exec.Command("git", "rev-parse", "--git-dir")
				if err := cmd.Run(); err != nil {
					// initialize a git repository
					log("initializing git repository...\n")
					initCmd := exec.Command("git", "init")
					initCmd.Stderr = os.Stderr
					initCmd.Stdout = os.Stdout
					if err := initCmd.Run(); err != nil {
						return fmt.Errorf("failed to initialize git repository: %w", err)
					}
				}

				var defaultOwner string
				var defaultIdentifier string

				// check if nip34.json already exists
				existingConfig, err := readNip34ConfigFile("")
				if err == nil {
					// file exists
					if !c.Bool("force") && !c.Bool("interactive") {
						return fmt.Errorf("nip34.json already exists, use --force to overwrite or --interactive to update")
					}

					defaultIdentifier = existingConfig.Identifier
					defaultOwner = existingConfig.Owner
				} else {
					// extract info from nostr:// git remotes (this is just for migrating from ngit)
					output, err := exec.Command("git", "remote", "-v").Output()
					if err == nil {
						for _, remote := range strings.Split(strings.TrimSpace(string(output)), "\n") {
							if !strings.Contains(remote, "nostr://") {
								continue
							}
							parts := strings.Fields(remote)
							if len(parts) < 2 {
								continue
							}
							// parse nostr://npub.../relay_hostname/identifier
							remoteOwner, remoteIdentifier, relays, err := parseRepositoryAddress(ctx, parts[1])
							if err != nil || len(relays) == 0 {
								continue
							}
							defaultIdentifier = remoteIdentifier
							defaultOwner = nip19.EncodeNpub(remoteOwner)
						}
					}
				}

				// get repository base directory name for defaults
				if defaultIdentifier == "" {
					cwd, err := os.Getwd()
					if err != nil {
						return fmt.Errorf("failed to get current directory: %w", err)
					}
					defaultIdentifier = filepath.Base(cwd)
				}

				// prompt for identifier first
				var identifier string
				if c.String("identifier") != "" {
					identifier = c.String("identifier")
				} else if c.Bool("interactive") {
					if err := survey.AskOne(&survey.Input{
						Message: "identifier",
						Default: defaultIdentifier,
					}, &identifier); err != nil {
						return err
					}
				} else {
					identifier = defaultIdentifier
				}

				// prompt for owner pubkey
				var owner nostr.PubKey
				var ownerStr string
				if c.String("owner") != "" {
					owner, err = parsePubKey(c.String("owner"))
					if err != nil {
						return fmt.Errorf("invalid owner pubkey: %w", err)
					}
					ownerStr = nip19.EncodeNpub(owner)
				} else if c.Bool("interactive") {
					for {
						if err := survey.AskOne(&survey.Input{
							Message: "owner (npub, nip05 or hex)",
							Default: defaultOwner,
						}, &ownerStr); err != nil {
							return err
						}
						owner, err = parsePubKey(ownerStr)
						if err == nil {
							ownerStr = nip19.EncodeNpub(owner)
							break
						}
					}
				} else {
					return fmt.Errorf("owner pubkey is required (use --owner or --interactive)")
				}

				// try to fetch existing repository announcement (kind 30617)
				var fetchedRepo *nip34.Repository
				if existingConfig.Identifier == "" {
					log("  searching for existing events... ")
					repo, _, _, _, err := fetchRepositoryAndState(ctx, owner, identifier, nil)
					if err == nil && repo.Event.ID != nostr.ZeroID {
						fetchedRepo = &repo
						log("found one from %s.\n", repo.Event.CreatedAt.Time().Format(time.DateOnly))
					} else {
						log("none found.\n")
					}
				}

				// set config with fetched values or defaults
				var config Nip34Config
				if fetchedRepo != nil {
					config = RepositoryToConfig(*fetchedRepo)
				} else if existingConfig.Identifier != "" {
					config = existingConfig
				} else {
					// get earliest unique commit
					var earliestCommit string
					if output, err := exec.Command("git", "rev-list", "--max-parents=0", "HEAD").Output(); err == nil {
						earliestCommit = strings.TrimSpace(string(output))
					}

					config = Nip34Config{
						Identifier:           identifier,
						Owner:                ownerStr,
						Name:                 identifier,
						Description:          "",
						GraspServers:         []string{"gitnostr.com", "relay.ngit.dev"},
						EarliestUniqueCommit: earliestCommit,
					}
				}

				// helper to get value from flags, existing config, or default
				getValue := func(existingVal, flagVal, defaultVal string) string {
					if flagVal != "" {
						return flagVal
					}
					if existingVal != "" {
						return existingVal
					}
					return defaultVal
				}

				getSliceValue := func(existingVals, flagVals, defaultVals []string) []string {
					if len(flagVals) > 0 {
						return flagVals
					}
					if len(existingVals) > 0 {
						return existingVals
					}
					return defaultVals
				}

				// override with flags and existing config
				// (identifier and ownerStr already hold the flag value, the interactive answer or the default)
				config.Identifier = identifier
				config.Name = getValue(existingConfig.Name, c.String("name"), config.Name)
				config.Description = getValue(existingConfig.Description, c.String("description"), config.Description)
				config.Owner = ownerStr
				config.GraspServers = getSliceValue(existingConfig.GraspServers, c.StringSlice("grasp-servers"), config.GraspServers)
				config.EarliestUniqueCommit = getValue(existingConfig.EarliestUniqueCommit, c.String("earliest-unique-commit"), config.EarliestUniqueCommit)

				if c.Bool("interactive") {
					// prompt for name
					if err := survey.AskOne(&survey.Input{
						Message: "name",
						Default: config.Name,
					}, &config.Name); err != nil {
						return err
					}

					// prompt for description
					if err := survey.AskOne(&survey.Input{
						Message: "description",
						Default: config.Description,
					}, &config.Description); err != nil {
						return err
					}

					// prompt for grasp servers
					graspServers, err := promptForStringList("grasp servers", config.GraspServers, []string{
						"gitnostr.com",
						"relay.ngit.dev",
						"pyramid.fiatjaf.com",
						"git.shakespeare.diy",
					}, graspServerHost, nil)
					if err != nil {
						return err
					}
					config.GraspServers = graspServers

					// prompt for earliest unique commit
					if err := survey.AskOne(&survey.Input{
						Message: "earliest unique commit",
						Default: config.EarliestUniqueCommit,
					}, &config.EarliestUniqueCommit); err != nil {
						return err
					}

					log("\n")
				}

				if err := config.Validate(); err != nil {
					return fmt.Errorf("invalid config: %w", err)
				}

				// write config file
				if err := writeNip34ConfigFile("", config); err != nil {
					return err
				}

				log("created %s\n", color.GreenString("nip34.json"))

				// setup git remotes
				gitSetupRemotes(ctx, "", config.ToRepository())

				// gitignore it
				excludeNip34ConfigFile("")

				log("edit %s if needed, then run %s to publish.\n",
					color.CyanString("nip34.json"),
					color.CyanString("nak git sync"))

				return nil
			},
		},
		{
			Name:  "sync",
			Usage: "sync repository with relays",
			Action: func(ctx context.Context, c *cli.Command) error {
				kr, _, _ := gatherKeyerFromArguments(ctx, c)
				_, _, err := gitSync(ctx, kr, false)
				return err
			},
		},
		{
			Name:        "clone",
			Usage:       "clone a NIP-34 repository from a nostr:// URI",
			Description: `the <repository> parameter maybe in the form "<npub, hex, nprofile or nip05>/<identifier>", ngit-style like "nostr://<npub>/<relay>/<identifier>" or "nostr://<npub>/<identifier>" or an "naddr1..." code.`,
			ArgsUsage:   "<repository> [directory]",
			Action: func(ctx context.Context, c *cli.Command) error {
				args := c.Args()
				if args.Len() == 0 {
					return fmt.Errorf("missing repository address")
				}

				owner, identifier, relayHints, err := parseRepositoryAddress(ctx, args.Get(0))
				if err != nil {
					return fmt.Errorf("failed to parse remote url '%s': %s", args.Get(0), err)
				}

				// fetch repository metadata and state
				repo, _, _, state, err := fetchRepositoryAndState(ctx, owner, identifier, relayHints)
				if err != nil {
					return err
				}

				// determine target directory
				targetDir := ""
				if args.Len() >= 2 {
					targetDir = args.Get(1)
				} else {
					targetDir = repo.ID
				}
				if targetDir == "" {
					targetDir = repo.ID
				}

				// if targetDir exists and is non-empty, bail
				if fi, err := os.Stat(targetDir); err == nil && fi.IsDir() {
					entries, err := os.ReadDir(targetDir)
					if err == nil && len(entries) > 0 {
						return fmt.Errorf("target directory '%s' already exists and is not empty", targetDir)
					}
				}

				// create directory
				if err := os.MkdirAll(targetDir, 0755); err != nil {
					return fmt.Errorf("failed to create directory '%s': %w", targetDir, err)
				}

				// initialize git inside the directory
				initCmd := exec.Command("git", "init")
				initCmd.Dir = targetDir
				if err := initCmd.Run(); err != nil {
					return fmt.Errorf("failed to initialize git repository: %w", err)
				}

				// write nip34.json inside cloned directory
				localConfig := RepositoryToConfig(repo)

				if err := localConfig.Validate(); err != nil {
					return fmt.Errorf("invalid config: %w", err)
				}

				// write nip34.json
				if err := writeNip34ConfigFile(targetDir, localConfig); err != nil {
					return err
				}

				// add nip34.json to .git/info/exclude in cloned repo
				excludeNip34ConfigFile(targetDir)

				// setup git remotes
				gitSetupRemotes(ctx, targetDir, repo)

				// fetch from each grasp remote
				fetchFromRemotes(ctx, targetDir, repo)

				// if we have a state with a HEAD, try to reset to it
				if state != nil && state.HEAD != "" {
					if headCommit, ok := state.Branches[state.HEAD]; ok {
						// check if we have that commit
						checkCmd := exec.Command("git", "cat-file", "-e", headCommit)
						checkCmd.Dir = targetDir
						if err := checkCmd.Run(); err == nil {
							// commit exists, reset to it
							log("resetting to commit %s...\n", color.CyanString(headCommit))
							resetCmd := exec.Command("git", "reset", "--hard", headCommit)
							resetCmd.Dir = targetDir
							resetCmd.Stderr = os.Stderr
							if err := resetCmd.Run(); err != nil {
								log("! failed to reset: %v\n", color.YellowString("%v", err))
							}
						}
					}
				}

				// update refs from state
				if state != nil {
					gitUpdateRefs(ctx, targetDir, *state)
				}

				log("cloned into %s\n", color.GreenString(targetDir))
				return nil
			},
		},
		{
			Name:      "download",
			Usage:     "download a file from a NIP-34 repository",
			ArgsUsage: "<repository> <path>",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "output",
					Aliases: []string{"O"},
					Usage:   "output path (use '-' for stdout)",
				},
				&cli.StringFlag{
					Name:    "ref",
					Aliases: []string{"r"},
					Usage:   "git ref/tag/branch/commit to read from",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				args := c.Args()
				if args.Len() < 2 {
					return fmt.Errorf("missing repository and path")
				}

				repo := args.Get(0)
				path := args.Get(1)
				outputPath := c.String("output")
				ref := strings.TrimSpace(c.String("ref"))

				if outputPath == "" {
					cleaned := strings.TrimRight(path, "/")
					base := filepath.Base(cleaned)
					if base == "." || base == "/" || base == "" {
						return fmt.Errorf("cannot determine output filename from path '%s', use --output", path)
					}
					outputPath = base
				}

				if outputPath != "-" {
					if fi, err := os.Stat(outputPath); err == nil && fi.IsDir() {
						return fmt.Errorf("output path '%s' is a directory", outputPath)
					}
				}

				var gitURLs []string
				if strings.HasPrefix(repo, "http://") || strings.HasPrefix(repo, "https://") {
					gitURLs = []string{strings.TrimRight(repo, "/")}
				} else {
					owner, identifier, relayHints, err := parseRepositoryAddress(ctx, repo)
					if err != nil {
						return fmt.Errorf("failed to parse repository address '%s': %w", repo, err)
					}

					repo, _, _, state, err := fetchRepositoryAndState(ctx, owner, identifier, relayHints)
					if err != nil {
						var stateErr *StateErr
						if ref == "" || !errors.As(err, &stateErr) {
							return err
						}
					}

					if ref == "" && state != nil && state.HEAD != "" {
						ref = state.HEAD
					}

					for _, url := range repo.Clone {
						if strings.HasPrefix(url, "http") {
							gitURLs = append(gitURLs, url)
						}
					}
				}

				if len(gitURLs) == 0 {
					return fmt.Errorf("no HTTP git URLs found for repository")
				}

				var lastErr error
				for _, url := range gitURLs {
					if lastErr != nil {
						log("%s\n", color.HiRedString(lastErr.Error()))
					}
					lastErr = nil

					{
						printUrl := color.BlueString(url)
						if grasp.IsGraspURL(url) {
							printUrl = color.HiYellowString(strings.Split(url, "/")[2])
						}
						log("attempting download from %s... ", printUrl)
					}

					info, err := gitnaturalapi.GetInfoRefs(url)
					if err != nil {
						lastErr = err
						continue
					}

					var commitHash string

					if ref == "" {
						if symref, ok := info.Symrefs["HEAD"]; ok && symref != "" {
							commitHash, _ = info.Refs[symref]
						} else if head, ok := info.Refs["HEAD"]; ok && head != "" {
							commitHash = head
						} else {
							lastErr = fmt.Errorf("could not resolve default ref for %s", url)
							continue
						}
					}

					if gitHashRe.MatchString(ref) {
						commitHash = ref
					} else if strings.HasPrefix(ref, "refs/") {
						if ch, ok := info.Refs[ref]; ok {
							commitHash = ch
						}
					} else {
						if ch, ok := info.Refs["refs/heads/"+ref]; ok {
							commitHash = ch
						} else if ch, ok := info.Refs["refs/tags/"+ref]; ok {
							commitHash = ch
						} else if sr, ok := info.Symrefs[ref]; ok {
							commitHash = info.Refs[sr]
						}
					}

					if commitHash == "" {
						lastErr = fmt.Errorf("couldn't get a commit hash for ref '%s'", ref)
						continue
					}

					if !gitHashRe.MatchString(commitHash) {
						lastErr = fmt.Errorf("couldn't invalid commit hash for ref '%s': '%s'", ref, commitHash)
						continue
					}

					entry, err := gitnaturalapi.GetObjectByPath(url, commitHash, path)
					if err != nil {
						lastErr = err
						continue
					}
					if entry == nil {
						lastErr = fmt.Errorf("path '%s' not found", path)
						continue
					}
					if entry.IsDir {
						lastErr = fmt.Errorf("path '%s' is a directory", path)
						continue
					}

					obj, err := gitnaturalapi.GetObject(url, entry.Hash)
					if err != nil {
						lastErr = fmt.Errorf("download error: %s", err)
						continue
					}
					if obj == nil {
						lastErr = fmt.Errorf("object for '%s' not found", path)
						continue
					}
					if obj.Type != gitnaturalapi.ObjectTypeBlob {
						lastErr = fmt.Errorf("object at '%s' is not a file", path)
						continue
					}

					if outputPath == "-" {
						if _, err = os.Stdout.Write(obj.Data); err != nil {
							return err
						}
						log("\nprinted object %s to stdout\n", color.CyanString(obj.Hash))
						return nil
					}

					if err := os.WriteFile(outputPath, obj.Data, 0644); err != nil {
						return fmt.Errorf("failed to write %s: %w", outputPath, err)
					}

					log("\nsaved object %s to %s\n", color.CyanString(obj.Hash), color.GreenString(outputPath))
					return nil
				}

				if lastErr != nil {
					log("%s\n", color.HiRedString(lastErr.Error()))
				}

				return fmt.Errorf("failed to download '%s' from '%s'", path, repo)
			},
		},
		{
			Name:        "ls",
			Usage:       "list files in a remote NIP-34 repository",
			Description: "the <repository> parameter may be a git http(s) url or a repository address as accepted by 'nak git clone'. when given a subpath, lists the contents of that directory.",
			ArgsUsage:   "<repository> [path]",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "ref",
					Aliases: []string{"r"},
					Usage:   "git ref/tag/branch/commit to read from",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				args := c.Args()
				if args.Len() == 0 {
					return fmt.Errorf("missing repository address")
				}

				gitURLs, state, err := resolveGitNaturalURLs(ctx, args.Get(0))
				if err != nil {
					return err
				}
				if len(gitURLs) == 0 {
					return fmt.Errorf("no HTTP git URLs found for repository")
				}

				path := strings.TrimSpace(args.Get(1))
				ref := strings.TrimSpace(c.String("ref"))

				var lastErr error
				for _, url := range gitURLs {
					if lastErr != nil {
						log("%s\n", color.HiRedString(lastErr.Error()))
					}
					lastErr = nil

					commit, err := resolveGitNaturalRef(url, ref, state)
					if err != nil {
						lastErr = err
						continue
					}

					tree, err := gitnaturalapi.GetDirectoryTreeAt(url, commit, nil)
					if err != nil {
						lastErr = err
						continue
					}

					if path != "" {
						tree, err = gitTreeAtPath(tree, path)
						if err != nil {
							return err
						}
					}

					for _, dir := range tree.Directories {
						stdout(color.HiBlueString(dir.Name + "/"))
					}
					for _, file := range tree.Files {
						stdout(color.HiWhiteString(file.Name))
					}
					return nil
				}

				if lastErr != nil {
					log("%s\n", color.HiRedString(lastErr.Error()))
				}
				return fmt.Errorf("failed to list '%s' from '%s'", path, args.Get(0))
			},
		},
		{
			Name:        "cat",
			Usage:       "print the contents of a file in a remote NIP-34 repository",
			Description: "the <repository> parameter may be a git http(s) url or a repository address as accepted by 'nak git clone'.",
			ArgsUsage:   "<repository> <path>",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "ref",
					Aliases: []string{"r"},
					Usage:   "git ref/tag/branch/commit to read from",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				args := c.Args()
				if args.Len() < 2 {
					return fmt.Errorf("missing repository and path")
				}

				gitURLs, state, err := resolveGitNaturalURLs(ctx, args.Get(0))
				if err != nil {
					return err
				}
				if len(gitURLs) == 0 {
					return fmt.Errorf("no HTTP git URLs found for repository")
				}

				path := strings.TrimSpace(args.Get(1))
				ref := strings.TrimSpace(c.String("ref"))

				var lastErr error
				for _, url := range gitURLs {
					if lastErr != nil {
						log("%s\n", color.HiRedString(lastErr.Error()))
					}
					lastErr = nil

					commit, err := resolveGitNaturalRef(url, ref, state)
					if err != nil {
						lastErr = err
						continue
					}

					// the path is walked here instead of with GetObjectByPath()
					// because that also treats '\' as a separator, while in git
					// path names a backslash is just a regular character
					segments := strings.Split(strings.Trim(path, "/"), "/")
					name := segments[len(segments)-1]

					depth := len(segments)
					tree, err := gitnaturalapi.GetDirectoryTreeAt(url, commit, &depth)
					if err != nil {
						lastErr = err
						continue
					}

					if len(segments) > 1 {
						tree, err = gitTreeAtPath(tree, strings.Join(segments[:len(segments)-1], "/"))
						if err != nil {
							return err
						}
					}

					hash := ""
					for _, file := range tree.Files {
						if file.Name == name {
							hash = file.Hash
							break
						}
					}
					if hash == "" {
						for _, dir := range tree.Directories {
							if dir.Name == name {
								return fmt.Errorf("path '%s' is a directory", path)
							}
						}
						return fmt.Errorf("path '%s' not found", path)
					}

					obj, err := gitnaturalapi.GetObject(url, hash)
					if err != nil {
						lastErr = fmt.Errorf("download error: %s", err)
						continue
					}
					if obj == nil {
						return fmt.Errorf("object for '%s' not found", path)
					}
					if obj.Type != gitnaturalapi.ObjectTypeBlob {
						return fmt.Errorf("object at '%s' is not a file", path)
					}

					if _, err := os.Stdout.Write(obj.Data); err != nil {
						return err
					}
					return nil
				}

				if lastErr != nil {
					log("%s\n", color.HiRedString(lastErr.Error()))
				}
				return fmt.Errorf("failed to read '%s' from '%s'", path, args.Get(0))
			},
		},
		{
			Name:        "history",
			Usage:       "print the commit history of a remote NIP-34 repository",
			Description: "the <repository> parameter may be a git http(s) url or a repository address as accepted by 'nak git clone'.",
			ArgsUsage:   "<repository>",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "ref",
					Aliases: []string{"r"},
					Usage:   "git ref/tag/branch/commit to read from",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				args := c.Args()
				if args.Len() == 0 {
					return fmt.Errorf("missing repository address")
				}

				gitURLs, state, err := resolveGitNaturalURLs(ctx, args.Get(0))
				if err != nil {
					return err
				}
				if len(gitURLs) == 0 {
					return fmt.Errorf("no HTTP git URLs found for repository")
				}

				ref := strings.TrimSpace(c.String("ref"))

				var lastErr error
				for _, url := range gitURLs {
					if lastErr != nil {
						log("%s\n", color.HiRedString(lastErr.Error()))
					}
					lastErr = nil

					commit, err := resolveGitNaturalRef(url, ref, state)
					if err != nil {
						lastErr = err
						continue
					}

					commits, err := gitnaturalapi.FetchCommitsOnly(url, commit, nil)
					if err != nil {
						lastErr = err
						continue
					}

					for _, c := range commits {
						date := time.Unix(c.Author.Timestamp, 0).Format(time.DateOnly)
						stdout(
							color.CyanString(shortCommitID(c.Hash, 8)),
							color.HiBlueString(c.Author.Name),
							color.HiBlackString(date),
							color.HiWhiteString(firstLine(c.Message)),
						)
					}
					return nil
				}

				if lastErr != nil {
					log("%s\n", color.HiRedString(lastErr.Error()))
				}
				return fmt.Errorf("failed to fetch history from '%s'", args.Get(0))
			},
		},
		{
			Name:  "push",
			Usage: "push git changes",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "force",
					Aliases: []string{"f"},
					Usage:   "force push to git remotes",
				},
				&cli.BoolFlag{
					Name:  "tags",
					Usage: "push all refs under refs/tags",
				},
				&cli.BoolFlag{
					Name:  "no-announcement",
					Usage: "skip publishing updated repository announcement event",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				// setup signer
				kr, _, err := gatherKeyerFromArguments(ctx, c)
				if err != nil {
					return fmt.Errorf("failed to gather keyer: %w", err)
				}

				// log publishing as npub
				currentPk, _ := kr.GetPublicKey(ctx)
				currentNpub := nip19.EncodeNpub(currentPk)
				log("publishing as %s\n", color.CyanString(currentNpub))

				// sync to ensure everything is up to date
				repo, state, err := gitSync(ctx, kr, c.Bool("no-announcement"))
				if err != nil {
					return fmt.Errorf("failed to sync: %w", err)
				}

				currentPk, err = ensureGitRepositoryOwner(ctx, kr, repo, "push")
				if err != nil {
					return err
				}

				// figure out which branches to push
				localBranch, remoteBranch, err := figureOutBranches(c, c.Args().First(), true)
				if err != nil {
					return err
				}

				// get commit for the local branch
				res, err := exec.Command("git", "rev-parse", localBranch).Output()
				if err != nil {
					return fmt.Errorf("failed to get commit for branch %s: %w", localBranch, err)
				}
				currentCommit := strings.TrimSpace(string(res))

				logverbose("pushing branch %s to remote branch %s, commit: %s\n", localBranch, remoteBranch, currentCommit)

				// create a new state if we didn't find any
				if state == nil {
					state = &nip34.RepositoryState{
						ID:       repo.ID,
						Branches: make(map[string]string),
						Tags:     make(map[string]string),
					}
				}

				// update the branch
				if !c.Bool("force") {
					if prevCommit, exists := state.Branches[remoteBranch]; exists {
						// check if prevCommit is an ancestor of currentCommit (fast-forward check)
						cmd := exec.Command("git", "merge-base", "--is-ancestor", prevCommit, currentCommit)
						if err := cmd.Run(); err != nil {
							return fmt.Errorf("non-fast-forward push not allowed, use --force to override")
						}
					}
				}
				state.Branches[remoteBranch] = currentCommit
				log("- setting branch %s to commit %s\n", color.CyanString(remoteBranch), color.CyanString(currentCommit))

				// set the HEAD to the local branch if none is set
				if state.HEAD == "" {
					state.HEAD = remoteBranch
					log("- setting HEAD to branch %s\n", color.CyanString(remoteBranch))
				}

				if c.Bool("tags") {
					// add all refs/tags
					output, err := exec.Command("git", "show-ref", "--tags").Output()
					if err != nil && err.Error() != "exit status 1" {
						// exit status 1 is returned when there are no tags, which should be ok for us
						return fmt.Errorf("failed to get local tags: %s", err)
					} else {
						lines := strings.Split(strings.TrimSpace(string(output)), "\n")
						for _, line := range lines {
							line = strings.TrimSpace(line)
							if line == "" {
								continue
							}
							parts := strings.Fields(line)
							if len(parts) != 2 {
								continue
							}
							commitHash := parts[0]
							ref := parts[1]

							tagName := strings.TrimPrefix(ref, "refs/tags/")

							if !c.Bool("force") {
								// if --force is not passed then we can't overwrite tags
								if existingHash, exists := state.Tags[tagName]; exists && existingHash != commitHash {
									return fmt.Errorf("tag %s that is already published pointing to %s, call with --force to overwrite", tagName, existingHash)
								}
							}
							state.Tags[tagName] = commitHash
							log("- setting tag %s to commit %s\n", color.CyanString(tagName), color.CyanString(commitHash))
						}
					}
				}

				// create and sign the new state event
				newStateEvent := state.ToEvent()
				err = kr.SignEvent(ctx, &newStateEvent)
				if err != nil {
					return fmt.Errorf("error signing state event: %w", err)
				}

				log("- publishing updated repository state to " + color.CyanString("%v", repo.Relays) + "\n")
				for res := range sys.Pool.PublishMany(ctx, repo.Relays, newStateEvent) {
					if res.Error != nil {
						log("! error publishing event to %s: %v\n", color.YellowString(res.RelayURL), res.Error)
					} else {
						log("> published to %s\n", color.GreenString(res.RelayURL))
					}
				}

				// push to each grasp remote
				pushSuccesses := 0
				for _, relay := range repo.Relays {
					relayURL := nostr.NormalizeURL(relay)
					remoteName := gitRemoteName(relayURL)

					log("pushing to %s...\n", color.CyanString(remoteName))
					pushArgs := []string{"push", remoteName, fmt.Sprintf("%s:refs/heads/%s", localBranch, remoteBranch)}
					if c.Bool("force") {
						pushArgs = append(pushArgs, "--force")
					}
					if c.Bool("tags") {
						pushArgs = append(pushArgs, "--tags")
					}
					pushCmd := exec.Command("git", pushArgs...)
					pushCmd.Stderr = os.Stderr
					pushCmd.Stdout = os.Stdout
					if err := pushCmd.Run(); err != nil {
						log("! failed to push to %s: %v\n", color.YellowString(remoteName), err)
					} else {
						log("> pushed to %s\n", color.GreenString(remoteName))
						pushSuccesses++
					}
				}

				if pushSuccesses == 0 {
					return fmt.Errorf("failed to push to any remote")
				}

				gitUpdateRefs(ctx, "", *state)

				return nil
			},
		},
		{
			Name:  "pull",
			Usage: "pull git changes",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:  "rebase",
					Usage: "rebase instead of merge",
				},
				&cli.BoolFlag{
					Name:  "ff-only",
					Usage: "only allow fast-forward merges",
				},
				&cli.BoolFlag{
					Name:  "ff",
					Usage: "allow fast-forward merges",
				},
				&cli.BoolFlag{
					Name:  "no-ff",
					Usage: "always perform a merge instead of fast-forwarding",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				// sync to fetch latest state and metadata
				_, state, err := gitSync(ctx, nil, false)
				if err != nil {
					return fmt.Errorf("failed to sync: %w", err)
				}

				// figure out which branches to pull
				localBranch, remoteBranch, err := figureOutBranches(c, c.Args().First(), false)
				if err != nil {
					return err
				}

				// get the commit from state for the remote branch
				if state == nil || state.Event.ID == nostr.ZeroID {
					return fmt.Errorf("no repository state found")
				}

				targetCommit, ok := state.Branches[remoteBranch]
				if !ok {
					return fmt.Errorf("branch '%s' not found in repository state", remoteBranch)
				}

				// check if the commit exists locally
				checkCmd := exec.Command("git", "cat-file", "-e", targetCommit)
				if err := checkCmd.Run(); err != nil {
					return fmt.Errorf("commit %s not found locally, try 'nak git fetch' first", targetCommit)
				}

				// determine merge strategy
				var strategy string
				strategiesSpecified := 0
				if c.Bool("rebase") {
					strategy = "rebase"
					strategiesSpecified++
				}
				if c.Bool("ff-only") {
					strategy = "ff-only"
					strategiesSpecified++
				}
				if c.Bool("no-ff") {
					strategy = "no-ff"
					strategiesSpecified++
				}
				if c.Bool("ff") {
					strategy = "ff"
					strategiesSpecified++
				}

				if strategiesSpecified > 1 {
					return fmt.Errorf("flags --rebase, --ff-only, --ff, --no-ff are mutually exclusive")
				}

				if strategy == "" {
					// check git config for pull.rebase
					cmd := exec.Command("git", "config", "--get", "pull.rebase")
					output, err := cmd.Output()
					if err == nil && strings.TrimSpace(string(output)) == "true" {
						strategy = "rebase"
					} else if err == nil && strings.TrimSpace(string(output)) == "false" {
						strategy = "ff"
					} else {
						// check git config for pull.ff
						cmd := exec.Command("git", "config", "--get", "pull.ff")
						output, err := cmd.Output()
						if err == nil && strings.TrimSpace(string(output)) == "only" {
							strategy = "ff-only"
						}
					}
				}

				// execute the merge or rebase
				switch strategy {
				case "rebase":
					log("rebasing %s onto %s...\n", color.CyanString(localBranch), color.CyanString(targetCommit))
					rebaseCmd := exec.Command("git", "rebase", targetCommit)
					rebaseCmd.Stderr = os.Stderr
					rebaseCmd.Stdout = os.Stdout
					if err := rebaseCmd.Run(); err != nil {
						return fmt.Errorf("rebase failed: %w", err)
					}
				case "ff-only":
					log("pulling %s into %s (fast-forward only)...\n", color.CyanString(targetCommit), color.CyanString(localBranch))
					mergeCmd := exec.Command("git", "merge", "--ff-only", targetCommit)
					mergeCmd.Stderr = os.Stderr
					mergeCmd.Stdout = os.Stdout
					if err := mergeCmd.Run(); err != nil {
						return fmt.Errorf("merge failed: %w", err)
					}
				case "no-ff":
					log("pulling %s into %s (no fast-forward)...\n", color.CyanString(targetCommit), color.CyanString(localBranch))
					mergeCmd := exec.Command("git", "merge", "--no-ff", targetCommit)
					mergeCmd.Stderr = os.Stderr
					mergeCmd.Stdout = os.Stdout
					if err := mergeCmd.Run(); err != nil {
						return fmt.Errorf("merge failed: %w", err)
					}
				case "ff":
					log("pulling %s into %s...\n", color.CyanString(targetCommit), color.CyanString(localBranch))
					mergeCmd := exec.Command("git", "merge", "--ff", targetCommit)
					mergeCmd.Stderr = os.Stderr
					mergeCmd.Stdout = os.Stdout
					if err := mergeCmd.Run(); err != nil {
						return fmt.Errorf("merge failed: %w", err)
					}
				default:
					// get current commit
					res, err := exec.Command("git", "rev-parse", localBranch).Output()
					if err != nil {
						return fmt.Errorf("failed to get current commit for branch %s: %w", localBranch, err)
					}
					currentCommit := strings.TrimSpace(string(res))

					// check if fast-forward possible
					cmd := exec.Command("git", "merge-base", "--is-ancestor", currentCommit, targetCommit)
					if err := cmd.Run(); err != nil {
						return fmt.Errorf("fast-forward merge not possible, specify --rebase, --ff-only, --ff, or --no-ff; or use git config")
					}

					// do fast-forward
					log("fast-forwarding to %s...\n", color.CyanString(targetCommit))
					mergeCmd := exec.Command("git", "merge", "--ff-only", targetCommit)
					mergeCmd.Stderr = os.Stderr
					mergeCmd.Stdout = os.Stdout
					if err := mergeCmd.Run(); err != nil {
						return fmt.Errorf("fast-forward failed: %w", err)
					}
				}

				log("pull complete\n")
				return nil
			},
		},
		{
			Name:  "fetch",
			Usage: "fetch git data",
			Action: func(ctx context.Context, c *cli.Command) error {
				_, _, err := gitSync(ctx, nil, false)
				return err
			},
		},
		{
			Name:        "patch",
			Usage:       "patch-related operations",
			Description: "when called directly, lists open patches; with an patch id prefix, displays that patch with threaded discussions.",
			ArgsUsage:   "[id-prefix]",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:  "applied",
					Usage: "list only applied/merged patches",
				},
				&cli.BoolFlag{
					Name:  "closed",
					Usage: "list only closed patches",
				},
				&cli.BoolFlag{
					Name:  "all",
					Usage: "list all patches, including applied and closed",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				repo, err := readGitRepositoryFromConfig()
				if err != nil {
					return err
				}

				events, err := fetchGitRepoRelatedEvents(ctx, repo, 1617)
				if err != nil {
					return err
				}

				prefix := strings.TrimSpace(c.Args().First())
				if prefix == "" {
					// list
					statuses, err := fetchIssueStatus(ctx, repo, events)
					if err != nil {
						return err
					}

					if len(events) == 0 {
						log("no patches found\n")
						return nil
					}

					showApplied := c.Bool("applied")
					showClosed := c.Bool("closed")
					showAll := c.Bool("all")

					// preload metadata from everybody
					wg := sync.WaitGroup{}
					for _, evt := range events {
						wg.Go(func() {
							sys.FetchProfileMetadata(ctx, evt.PubKey)
						})
					}
					wg.Wait()

					// now render
					for _, evt := range events {
						id := evt.ID.Hex()

						status := statusLabelForEvent(evt.ID, statuses, false)
						if !showAll {
							if showApplied || showClosed {
								isApplied := status == "applied/merged"
								isClosed := status == "closed"
								if !(showApplied && isApplied || showClosed && isClosed) {
									continue
								}
							} else if status == "applied/merged" || status == "closed" {
								continue
							}
						}

						date := evt.CreatedAt.Time().Format(time.DateOnly)
						subject := patchSubjectPreview(evt, 72)
						statusDisplayText := status
						if status == "applied/merged" {
							statusDisplayText = "applied"
						}
						statusDisplay := colorizeGitStatus(statusDisplayText)

						if status == "applied/merged" {
							if statusEvt, ok := statuses[evt.ID]; ok {
								if commit := patchAppliedCommitPreview(statusEvt); commit != "" {
									statusDisplay = statusDisplay + color.HiBlackString(" (%s)", commit)
								}
							}
						}

						stdout(color.CyanString(id[:6]), statusDisplay, color.HiBlackString(date), color.HiBlueString(authorPreview(ctx, evt.PubKey)), color.HiWhiteString(subject))
					}

					return nil
				} else {
					// view single
					evt, err := findEventByPrefix(events, prefix)
					if err != nil {
						return err
					}

					statuses, err := fetchIssueStatus(ctx, repo, []nostr.RelayEvent{evt})
					if err != nil {
						return err
					}

					return showThreadWithComments(ctx, repo.Relays, evt, statusLabelForEvent(evt.ID, statuses, false), nil)
				}
			},
			Commands: []*cli.Command{
				{
					Name:  "send",
					Usage: "edit and send a patch event (kind 1617)",
					Action: func(ctx context.Context, c *cli.Command) error {
						kr, _, err := gatherKeyerFromArguments(ctx, c)
						if err != nil {
							return fmt.Errorf("failed to gather keyer: %w", err)
						}

						repo, err := readGitRepositoryFromConfig()
						if err != nil {
							return err
						}

						if c.Args().Len() != 1 {
							return fmt.Errorf("must specify a commit to send as a patch, 'HEAD^' for the latest")
						}

						patchData, err := exec.Command("git", "format-patch", "--stdout", "--histogram", c.Args().First()).Output()
						if err != nil {
							stderr := ""
							if ee, ok := err.(*exec.ExitError); ok {
								stderr = strings.TrimSpace(string(ee.Stderr))
							}
							if stderr != "" {
								return fmt.Errorf("git format-patch failed: %s", stderr)
							}
							return fmt.Errorf("git format-patch failed: %w", err)
						}

						if len(patchData) == 0 {
							return fmt.Errorf("git format-patch returned empty output")
						}
						if len(patchData) > 10*1024 {
							return fmt.Errorf("patch too large: %d bytes (limit is 10240 bytes)", len(patchData))
						}

						content, err := editWithDefaultEditor(
							"nak-git-patch.patch",
							string(patchData),
							true,
						)
						if err != nil {
							return err
						}

						if strings.TrimSpace(content) == "" {
							return fmt.Errorf("empty patch content, aborting")
						}
						if len(content) > 10_000 {
							return fmt.Errorf("patch too large: %d bytes (limit is 10000 bytes)", len(content))
						}

						cmd := exec.Command("git", "apply", "--check", "--3way", "--whitespace=nowarn", "-")
						cmd.Stdin = strings.NewReader(content)

						if out, err := cmd.CombinedOutput(); err != nil {
							msg := strings.TrimSpace(string(out))
							if msg == "" {
								return fmt.Errorf("edited patch is not applicable")
							}
							return fmt.Errorf("edited patch is not applicable: %s", msg)
						}

						evt := nostr.Event{
							CreatedAt: nostr.Now(),
							Kind:      1617,
							Tags: nostr.Tags{
								nostr.Tag{"a", fmt.Sprintf("30617:%s:%s", repo.Event.PubKey.Hex(), repo.ID)},
								nostr.Tag{"p", repo.Event.PubKey.Hex()},
							},
							Content: content,
						}
						if repo.EarliestUniqueCommitID != "" {
							evt.Tags = append(evt.Tags, nostr.Tag{"r", repo.EarliestUniqueCommitID})
						}
						if err := kr.SignEvent(ctx, &evt); err != nil {
							return fmt.Errorf("failed to sign patch event: %w", err)
						}

						if err := confirmGitEventToBeSent(evt, repo.Relays, "send this patch event"); err != nil {
							return err
						}

						return publishGitEventToRepoRelays(ctx, evt, repo.Relays)
					},
				},
				{
					Name:      "reply",
					Usage:     "reply to a patch with a NIP-22 comment event",
					ArgsUsage: "<id-prefix>",
					Action: func(ctx context.Context, c *cli.Command) error {
						return gitDiscussionReply(ctx, c, 1617, "patch", patchSubjectPreview)
					},
				},
				{
					Name:      "close",
					Usage:     "close a patch by publishing a status event",
					ArgsUsage: "<id-prefix>",
					Flags: []cli.Flag{
						&cli.BoolFlag{
							Name:  "applied",
							Usage: "mark the patch as applied instead of closed",
						},
					},
					Action: func(ctx context.Context, c *cli.Command) error {
						return gitDiscussionClose(ctx, c, 1617, "patch", c.Bool("applied"))
					},
				},
				{
					Name:      "apply",
					Usage:     "apply a patch to current branch",
					ArgsUsage: "<id-prefix>",
					Flags: []cli.Flag{
						&cli.BoolFlag{
							Name:  "without-key",
							Usage: "apply patch without requiring a signer and skip status publication",
						},
					},
					Action: func(ctx context.Context, c *cli.Command) error {
						prefix := strings.TrimSpace(c.Args().First())
						if prefix == "" {
							return fmt.Errorf("missing patch id prefix")
						}

						repo, err := readGitRepositoryFromConfig()
						if err != nil {
							return err
						}

						var kr nostr.Keyer
						signerPubkey := nostr.ZeroPK
						if !c.Bool("without-key") {
							kr, _, err = gatherKeyerFromArguments(ctx, c)
							if err != nil {
								return fmt.Errorf("failed to gather keyer (or use --without-key): %w", err)
							}

							signerPubkey, err = ensureGitRepositoryOwner(ctx, kr, repo, "apply patches")
							if err != nil {
								return err
							}
						}

						patches, err := fetchGitRepoRelatedEvents(ctx, repo, 1617)
						if err != nil {
							return err
						}

						evt, err := findEventByPrefix(patches, prefix)
						if err != nil {
							return err
						}

						previousHead := ""
						if output, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
							previousHead = strings.TrimSpace(string(output))
						}

						// apply patch
						cmd := exec.Command("git", "am", "--3way")
						cmd.Stdin = strings.NewReader(evt.Content)
						cmd.Stdout = os.Stdout
						cmd.Stderr = os.Stderr
						if err := cmd.Run(); err != nil {
							return fmt.Errorf("failed to apply patch with git am: %w (if needed, run 'git am --abort')", err)
						}

						log("applied patch %s\n", color.GreenString(evt.ID.Hex()[:6]))

						appliedCommits := []string{}
						if previousHead != "" {
							if output, err := exec.Command("git", "rev-list", "--reverse", previousHead+"..HEAD").Output(); err == nil {
								for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
									commit := strings.TrimSpace(line)
									if commit != "" {
										appliedCommits = append(appliedCommits, commit)
									}
								}
							}
						}
						if len(appliedCommits) == 0 {
							if output, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
								commit := strings.TrimSpace(string(output))
								if commit != "" {
									appliedCommits = append(appliedCommits, commit)
								}
							}
						}

						if kr != nil {
							statusEvt := nostr.Event{
								CreatedAt: nostr.Now(),
								Kind:      1631,
								Tags: nostr.Tags{
									nostr.Tag{"e", evt.ID.Hex()},
									nostr.Tag{"a", fmt.Sprintf("30617:%s:%s", repo.Event.PubKey.Hex(), repo.ID)},
									nostr.Tag{"p", evt.PubKey.Hex()},
								},
							}

							if signerPubkey != repo.Event.PubKey {
								statusEvt.Tags = append(statusEvt.Tags, nostr.Tag{"p", repo.Event.PubKey.Hex()})
							}

							if len(appliedCommits) > 0 {
								tag := nostr.Tag{"applied-as-commits"}
								tag = append(tag, appliedCommits...)
								statusEvt.Tags = append(statusEvt.Tags, tag)
							}

							if err := kr.SignEvent(ctx, &statusEvt); err != nil {
								return fmt.Errorf("patch applied, but failed to sign applied status event: %w", err)
							}

							if err := publishGitEventToRepoRelays(ctx, statusEvt, repo.Relays); err != nil {
								return fmt.Errorf("patch applied, but failed to publish applied status event: %w", err)
							}
						}

						return nil
					},
				},
				{
					Name:      "pull",
					Usage:     "fetch a patch, apply it, and create refs/nostr/patch/<id> without touching the current branch",
					ArgsUsage: "<id-prefix>",
					Action: func(ctx context.Context, c *cli.Command) error {
						prefix := strings.TrimSpace(c.Args().First())
						if prefix == "" {
							return fmt.Errorf("missing patch id prefix")
						}

						repo, err := readGitRepositoryFromConfig()
						if err != nil {
							return err
						}

						patches, err := fetchGitRepoRelatedEvents(ctx, repo, 1617)
						if err != nil {
							return err
						}

						evt, err := findEventByPrefix(patches, prefix)
						if err != nil {
							return err
						}

						// we apply the patch to the current branch and then reset it back,
						// so a dirty worktree would be wiped out by the reset
						if output, err := exec.Command("git", "status", "--porcelain", "--untracked-files=no").Output(); err == nil &&
							len(strings.TrimSpace(string(output))) > 0 {
							return fmt.Errorf("working tree has uncommitted changes, commit or stash them before running 'patch pull'")
						}

						previousHead := ""
						if output, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
							previousHead = strings.TrimSpace(string(output))
						}

						cmd := exec.Command("git", "am", "--3way")
						cmd.Stdin = strings.NewReader(evt.Content)
						cmd.Stdout = os.Stdout
						cmd.Stderr = os.Stderr
						if err := cmd.Run(); err != nil {
							return fmt.Errorf("failed to apply patch with git am: %w (if needed, run 'git am --abort')", err)
						}

						newHead := ""
						if output, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
							newHead = strings.TrimSpace(string(output))
						}
						if newHead == "" || newHead == previousHead {
							return fmt.Errorf("patch did not create any new commits")
						}

						refName := fmt.Sprintf("refs/nostr/patch/%s", evt.ID.Hex())
						if err := exec.Command("git", "update-ref", refName, newHead).Run(); err != nil {
							return fmt.Errorf("patch applied but failed to create ref %s: %w", refName, err)
						}

						if previousHead != "" {
							if err := exec.Command("git", "reset", "--hard", previousHead).Run(); err != nil {
								return fmt.Errorf("patch applied and ref created, but failed to reset HEAD: %w", err)
							}
						}

						log("created ref %s at %s\n", color.GreenString(refName), color.CyanString(newHead))
						log("inspect it with %s or check it out with %s\n",
							color.CyanString("git log "+refName),
							color.CyanString(fmt.Sprintf("git checkout -b patch-%s %s", evt.ID.Hex()[:6], refName)),
						)
						return nil
					},
				},
			},
		},
		{
			Name:        "pr",
			Usage:       "pull-request-related operations",
			Description: "when called directly, lists open pull requests; with a pull request id prefix, displays that pull request with threaded discussions.",
			ArgsUsage:   "[id-prefix]",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:  "merged",
					Usage: "list only merged pull requests",
				},
				&cli.BoolFlag{
					Name:  "closed",
					Usage: "list only closed pull requests",
				},
				&cli.BoolFlag{
					Name:  "all",
					Usage: "list all pull requests, including merged and closed",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				repo, err := readGitRepositoryFromConfig()
				if err != nil {
					return err
				}

				events, err := fetchGitRepoRelatedEvents(ctx, repo, nostr.KindGitPullRequest)
				if err != nil {
					return err
				}

				prefix := strings.TrimSpace(c.Args().First())
				if prefix == "" {
					// list
					statuses, err := fetchIssueStatus(ctx, repo, events)
					if err != nil {
						return err
					}

					if len(events) == 0 {
						log("no pull requests found\n")
						return nil
					}

					showMerged := c.Bool("merged")
					showClosed := c.Bool("closed")
					showAll := c.Bool("all")

					// preload metadata from everybody
					wg := sync.WaitGroup{}
					for _, evt := range events {
						wg.Go(func() {
							sys.FetchProfileMetadata(ctx, evt.PubKey)
						})
					}
					wg.Wait()

					// now render
					for _, evt := range events {
						id := evt.ID.Hex()

						status := statusLabelForEvent(evt.ID, statuses, false)
						if !showAll {
							if showMerged || showClosed {
								isMerged := status == "applied/merged"
								isClosed := status == "closed"
								if !(showMerged && isMerged || showClosed && isClosed) {
									continue
								}
							} else if status == "applied/merged" || status == "closed" {
								continue
							}
						}

						date := evt.CreatedAt.Time().Format(time.DateOnly)
						subject := prSubjectPreview(evt, 72)
						statusDisplayText := status
						if status == "applied/merged" {
							statusDisplayText = "merged"
						}
						statusDisplay := colorizeGitStatus(statusDisplayText)

						stdout(color.CyanString(id[:6]), statusDisplay, color.HiBlackString(date), color.HiBlueString(authorPreview(ctx, evt.PubKey)), color.HiWhiteString(subject))
					}

					return nil
				} else {
					// view single
					evt, err := findEventByPrefix(events, prefix)
					if err != nil {
						return err
					}

					statuses, err := fetchIssueStatus(ctx, repo, []nostr.RelayEvent{evt})
					if err != nil {
						return err
					}

					return showPullRequestWithComments(ctx, repo, evt, statusLabelForEvent(evt.ID, statuses, false))
				}
			},
			Commands: []*cli.Command{
				{
					Name:        "send",
					Usage:       "create and send a pull request event (kind 1618)",
					ArgsUsage:   "[branch]",
					Description: "pushes the tip of the given branch (or the current branch) to refs/nostr/<event-id> on the repository's grasp servers, then publishes a kind 1618 pull request event pointing to it.",
					Action: func(ctx context.Context, c *cli.Command) error {
						kr, _, err := gatherKeyerFromArguments(ctx, c)
						if err != nil {
							return fmt.Errorf("failed to gather keyer: %w", err)
						}

						_, selfName, selfNpub, err := keyerIdentity(ctx, kr)
						if err != nil {
							return fmt.Errorf("failed to get current identity: %w", err)
						}

						// sync to set up remotes and fetch the latest metadata/state
						repo, state, err := gitSync(ctx, kr, false)
						if err != nil {
							return fmt.Errorf("failed to sync: %w", err)
						}

						if len(repo.Clone) == 0 {
							return fmt.Errorf("repository has no clone urls to host the pull request branch")
						}

						// figure out which branch to send
						localBranch, _, err := figureOutBranches(c, c.Args().First(), true)
						if err != nil {
							return err
						}

						res, err := exec.Command("git", "rev-parse", localBranch).Output()
						if err != nil {
							return fmt.Errorf("failed to get commit for branch %s: %w", localBranch, err)
						}
						tip := strings.TrimSpace(string(res))

						// best-effort merge-base against the repository's HEAD branch
						mergeBase := ""
						if state != nil && state.HEAD != "" {
							if targetCommit, ok := state.Branches[state.HEAD]; ok {
								mergeBase = gitMergeBase(tip, targetCommit)
							}
						}

						content, err := editWithDefaultEditor(
							"nak-git-pr/NOTES_EDITMSG",
							strings.TrimSpace(fmt.Sprintf(`# creating pull request as '%s' ('%s')
# on repository '%s'
# branch '%s' at commit %s
# the first line will be used as the pull request subject
my great feature

# the remaining lines are the pull request description (markdown)
please merge

# lines starting with '#' are ignored
`, selfName, selfNpub, repo.ID, localBranch, shortCommitID(tip, 8))),
							true,
						)
						if err != nil {
							return err
						}

						subject, body, err := parsePRCreateContent(content)
						if err != nil {
							return err
						}

						evt := nostr.Event{
							CreatedAt: nostr.Now(),
							Kind:      nostr.KindGitPullRequest,
							Tags: nostr.Tags{
								nostr.Tag{"a", fmt.Sprintf("30617:%s:%s", repo.Event.PubKey.Hex(), repo.ID)},
								nostr.Tag{"p", repo.Event.PubKey.Hex()},
								nostr.Tag{"subject", subject},
								nostr.Tag{"c", tip},
								nostr.Tag{"branch-name", localBranch},
							},
							Content: body,
						}
						if repo.EarliestUniqueCommitID != "" {
							evt.Tags = append(evt.Tags, nostr.Tag{"r", repo.EarliestUniqueCommitID})
						}
						evt.Tags = append(evt.Tags, append(nostr.Tag{"clone"}, repo.Clone...))
						if mergeBase != "" {
							evt.Tags = append(evt.Tags, nostr.Tag{"merge-base", mergeBase})
						}

						if err := kr.SignEvent(ctx, &evt); err != nil {
							return fmt.Errorf("failed to sign pull request event: %w", err)
						}

						if err := confirmGitEventToBeSent(evt, repo.Relays, "send this pull request"); err != nil {
							return err
						}

						// push the tip to refs/nostr/<pr-event-id> on the grasp remotes before publishing
						refName := "refs/nostr/" + evt.ID.Hex()
						if gitPushCommitToGraspRefs(repo, tip, refName, false) == 0 {
							return fmt.Errorf("failed to push pull request branch to any grasp remote; not publishing")
						}

						return publishGitEventToRepoRelays(ctx, evt, repo.Relays)
					},
				},
				{
					Name:        "update",
					Usage:       "update the tip of an existing pull request (kind 1619)",
					ArgsUsage:   "<id-prefix> [branch]",
					Description: "pushes the new tip to refs/nostr/<event-id> on the repository's grasp servers, then publishes a kind 1619 pull request update event.",
					Action: func(ctx context.Context, c *cli.Command) error {
						prefix := strings.TrimSpace(c.Args().First())
						if prefix == "" {
							return fmt.Errorf("missing pull request id prefix")
						}

						kr, _, err := gatherKeyerFromArguments(ctx, c)
						if err != nil {
							return fmt.Errorf("failed to gather keyer: %w", err)
						}

						repo, state, err := gitSync(ctx, kr, false)
						if err != nil {
							return fmt.Errorf("failed to sync: %w", err)
						}

						if len(repo.Clone) == 0 {
							return fmt.Errorf("repository has no clone urls to host the pull request branch")
						}

						prs, err := fetchGitRepoRelatedEvents(ctx, repo, nostr.KindGitPullRequest)
						if err != nil {
							return err
						}

						prEvt, err := findEventByPrefix(prs, prefix)
						if err != nil {
							return err
						}

						signerPk, err := kr.GetPublicKey(ctx)
						if err != nil {
							return fmt.Errorf("failed to get signer public key: %w", err)
						}
						if signerPk != prEvt.PubKey {
							return fmt.Errorf("only the pull request author (%s) can update it", nip19.EncodeNpub(prEvt.PubKey))
						}

						localBranch, _, err := figureOutBranches(c, c.Args().Get(1), true)
						if err != nil {
							return err
						}

						res, err := exec.Command("git", "rev-parse", localBranch).Output()
						if err != nil {
							return fmt.Errorf("failed to get commit for branch %s: %w", localBranch, err)
						}
						tip := strings.TrimSpace(string(res))

						mergeBase := ""
						if state != nil && state.HEAD != "" {
							if targetCommit, ok := state.Branches[state.HEAD]; ok {
								mergeBase = gitMergeBase(tip, targetCommit)
							}
						}

						evt := nostr.Event{
							CreatedAt: nostr.Now(),
							Kind:      nostr.KindGitPullRequestUpdate,
							Tags: nostr.Tags{
								nostr.Tag{"a", fmt.Sprintf("30617:%s:%s", repo.Event.PubKey.Hex(), repo.ID)},
								nostr.Tag{"p", repo.Event.PubKey.Hex()},
								nostr.Tag{"E", prEvt.ID.Hex(), prEvt.Relay.URL},
								nostr.Tag{"P", prEvt.PubKey.Hex()},
								nostr.Tag{"K", strconv.Itoa(int(nostr.KindGitPullRequest))},
								nostr.Tag{"c", tip},
							},
						}
						if repo.EarliestUniqueCommitID != "" {
							evt.Tags = append(evt.Tags, nostr.Tag{"r", repo.EarliestUniqueCommitID})
						}
						evt.Tags = append(evt.Tags, append(nostr.Tag{"clone"}, repo.Clone...))
						if mergeBase != "" {
							evt.Tags = append(evt.Tags, nostr.Tag{"merge-base", mergeBase})
						}

						if err := kr.SignEvent(ctx, &evt); err != nil {
							return fmt.Errorf("failed to sign pull request update event: %w", err)
						}

						if err := confirmGitEventToBeSent(evt, repo.Relays, "send this pull request update"); err != nil {
							return err
						}

						refName := "refs/nostr/" + evt.ID.Hex()
						if gitPushCommitToGraspRefs(repo, tip, refName, false) == 0 {
							return fmt.Errorf("failed to push updated pull request branch to any grasp remote; not publishing")
						}

						return publishGitEventToRepoRelays(ctx, evt, repo.Relays)
					},
				},
				{
					Name:      "reply",
					Usage:     "reply to a pull request with a NIP-22 comment event",
					ArgsUsage: "<id-prefix>",
					Action: func(ctx context.Context, c *cli.Command) error {
						return gitDiscussionReply(ctx, c, nostr.KindGitPullRequest, "pull request", prSubjectPreview)
					},
				},
				{
					Name:      "close",
					Usage:     "close a pull request by publishing a status event",
					ArgsUsage: "<id-prefix>",
					Flags: []cli.Flag{
						&cli.BoolFlag{
							Name:  "merged",
							Usage: "mark the pull request as merged instead of closed",
						},
					},
					Action: func(ctx context.Context, c *cli.Command) error {
						return gitDiscussionClose(ctx, c, nostr.KindGitPullRequest, "pull request", c.Bool("merged"))
					},
				},
				{
					Name:      "pull",
					Usage:     "fetch a pull request's tip into refs/nostr/pr/<id>",
					ArgsUsage: "<id-prefix>",
					Action: func(ctx context.Context, c *cli.Command) error {
						prefix := strings.TrimSpace(c.Args().First())
						if prefix == "" {
							return fmt.Errorf("missing pull request id prefix")
						}

						repo, err := readGitRepositoryFromConfig()
						if err != nil {
							return err
						}

						prs, err := fetchGitRepoRelatedEvents(ctx, repo, nostr.KindGitPullRequest)
						if err != nil {
							return err
						}

						prEvt, err := findEventByPrefix(prs, prefix)
						if err != nil {
							return err
						}

						refName, commit, err := gitFetchPullRequestIntoRef(ctx, repo, prEvt)
						if err != nil {
							return err
						}

						log("created ref %s at %s\n", color.GreenString(refName), color.CyanString(commit))
						log("inspect it with %s or check it out with %s\n",
							color.CyanString("git log "+refName),
							color.CyanString(fmt.Sprintf("git checkout -b pr-%s %s", prEvt.ID.Hex()[:6], refName)),
						)
						return nil
					},
				},
				{
					Name:      "merge",
					Usage:     "merge a pull request into the current branch and publish a merged status event",
					ArgsUsage: "<id-prefix>",
					Flags: []cli.Flag{
						&cli.BoolFlag{
							Name:  "without-key",
							Usage: "merge without requiring a signer and skip status publication",
						},
					},
					Action: func(ctx context.Context, c *cli.Command) error {
						prefix := strings.TrimSpace(c.Args().First())
						if prefix == "" {
							return fmt.Errorf("missing pull request id prefix")
						}

						repo, err := readGitRepositoryFromConfig()
						if err != nil {
							return err
						}

						var kr nostr.Keyer
						signerPubkey := nostr.ZeroPK
						if !c.Bool("without-key") {
							kr, _, err = gatherKeyerFromArguments(ctx, c)
							if err != nil {
								return fmt.Errorf("failed to gather keyer (or use --without-key): %w", err)
							}

							signerPubkey, err = ensureGitRepositoryOwner(ctx, kr, repo, "merge pull requests")
							if err != nil {
								return err
							}
						}

						prs, err := fetchGitRepoRelatedEvents(ctx, repo, nostr.KindGitPullRequest)
						if err != nil {
							return err
						}

						prEvt, err := findEventByPrefix(prs, prefix)
						if err != nil {
							return err
						}

						// fetch the pull request's tip into refs/nostr/pr/<id> (same as 'pr pull')
						refName, _, err := gitFetchPullRequestIntoRef(ctx, repo, prEvt)
						if err != nil {
							return err
						}

						previousHead := ""
						if output, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
							previousHead = strings.TrimSpace(string(output))
						}

						// merge the fetched ref into the current branch
						mergeMsg := fmt.Sprintf("Merge pull request %s", prEvt.ID.Hex()[:8])
						mergeCmd := exec.Command("git", "merge", "--no-ff", "-m", mergeMsg, refName)
						mergeCmd.Stdout = os.Stdout
						mergeCmd.Stderr = os.Stderr
						if err := mergeCmd.Run(); err != nil {
							return fmt.Errorf("failed to merge pull request: %w (if needed, run 'git merge --abort')", err)
						}

						mergeCommit := ""
						if output, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
							mergeCommit = strings.TrimSpace(string(output))
						}

						// 'git merge' exits 0 without creating a commit when the tip is
						// already contained in the current branch, don't publish a bogus
						// merged status in that case
						if mergeCommit == previousHead {
							return fmt.Errorf("nothing to merge, the pull request tip is already contained in the current branch")
						}

						log("merged pull request %s\n", color.GreenString(prEvt.ID.Hex()[:6]))

						appliedCommits := []string{}
						if previousHead != "" {
							if output, err := exec.Command("git", "rev-list", "--reverse", previousHead+"..HEAD").Output(); err == nil {
								for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
									commit := strings.TrimSpace(line)
									if commit != "" && commit != mergeCommit {
										appliedCommits = append(appliedCommits, commit)
									}
								}
							}
						}

						if kr != nil {
							statusEvt := nostr.Event{
								CreatedAt: nostr.Now(),
								Kind:      1631,
								Tags: nostr.Tags{
									nostr.Tag{"e", prEvt.ID.Hex()},
									nostr.Tag{"a", fmt.Sprintf("30617:%s:%s", repo.Event.PubKey.Hex(), repo.ID)},
									nostr.Tag{"p", prEvt.PubKey.Hex()},
								},
							}

							if signerPubkey != repo.Event.PubKey {
								statusEvt.Tags = append(statusEvt.Tags, nostr.Tag{"p", repo.Event.PubKey.Hex()})
							}

							if mergeCommit != "" {
								statusEvt.Tags = append(statusEvt.Tags, nostr.Tag{"merge-commit", mergeCommit})
							}

							if len(appliedCommits) > 0 {
								tag := nostr.Tag{"applied-as-commits"}
								tag = append(tag, appliedCommits...)
								statusEvt.Tags = append(statusEvt.Tags, tag)
							}

							if err := kr.SignEvent(ctx, &statusEvt); err != nil {
								return fmt.Errorf("pull request merged, but failed to sign merged status event: %w", err)
							}

							if err := publishGitEventToRepoRelays(ctx, statusEvt, repo.Relays); err != nil {
								return fmt.Errorf("pull request merged, but failed to publish merged status event: %w", err)
							}
						}

						return nil
					},
				},
			},
		},
		{
			Name:        "issue",
			Usage:       "issue-related operations",
			Description: "when called directly, lists open issues; with an issue id prefix, displays that issue with threaded discussions.",
			ArgsUsage:   "[id-prefix]",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:  "closed",
					Usage: "list only closed issues",
				},
				&cli.BoolFlag{
					Name:  "all",
					Usage: "list all issues, including closed",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				repo, err := readGitRepositoryFromConfig()
				if err != nil {
					return err
				}

				events, err := fetchGitRepoRelatedEvents(ctx, repo, 1621)
				if err != nil {
					return err
				}

				prefix := strings.TrimSpace(c.Args().First())
				if prefix == "" {
					// list
					statuses, err := fetchIssueStatus(ctx, repo, events)
					if err != nil {
						return err
					}

					if len(events) == 0 {
						log("no issues found\n")
						return nil
					}

					showClosed := c.Bool("closed")
					showAll := c.Bool("all")

					// preload metadata from everybody
					wg := sync.WaitGroup{}
					for _, evt := range events {
						wg.Go(func() {
							sys.FetchProfileMetadata(ctx, evt.PubKey)
						})
					}
					wg.Wait()

					// now render
					for _, evt := range events {
						id := evt.ID.Hex()
						status := statusLabelForEvent(evt.ID, statuses, true)
						if !showAll {
							if showClosed {
								if status != "closed" {
									continue
								}
							} else if status == "closed" {
								continue
							}
						}

						author := authorPreview(ctx, evt.PubKey)

						subject := issueSubjectPreview(evt, 72)
						date := evt.CreatedAt.Time().Format(time.DateOnly)
						stdout(color.CyanString(id[:6]), colorizeGitStatus(status), color.HiBlackString(date), color.HiBlueString(author), color.HiWhiteString(subject))
					}

					return nil
				} else {
					// view single
					evt, err := findEventByPrefix(events, prefix)
					if err != nil {
						return err
					}

					statuses, err := fetchIssueStatus(ctx, repo, []nostr.RelayEvent{evt})
					if err != nil {
						return err
					}

					return showThreadWithComments(ctx, repo.Relays, evt, statusLabelForEvent(evt.ID, statuses, true), nil)
				}
			},
			Commands: []*cli.Command{
				{
					Name:  "create",
					Usage: "edit and send an issue event (kind 1621)",
					Action: func(ctx context.Context, c *cli.Command) error {
						kr, _, err := gatherKeyerFromArguments(ctx, c)
						if err != nil {
							return fmt.Errorf("failed to gather keyer: %w", err)
						}

						_, selfName, selfNpub, err := keyerIdentity(ctx, kr)
						if err != nil {
							return fmt.Errorf("failed to get current identity: %w", err)
						}

						repo, err := readGitRepositoryFromConfig()
						if err != nil {
							return err
						}

						content, err := editWithDefaultEditor(
							"nak-git-issue/NOTES_EDITMSG",
							strings.TrimSpace(fmt.Sprintf(`# creating as '%s' ('%s')
# creating issue on repository '%s'
# the first line will be used as the issue subject
everything is broken

# the remaining lines will be the body
please fix

# lines starting with '#' are ignored
`, selfName, selfNpub, repo.ID)),
							true,
						)
						if err != nil {
							return err
						}

						subject, body, err := parseIssueCreateContent(content)
						if err != nil {
							return err
						}

						evt := nostr.Event{
							CreatedAt: nostr.Now(),
							Kind:      1621,
							Tags: nostr.Tags{
								nostr.Tag{"a", fmt.Sprintf("30617:%s:%s", repo.Event.PubKey.Hex(), repo.ID)},
								nostr.Tag{"p", repo.Event.PubKey.Hex()},
								nostr.Tag{"subject", subject},
							},
							Content: body,
						}
						if err := kr.SignEvent(ctx, &evt); err != nil {
							return fmt.Errorf("failed to sign issue event: %w", err)
						}

						if err := confirmGitEventToBeSent(evt, repo.Relays, "create this issue"); err != nil {
							return err
						}

						return publishGitEventToRepoRelays(ctx, evt, repo.Relays)
					},
				},
				{
					Name:      "reply",
					Usage:     "reply to an issue with a NIP-22 comment event",
					ArgsUsage: "<id-prefix>",
					Action: func(ctx context.Context, c *cli.Command) error {
						return gitDiscussionReply(ctx, c, 1621, "issue", issueSubjectPreview)
					},
				},
				{
					Name:      "close",
					Usage:     "close an issue by publishing a status event",
					ArgsUsage: "<id-prefix>",
					Action: func(ctx context.Context, c *cli.Command) error {
						return gitDiscussionClose(ctx, c, 1621, "issue", false)
					},
				},
			},
		},
		{
			Name:  "status",
			Usage: "show repository status and synchronization information",
			Action: func(ctx context.Context, c *cli.Command) error {
				// read local config
				localConfig, err := readNip34ConfigFile("")
				if err != nil {
					return fmt.Errorf("failed to read nip34.json: %w (run 'nak git init' first)", err)
				}

				// parse owner
				owner, err := parsePubKey(localConfig.Owner)
				if err != nil {
					return fmt.Errorf("invalid owner public key: %w", err)
				}

				repo := localConfig.ToRepository()
				stdout("\n" + color.CyanString("metadata:"))
				stdout("  identifier:", color.CyanString(repo.ID))
				stdout("  name:", color.CyanString(repo.Name))
				stdout("  owner:", color.CyanString(nip19.EncodeNpub(repo.Event.PubKey)))
				stdout("  description:", color.CyanString(repo.Description))
				stdout("  earliest unique commit:", color.CyanString(repo.EarliestUniqueCommitID))

				// fetch repository announcement and state from relays
				_, _, upToDateRelays, state, err := fetchRepositoryAndState(
					ctx, owner, localConfig.Identifier, localConfig.GraspServers)
				if err != nil {
					// create a local repo object for display purposes
					log("failed to fetch repository announcement from relays: %s\n", err)
				}

				stateHEAD := ""
				if state == nil {
					stdout(color.YellowString("\n repository state not published."))
				} else {
					stateHEAD = state.Branches[state.HEAD]
				}

				stdout("\n" + color.CyanString("grasp status:"))
				rows := make([][3]string, len(localConfig.GraspServers))
				for s, server := range localConfig.GraspServers {
					row := [3]string{}

					url := graspServerHost(server)
					row[0] = url

					upToDate := upToDateRelays != nil && slices.ContainsFunc(upToDateRelays, func(s string) bool { return graspServerHost(s) == url })
					if upToDate {
						row[1] = color.GreenString("announcement up-to-date")
					} else {
						row[1] = color.YellowString("announcement outdated")
					}

					if state != nil {
						remoteName := gitRemoteName(url)
						refSpec := fmt.Sprintf("refs/remotes/%s/HEAD", remoteName)
						lsRemoteCmd := exec.Command("git", "rev-parse", "--verify", refSpec)
						commitOutput, err := lsRemoteCmd.Output()
						if err != nil {
							row[2] = color.YellowString("repository not pushed")
						} else {
							commit := strings.TrimSpace(string(commitOutput))
							if commit == stateHEAD {
								row[2] = color.GreenString("repository synced with state")
							} else {
								short := func(s string) string {
									if len(s) > 5 {
										return s[0:5]
									}
									return s
								}
								row[2] = color.YellowString("mismatched HEAD state=%s, pushed=%s", short(stateHEAD), short(commit))
							}
						}
					}

					rows[s] = row
				}

				maxCol := [3]int{}
				for i := range maxCol {
					for _, row := range rows {
						if len(row[i]) > maxCol[i] {
							maxCol[i] = len(row[i])
						}
					}
				}
				for _, row := range rows {
					line := "  " + row[0] + strings.Repeat(" ", maxCol[0]-len(row[0])) + "   " + strings.Repeat(" ", maxCol[1]-len(row[1])) + row[1] + "   " + strings.Repeat(" ", maxCol[2]-len(row[2])) + row[2]
					stdout(line)
				}

				return nil
			},
		},
	},
}

func promptForStringList(
	name string,
	defaults []string,
	alternatives []string,
	normalize func(string) string,
	validate func(string) bool,
) ([]string, error) {
	options := make([]string, 0, len(defaults)+len(alternatives)+1)
	options = append(options, defaults...)

	// add existing not in options
	for _, item := range alternatives {
		if !slices.Contains(options, item) {
			options = append(options, item)
		}
	}

	options = append(options, "add another")

	selected := make([]string, len(defaults))
	copy(selected, defaults)

	for {
		newSelected := []string{}
		if err := survey.AskOne(&survey.MultiSelect{
			Message:  name,
			Options:  options,
			Default:  selected,
			PageSize: 20,
		}, &newSelected); err != nil {
			return nil, err
		}
		selected = newSelected

		if slices.Contains(selected, "add another") {
			selected = slices.DeleteFunc(selected, func(s string) bool { return s == "add another" })

			var newItem string
			if err := survey.AskOne(&survey.Input{
				Message: fmt.Sprintf("enter new %s", strings.TrimSuffix(name, "s")),
			}, &newItem); err != nil {
				return nil, err
			}

			if newItem != "" {
				if normalize != nil {
					newItem = normalize(newItem)
				}
				if validate != nil && !validate(newItem) {
					// invalid, ask again
					continue
				}

				if !slices.Contains(options, newItem) {
					options = append(options, newItem)
					// swap to put "add another" at end
					options[len(options)-1], options[len(options)-2] = options[len(options)-2], options[len(options)-1]
				}
				if !slices.Contains(selected, newItem) {
					selected = append(selected, newItem)
				}
			}
		} else {
			break
		}
	}

	return selected, nil
}

func readGitRepositoryFromConfig() (nip34.Repository, error) {
	localConfig, err := readNip34ConfigFile("")
	if err != nil {
		return nip34.Repository{}, err
	}

	repo := localConfig.ToRepository()
	if len(repo.Relays) == 0 {
		return nip34.Repository{}, fmt.Errorf("no relays configured in nip34.json")
	}

	return repo, nil
}

func confirmGitEventToBeSent(evt nostr.Event, relays []string, question string) error {
	pretty, err := json.MarshalIndent(evt, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode event for preview: %w", err)
	}

	stdout(string(pretty))
	stdout("relays:", strings.Join(relays, " "))

	if !askConfirmation(question + "? [y/n] ") {
		return fmt.Errorf("aborted")
	}

	return nil
}

func publishGitEventToRepoRelays(ctx context.Context, evt nostr.Event, relays []string) error {
	successes := make([]string, 0, len(relays))

	for res := range sys.Pool.PublishMany(ctx, relays, evt) {
		if res.Error != nil {
			log("! error publishing event to %s: %v\n", color.YellowString(res.RelayURL), res.Error)
		} else {
			log("> published to %s\n", color.GreenString(res.Relay.URL))
			successes = append(successes, res.Relay.URL)
		}
	}

	if len(successes) == 0 {
		return fmt.Errorf("failed to publish event to any relay")
	}

	nevent := nip19.EncodeNevent(evt.ID, successes, nostr.ZeroPK)
	log("event: %s\n", color.CyanString(nevent))
	return nil
}

func fetchGitRepoRelatedEvents(
	ctx context.Context,
	repo nip34.Repository,
	kind nostr.Kind,
) ([]nostr.RelayEvent, error) {
	events := make([]nostr.RelayEvent, 0, 30)
	for ie := range sys.Pool.FetchMany(ctx, repo.Relays, nostr.Filter{
		Kinds: []nostr.Kind{kind},
		Tags: nostr.TagMap{
			"a": []string{fmt.Sprintf("30617:%s:%s", repo.Event.PubKey.Hex(), repo.ID)},
		},
		Limit: 500,
	}, nostr.SubscriptionOptions{Label: "nak-git"}) {
		events = append(events, ie)
	}
	slices.SortFunc(events, nostr.CompareRelayEvent)
	return events, nil
}

func fetchIssueStatus(
	ctx context.Context,
	repo nip34.Repository,
	issues []nostr.RelayEvent,
) (map[nostr.ID]nostr.Event, error) {
	latest := make(map[nostr.ID]nostr.Event)
	eTags := make([]string, len(issues))
	for i, iss := range issues {
		eTags[i] = iss.ID.Hex()
	}

	for ie := range sys.Pool.FetchMany(ctx, repo.Relays, nostr.Filter{
		Kinds:   []nostr.Kind{1630, 1631, 1632, 1633},
		Tags:    nostr.TagMap{"e": eTags},
		Authors: []nostr.PubKey{repo.PubKey},
		Limit:   500,
	}, nostr.SubscriptionOptions{Label: "nak-git"}) {
		targetHex := ""
		for _, tag := range ie.Event.Tags {
			if len(tag) < 2 || tag[0] != "e" {
				continue
			}
			if targetHex == "" {
				targetHex = tag[1]
			}
			if len(tag) >= 4 && tag[3] == "root" {
				targetHex = tag[1]
				break
			}
		}
		if targetHex == "" {
			continue
		}
		targetID, err := nostr.IDFromHex(targetHex)
		if err != nil {
			continue
		}
		if prev, ok := latest[targetID]; !ok || ie.Event.CreatedAt > prev.CreatedAt {
			latest[targetID] = ie.Event
		}
	}

	return latest, nil
}

func gitDiscussionReply(
	ctx context.Context,
	c *cli.Command,
	discussionKind nostr.Kind,
	discussionName string,
	subjectPreview func(nostr.RelayEvent, int) string,
) error {
	prefix := strings.TrimSpace(c.Args().First())
	if prefix == "" {
		return fmt.Errorf("missing %s id prefix", discussionName)
	}

	kr, _, err := gatherKeyerFromArguments(ctx, c)
	if err != nil {
		return fmt.Errorf("failed to gather keyer: %w", err)
	}

	_, selfName, selfNpub, err := keyerIdentity(ctx, kr)
	if err != nil {
		return fmt.Errorf("failed to get current identity: %w", err)
	}

	repo, err := readGitRepositoryFromConfig()
	if err != nil {
		return err
	}

	discussions, err := fetchGitRepoRelatedEvents(ctx, repo, discussionKind)
	if err != nil {
		return err
	}

	discussionEvt, err := findEventByPrefix(discussions, prefix)
	if err != nil {
		return err
	}

	comments, err := fetchThreadComments(ctx, repo.Relays, discussionEvt.ID, nil)
	if err != nil {
		return err
	}

	subject := subjectPreview(discussionEvt, 72)
	if subject == "" {
		subject = "<untitled>"
	}
	pm := sys.FetchProfileMetadata(ctx, discussionEvt.PubKey)
	headerLines := []string{
		fmt.Sprintf("commenting as '%s' ('%s')", selfName, selfNpub),
		fmt.Sprintf("commenting on %s '%s' '%s' by '%s' ('%s') on repository '%s'", discussionName, discussionEvt.ID.Hex()[:6], subject, pm.ShortName(), pm.NpubShort(), repo.ID),
	}

	edited, err := editWithDefaultEditor(
		fmt.Sprintf("nak-git-%s-reply/NOTES_EDITMSG", discussionName),
		threadReplyEditorTemplate(ctx, headerLines, discussionEvt, comments),
		true,
	)
	if err != nil {
		return err
	}

	content, parentEvt, err := parseThreadReplyContent(discussionEvt, comments, edited)
	if err != nil {
		return err
	}

	if parentEvt.ID == discussionEvt.ID {
		log("> replying to %s %s (%s)\n",
			discussionName,
			color.CyanString(discussionEvt.ID.Hex()[:6]),
			color.HiWhiteString(subjectPreview(discussionEvt, 72)),
		)
	} else {
		log("> replying to comment %s by %s on %s %s\n",
			color.CyanString(parentEvt.ID.Hex()[:6]),
			color.HiBlueString(authorPreview(ctx, parentEvt.PubKey)),
			discussionName,
			color.CyanString(discussionEvt.ID.Hex()[:6]),
		)
	}

	evt := nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      1111,
		Tags: nostr.Tags{
			nostr.Tag{"E", discussionEvt.ID.Hex(), discussionEvt.Relay.URL},
			nostr.Tag{"e", parentEvt.ID.Hex(), parentEvt.Relay.URL},
			nostr.Tag{"P", discussionEvt.PubKey.Hex()},
			nostr.Tag{"p", parentEvt.PubKey.Hex()},
			nostr.Tag{"K", strconv.Itoa(int(discussionEvt.Kind))},
		},
		Content: content,
	}
	if err := kr.SignEvent(ctx, &evt); err != nil {
		return fmt.Errorf("failed to sign %s reply event: %w", discussionName, err)
	}
	if err := confirmGitEventToBeSent(evt, repo.Relays, fmt.Sprintf("send this %s reply", discussionName)); err != nil {
		return err
	}

	return publishGitEventToRepoRelays(ctx, evt, repo.Relays)
}

func gitDiscussionClose(
	ctx context.Context,
	c *cli.Command,
	discussionKind nostr.Kind,
	discussionName string,
	applied bool,
) error {
	prefix := strings.TrimSpace(c.Args().First())
	if prefix == "" {
		return fmt.Errorf("missing %s id prefix", discussionName)
	}

	kr, _, err := gatherKeyerFromArguments(ctx, c)
	if err != nil {
		return fmt.Errorf("failed to gather keyer: %w", err)
	}

	repo, err := readGitRepositoryFromConfig()
	if err != nil {
		return err
	}

	discussions, err := fetchGitRepoRelatedEvents(ctx, repo, discussionKind)
	if err != nil {
		return err
	}

	discussionEvt, err := findEventByPrefix(discussions, prefix)
	if err != nil {
		return err
	}

	signerPubkey, err := ensureGitRepositoryOwner(ctx, kr, repo, "close discussions")
	if err != nil {
		return err
	}

	statusKind := nostr.Kind(1632)
	statusLabel := "closed"
	if applied {
		statusKind = 1631
		statusLabel = "applied"
	}

	statusEvt := nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      statusKind,
		Tags: nostr.Tags{
			nostr.Tag{"e", discussionEvt.ID.Hex()},
			nostr.Tag{"a", fmt.Sprintf("30617:%s:%s", repo.Event.PubKey.Hex(), repo.ID)},
		},
	}

	if discussionEvt.PubKey != signerPubkey {
		statusEvt.Tags = append(statusEvt.Tags,
			nostr.Tag{"p", discussionEvt.PubKey.Hex()},
		)
	}

	if signerPubkey != repo.Event.PubKey {
		statusEvt.Tags = append(statusEvt.Tags, nostr.Tag{"p", repo.Event.PubKey.Hex()})
	}

	if err := kr.SignEvent(ctx, &statusEvt); err != nil {
		return fmt.Errorf("failed to sign %s %s status event: %w", discussionName, statusLabel, err)
	}

	if err := confirmGitEventToBeSent(statusEvt, repo.Relays, fmt.Sprintf("mark this %s as %s", discussionName, statusLabel)); err != nil {
		return err
	}

	if err := publishGitEventToRepoRelays(ctx, statusEvt, repo.Relays); err != nil {
		return fmt.Errorf("failed to publish %s %s status event: %w", discussionName, statusLabel, err)
	}

	log("marked %s %s as %s\n", discussionName, color.GreenString(discussionEvt.ID.Hex()[:6]), statusLabel)
	return nil
}

func ensureGitRepositoryOwner(ctx context.Context, kr nostr.Keyer, repo nip34.Repository, action string) (nostr.PubKey, error) {
	pubkey, err := kr.GetPublicKey(ctx)
	if err != nil {
		return nostr.ZeroPK, fmt.Errorf("failed to get signer public key: %w", err)
	}

	if pubkey != repo.Event.PubKey {
		return nostr.ZeroPK, fmt.Errorf("current user '%s' is not allowed to %s", nip19.EncodeNpub(pubkey), action)
	}

	return pubkey, nil
}

var (
	patchPrefixRe = regexp.MustCompile(`(?i)^\[patch[^\]]*\]\s*`)
	gitHashRe     = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
)

func patchSubjectPreview(evt nostr.RelayEvent, maxChars int) string {
	for _, line := range strings.Split(evt.Content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Subject:") {
			continue
		}

		subject := strings.TrimSpace(strings.TrimPrefix(line, "Subject:"))
		subject = strings.TrimSpace(patchPrefixRe.ReplaceAllString(subject, ""))
		if subject == "" {
			return ""
		}

		if maxChars <= 0 {
			return subject
		}

		runes := []rune(subject)
		if len(runes) <= maxChars {
			return subject
		}

		if maxChars <= 3 {
			return string(runes[:maxChars])
		}

		return string(runes[:maxChars-3]) + "..."
	}

	return ""
}

func issueSubjectPreview(evt nostr.RelayEvent, maxChars int) string {
	if tag := evt.Tags.Find("subject"); len(tag) >= 2 {
		subject := strings.TrimSpace(tag[1])
		if subject != "" {
			return clampWithEllipsis(subject, maxChars)
		}
	}

	for _, line := range strings.Split(evt.Content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return clampWithEllipsis(line, maxChars)
		}
	}

	return ""
}

// prSubjectPreview returns a short subject line for a pull request, using the
// "subject" tag when present and falling back to the first content line.
func prSubjectPreview(evt nostr.RelayEvent, maxChars int) string {
	return issueSubjectPreview(evt, maxChars)
}

// pullRequestTipAndClones returns the current tip commit, clone urls and the id
// of the event that carries that tip (the original pull request, or the latest
// kind 1619 update published by the pull request author). The source event id is
// where the tip was pushed as refs/nostr/<source-event-id> on the grasp servers.
func pullRequestTipAndClones(ctx context.Context, repo nip34.Repository, pr nostr.RelayEvent) (commit string, cloneURLs []string, sourceEventID nostr.ID) {
	collectClones := func(tags nostr.Tags) []string {
		var urls []string
		for _, tag := range tags {
			if len(tag) >= 2 && tag[0] == "clone" {
				urls = append(urls, tag[1:]...)
			}
		}
		return urls
	}

	if tag := pr.Tags.Find("c"); len(tag) >= 2 {
		commit = tag[1]
	}
	cloneURLs = collectClones(pr.Tags)
	sourceEventID = pr.ID

	latest := pr.CreatedAt
	for ie := range sys.Pool.FetchMany(ctx, repo.Relays, nostr.Filter{
		Kinds:   []nostr.Kind{nostr.KindGitPullRequestUpdate},
		Authors: []nostr.PubKey{pr.PubKey},
		Tags:    nostr.TagMap{"E": []string{pr.ID.Hex()}},
		Limit:   100,
	}, nostr.SubscriptionOptions{Label: "nak-git"}) {
		if ie.Event.CreatedAt <= latest {
			continue
		}
		if tag := ie.Event.Tags.Find("c"); len(tag) >= 2 {
			latest = ie.Event.CreatedAt
			commit = tag[1]
			sourceEventID = ie.Event.ID
			if updated := collectClones(ie.Event.Tags); len(updated) > 0 {
				cloneURLs = updated
			}
		}
	}

	return commit, cloneURLs, sourceEventID
}

// gitFetchPullRequestIntoRef fetches the current tip of the given pull request
// (following kind 1619 updates) into refs/nostr/pr/<pr-event-id> and returns the
// ref name and the tip commit. It tries the server ref refs/nostr/<source-event-id>
// first and falls back to fetching the bare commit hash.
func gitFetchPullRequestIntoRef(ctx context.Context, repo nip34.Repository, prEvt nostr.RelayEvent) (refName string, commit string, err error) {
	commit, cloneURLs, sourceEventID := pullRequestTipAndClones(ctx, repo, prEvt)
	if commit == "" {
		return "", "", fmt.Errorf("pull request %s has no tip commit", prEvt.ID.Hex()[:6])
	}
	if len(cloneURLs) == 0 {
		return "", "", fmt.Errorf("pull request %s has no clone urls to fetch from", prEvt.ID.Hex()[:6])
	}

	refName = fmt.Sprintf("refs/nostr/pr/%s", prEvt.ID.Hex())
	serverRef := "refs/nostr/" + sourceEventID.Hex()

	var lastErr error
	for _, url := range cloneURLs {
		if !strings.HasPrefix(url, "http") {
			continue
		}

		// first try fetching the server ref that send/update pushed the tip to,
		// which works even on servers that don't serve arbitrary commit hashes
		log("fetching %s from %s...\n", color.CyanString(serverRef), color.BlueString(url))
		fetchRefCmd := exec.Command("git", "fetch", url, fmt.Sprintf("+%s:%s", serverRef, refName))
		fetchRefCmd.Stderr = os.Stderr
		if err := fetchRefCmd.Run(); err == nil {
			if !gitHashRe.MatchString(commit) {
				return refName, commit, nil
			}

			// make sure the server actually served the tip declared in the signed event
			// (the declared tip may be abbreviated, so compare by prefix)
			if output, err := exec.Command("git", "rev-parse", refName).Output(); err == nil &&
				strings.HasPrefix(strings.TrimSpace(string(output)), commit) {
				return refName, commit, nil
			}

			// it points elsewhere, refuse it and try the declared commit directly below
			lastErr = fmt.Errorf("ref %s from %s doesn't match the declared tip %s", serverRef, url, shortCommitID(commit, 8))
		} else {
			lastErr = err
		}

		// fall back to fetching the bare commit hash (requires the server to
		// allow fetching arbitrary commits)
		if !gitHashRe.MatchString(commit) {
			continue
		}

		log("fetching commit %s from %s...\n", color.CyanString(shortCommitID(commit, 8)), color.BlueString(url))
		fetchCmd := exec.Command("git", "fetch", url, commit)
		fetchCmd.Stderr = os.Stderr
		if err := fetchCmd.Run(); err != nil {
			lastErr = err
			continue
		}

		// make sure the commit really arrived before creating the ref
		if err := exec.Command("git", "cat-file", "-e", commit).Run(); err != nil {
			lastErr = fmt.Errorf("commit %s not present after fetch from %s", shortCommitID(commit, 8), url)
			continue
		}

		if err := exec.Command("git", "update-ref", refName, commit).Run(); err != nil {
			return "", "", fmt.Errorf("fetched commit but failed to create ref %s: %w", refName, err)
		}

		return refName, commit, nil
	}

	if lastErr != nil {
		return "", "", fmt.Errorf("failed to fetch pull request commit from any clone url: %w", lastErr)
	}
	return "", "", fmt.Errorf("no usable (http) clone url found for pull request %s", prEvt.ID.Hex()[:6])
}

func showPullRequestWithComments(
	ctx context.Context,
	repo nip34.Repository,
	evt nostr.RelayEvent,
	status string,
) error {
	comments, err := fetchThreadComments(ctx, repo.Relays, evt.ID, nil)
	if err != nil {
		return err
	}

	printThreadMetadata(ctx, os.Stdout, evt, status, true)

	commit, cloneURLs, _ := pullRequestTipAndClones(ctx, repo, evt)
	if commit != "" {
		stdout(color.CyanString("tip commit:"), color.HiWhiteString(commit))
	}
	if tag := evt.Tags.Find("branch-name"); len(tag) >= 2 && tag[1] != "" {
		stdout(color.CyanString("branch:"), color.HiWhiteString(tag[1]))
	}
	if tag := evt.Tags.Find("merge-base"); len(tag) >= 2 && tag[1] != "" {
		stdout(color.CyanString("merge-base:"), color.HiWhiteString(tag[1]))
	}
	if len(cloneURLs) > 0 {
		stdout(color.CyanString("clone:"), color.HiWhiteString(strings.Join(cloneURLs, " ")))
	}

	stdout("")
	stdout(evt.Content)

	if len(comments) > 0 {
		stdout("")
		stdout(color.CyanString("comments:"))
		printThreadedComments(ctx, os.Stdout, comments, evt.ID, true)
	}

	return nil
}

// gitMergeBase returns the best common ancestor of two commits, or an empty
// string if it cannot be determined (this is always best-effort).
func gitMergeBase(a, b string) string {
	if a == "" || b == "" {
		return ""
	}
	out, err := exec.Command("git", "merge-base", a, b).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitPushCommitToGraspRefs pushes the given commit to refName on every grasp
// remote of the repository, returning the number of successful pushes.
func gitPushCommitToGraspRefs(repo nip34.Repository, commit string, refName string, force bool) int {
	successes := 0
	for _, relay := range repo.Relays {
		remoteName := gitRemoteName(nostr.NormalizeURL(relay))

		log("pushing %s to %s on %s...\n", color.CyanString(shortCommitID(commit, 8)), color.CyanString(refName), color.CyanString(remoteName))
		pushArgs := []string{"push"}
		if force {
			pushArgs = append(pushArgs, "--force")
		}
		pushArgs = append(pushArgs, remoteName, fmt.Sprintf("%s:%s", commit, refName))
		pushCmd := exec.Command("git", pushArgs...)
		pushCmd.Stderr = os.Stderr
		pushCmd.Stdout = os.Stdout
		if err := pushCmd.Run(); err != nil {
			log("! failed to push to %s: %v\n", color.YellowString(remoteName), err)
		} else {
			log("> pushed to %s\n", color.GreenString(remoteName))
			successes++
		}
	}
	return successes
}

func parseIssueCreateContent(content string) (subject string, body string, err error) {
	lines := strings.Split(content, "\n")
	var bodyb strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}

		if subject == "" {
			subject = line
			continue
		}

		bodyb.WriteString(line)
		bodyb.WriteByte('\n')
	}

	if subject == "" {
		return "", "", fmt.Errorf("issue subject cannot be empty")
	}

	body = strings.TrimSpace(bodyb.String())
	return subject, body, nil
}

func parsePRCreateContent(content string) (subject string, body string, err error) {
	subject, body, err = parseIssueCreateContent(content)
	if err != nil {
		return "", "", fmt.Errorf("pull request subject cannot be empty")
	}
	return subject, body, nil
}

func statusLabelForEvent(id nostr.ID, statuses map[nostr.ID]nostr.Event, isIssue bool) string {
	statusEvt, ok := statuses[id]
	if !ok {
		return "open"
	}

	switch statusEvt.Kind {
	case 1630:
		return "open"
	case 1631:
		return "applied/merged"
	case 1632:
		return "closed"
	case 1633:
		return "draft"
	default:
		return "open"
	}
}

func patchAppliedCommitPreview(statusEvt nostr.Event) string {
	if statusEvt.Kind != 1631 {
		return ""
	}

	if tag := statusEvt.Tags.Find("merge-commit"); len(tag) >= 2 {
		return shortCommitID(tag[1], 5)
	}

	for _, tag := range statusEvt.Tags {
		if len(tag) < 2 || tag[0] != "applied-as-commits" {
			continue
		}

		for i := 1; i < len(tag); i++ {
			if commit := shortCommitID(tag[i], 5); commit != "" {
				return commit
			}
		}
	}

	return ""
}

func shortCommitID(commit string, n int) string {
	commit = strings.TrimSpace(commit)
	if commit == "" || n <= 0 {
		return ""
	}
	if len(commit) <= n {
		return commit
	}
	return commit[:n]
}

func colorizeGitStatus(status string) string {
	switch status {
	case "open":
		return color.YellowString(status)
	case "resolved", "applied/merged", "merged":
		return color.GreenString(status)
	case "closed":
		return color.RedString(status)
	case "draft":
		return color.BlueString(status)
	default:
		return status
	}
}

func gitSync(ctx context.Context, signer nostr.Keyer, skipAnnouncement bool) (nip34.Repository, *nip34.RepositoryState, error) {
	// read current nip34.json
	localConfig, err := readNip34ConfigFile("")
	if err != nil {
		return nip34.Repository{}, nil, err
	}

	// parse owner
	owner, err := parsePubKey(localConfig.Owner)
	if err != nil {
		return nip34.Repository{}, nil, fmt.Errorf("invalid owner public key: %w", err)
	}

	// fetch repository announcement and state from relays
	repo, upToDateAnnouncementEvent, upToDateRelays, state, err := fetchRepositoryAndState(ctx, owner, localConfig.Identifier, localConfig.GraspServers)
	if !skipAnnouncement {
		notUpToDate := func(graspServer string) bool {
			return !slices.Contains(upToDateRelays, nostr.NormalizeURL(graspServer))
		}
		if upToDateRelays == nil || slices.ContainsFunc(localConfig.GraspServers, notUpToDate) {
			var relays []string
			if upToDateRelays == nil {
				// condition 1
				relays = append(sys.FetchOutboxRelays(ctx, owner, 3), localConfig.GraspServers...)
				log("couldn't fetch repository metadata (%s), will publish now\n", err)
			} else {
				// condition 2
				relays = make([]string, 0, len(localConfig.GraspServers)-1)
				for _, gs := range localConfig.GraspServers {
					if notUpToDate(gs) {
						relays = append(relays, graspServerHost(gs))
					}
				}
				log("some grasp servers (%v) are not up-to-date, will publish to them\n", relays)
			}
			var event nostr.Event
			if upToDateAnnouncementEvent != nil {
				// publish the latest event to the other relays
				event = *upToDateAnnouncementEvent
				repo = nip34.ParseRepository(event)
			} else {
				// create a local repository object from config and publish it
				localRepo := localConfig.ToRepository()
				if signer != nil {
					signerPk, err := signer.GetPublicKey(ctx)
					if err != nil {
						return repo, nil, fmt.Errorf("failed to get signer pubkey: %w", err)
					}
					if signerPk != owner {
						return repo, nil, fmt.Errorf("provided signer pubkey does not match owner, can't publish repository")
					} else {
						event = localRepo.ToEvent()
						if err := signer.SignEvent(ctx, &event); err != nil {
							return repo, state, fmt.Errorf("failed to sign announcement: %w", err)
						}
					}
				} else {
					return repo, nil, fmt.Errorf("no signer provided to publish repository (run 'nak git sync' with the '--sec' flag)")
				}

				repo = localRepo
			}

			for res := range sys.Pool.PublishMany(ctx, relays, event) {
				if res.Error != nil {
					log("! error publishing to %s: %v\n", color.YellowString(res.RelayURL), res.Error)
				} else {
					log("> published to %s\n", color.GreenString(res.RelayURL))
				}
			}
		} else {
			if err != nil {
				if _, ok := err.(StateErr); ok {
					// some error with the state, just do nothing and proceed
				} else {
					// actually fail with this error we don't know about
					return repo, nil, err
				}
			}

			// check if local config differs from remote announcement
			// construct local repo from config for comparison
			localRepo := localConfig.ToRepository()

			// nip34.json doesn't track web urls or maintainers, so carry them over
			// from the fetched announcement instead of stripping them on republish
			localRepo.Web = repo.Web
			localRepo.Maintainers = repo.Maintainers

			// check if we need to update local config or publish new announcement
			if !repo.Equals(localRepo) {
				// check modification times
				configPath := filepath.Join(findGitRoot(""), "nip34.json")
				if fi, err := os.Stat(configPath); err == nil {
					configModTime := fi.ModTime()
					announcementTime := repo.Event.CreatedAt.Time()

					if configModTime.After(announcementTime) {
						// local config is newer, publish new announcement if signer is available and matches owner
						if signer != nil {
							signerPk, err := signer.GetPublicKey(ctx)
							if err != nil {
								return repo, state, fmt.Errorf("failed to get signer pubkey: %w", err)
							}
							if signerPk != owner {
								log("local configuration is newer, but signer pubkey does not match owner, skipping announcement publish\n")
							} else {
								log("local configuration is newer, publishing updated repository announcement...\n")
								announcementEvent := localRepo.ToEvent()
								announcementEvent.CreatedAt = nostr.Timestamp(configModTime.Unix())
								if err := signer.SignEvent(ctx, &announcementEvent); err != nil {
									return repo, state, fmt.Errorf("failed to sign announcement: %w", err)
								}

								relays := append(sys.FetchOutboxRelays(ctx, owner, 3), localConfig.GraspServers...)
								for res := range sys.Pool.PublishMany(ctx, relays, announcementEvent) {
									if res.Error != nil {
										log("! error publishing to %s: %v\n", color.YellowString(res.RelayURL), res.Error)
									} else {
										log("> published to %s\n", color.GreenString(res.RelayURL))
									}
								}
								repo = nip34.ParseRepository(announcementEvent)
							}
						} else {
							log("local configuration is newer than remote, but no signer provided to publish update\n")
						}
					} else {
						// remote is newer, update local config
						log("remote announcement is newer than local, updating local configuration...\n")
						localConfig.Name = repo.Name
						localConfig.Description = repo.Description
						localConfig.EarliestUniqueCommit = repo.EarliestUniqueCommitID
						if err := writeNip34ConfigFile("", localConfig); err != nil {
							log("! failed to update local config: %v\n", err)
						}
					}
				}
			}
		}
	} else {
		if err != nil {
			if _, ok := err.(StateErr); ok {
				// some error with the state, just do nothing and proceed
			} else {
				// actually fail with this error we don't know about
				return repo, nil, err
			}
		}
	}

	// setup remotes
	gitSetupRemotes(ctx, "", repo)

	// fetch from each grasp remote
	fetchFromRemotes(ctx, "", repo)

	// update refs from state
	if state != nil {
		gitUpdateRefs(ctx, "", *state)
	}

	return repo, state, nil
}

func fetchFromRemotes(ctx context.Context, targetDir string, repo nip34.Repository) {
	// fetch from each grasp remote
	for _, grasp := range repo.Relays {
		remoteName := gitRemoteName(grasp)

		logverbose("fetching from %s...\n", remoteName)
		fetchCmd := exec.Command("git", "fetch", remoteName)
		if targetDir != "" {
			fetchCmd.Dir = targetDir
		}
		fetchCmd.Stderr = os.Stderr
		if err := fetchCmd.Run(); err != nil {
			logverbose("failed to fetch from %s: %v\n", remoteName, err)
		}
	}
}

func gitSetupRemotes(ctx context.Context, dir string, repo nip34.Repository) {
	// get list of all remotes
	listCmd := exec.Command("git", "remote")
	if dir != "" {
		listCmd.Dir = dir
	}
	output, err := listCmd.Output()
	if err != nil {
		logverbose("failed to list remotes: %v\n", err)
		return
	}

	// delete all nip34/grasp/ remotes that we don't have anymore in repo
	remotes := strings.Split(strings.TrimSpace(string(output)), "\n")
	for i, remote := range remotes {
		remote = strings.TrimSpace(remote)
		remotes[i] = remote

		if strings.HasPrefix(remote, "nip34/grasp/") {
			graspURL := rebuildGraspURLFromRemote(remote)

			getUrlCmd := exec.Command("git", "remote", "get-url", remote)
			if dir != "" {
				getUrlCmd.Dir = dir
			}
			if output, err := getUrlCmd.Output(); err != nil {
				panic(fmt.Errorf("failed to read remote (%s) url from git: %s", remote, err))
			} else {
				// check if the remote url is correct so we can update it if not
				gitURL := fmt.Sprintf("http%s/%s/%s.git", nostr.NormalizeURL(graspURL)[2:], nip19.EncodeNpub(repo.PubKey), repo.ID)
				if strings.TrimSpace(string(output)) != gitURL {
					goto delete
				}
			}

			// check if this remote is not present in our grasp list anymore
			if !slices.Contains(repo.Relays, nostr.NormalizeURL(graspURL)) {
				goto delete
			}

			continue

		delete:
			logverbose("deleting remote %s\n", remote)
			delCmd := exec.Command("git", "remote", "remove", remote)
			if dir != "" {
				delCmd.Dir = dir
			}
			if err := delCmd.Run(); err != nil {
				logverbose("failed to remove remote %s: %v\n", remote, err)
			}
		}
	}

	// create new remotes for each grasp server
	remotes = strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, relay := range repo.Relays {
		remote := gitRemoteName(relay)
		gitURL := fmt.Sprintf("http%s/%s/%s.git", nostr.NormalizeURL(relay)[2:], nip19.EncodeNpub(repo.PubKey), repo.ID)

		if slices.Contains(remotes, remote) {
			continue
		}

		logverbose("adding new remote for '%s'\n", relay)
		addCmd := exec.Command("git", "remote", "add", remote, gitURL)
		if dir != "" {
			addCmd.Dir = dir
		}
		if out, err := addCmd.Output(); err != nil {
			var stderr string
			if exiterr, ok := err.(*exec.ExitError); ok {
				stderr = string(exiterr.Stderr)
			}
			logverbose("failed to add remote %s: %s %s\n", remote, stderr, string(out))
		}
	}
}

func gitUpdateRefs(ctx context.Context, dir string, state nip34.RepositoryState) {
	// delete all existing nip34/state refs
	showRefCmd := exec.Command("git", "show-ref")
	if dir != "" {
		showRefCmd.Dir = dir
	}
	output, err := showRefCmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			parts := strings.Fields(line)
			if len(parts) >= 2 && strings.Contains(parts[1], "refs/heads/nip34/state/") {
				delCmd := exec.Command("git", "update-ref", "-d", parts[1])
				if dir != "" {
					delCmd.Dir = dir
				}
				delCmd.Run()
			}
		}
	}

	// create refs for each branch in state
	for branchName, commit := range state.Branches {
		// skip non-refs branches
		if !strings.HasPrefix(branchName, "refs/") {
			branchName = "refs/heads/" + branchName
		}

		refName := "refs/heads/nip34/state/" + strings.TrimPrefix(branchName, "refs/heads/")
		updateCmd := exec.Command("git", "update-ref", refName, commit)
		if dir != "" {
			updateCmd.Dir = dir
		}
		if err := updateCmd.Run(); err != nil {
			logverbose("failed to update ref %s: %v\n", refName, err)
		}
	}

	// create ref for HEAD
	if state.HEAD != "" {
		if headCommit, ok := state.Branches[state.HEAD]; ok {
			headRefName := "refs/heads/nip34/state/HEAD"
			updateCmd := exec.Command("git", "update-ref", headRefName, headCommit)
			if dir != "" {
				updateCmd.Dir = dir
			}
			if err := updateCmd.Run(); err != nil {
				logverbose("failed to update HEAD ref: %v\n", err)
			}
		}
	}
}

func fetchRepositoryAndState(
	ctx context.Context,
	pubkey nostr.PubKey,
	identifier string,
	relayHints []string,
) (repo nip34.Repository, upToDateAnnouncementEvent *nostr.Event, upToDateRelays []string, state *nip34.RepositoryState, err error) {
	// fetch repository announcement (30617)
	relays := nostr.AppendUnique(relayHints, sys.FetchOutboxRelays(ctx, pubkey, 3)...)
	for ie := range sys.Pool.FetchMany(ctx, relays, nostr.Filter{
		Kinds:   []nostr.Kind{30617},
		Authors: []nostr.PubKey{pubkey},
		Tags: nostr.TagMap{
			"d": []string{identifier},
		},
		Limit: 2,
	}, nostr.SubscriptionOptions{
		Label: "nak-git",
		CheckDuplicate: func(id nostr.ID, relay string) bool {
			return false
		},
	}) {
		if ie.Event.CreatedAt > repo.CreatedAt {
			repo = nip34.ParseRepository(ie.Event)

			// reset this list as the previous was for relays with the older version
			upToDateRelays = []string{ie.Relay.URL}

			upToDateAnnouncementEvent = &ie.Event
		} else if ie.Event.CreatedAt == repo.CreatedAt {
			// we discard this because it's the same, but this relay is up-to-date
			upToDateRelays = append(upToDateRelays, ie.Relay.URL)
		}
	}
	if repo.Event.ID == nostr.ZeroID {
		return repo, nil, upToDateRelays, state, fmt.Errorf("no repository announcement (kind 30617) found for %s", identifier)
	}

	// fetch repository state (30618)
	var stateErr *StateErr
	for ie := range sys.Pool.FetchMany(ctx, repo.Relays, nostr.Filter{
		Kinds:   []nostr.Kind{30618},
		Authors: []nostr.PubKey{pubkey},
		Tags: nostr.TagMap{
			"d": []string{identifier},
		},
		Limit: 2,
	}, nostr.SubscriptionOptions{Label: "nak-git"}) {
		if state == nil || ie.Event.CreatedAt > state.CreatedAt {
			state_ := nip34.ParseRepositoryState(ie.Event)

			if state_.HEAD == "" {
				stateErr = &StateErr{"state is missing HEAD"}
				continue
			}
			if _, ok := state_.Branches[state_.HEAD]; !ok {
				stateErr = &StateErr{fmt.Sprintf("state is missing commit for HEAD branch '%s'", state_.HEAD)}
				continue
			}

			stateErr = nil
			state = &state_
		}
	}
	if stateErr != nil {
		return repo, upToDateAnnouncementEvent, upToDateRelays, state, stateErr
	}

	return repo, upToDateAnnouncementEvent, upToDateRelays, state, nil
}

type StateErr struct{ string }

func (s StateErr) Error() string { return string(s.string) }

func findGitRoot(startDir string) string {
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return ""
		}
	}

	// make absolute
	if !filepath.IsAbs(startDir) {
		if abs, err := filepath.Abs(startDir); err == nil {
			startDir = abs
		}
	}

	currentDir := startDir
	for {
		gitDir := filepath.Join(currentDir, ".git")
		if fi, err := os.Stat(gitDir); err == nil {
			if fi.IsDir() {
				return currentDir
			}
			// .git might be a file (for submodules/worktrees)
			return currentDir
		}

		// move to parent directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// reached root without finding .git
			return ""
		}
		currentDir = parentDir
	}
}

func readNip34ConfigFile(baseDir string) (Nip34Config, error) {
	var localConfig Nip34Config

	// find git root
	gitRoot := findGitRoot(baseDir)
	if gitRoot == "" {
		return localConfig, fmt.Errorf("not in a git repository")
	}

	data, err := os.ReadFile(filepath.Join(gitRoot, "nip34.json"))
	if err != nil {
		return localConfig, fmt.Errorf("failed to read nip34.json: %w (run 'nak git init' first)", err)
	}
	if err := json.Unmarshal(data, &localConfig); err != nil {
		return localConfig, fmt.Errorf("failed to parse nip34.json: %w", err)
	}

	// normalize grasp relay URLs
	for i := range localConfig.GraspServers {
		localConfig.GraspServers[i] = graspServerHost(localConfig.GraspServers[i])
	}

	if err := localConfig.Validate(); err != nil {
		return localConfig, fmt.Errorf("nip34.json is invalid: %w", err)
	}

	return localConfig, nil
}

func excludeNip34ConfigFile(baseDir string) {
	// find git root
	gitRoot := findGitRoot(baseDir)
	if gitRoot == "" {
		log(color.YellowString("not in a git repository, skipping exclude\n"))
		return
	}

	excludePath := filepath.Join(gitRoot, ".git", "info", "exclude")
	excludeContent, err := os.ReadFile(excludePath)
	if err != nil {
		// file doesn't exist, create it
		excludeContent = []byte("")
	}

	// check if nip34.json is already in exclude
	if !strings.Contains(string(excludeContent), "nip34.json") {
		newContent := string(excludeContent)
		if len(newContent) > 0 && !strings.HasSuffix(newContent, "\n") {
			newContent += "\n"
		}
		newContent += "nip34.json\n"
		if err := os.WriteFile(excludePath, []byte(newContent), 0644); err != nil {
			log(color.YellowString("failed to add nip34.json to .git/info/exclude: %v\n", err))
		} else {
			log("added nip34.json to %s\n", color.GreenString(".git/info/exclude"))
		}
	}
}

func writeNip34ConfigFile(baseDir string, cfg Nip34Config) error {
	// find git root (or use baseDir if it doesn't have .git yet, for initial setup)
	gitRoot := findGitRoot(baseDir)
	if gitRoot == "" {
		// not in a git repo yet, use the provided baseDir
		if baseDir == "" {
			var err error
			baseDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}
		}
		gitRoot = baseDir
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal nip34.json: %w", err)
	}

	configPath := filepath.Join(gitRoot, "nip34.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", configPath, err)
	}

	return nil
}

func parseRepositoryAddress(
	ctx context.Context,
	address string,
) (owner nostr.PubKey, identifier string, relayHints []string, err error) {
	// format 1: naddr1... (NIP-19 address pointer)
	if strings.HasPrefix(address, "naddr1") {
		prefix, data, err := nip19.Decode(address)
		if err != nil {
			return nostr.PubKey{}, "", nil, fmt.Errorf("invalid naddr: %w", err)
		}
		if prefix != "naddr" {
			return nostr.PubKey{}, "", nil, fmt.Errorf("expected naddr, got %s", prefix)
		}
		ptr := data.(nostr.EntityPointer)
		return ptr.PublicKey, ptr.Identifier, ptr.Relays, nil
	}

	// format 2: nostr://<npub_or_nip05>/<relay_hostname>/<identifier> (ngit-style)
	// format 2b: nostr://<npub_or_nip05>/<identifier> (without relay)
	if strings.HasPrefix(address, "nostr://") {
		parts := strings.Split(address, "/")
		if len(parts) == 5 {
			// nostr://<owner>/<relay>/<identifier>
			owner, err = parsePubKey(parts[2])
			if err != nil {
				return nostr.PubKey{}, "", nil, fmt.Errorf("invalid owner in URL: %w", err)
			}

			relayHost, err := url.PathUnescape(parts[3])
			if err != nil {
				return nostr.PubKey{}, "", nil, fmt.Errorf("invalid relay in URL: %w", err)
			}
			identifier, err = url.PathUnescape(parts[4])
			if err != nil {
				return nostr.PubKey{}, "", nil, fmt.Errorf("invalid identifier in URL: %w", err)
			}

			if strings.HasPrefix(relayHost, "wss:") || strings.HasPrefix(relayHost, "ws:") {
				relayHints = []string{relayHost}
			} else {
				relayHints = []string{"wss://" + relayHost}
			}

			return owner, identifier, relayHints, nil
		} else if len(parts) == 4 {
			// nostr://<owner>/<identifier>
			owner, err = parsePubKey(parts[2])
			if err != nil {
				return nostr.PubKey{}, "", nil, fmt.Errorf("invalid owner in URL: %w", err)
			}

			identifier, err = url.PathUnescape(parts[3])
			if err != nil {
				return nostr.PubKey{}, "", nil, fmt.Errorf("invalid identifier in URL: %w", err)
			}
			return owner, identifier, nil, nil
		} else {
			return nostr.PubKey{}, "", nil, fmt.Errorf(
				"invalid nostr URL format, expected nostr://<npub|nip05>/<identifier> or nostr://<npub|nip05>/<relay>/<identifier>, got: %s", address,
			)
		}
	}

	// format 3: <npub, hex, nprofile or nip05>/<identifier>
	parts := strings.SplitN(address, "/", 2)
	if len(parts) != 2 {
		return nostr.PubKey{}, "", nil, fmt.Errorf(
			"invalid repository address format, expected <npub|hex|nprofile|nip05>/<identifier>, got: %s", address,
		)
	}

	ownerPart := parts[0]
	identifier = parts[1]

	// try to parse as pubkey (npub, nprofile, or hex)
	owner, err = parsePubKey(ownerPart)
	if err != nil {
		return nostr.PubKey{}, "", nil, fmt.Errorf("invalid owner identifier '%s': %w", ownerPart, err)
	}

	// if it was an nprofile, extract relays
	if strings.HasPrefix(ownerPart, "nprofile") {
		if _, data, err := nip19.Decode(ownerPart); err == nil {
			if profile, ok := data.(nostr.ProfilePointer); ok {
				relayHints = profile.Relays
			}
		}
	}

	return owner, identifier, relayHints, nil
}

func figureOutBranches(c *cli.Command, refspec string, isPush bool) (
	localBranch string,
	remoteBranch string,
	err error,
) {
	var src, dst string

	// parse refspec if provided
	if refspec != "" && strings.Contains(refspec, ":") {
		parts := strings.Split(refspec, ":")
		if len(parts) == 2 {
			src = parts[0]
			dst = parts[1]
		} else {
			return "", "", fmt.Errorf("invalid branch spec: %s", refspec)
		}
	} else if refspec != "" {
		src = refspec
	}

	// assign src/dst to local/remote based on push vs pull
	if isPush {
		if src != "" {
			localBranch = src
		}
		if dst != "" {
			remoteBranch = dst
		}
	} else {
		if src != "" {
			remoteBranch = src
		}
		if dst != "" {
			localBranch = dst
		}
	}

	// get current branch if not specified
	if localBranch == "" {
		cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		output, err := cmd.Output()
		if err != nil {
			return "", "", fmt.Errorf("failed to get current branch: %w", err)
		}
		localBranch = strings.TrimSpace(string(output))
	}

	// get the remote branch from git config if not specified
	if remoteBranch == "" {
		cmd := exec.Command("git", "config", "--get", fmt.Sprintf("branch.%s.merge", localBranch))
		output, err := cmd.Output()
		if err == nil {
			// parse refs/heads/<branch-name> to get just the branch name
			mergeRef := strings.TrimSpace(string(output))
			if strings.HasPrefix(mergeRef, "refs/heads/") {
				remoteBranch = strings.TrimPrefix(mergeRef, "refs/heads/")
			} else {
				remoteBranch = mergeRef
			}
		}

		if remoteBranch == "" {
			// no upstream configured, assume same branch name
			remoteBranch = localBranch
		}
	}

	return localBranch, remoteBranch, nil
}

type Nip34Config struct {
	Identifier           string   `json:"identifier"`
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Owner                string   `json:"owner"`
	GraspServers         []string `json:"grasp-servers"`
	EarliestUniqueCommit string   `json:"earliest-unique-commit"`
}

func RepositoryToConfig(repo nip34.Repository) Nip34Config {
	config := Nip34Config{
		Identifier:           repo.ID,
		Name:                 repo.Name,
		Description:          repo.Description,
		Owner:                nip19.EncodeNpub(repo.Event.PubKey),
		GraspServers:         make([]string, 0, len(repo.Relays)),
		EarliestUniqueCommit: repo.EarliestUniqueCommitID,
	}
	for _, r := range repo.Relays {
		config.GraspServers = append(config.GraspServers, graspServerHost(r))
	}
	return config
}

func (localConfig Nip34Config) Validate() error {
	_, err := parsePubKey(localConfig.Owner)
	if err != nil {
		return fmt.Errorf("owner pubkey '%s' is not valid: %w", localConfig.Owner, err)
	}
	return nil
}

func (localConfig Nip34Config) ToRepository() nip34.Repository {
	owner, err := parsePubKey(localConfig.Owner)
	if err != nil {
		panic(err)
	}

	localRepo := nip34.Repository{
		ID:                     localConfig.Identifier,
		Name:                   localConfig.Name,
		Description:            localConfig.Description,
		EarliestUniqueCommitID: localConfig.EarliestUniqueCommit,
		Event: nostr.Event{
			PubKey: owner,
		},
	}
	for _, server := range localConfig.GraspServers {
		graspServerURL := nostr.NormalizeURL(server)
		url := fmt.Sprintf("http%s/%s/%s.git",
			graspServerURL[2:], nip19.EncodeNpub(localRepo.PubKey), localConfig.Identifier)
		localRepo.Clone = append(localRepo.Clone, url)
		localRepo.Relays = append(localRepo.Relays, graspServerURL)
	}

	return localRepo
}

func gitRemoteName(graspURL string) string {
	host := graspServerHost(graspURL)
	host = strings.Replace(host, ":", "__", 1)
	return "nip34/grasp/" + host
}

func rebuildGraspURLFromRemote(remoteName string) string {
	host := strings.TrimPrefix(remoteName, "nip34/grasp/")
	return strings.Replace(host, "__", ":", 1)
}

func graspServerHost(s string) string {
	// NormalizeURL returns "" for empty or unparseable inputs
	parts := strings.SplitN(nostr.NormalizeURL(s), "/", 3)
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}

// resolveGitNaturalURLs turns a repository argument into a list of git http(s)
// urls suitable for the gitnaturalapi functions. the argument may be a plain
// git http(s) url or a repository address as accepted by 'nak git clone'.
func resolveGitNaturalURLs(ctx context.Context, repoArg string) (gitURLs []string, state *nip34.RepositoryState, err error) {
	if strings.HasPrefix(repoArg, "http://") || strings.HasPrefix(repoArg, "https://") {
		return []string{strings.TrimRight(repoArg, "/")}, nil, nil
	}

	owner, identifier, relayHints, err := parseRepositoryAddress(ctx, repoArg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse repository address '%s': %w", repoArg, err)
	}

	repo, _, _, state, err := fetchRepositoryAndState(ctx, owner, identifier, relayHints)
	if err != nil {
		var stateErr *StateErr
		if !errors.As(err, &stateErr) {
			return nil, nil, err
		}
	}

	for _, url := range repo.Clone {
		if strings.HasPrefix(url, "http") {
			gitURLs = append(gitURLs, url)
		}
	}

	return gitURLs, state, nil
}

// resolveGitNaturalRef resolves a user-supplied ref (or the repository's HEAD,
// possibly from its published state) into a commit hash for a given git url.
func resolveGitNaturalRef(url string, ref string, state *nip34.RepositoryState) (string, error) {
	var info *gitnaturalapi.InfoRefsUploadPackResponse

	if ref == "" && state != nil && state.HEAD != "" {
		ref = state.HEAD
	}

	if ref != "" && !gitHashRe.MatchString(ref) {
		var err error
		info, err = gitnaturalapi.GetInfoRefs(url)
		if err != nil {
			return "", err
		}
	}

	var commit string
	switch {
	case gitHashRe.MatchString(ref):
		commit = ref
	case strings.HasPrefix(ref, "refs/"):
		if info != nil {
			commit = info.Refs[ref]
		}
	default:
		if info == nil {
			var err error
			info, err = gitnaturalapi.GetInfoRefs(url)
			if err != nil {
				return "", err
			}
		}
		if ref == "" {
			if symref, ok := info.Symrefs["HEAD"]; ok && symref != "" {
				commit, _ = info.Refs[symref]
			} else if head, ok := info.Refs["HEAD"]; ok && head != "" {
				commit = head
			}
		} else if ch, ok := info.Refs["refs/heads/"+ref]; ok {
			commit = ch
		} else if ch, ok := info.Refs["refs/tags/"+ref]; ok {
			commit = ch
		} else if sr, ok := info.Symrefs[ref]; ok {
			commit = info.Refs[sr]
		}
	}

	if commit == "" {
		return "", fmt.Errorf("couldn't resolve ref '%s'", ref)
	}
	if !gitHashRe.MatchString(commit) {
		return "", fmt.Errorf("invalid commit hash for ref '%s': '%s'", ref, commit)
	}

	return commit, nil
}

// gitTreeAtPath navigates a fully loaded git tree to the directory at the given
// path, failing if any segment is a file or doesn't exist.
func gitTreeAtPath(tree *gitnaturalapi.Tree, path string) (*gitnaturalapi.Tree, error) {
	path = strings.Trim(path, "/")
	if path == "" {
		return tree, nil
	}

	for _, segment := range strings.Split(path, "/") {
		found := false
		for _, dir := range tree.Directories {
			if dir.Name == segment {
				if dir.Content == nil {
					return nil, fmt.Errorf("directory '%s' not found in fetched tree", path)
				}
				tree = dir.Content
				found = true
				break
			}
		}
		if !found {
			for _, file := range tree.Files {
				if file.Name == segment {
					return nil, fmt.Errorf("path '%s' is a file, not a directory", path)
				}
			}
			return nil, fmt.Errorf("path '%s' not found", path)
		}
	}

	return tree, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i != -1 {
		return s[:i]
	}
	return s
}
