package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nip34"
	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

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

type nostrRemote struct {
	formatted  string
	owner      nostr.PubKey
	identifier string
	relayHost  string
}

var git = &cli.Command{
	Name:  "git",
	Usage: "git-related operations",
	Commands: []*cli.Command{
		gitInit,
		gitClone,
		gitPush,
		gitPull,
		gitFetch,
		gitAnnounce,
	},
}

var gitInit = &cli.Command{
	Name:  "init",
	Usage: "initialize a NIP-34 repository configuration",
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
			return fmt.Errorf("current directory is not a git repository")
		}

		// check if nip34.json already exists
		var existingConfig Nip34Config
		if data, err := os.ReadFile("nip34.json"); err == nil {
			// file exists, read it
			if !c.Bool("force") && !c.Bool("interactive") {
				return fmt.Errorf("nip34.json already exists, use --force to overwrite or --interactive to update")
			}
			if err := json.Unmarshal(data, &existingConfig); err != nil {
				return fmt.Errorf("failed to parse existing nip34.json: %s", err)
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
		var defaultCloneURLs []string
		if output, err := exec.Command("git", "remote", "-v").Output(); err == nil {
			remotes := strings.Split(strings.TrimSpace(string(output)), "\n")
			for _, remote := range remotes {
				if strings.Contains(remote, "nostr://") {
					parts := strings.Fields(remote)
					if len(parts) >= 2 {
						nostrURL := parts[1]
						// parse nostr://npub.../relay_hostname/identifier
						if remote, err := parseRemote(nostrURL); err == nil {
							// convert to https://relay_hostname/npub.../identifier.git
							cloneURL := fmt.Sprintf("http%s/%s/%s.git",
								nostr.NormalizeURL(remote.relayHost)[2:], nip19.EncodeNpub(remote.owner), remote.identifier)
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

		// write config file
		if err := writeNip34ConfigFile("", config); err != nil {
			return err
		}

		log("created %s\n", color.GreenString("nip34.json"))

		// parse owner to npub
		owner, err := parsePubKey(config.Owner)
		if err != nil {
			return fmt.Errorf("invalid owner public key: %w", err)
		}

		// check existing git remotes
		remote, _, _, err := getGitNostrRemote(c, false)
		if err != nil {
			remoteURL := fmt.Sprintf("nostr://%s/%s/%s",
				nip19.EncodeNpub(owner), config.GraspServers[0], config.Identifier)
			cmd = exec.Command("git", "remote", "add", "origin", remoteURL)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to add git remote: %w", err)
			}
			log("added git remote: %s\n", remoteURL)
		} else {
			// validate existing remote
			if remote.owner != owner {
				return fmt.Errorf("git remote npub '%s' does not match owner '%s'",
					nip19.EncodeNpub(remote.owner), nip19.EncodeNpub(owner))
			}
			if !slices.Contains(config.GraspServers, nostr.NormalizeURL(remote.relayHost)) {
				return fmt.Errorf("git remote relay '%s' not in grasp servers %v",
					remote.relayHost, config.GraspServers)
			}
			if remote.identifier != config.Identifier {
				return fmt.Errorf("git remote identifier '%s' does not match config '%s'",
					remote.identifier, config.Identifier)
			}
		}

		// gitignore it
		excludeNip34ConfigFile()

		log("edit %s if needed, then run %s to publish.\n",
			color.CyanString("nip34.json"),
			color.CyanString("nak git announce"))

		return nil
	},
}

func repositoriesEqual(a, b nip34.Repository) bool {
	if a.ID != b.ID || a.Name != b.Name || a.Description != b.Description {
		return false
	}
	if a.EarliestUniqueCommitID != b.EarliestUniqueCommitID {
		return false
	}
	if len(a.Web) != len(b.Web) || len(a.Clone) != len(b.Clone) ||
		len(a.Relays) != len(b.Relays) || len(a.Maintainers) != len(b.Maintainers) {
		return false
	}
	for i := range a.Web {
		if a.Web[i] != b.Web[i] {
			return false
		}
	}
	for i := range a.Clone {
		if a.Clone[i] != b.Clone[i] {
			return false
		}
	}
	for i := range a.Relays {
		if a.Relays[i] != b.Relays[i] {
			return false
		}
	}
	for i := range a.Maintainers {
		if a.Maintainers[i] != b.Maintainers[i] {
			return false
		}
	}
	return true
}

func promptForConfig(config *Nip34Config) error {
	rlConfig := &readline.Config{
		Stdout:                 os.Stderr,
		InterruptPrompt:        "^C",
		DisableAutoSaveHistory: true,
	}

	rl, err := readline.NewEx(rlConfig)
	if err != nil {
		return err
	}
	defer rl.Close()

	promptString := func(currentVal *string, prompt string) error {
		rl.SetPrompt(color.YellowString("%s [%s]: ", prompt, *currentVal))
		answer, err := rl.Readline()
		if err != nil {
			return err
		}
		answer = strings.TrimSpace(answer)
		if answer != "" {
			*currentVal = answer
		}
		return nil
	}

	promptSlice := func(currentVal *[]string, prompt string) error {
		defaultStr := strings.Join(*currentVal, ", ")
		rl.SetPrompt(color.YellowString("%s (comma-separated) [%s]: ", prompt, defaultStr))
		answer, err := rl.Readline()
		if err != nil {
			return err
		}
		answer = strings.TrimSpace(answer)
		if answer != "" {
			parts := strings.Split(answer, ",")
			result := make([]string, 0, len(parts))
			for _, p := range parts {
				if trimmed := strings.TrimSpace(p); trimmed != "" {
					result = append(result, trimmed)
				}
			}
			*currentVal = result
		}
		return nil
	}

	log("\nenter repository details (press Enter to keep default):\n\n")

	if err := promptString(&config.Identifier, "identifier"); err != nil {
		return err
	}
	if err := promptString(&config.Name, "name"); err != nil {
		return err
	}
	if err := promptString(&config.Description, "description"); err != nil {
		return err
	}
	if err := promptString(&config.Owner, "owner (npub or hex)"); err != nil {
		return err
	}
	if err := promptSlice(&config.GraspServers, "grasp servers"); err != nil {
		return err
	}
	if err := promptSlice(&config.Web, "web URLs"); err != nil {
		return err
	}
	if err := promptSlice(&config.Maintainers, "other maintainers"); err != nil {
		return err
	}

	log("\n")
	return nil
}

var gitClone = &cli.Command{
	Name:      "clone",
	Usage:     "clone a NIP-34 repository from a nostr:// URI",
	ArgsUsage: "nostr://<npub>/<relay>/<identifier> [directory]",
	Action: func(ctx context.Context, c *cli.Command) error {
		args := c.Args()
		if args.Len() == 0 {
			return fmt.Errorf("missing repository URI (expected nostr://<npub>/<relay>/<identifier>)")
		}

		remote, err := parseRemote(args.Get(0))
		if err != nil {
			return fmt.Errorf("failed to parse remote url '%s': %s", args.Get(0), err)
		}

		repo, state, err := fetchRepositoryAndState(ctx, remote)
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

		// decide which clone URL to use
		if len(repo.Clone) == 0 {
			return fmt.Errorf("no clone urls found for repository")
		}

		cloned := false
		for _, url := range repo.Clone {
			log("- cloning %s... ", color.CyanString(url))
			if err := tryCloneAndCheckState(ctx, url, targetDir, &state); err != nil {
				log(color.YellowString("failed: %v\n", err))
				continue
			}
			log("%s\n", color.GreenString("ok"))
			cloned = true
			break
		}

		if !cloned {
			return fmt.Errorf("failed to clone")
		}

		// write nip34.json inside cloned directory
		// normalize relay URLs for consistency
		normalizedRelays := make([]string, 0, len(repo.Relays))
		for _, r := range repo.Relays {
			normalizedRelays = append(normalizedRelays, nostr.NormalizeURL(r))
		}

		localConfig := Nip34Config{
			Identifier:           repo.ID,
			Name:                 repo.Name,
			Description:          repo.Description,
			Web:                  repo.Web,
			Owner:                nip19.EncodeNpub(repo.Event.PubKey),
			GraspServers:         normalizedRelays,
			EarliestUniqueCommit: repo.EarliestUniqueCommitID,
			Maintainers:          make([]string, 0, len(repo.Maintainers)),
		}
		for _, m := range repo.Maintainers {
			localConfig.Maintainers = append(localConfig.Maintainers, nip19.EncodeNpub(m))
		}

		// write nip34.json
		if err := writeNip34ConfigFile("", localConfig); err != nil {
			return err
		}

		// add nip34.json to .git/info/exclude in cloned repo
		excludeNip34ConfigFile()

		log("cloned into %s\n", color.GreenString(targetDir))
		return nil
	},
}

var gitPush = &cli.Command{
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

		// read nip34.json configuration
		localConfig, err := readNip34ConfigFile("")
		if err != nil {
			return err
		}

		// get git remotes
		nostrRemote, localBranch, remoteBranch, err := getGitNostrRemote(c, true)
		if err != nil {
			return err
		}

		repo, state, err := fetchRepositoryAndState(ctx, nostrRemote)
		if err != nil {
			return err
		}

		if err := gitSanityCheck(localConfig, nostrRemote); err != nil {
			return err
		}

		// check if signer matches owner or is in maintainers
		if currentPk != repo.PubKey && !slices.Contains(repo.Maintainers, currentPk) {
			return fmt.Errorf("current user '%s' is not allowed to push", nip19.EncodeNpub(currentPk))
		}

		if state.Event.ID != nostr.ZeroID {
			logverbose("found state event: %s\n", state.Event.ID)
		}

		// get commit for the local branch
		res, err := exec.Command("git", "rev-parse", localBranch).Output()
		if err != nil {
			return fmt.Errorf("failed to get commit for branch %s: %w", localBranch, err)
		}
		currentCommit := strings.TrimSpace(string(res))

		logverbose("pushing branch %s to remote branch %s, commit: %s\n", localBranch, remoteBranch, currentCommit)

		// create a new state if we didn't find any
		if state.Event.ID == nostr.ZeroID {
			state = nip34.RepositoryState{
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

		// push to git clone URLs
		for _, cloneURL := range repo.Clone {
			log("> pushing to: %s\n", color.CyanString(cloneURL))
			args := []string{"push"}
			if c.Bool("force") {
				args = append(args, "--force")
			}
			args = append(args,
				cloneURL,
				fmt.Sprintf("refs/heads/%s:refs/heads/%s", localBranch, remoteBranch),
			)
			cmd := exec.Command("git", args...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				log("> failed to push to %s: %v\n%s\n", color.RedString(cloneURL), err, string(output))
			} else {
				log("> successfully pushed to %s\n", color.GreenString(cloneURL))
			}
		}

		return nil
	},
}

var gitPull = &cli.Command{
	Name:  "pull",
	Usage: "pull git changes",
	Action: func(ctx context.Context, c *cli.Command) error {
		_, _, remoteBranch, err := gitFetchInternal(ctx, c)
		if err != nil {
			return err
		}

		cmd := exec.Command("git", "checkout", fmt.Sprintf("origin/%s", remoteBranch))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to checkout %s: %w", remoteBranch, err)
		}
		log("- checked out to %s\n", color.CyanString(remoteBranch))

		return nil
	},
}

var gitFetch = &cli.Command{
	Name:  "fetch",
	Usage: "fetch git data",
	Action: func(ctx context.Context, c *cli.Command) error {
		_, _, _, err := gitFetchInternal(ctx, c)
		return err
	},
}

var gitAnnounce = &cli.Command{
	Name:  "announce",
	Usage: "announce repository to Nostr",
	Flags: defaultKeyFlags,
	Action: func(ctx context.Context, c *cli.Command) error {
		// check if current directory is a git repository
		cmd := exec.Command("git", "rev-parse", "--git-dir")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("current directory is not a git repository")
		}

		// read nip34.json configuration
		localConfig, err := readNip34ConfigFile("")
		if err != nil {
			return err
		}

		// get git remotes
		nostrRemote, _, _, err := getGitNostrRemote(c, false)
		if err != nil {
			return err
		}

		if err := gitSanityCheck(localConfig, nostrRemote); err != nil {
			return err
		}
		owner, _ := parsePubKey(localConfig.Owner)

		// setup signer
		kr, _, err := gatherKeyerFromArguments(ctx, c)
		if err != nil {
			return fmt.Errorf("failed to gather keyer: %w", err)
		}
		currentPk, _ := kr.GetPublicKey(ctx)

		// current signer must match owner otherwise we can't announce
		if currentPk != owner {
			return fmt.Errorf("current user is not the owner of this repository, can't announce")
		}

		// convert local config to nip34.Repository
		localRepo := nip34.Repository{
			ID:                     localConfig.Identifier,
			Name:                   localConfig.Name,
			Description:            localConfig.Description,
			Web:                    localConfig.Web,
			EarliestUniqueCommitID: localConfig.EarliestUniqueCommit,
			Maintainers:            []nostr.PubKey{},
		}
		for _, server := range localConfig.GraspServers {
			graspRelayURL := nostr.NormalizeURL(server)
			url := fmt.Sprintf("http%s/%s/%s.git",
				graspRelayURL[2:], nip19.EncodeNpub(owner), localConfig.Identifier)
			localRepo.Clone = append(localRepo.Clone, url)
			localRepo.Relays = append(localRepo.Relays, graspRelayURL)
		}
		for _, maintainer := range localConfig.Maintainers {
			if pk, err := parsePubKey(maintainer); err == nil {
				localRepo.Maintainers = append(localRepo.Maintainers, pk)
			} else {
				log(color.YellowString("invalid maintainer pubkey '%s': %v\n", maintainer, err))
			}
		}

		// these are the relays where we'll publish the announcement to
		relays := append(sys.FetchOutboxRelays(ctx, owner, 3), localConfig.GraspServers...)
		for i := range relays {
			relays[i] = nostr.NormalizeURL(relays[i])
		}

		// fetch repository announcement (30617) events
		oks := make([]bool, len(relays))
		var repo nip34.Repository
		results := sys.Pool.FetchMany(ctx, relays, nostr.Filter{
			Kinds: []nostr.Kind{30617},
			Tags: nostr.TagMap{
				"d": []string{localConfig.Identifier},
			},
			Limit: 1,
		}, nostr.SubscriptionOptions{
			Label:          "nak-git-announce",
			CheckDuplicate: func(id nostr.ID, relay string) bool { return false }, // get the same event from multiple relays
		})
		for ie := range results {
			repo = nip34.ParseRepository(ie.Event)

			// check if this is ok or the announcement in this relay needs to be updated
			if repositoriesEqual(repo, localRepo) {
				relayIdx := slices.Index(relays, ie.Relay.URL)
				oks[relayIdx] = true
			}
		}

		// publish repository announcement if needed
		if slices.Contains(oks, false) {
			announcementEvent := localRepo.ToEvent()
			if err := kr.SignEvent(ctx, &announcementEvent); err != nil {
				return fmt.Errorf("failed to sign announcement event: %w", err)
			}

			targets := make([]string, 0, len(oks))
			for i, ok := range oks {
				if !ok {
					targets = append(targets, relays[i])
				}
			}
			log("- publishing repository announcement to " + color.CyanString("%v", targets) + "\n")
			for res := range sys.Pool.PublishMany(ctx, targets, announcementEvent) {
				if res.Error != nil {
					log("! error publishing announcement to relay %s: %v\n", color.YellowString(res.RelayURL), res.Error)
				} else {
					log("> published announcement to relay %s\n", color.GreenString(res.RelayURL))
				}
			}
		}

		return nil
	},
}

func getGitNostrRemote(c *cli.Command, isPush bool) (
	remote nostrRemote,
	localBranch string,
	remoteBranch string,
	err error,
) {
	// remote
	var remoteName string
	var cmd *exec.Cmd
	args := c.Args()
	if args.Len() > 0 {
		remoteName = args.Get(0)
	} else {
		// get current branch
		cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		output, err := cmd.Output()
		if err != nil {
			return remote, "", "", fmt.Errorf("failed to get current branch: %w", err)
		}
		branch := strings.TrimSpace(string(output))

		// get remote for branch
		cmd = exec.Command("git", "config", "--get", fmt.Sprintf("branch.%s.remote", branch))
		output, err = cmd.Output()
		if err != nil {
			remoteName = "origin"
		} else {
			remoteName = strings.TrimSpace(string(output))
		}
	}
	// get the URL
	cmd = exec.Command("git", "remote", "get-url", remoteName)
	output, err := cmd.Output()
	if err != nil {
		return remote, "", "", fmt.Errorf("remote '%s' does not exist", remoteName)
	}

	remoteURL := strings.TrimSpace(string(output))
	if !strings.Contains(remoteURL, "nostr://") {
		return remote, "", "", fmt.Errorf("remote '%s' is not a nostr remote: %s", remoteName, remoteURL)
	}

	// branch (src and dst)
	if args.Len() > 1 {
		var src, dst string
		branchSpec := args.Get(1)
		if strings.Contains(branchSpec, ":") {
			parts := strings.Split(branchSpec, ":")
			if len(parts) == 2 {
				src = parts[0]
				dst = parts[1]
			} else {
				return remote, "", "", fmt.Errorf("invalid branch spec: %s", branchSpec)
			}
		} else {
			src = branchSpec
		}
		if isPush {
			localBranch = src
			remoteBranch = dst
		} else {
			localBranch = dst
			remoteBranch = src
		}
	} else {
		// get current branch
		cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		output, err := cmd.Output()
		if err != nil {
			return remote, "", "", fmt.Errorf("failed to get current branch: %w", err)
		}
		localBranch = strings.TrimSpace(string(output))
	}

	// get the target branch from git config
	if isPush {
		cmd = exec.Command("git", "config", "--get", fmt.Sprintf("branch.%s.merge", localBranch))
		output, err = cmd.Output()
		if err == nil {
			// parse refs/heads/<branch-name> to get just the branch name
			mergeRef := strings.TrimSpace(string(output))
			if strings.HasPrefix(mergeRef, "refs/heads/") {
				remoteBranch = strings.TrimPrefix(mergeRef, "refs/heads/")
			} else {
				// fallback if it's not in expected format
				remoteBranch = localBranch
			}
		}

		if remoteBranch == "" {
			// no upstream configured, assume same branch name
			remoteBranch = localBranch
		}
	} else {
		if localBranch == "" {
			// no local branch configured, assume same branch name
			localBranch = remoteBranch
		}
	}

	// parse remote
	remote, err = parseRemote(remoteURL)

	return remote, localBranch, remoteBranch, err
}

func parseRemote(remoteURL string) (remote nostrRemote, err error) {
	parts := strings.Split(remoteURL, "/")
	if len(parts) != 5 {
		return remote, fmt.Errorf(
			"invalid nostr URL format, expected nostr://<npub>/<relay_hostname>/<identifier>, got: %s", remoteURL,
		)
	}

	prefix, data, err := nip19.Decode(parts[2])
	if err != nil || prefix != "npub" {
		return remote, fmt.Errorf("invalid owner public key: %w", err)
	}
	remote.formatted = remoteURL
	remote.owner = data.(nostr.PubKey)
	remote.relayHost = parts[3]
	remote.identifier = parts[4]

	return remote, nil
}

func gitSanityCheck(
	localConfig Nip34Config,
	remote nostrRemote,
) error {
	owner, err := parsePubKey(localConfig.Owner)
	if err != nil {
		return fmt.Errorf("invalid owner pubkey in nip34.json: %w", err)
	}

	if owner != remote.owner {
		return fmt.Errorf("owner in nip34.json does not match git remote npub")
	}
	if remote.identifier != localConfig.Identifier {
		return fmt.Errorf("git remote identifier '%s' differs from nip34.json identifier '%s'",
			remote.identifier, localConfig.Identifier)
	}
	if !slices.Contains(localConfig.GraspServers, nostr.NormalizeURL(remote.relayHost)) {
		return fmt.Errorf("git remote relay '%s' not in grasp servers %v", remote.relayHost, localConfig.GraspServers)
	}
	return nil
}

func tryCloneAndCheckState(ctx context.Context, cloneURL, targetDir string, state *nip34.RepositoryState) (err error) {
	// if we get here we know we were the ones who created the target directory, so we're safe to remove it
	defer func() {
		if err != nil {
			if err := os.RemoveAll(targetDir); err != nil {
				log("failed to remove '%s' when handling error from clone: %s", targetDir, err)
			}
		}
	}()

	cmd := exec.CommandContext(ctx, "git", "clone", cloneURL, targetDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %v: %s", err, strings.TrimSpace(string(output)))
	}

	// if we don't have any state information, we can't verify anything
	if state == nil || state.Event.ID == nostr.ZeroID {
		return nil
	}

	// check that the HEAD branch matches the state HEAD
	cmd = exec.Command("git", "-C", targetDir, "rev-parse", "--abbrev-ref", "HEAD")
	headOut, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to read HEAD")
	}
	currentBranch := strings.TrimSpace(string(headOut))
	if currentBranch != state.HEAD {
		return fmt.Errorf("received HEAD '%s' isn't the expected '%s'", currentBranch, state.HEAD)
	}

	// verify the HEAD branch only as it's the only one we have
	expectedCommit := state.Branches[state.HEAD] // we've tested before if state has this
	cmd = exec.Command("git", "-C", targetDir, "rev-parse", state.HEAD)
	actualOut, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check commit for '%s': %s", state.HEAD, err)
	}
	actualCommit := strings.TrimSpace(string(actualOut))
	if actualCommit != expectedCommit {
		return fmt.Errorf("branch %s is at %s, expected %s", state.HEAD, actualCommit, expectedCommit)
	}

	// set nostr remote
	parsed, _ := url.Parse(cloneURL)
	repoURI := fmt.Sprintf("nostr://%s/%s/%s",
		nip19.EncodeNpub(state.PubKey),
		parsed.Host,
		state.ID,
	)
	cmd = exec.Command("git", "-C", targetDir, "remote", "set-url", "origin", repoURI)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add git remote: %v\n", err)
	}

	return nil
}

func gitFetchInternal(
	ctx context.Context,
	c *cli.Command,
) (state nip34.RepositoryState, localBranch string, remoteBranch string, err error) {
	localConfig, err := readNip34ConfigFile("")
	if err != nil {
		return state, "", "", err
	}

	nostrRemote, localBranch, remoteBranch, err := getGitNostrRemote(c, false)
	if err != nil {
		return state, localBranch, remoteBranch, err
	}

	if err := gitSanityCheck(localConfig, nostrRemote); err != nil {
		return state, localBranch, remoteBranch, err
	}

	// fetch repo and state
	repo, state, err := fetchRepositoryAndState(ctx, nostrRemote)
	if err != nil {
		return state, localBranch, remoteBranch, err
	}

	// try fetch from each clone url
	fetched := false
	for _, cloneURL := range repo.Clone {
		log("- fetching from %s... ", color.CyanString(cloneURL))
		// construct git fetch command with appropriate refspec
		args := []string{"fetch", cloneURL}
		if remoteBranch != "" {
			// fetch specific branch when refspec is provided
			refspec := fmt.Sprintf("%s:%s", remoteBranch, localBranch)
			args = append(args, refspec)
		}
		args = append(args, "--update-head-ok")
		cmd := exec.Command("git", args...)
		if err := cmd.Run(); err != nil {
			log(color.YellowString("failed: %v\n", err))
			continue
		}

		// check downloaded remote branches
		mismatch := false
		for branch, commit := range state.Branches {
			cmd = exec.Command("git", "rev-parse", fmt.Sprintf("refs/remotes/origin/%s", branch))
			out, err := cmd.Output()
			if err != nil {
				log(color.YellowString("branch %s not found\n", branch))
				mismatch = true
				break
			}
			if strings.TrimSpace(string(out)) != commit {
				log(color.YellowString("branch %s commit mismatch: got %s, expected %s\n", branch, strings.TrimSpace(string(out)), commit))
				mismatch = true
				break
			}
		}
		if !mismatch {
			log("%s\n", color.GreenString("ok"))
			fetched = true
			break
		} else {
			log(color.YellowString("mismatch\n"))
		}
	}

	if !fetched {
		return state, localBranch, remoteBranch, fmt.Errorf("failed to fetch from any clone url")
	}

	return state, localBranch, remoteBranch, nil
}

func fetchRepositoryAndState(
	ctx context.Context,
	remote nostrRemote,
) (repo nip34.Repository, state nip34.RepositoryState, err error) {
	primaryRelay := nostr.NormalizeURL(remote.relayHost)

	// fetch repository announcement (30617)
	relays := appendUnique([]string{primaryRelay}, sys.FetchOutboxRelays(ctx, remote.owner, 3)...)
	for ie := range sys.Pool.FetchMany(ctx, relays, nostr.Filter{
		Kinds:   []nostr.Kind{30617},
		Authors: []nostr.PubKey{remote.owner},
		Tags: nostr.TagMap{
			"d": []string{remote.identifier},
		},
		Limit: 2,
	}, nostr.SubscriptionOptions{Label: "nak-git-clone-meta"}) {
		if ie.Event.CreatedAt > repo.CreatedAt {
			repo = nip34.ParseRepository(ie.Event)
		}
	}
	if repo.Event.ID == nostr.ZeroID {
		return repo, state, fmt.Errorf("no repository announcement (kind 30617) found for %s", remote.identifier)
	}

	// fetch repository state (30618)
	var stateFound bool
	var stateErr error
	for ie := range sys.Pool.FetchMany(ctx, repo.Relays, nostr.Filter{
		Kinds:   []nostr.Kind{30618},
		Authors: []nostr.PubKey{remote.owner},
		Tags: nostr.TagMap{
			"d": []string{remote.identifier},
		},
		Limit: 2,
	}, nostr.SubscriptionOptions{Label: "nak-git-clone-meta"}) {
		if ie.Event.CreatedAt > state.CreatedAt {
			state = nip34.ParseRepositoryState(ie.Event)
			stateFound = true

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
	if !stateFound {
		return repo, state, fmt.Errorf("no repository state (kind 30618) found")
	}
	if stateErr != nil {
		return repo, state, stateErr
	}

	return repo, state, nil
}

func readNip34ConfigFile(baseDir string) (Nip34Config, error) {
	var localConfig Nip34Config

	// TODO: the baseDir should inspect parents until we reach the directory that has the ".git"

	data, err := os.ReadFile(filepath.Join(baseDir, "nip34.json"))
	if err != nil {
		return localConfig, fmt.Errorf("failed to read nip34.json: %w (run 'nak git init' first)", err)
	}
	if err := json.Unmarshal(data, &localConfig); err != nil {
		return localConfig, fmt.Errorf("failed to parse nip34.json: %w", err)
	}
	return localConfig, nil
}

func excludeNip34ConfigFile() {
	// TODO: inspect parents until we reach the directory that has the ".git"

	excludePath := ".git/info/exclude"
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
	// TODO: baseDir should inspect parents until we reach the directory that has the ".git"

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal nip34.json: %w", err)
	}

	configPath := filepath.Join(baseDir, "nip34.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", configPath, err)
	}

	return nil
}
