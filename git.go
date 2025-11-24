package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nip34"
	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

var git = &cli.Command{
	Name:  "git",
	Usage: "git-related operations",
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
				&cli.StringSliceFlag{
					Name:  "web",
					Usage: "web URLs for the repository (can be used multiple times)",
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
				&cli.StringSliceFlag{
					Name:  "maintainers",
					Usage: "maintainer public keys as npub or hex (can be used multiple times)",
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

				// check if nip34.json already exists
				existingConfig, err := readNip34ConfigFile("")
				if err == nil {
					// file exists
					if !c.Bool("force") && !c.Bool("interactive") {
						return fmt.Errorf("nip34.json already exists, use --force to overwrite or --interactive to update")
					}
				}

				// get repository base directory name for defaults
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("failed to get current directory: %w", err)
				}
				baseName := filepath.Base(cwd)

				// get earliest unique commit
				var earliestCommit string
				if output, err := exec.Command("git", "rev-list", "--max-parents=0", "HEAD").Output(); err == nil {
					earliest := strings.Split(strings.TrimSpace(string(output)), "\n")
					if len(earliest) > 0 {
						earliestCommit = earliest[0]
					}
				}

				// extract clone URLs from nostr:// git remotes
				// (this is just for migrating from ngit)
				var defaultCloneURLs []string
				if output, err := exec.Command("git", "remote", "-v").Output(); err == nil {
					remotes := strings.Split(strings.TrimSpace(string(output)), "\n")
					for _, remote := range remotes {
						if strings.Contains(remote, "nostr://") {
							parts := strings.Fields(remote)
							if len(parts) >= 2 {
								nostrURL := parts[1]
								// parse nostr://npub.../relay_hostname/identifier
								if owner, identifier, relays, err := parseRepositoryAddress(ctx, nostrURL); err == nil && len(relays) > 0 {
									relayURL := relays[0]
									// convert to https://relay_hostname/npub.../identifier.git
									cloneURL := fmt.Sprintf("http%s/%s/%s.git",
										relayURL[2:], nip19.EncodeNpub(owner), identifier)
									defaultCloneURLs = appendUnique(defaultCloneURLs, cloneURL)
								}
							}
						}
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

				config := Nip34Config{
					Identifier:           getValue(existingConfig.Identifier, c.String("identifier"), baseName),
					Name:                 getValue(existingConfig.Name, c.String("name"), baseName),
					Description:          getValue(existingConfig.Description, c.String("description"), ""),
					Web:                  getSliceValue(existingConfig.Web, c.StringSlice("web"), []string{}),
					Owner:                getValue(existingConfig.Owner, c.String("owner"), ""),
					GraspServers:         getSliceValue(existingConfig.GraspServers, c.StringSlice("grasp-servers"), []string{"gitnostr.com", "relay.ngit.dev"}),
					EarliestUniqueCommit: getValue(existingConfig.EarliestUniqueCommit, c.String("earliest-unique-commit"), earliestCommit),
					Maintainers:          getSliceValue(existingConfig.Maintainers, c.StringSlice("maintainers"), []string{}),
				}

				if c.Bool("interactive") {
					if err := promptForConfig(&config); err != nil {
						return err
					}
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
					color.CyanString("nak git announce"))

				return nil
			},
		},
		{
			Name:  "sync",
			Usage: "sync repository with relays",
			Flags: defaultKeyFlags,
			Action: func(ctx context.Context, c *cli.Command) error {
				kr, _, _ := gatherKeyerFromArguments(ctx, c)
				_, _, err := gitSync(ctx, kr)
				return err
			},
		},
		{
			Name:        "clone",
			Usage:       "clone a NIP-34 repository from a nostr:// URI",
			Description: `the <repository> parameter maybe in the form "<npub, hex, nprofile or nip05>/<identifier>", ngit-style like "nostr://<npub>/<relay>/<identifier>" or an "naddr1..." code.`,
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
				repo, state, err := fetchRepositoryAndState(ctx, owner, identifier, relayHints)
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
				localConfig := Nip34Config{
					Identifier:           repo.ID,
					Name:                 repo.Name,
					Description:          repo.Description,
					Web:                  repo.Web,
					Owner:                nip19.EncodeNpub(repo.Event.PubKey),
					GraspServers:         make([]string, 0, len(repo.Relays)),
					EarliestUniqueCommit: repo.EarliestUniqueCommitID,
					Maintainers:          make([]string, 0, len(repo.Maintainers)),
				}
				for _, r := range repo.Relays {
					localConfig.GraspServers = append(localConfig.GraspServers, nostr.NormalizeURL(r))
				}
				for _, m := range repo.Maintainers {
					localConfig.Maintainers = append(localConfig.Maintainers, nip19.EncodeNpub(m))
				}

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
				if state.Event.ID != nostr.ZeroID && state.HEAD != "" {
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
			Name:  "push",
			Usage: "push git changes",
			Flags: append(defaultKeyFlags, &cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "force push to git remotes",
			}),
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
				repo, state, err := gitSync(ctx, kr)
				if err != nil {
					return fmt.Errorf("failed to sync: %w", err)
				}

				// figure out which branches to push
				localBranch, remoteBranch, err := figureOutBranches(c, c.Args().First(), true)
				if err != nil {
					return err
				}

				// check if signer matches owner or is in maintainers
				if currentPk != repo.Event.PubKey && !slices.Contains(repo.Maintainers, currentPk) {
					return fmt.Errorf("current user '%s' is not allowed to push", nip19.EncodeNpub(currentPk))
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
					remoteName := "nip34/grasp/" + strings.TrimPrefix(relayURL, "wss://")
					remoteName = strings.TrimPrefix(remoteName, "ws://")

					log("pushing to %s...\n", color.CyanString(remoteName))
					pushArgs := []string{"push", remoteName, fmt.Sprintf("%s:refs/heads/%s", localBranch, remoteBranch)}
					if c.Bool("force") {
						pushArgs = append(pushArgs, "--force")
					}
					pushCmd := exec.Command("git", pushArgs...)
					pushCmd.Stderr = os.Stderr
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
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				// sync to fetch latest state and metadata
				_, state, err := gitSync(ctx, nil)
				if err != nil {
					return fmt.Errorf("failed to sync: %w", err)
				}

				// figure out which branches to pull
				localBranch, remoteBranch, err := figureOutBranches(c, c.Args().First(), false)
				if err != nil {
					return err
				}

				// get the commit from state for the remote branch
				if state.Event.ID == nostr.ZeroID {
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

				// merge or rebase
				if c.Bool("rebase") {
					log("rebasing %s onto %s...\n", color.CyanString(localBranch), color.CyanString(targetCommit))
					rebaseCmd := exec.Command("git", "rebase", targetCommit)
					rebaseCmd.Stderr = os.Stderr
					rebaseCmd.Stdout = os.Stdout
					if err := rebaseCmd.Run(); err != nil {
						return fmt.Errorf("rebase failed: %w", err)
					}
				} else {
					log("merging %s into %s...\n", color.CyanString(targetCommit), color.CyanString(localBranch))
					mergeCmd := exec.Command("git", "merge", targetCommit)
					mergeCmd.Stderr = os.Stderr
					mergeCmd.Stdout = os.Stdout
					if err := mergeCmd.Run(); err != nil {
						return fmt.Errorf("merge failed: %w", err)
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
				_, _, err := gitSync(ctx, nil)
				return err
			},
		},
	},
}

func promptForStringList(
	name string,
	existing []string,
	defaults []string,
	normalize func(string) string,
	validate func(string) bool,
) ([]string, error) {
	options := make([]string, 0, len(defaults)+len(existing)+1)
	options = append(options, defaults...)
	options = append(options, "add another")

	// add existing not in options
	for _, item := range existing {
		if !slices.Contains(options, item) {
			options = append(options, item)
		}
	}

	selected := make([]string, len(existing))
	copy(selected, existing)

	for {
		prompt := &survey.MultiSelect{
			Message:  name,
			Options:  options,
			Default:  selected,
			PageSize: 20,
		}

		if err := survey.AskOne(prompt, &selected); err != nil {
			return nil, err
		}

		if slices.Contains(selected, "add another") {
			selected = slices.DeleteFunc(selected, func(s string) bool { return s == "add another" })

			singular := name
			if strings.HasSuffix(singular, "s") {
				singular = singular[:len(singular)-1]
			}

			newPrompt := &survey.Input{
				Message: fmt.Sprintf("enter new %s", singular),
			}
			var newItem string
			if err := survey.AskOne(newPrompt, &newItem); err != nil {
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

func promptForConfig(config *Nip34Config) error {
	log("\nenter repository details (use arrow keys to navigate, space to select/deselect, enter to confirm):\n\n")

	// prompt for identifier
	identifierPrompt := &survey.Input{
		Message: "identifier",
		Default: config.Identifier,
	}
	if err := survey.AskOne(identifierPrompt, &config.Identifier); err != nil {
		return err
	}

	// prompt for name
	namePrompt := &survey.Input{
		Message: "name",
		Default: config.Name,
	}
	if err := survey.AskOne(namePrompt, &config.Name); err != nil {
		return err
	}

	// prompt for description
	descPrompt := &survey.Input{
		Message: "description",
		Default: config.Description,
	}
	if err := survey.AskOne(descPrompt, &config.Description); err != nil {
		return err
	}

	// prompt for owner
	for {
		ownerPrompt := &survey.Input{
			Message: "owner (npub or hex)",
			Default: config.Owner,
		}
		if err := survey.AskOne(ownerPrompt, &config.Owner); err != nil {
			return err
		}
		if pubkey, err := parsePubKey(config.Owner); err == nil {
			config.Owner = pubkey.Hex()
			break
		}
	}

	// prompt for grasp servers
	graspServers, err := promptForStringList("grasp servers", config.GraspServers, []string{
		"gitnostr.com",
		"relay.ngit.dev",
		"pyramid.fiatjaf.com",
		"git.shakespeare.dyi",
	}, graspServerHost, nil)
	if err != nil {
		return err
	}
	config.GraspServers = graspServers

	// prompt for web URLs
	webURLs, err := promptForStringList("web URLs", config.Web, []string{
		fmt.Sprintf("https://gitworkshop.dev/%s/%s",
			nip19.EncodeNpub(nostr.MustPubKeyFromHex(config.Owner)),
			config.Identifier,
		),
	}, func(s string) string {
		return "http" + nostr.NormalizeURL(s)[2:]
	}, nil)
	if err != nil {
		return err
	}
	config.Web = webURLs

	// Prompt for maintainers
	maintainers, err := promptForStringList("maintainers", config.Maintainers, []string{}, nil, func(s string) bool {
		pk, err := parsePubKey(s)
		if err != nil {
			return false
		}
		if pk.Hex() == config.Owner {
			return false
		}
		return true
	})
	if err != nil {
		return err
	}
	config.Maintainers = maintainers

	log("\n")
	return nil
}

func gitSync(ctx context.Context, signer nostr.Keyer) (nip34.Repository, *nip34.RepositoryState, error) {
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
	repo, state, err := fetchRepositoryAndState(ctx, owner, localConfig.Identifier, localConfig.GraspServers)
	if err != nil {
		log("couldn't fetch repository metadata (%s), will publish now\n", err)
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
				event := localRepo.ToEvent()
				if err := signer.SignEvent(ctx, &event); err != nil {
					return repo, state, fmt.Errorf("failed to sign announcement: %w", err)
				}

				relays := append(sys.FetchOutboxRelays(ctx, owner, 3), localConfig.GraspServers...)
				for res := range sys.Pool.PublishMany(ctx, relays, event) {
					if res.Error != nil {
						log("! error publishing to %s: %v\n", color.YellowString(res.RelayURL), res.Error)
					} else {
						log("> published to %s\n", color.GreenString(res.RelayURL))
					}
				}
				repo = localRepo
			}
		} else {
			return repo, nil, fmt.Errorf("no signer provided to publish repository (run 'nak git sync' with the '--sec' flag)")
		}
	} else {
		// check if local config differs from remote announcement
		// construct local repo from config for comparison
		localRepo := localConfig.ToRepository()

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
					localConfig.Web = repo.Web
					localConfig.EarliestUniqueCommit = repo.EarliestUniqueCommitID
					localConfig.Maintainers = make([]string, 0, len(repo.Maintainers))
					for _, m := range repo.Maintainers {
						localConfig.Maintainers = append(localConfig.Maintainers, nip19.EncodeNpub(m))
					}
					if err := writeNip34ConfigFile("", localConfig); err != nil {
						log("! failed to update local config: %v\n", err)
					}
				}
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
		remoteName := "nip34/grasp/" + strings.Split(grasp, "/")[2]

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

	// delete all nip34/grasp/ remotes
	remotes := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, remote := range remotes {
		remote = strings.TrimSpace(remote)
		if strings.HasPrefix(remote, "nip34/grasp/") {
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
	for _, relay := range repo.Relays {
		relayURL := nostr.NormalizeURL(relay)
		remoteName := "nip34/grasp/" + strings.TrimPrefix(relayURL, "wss://")
		remoteName = strings.TrimPrefix(remoteName, "ws://")

		// construct the git URL
		gitURL := fmt.Sprintf("http%s/%s/%s.git",
			relayURL[2:], nip19.EncodeNpub(repo.PubKey), repo.ID)

		addCmd := exec.Command("git", "remote", "add", remoteName, gitURL)
		if dir != "" {
			addCmd.Dir = dir
		}
		if out, err := addCmd.Output(); err != nil {
			var stderr string
			if exiterr, ok := err.(*exec.ExitError); ok {
				stderr = string(exiterr.Stderr)
			}
			logverbose("failed to add remote %s: %s %s\n", remoteName, stderr, string(out))
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
			if len(parts) >= 2 && strings.Contains(parts[1], "refs/remotes/nip34/state/") {
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

		refName := "refs/remotes/nip34/state/" + strings.TrimPrefix(branchName, "refs/heads/")
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
			headRefName := "refs/remotes/nip34/state/HEAD"
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
) (repo nip34.Repository, state *nip34.RepositoryState, err error) {
	// fetch repository announcement (30617)
	relays := appendUnique(relayHints, sys.FetchOutboxRelays(ctx, pubkey, 3)...)
	for ie := range sys.Pool.FetchMany(ctx, relays, nostr.Filter{
		Kinds:   []nostr.Kind{30617},
		Authors: []nostr.PubKey{pubkey},
		Tags: nostr.TagMap{
			"d": []string{identifier},
		},
		Limit: 2,
	}, nostr.SubscriptionOptions{Label: "nak-git"}) {
		if ie.Event.CreatedAt > repo.CreatedAt {
			repo = nip34.ParseRepository(ie.Event)
		}
	}
	if repo.Event.ID == nostr.ZeroID {
		return repo, state, fmt.Errorf("no repository announcement (kind 30617) found for %s", identifier)
	}

	// fetch repository state (30618)
	var stateErr error
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
			state = &state_

			if state.HEAD == "" {
				stateErr = fmt.Errorf("state is missing HEAD")
				continue
			}
			if _, ok := state.Branches[state.HEAD]; !ok {
				stateErr = fmt.Errorf("state is missing commit for HEAD branch '%s'", state.HEAD)
				continue
			}

			stateErr = nil
		}
	}
	if stateErr != nil {
		return repo, state, stateErr
	}

	return repo, state, nil
}

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

	// format 2: nostr://<npub>/<relay_hostname>/<identifier> (ngit-style)
	if strings.HasPrefix(address, "nostr://") {
		parts := strings.Split(address, "/")
		if len(parts) != 5 {
			return nostr.PubKey{}, "", nil, fmt.Errorf(
				"invalid nostr URL format, expected nostr://<npub>/<relay_hostname>/<identifier>, got: %s", address,
			)
		}

		prefix, data, err := nip19.Decode(parts[2])
		if err != nil {
			return nostr.PubKey{}, "", nil, fmt.Errorf("invalid owner public key: %w", err)
		}
		if prefix != "npub" {
			return nostr.PubKey{}, "", nil, fmt.Errorf("expected npub in URL")
		}
		owner = data.(nostr.PubKey)
		relayHost := parts[3]
		identifier = parts[4]

		// construct relay hint from hostname
		if strings.HasPrefix(relayHost, "wss:") || strings.HasPrefix(relayHost, "ws:") {
			relayHints = []string{relayHost}
		} else {
			relayHints = []string{"wss://" + relayHost}
		}

		return owner, identifier, relayHints, nil
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

func graspServerHost(s string) string { return strings.SplitN(nostr.NormalizeURL(s), "/", 3)[2] }

type Nip34Config struct {
	Identifier           string   `json:"identifier"`
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Web                  []string `json:"web"`
	Owner                string   `json:"owner"`
	GraspServers         []string `json:"grasp-servers"`
	EarliestUniqueCommit string   `json:"earliest-unique-commit"`
	Maintainers          []string `json:"maintainers"`
}

func (localConfig Nip34Config) Validate() error {
	_, err := parsePubKey(localConfig.Owner)
	if err != nil {
		return fmt.Errorf("owner pubkey '%s' is not valid: %w", localConfig.Owner, err)
	}

	for _, maintainer := range localConfig.Maintainers {
		_, err := parsePubKey(maintainer)
		if err != nil {
			return fmt.Errorf("maintainer pubkey '%s' is not valid: %w", maintainer, err)
		}
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
		Web:                    localConfig.Web,
		EarliestUniqueCommitID: localConfig.EarliestUniqueCommit,
		Maintainers:            []nostr.PubKey{},
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
	for _, maintainer := range localConfig.Maintainers {
		pk, err := parsePubKey(maintainer)
		if err != nil {
			panic(err)
		}
		localRepo.Maintainers = append(localRepo.Maintainers, pk)
	}

	return localRepo
}
