package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nip34"
	"fiatjaf.com/nostr/sdk"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/urfave/cli/v3"
)

var mcpServer = &cli.Command{
	Name:                      "mcp",
	Usage:                     "pander to the AI gods",
	Description:               ``,
	DisableSliceFlagSeparator: true,
	Flags: append(
		defaultKeyFlags,
	),
	Action: func(ctx context.Context, c *cli.Command) error {
		s := server.NewMCPServer(
			"nak",
			version,
		)

		keyer, _, err := gatherKeyerFromArguments(ctx, c)
		if err != nil {
			return err
		}

		s.AddTool(mcp.NewTool("publish_note",
			mcp.WithDescription("Publish a short note event to Nostr with the given text content"),
			mcp.WithString("content", mcp.Description("Arbitrary string to be published"), mcp.Required()),
			mcp.WithString("relay", mcp.Description("Relay to publish the note to")),
			mcp.WithString("mention", mcp.Description("Nostr user's public key to be mentioned")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			content := required[string](r, "content")
			mention, _ := optional[string](r, "mention")
			relay, _ := optional[string](r, "relay")

			var relays []string

			evt := nostr.Event{
				Kind:      1,
				Tags:      nostr.Tags{{"client", "goose/nak"}},
				Content:   content,
				CreatedAt: nostr.Now(),
			}

			if mention != "" {
				pk, err := nostr.PubKeyFromHex(mention)
				if err != nil {
					return mcp.NewToolResultError("the given mention isn't a valid public key, it must be 32 bytes hex, like the ones returned by search_profile. Got error: " + err.Error()), nil
				}

				evt.Tags = append(evt.Tags, nostr.Tag{"p", pk.Hex()})
				// their inbox relays
				relays = sys.FetchInboxRelays(ctx, pk, 3)
			}

			if err := keyer.SignEvent(ctx, &evt); err != nil {
				return mcp.NewToolResultError("it was impossible to sign the event, so we can't proceed to publishwith publishing it."), nil
			}

			// our write relays
			relays = append(relays, sys.FetchOutboxRelays(ctx, evt.PubKey, 3)...)

			if len(relays) == 0 {
				relays = []string{"nos.lol", "relay.damus.io"}
			}

			// extra relay specified
			if relay != "" {
				relays = append(relays, relay)
			}

			result := strings.Builder{}
			result.WriteString(
				fmt.Sprintf("the event we generated has id '%s', kind '%d' and is signed by pubkey '%s'. ",
					evt.ID,
					evt.Kind,
					evt.PubKey,
				),
			)

			for res := range sys.Pool.PublishMany(ctx, relays, evt) {
				if res.Error != nil {
					result.WriteString(
						fmt.Sprintf("there was an error publishing the event to the relay %s. ",
							res.RelayURL),
					)
				} else {
					result.WriteString(
						fmt.Sprintf("the event was successfully published to the relay %s. ",
							res.RelayURL),
					)
				}
			}

			return mcp.NewToolResultText(result.String()), nil
		})

		s.AddTool(mcp.NewTool("resolve_nostr_uri",
			mcp.WithDescription("Resolve URIs prefixed with nostr:, including nostr:nevent1..., nostr:npub1..., nostr:nprofile1... and nostr:naddr1..."),
			mcp.WithString("uri", mcp.Description("URI to be resolved"), mcp.Required()),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			uri := required[string](r, "uri")
			if strings.HasPrefix(uri, "nostr:") {
				uri = uri[6:]
			}

			prefix, data, err := nip19.Decode(uri)
			if err != nil {
				return mcp.NewToolResultError("this Nostr uri is invalid"), nil
			}

			switch prefix {
			case "npub":
				pm := sys.FetchProfileMetadata(ctx, data.(nostr.PubKey))
				return mcp.NewToolResultText(
					fmt.Sprintf("this is a Nostr profile named '%s', their public key is '%s'",
						pm.ShortName(), pm.PubKey),
				), nil
			case "nprofile":
				pm, _ := sys.FetchProfileFromInput(ctx, uri)
				return mcp.NewToolResultText(
					fmt.Sprintf("this is a Nostr profile named '%s', their public key is '%s'",
						pm.ShortName(), pm.PubKey),
				), nil
			case "nevent":
				event, _, err := sys.FetchSpecificEventFromInput(ctx, uri, sdk.FetchSpecificEventParameters{
					WithRelays: false,
				})
				if err != nil {
					return mcp.NewToolResultError("Couldn't find this event anywhere"), nil
				}

				return mcp.NewToolResultText(
					fmt.Sprintf("this is a Nostr event: %s", event),
				), nil
			case "naddr":
				return mcp.NewToolResultError("For now we can't handle this kind of Nostr uri"), nil
			default:
				return mcp.NewToolResultError("We don't know how to handle this Nostr uri"), nil
			}
		})

		s.AddTool(mcp.NewTool("search_profile",
			mcp.WithDescription("Search for the public key of a Nostr user given their name"),
			mcp.WithString("name", mcp.Description("Name to be searched"), mcp.Required()),
			mcp.WithNumber("limit", mcp.Description("How many results to return")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name := required[string](r, "name")
			limit, _ := optional[float64](r, "limit")

			filter := nostr.Filter{Search: name, Kinds: []nostr.Kind{0}}
			if limit > 0 {
				filter.Limit = int(limit)
			}

			res := strings.Builder{}
			res.WriteString("Search results: ")
			l := 0
			for result := range sys.Pool.FetchMany(ctx, []string{"relay.nostr.band", "nostr.wine"}, filter, nostr.SubscriptionOptions{
				Label: "nak-mcp-search",
			}) {
				l++
				pm, _ := sdk.ParseMetadata(result.Event)
				res.WriteString(fmt.Sprintf("\n\nResult %d\nUser name: \"%s\"\nPublic key: \"%s\"\nDescription: \"%s\"\n",
					l, pm.ShortName(), pm.PubKey.Hex(), pm.About))

				if l >= int(limit) {
					break
				}
			}
			if l == 0 {
				return mcp.NewToolResultError("Couldn't find anyone with that name."), nil
			}
			return mcp.NewToolResultText(res.String()), nil
		})

		s.AddTool(mcp.NewTool("get_outbox_relay_for_pubkey",
			mcp.WithDescription("Get the best relay from where to read notes from a specific Nostr user"),
			mcp.WithString("pubkey", mcp.Description("Public key of Nostr user we want to know the relay from where to read"), mcp.Required()),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			pubkey, err := nostr.PubKeyFromHex(required[string](r, "pubkey"))
			if err != nil {
				return mcp.NewToolResultError("the pubkey given isn't a valid public key, it must be 32 bytes hex, like the ones returned by search_profile. Got error: " + err.Error()), nil
			}

			res := sys.FetchOutboxRelays(ctx, pubkey, 1)
			return mcp.NewToolResultText(res[0]), nil
		})

		s.AddTool(mcp.NewTool("read_events_from_relay",
			mcp.WithDescription("Makes a REQ query to one relay using specified parameters, this can be used to fetch notes from a profile"),
			mcp.WithString("relay", mcp.Description("relay URL to send the query to"), mcp.Required()),
			mcp.WithNumber("kind", mcp.Description("event kind number to include in the 'kinds' field"), mcp.Required()),
			mcp.WithNumber("limit", mcp.Description("maximum number of events to query"), mcp.Required()),
			mcp.WithString("pubkey", mcp.Description("pubkey to include in the 'authors' field, if this is not given we will read any events from this relay")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			relay := required[string](r, "relay")
			kind := int(required[float64](r, "kind"))
			limit := int(required[float64](r, "limit"))
			pubkey, hasPubKey := optional[string](r, "pubkey")

			filter := nostr.Filter{
				Limit: limit,
				Kinds: []nostr.Kind{nostr.Kind(kind)},
			}

			if hasPubKey {
				if pk, err := nostr.PubKeyFromHex(pubkey); err != nil {
					return mcp.NewToolResultError("the pubkey given isn't a valid public key, it must be 32 bytes hex, like the ones returned by search_profile. Got error: " + err.Error()), nil
				} else {
					filter.Authors = append(filter.Authors, pk)
				}
			}

			events := sys.Pool.FetchMany(ctx, []string{relay}, filter, nostr.SubscriptionOptions{
				Label: "nak-mcp-profile-events",
			})

			result := strings.Builder{}
			for ie := range events {
				result.WriteString("author public key: ")
				result.WriteString(ie.PubKey.Hex())
				result.WriteString("content: '")
				result.WriteString(ie.Content)
				result.WriteString("'")
				result.WriteString("\n---\n")
			}

			return mcp.NewToolResultText(result.String()), nil
		})

		// Kanban tools
		s.AddTool(mcp.NewTool("create_kanban_board",
			mcp.WithDescription("Create a new kanban board"),
			mcp.WithString("title", mcp.Description("Board title"), mcp.Required()),
			mcp.WithString("description", mcp.Description("Board description")),
			mcp.WithString("relay_urls", mcp.Description("Relay URLs to publish to (comma-separated)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createBoardMCP(ctx, keyer, r)
		})

		s.AddTool(mcp.NewTool("create_kanban_card",
			mcp.WithDescription("Create a new kanban card"),
			mcp.WithString("title", mcp.Description("Card title"), mcp.Required()),
			mcp.WithString("description", mcp.Description("Card description")),
			mcp.WithString("board_id", mcp.Description("Board identifier"), mcp.Required()),
			mcp.WithString("board_pubkey", mcp.Description("Board owner public key"), mcp.Required()),
			mcp.WithString("column", mcp.Description("Column name"), mcp.Required()),
			mcp.WithString("priority", mcp.Description("Card priority (low, medium, high)")),
			mcp.WithString("relay_urls", mcp.Description("Relay URLs to publish to (comma-separated)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createCardMCP(ctx, keyer, r)
		})

		s.AddTool(mcp.NewTool("update_kanban_card",
			mcp.WithDescription("Update any field of a kanban card (title, description, column, priority)"),
			mcp.WithString("card_title", mcp.Description("Card title to search for"), mcp.Required()),
			mcp.WithString("board_id", mcp.Description("Board identifier"), mcp.Required()),
			mcp.WithString("board_pubkey", mcp.Description("Board owner public key"), mcp.Required()),
			mcp.WithString("new_title", mcp.Description("New card title")),
			mcp.WithString("new_description", mcp.Description("New card description")),
			mcp.WithString("new_column", mcp.Description("New column name")),
			mcp.WithString("new_priority", mcp.Description("New card priority (low, medium, high)")),
			mcp.WithString("relay_urls", mcp.Description("Relay URLs to publish to (comma-separated)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return updateCardMCP(ctx, keyer, r)
		})

		// Keep move_kanban_card for backward compatibility
		s.AddTool(mcp.NewTool("move_kanban_card",
			mcp.WithDescription("Move a card to a different column"),
			mcp.WithString("card_title", mcp.Description("Card title to search for"), mcp.Required()),
			mcp.WithString("new_column", mcp.Description("Target column name"), mcp.Required()),
			mcp.WithString("board_id", mcp.Description("Board identifier"), mcp.Required()),
			mcp.WithString("board_pubkey", mcp.Description("Board owner public key"), mcp.Required()),
			mcp.WithString("relay_urls", mcp.Description("Relay URLs to publish to (comma-separated)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return moveCardMCP(ctx, keyer, r)
		})

		s.AddTool(mcp.NewTool("list_kanban_cards",
			mcp.WithDescription("List cards on a board"),
			mcp.WithString("board_id", mcp.Description("Board identifier"), mcp.Required()),
			mcp.WithString("board_pubkey", mcp.Description("Board owner public key"), mcp.Required()),
			mcp.WithString("column", mcp.Description("Filter by column")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of cards to return")),
			mcp.WithString("relay_urls", mcp.Description("Relay URLs to query (comma-separated)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return listCardsMCP(ctx, keyer, r)
		})

		s.AddTool(mcp.NewTool("get_kanban_board_info",
			mcp.WithDescription("Get board information and columns"),
			mcp.WithString("board_id", mcp.Description("Board identifier"), mcp.Required()),
			mcp.WithString("board_pubkey", mcp.Description("Board owner public key"), mcp.Required()),
			mcp.WithString("relay_urls", mcp.Description("Relay URLs to query (comma-separated)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getBoardInfoMCP(ctx, keyer, r)
		})
		
		// Git Repository Management Tools
		s.AddTool(mcp.NewTool("create_git_repo",
			mcp.WithDescription("Initialize a new NIP-34 git repository"),
			mcp.WithString("identifier", mcp.Description("Unique identifier for the repository"), mcp.Required()),
			mcp.WithString("name", mcp.Description("Repository name")),
			mcp.WithString("description", mcp.Description("Repository description")),
			mcp.WithString("owner", mcp.Description("Owner public key (npub or hex) - defaults to current keyer if not provided")),
			mcp.WithString("web", mcp.Description("Web URLs for the repository")),
			mcp.WithString("grasp_servers", mcp.Description("GRASP servers for the repository")),
			mcp.WithString("maintainers", mcp.Description("Maintainer public keys (npub or hex)")),
			mcp.WithString("directory", mcp.Description("Directory to initialize repository in (defaults to ./identifier)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			identifier := required[string](r, "identifier")
			name, _ := optional[string](r, "name")
			description, _ := optional[string](r, "description")
			owner, _ := optional[string](r, "owner")
			webURLs, _ := optional[[]string](r, "web")
			graspServers, _ := optional[[]string](r, "grasp_servers")
			maintainers, _ := optional[[]string](r, "maintainers")
			directory, _ := optional[string](r, "directory")

			// Default to creating a subdirectory named after identifier if not specified
			if directory == "" {
				currentDir, err := os.Getwd()
				if err != nil {
					return mcp.NewToolResultError("failed to get current directory: " + err.Error()), nil
				}
				directory = filepath.Join(currentDir, identifier)
			}

			// Create target directory if it doesn't exist
			if err := os.MkdirAll(directory, 0755); err != nil {
				return mcp.NewToolResultError("failed to create directory: " + err.Error()), nil
			}

			// Check if directory is a git repository, initialize if not
			gitDir := filepath.Join(directory, ".git")
			if _, err := os.Stat(gitDir); os.IsNotExist(err) {
				initCmd := exec.Command("git", "init")
				initCmd.Dir = directory
				if output, err := initCmd.CombinedOutput(); err != nil {
					return mcp.NewToolResultError("failed to initialize git repository: " + string(output)), nil
				}
			}

			// Set default owner to current keyer if not provided
			if owner == "" {
				currentPk, err := keyer.GetPublicKey(ctx)
				if err != nil {
					return mcp.NewToolResultError("failed to get current public key: " + err.Error()), nil
				}
				owner = nip19.EncodeNpub(currentPk)
			}

			// Parse owner public key
			ownerPk, err := parsePubKey(owner)
			if err != nil {
				return mcp.NewToolResultError("invalid owner public key: " + err.Error()), nil
			}

			// Get earliest unique commit if not specified
			var earliestCommit string
			if output, err := exec.Command("git", "-C", directory, "rev-list", "--max-parents=0", "HEAD").Output(); err == nil {
				earliestCommit = strings.TrimSpace(string(output))
			}

			// Set defaults
			if name == "" {
				name = identifier
			}
			if len(graspServers) == 0 {
				graspServers = []string{"gitnostr.com", "relay.ngit.dev"}
			}

			// Create config
			config := Nip34Config{
				Identifier:           identifier,
				Name:                 name,
				Description:          description,
				Web:                  webURLs,
				Owner:                nip19.EncodeNpub(ownerPk),
				GraspServers:         graspServers,
				EarliestUniqueCommit: earliestCommit,
				Maintainers:          maintainers,
			}

			if err := config.Validate(); err != nil {
				return mcp.NewToolResultError("invalid config: " + err.Error()), nil
			}

			// Write config file
			if err := writeNip34ConfigFile(directory, config); err != nil {
				return mcp.NewToolResultError("failed to write nip34.json: " + err.Error()), nil
			}

			// Setup git remotes
			repo := config.ToRepository()
			gitSetupRemotes(ctx, directory, repo)

			// Add to git exclude
			excludeNip34ConfigFile(directory)

			// Create default README.md if it doesn't exist
			readmePath := filepath.Join(directory, "README.md")
			if _, err := os.Stat(readmePath); os.IsNotExist(err) {
				// Debug: Print that we're creating README
				fmt.Fprintf(os.Stderr, "DEBUG: Creating README.md at %s\n", readmePath)
				readmeContent := fmt.Sprintf(`# %s

A new Nostr repository created with Nak.

## Getting Started

This repository is hosted on the Nostr network using NIP-34.

### Clone this repository

` + "```bash" + `
nak clone %s/%s
` + "```" + `

### Add files and commit

` + "```bash" + `
# Add your files
git add .

# Commit changes
git commit -m "Initial commit"

# Push to Nostr relays
nak git push
` + "```" + `

## About

This repository was created using the Nak tool for Nostr-based git repositories.
`, name, owner, identifier)

				if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
					return mcp.NewToolResultError("failed to create README.md: " + err.Error()), nil
				}

				// Add README.md to git
				addCmd := exec.Command("git", "add", "README.md")
				addCmd.Dir = directory
				if output, err := addCmd.CombinedOutput(); err != nil {
					return mcp.NewToolResultError("failed to add README.md to git: " + string(output)), nil
				}

				// Commit README.md
				commitCmd := exec.Command("git", "commit", "-m", "init")
				commitCmd.Dir = directory
				if output, err := commitCmd.CombinedOutput(); err != nil {
					return mcp.NewToolResultError("failed to commit README.md: " + string(output)), nil
				}
			}

			// Auto-sync repository to relays and push initial commit
			originalDir, _ := os.Getwd()
			defer os.Chdir(originalDir)
			os.Chdir(directory)
			
			// First sync the repository to establish connection with relays
			_, _, syncErr := gitSync(ctx, keyer)
			if syncErr != nil {
				result := fmt.Sprintf("Successfully created NIP-34 repository '%s' in directory '%s'\n", identifier, directory)
				result += fmt.Sprintf("Repository name: %s\n", name)
				result += fmt.Sprintf("Owner: %s\n", owner)
				result += fmt.Sprintf("GRASP servers: %v\n", graspServers)
				result += fmt.Sprintf("Gitworkshop Link: https://gitworkshop.dev/%s/%s\n", owner, identifier)
				result += fmt.Sprintf("\n⚠️  Warning: Failed to sync to relays: %s\n", syncErr.Error())
				result += "Repository created locally but may not be available on relays. Run 'nak git sync' and 'nak git push' manually to publish."
				return mcp.NewToolResultText(result), nil
			}

			// After successful sync, attempt to push the initial commit with retry logic
			// This handles the case where relays need time to set up the remote repository
			// Retry every 10 seconds for 5 minutes (30 attempts × 10 seconds = 300 seconds = 5 minutes)
			if pushErr := retryInitialPush(ctx, keyer, 30, 10*time.Second); pushErr != nil {
				result := fmt.Sprintf("Successfully created and synced NIP-34 repository '%s' in directory '%s'\n", identifier, directory)
				result += fmt.Sprintf("Repository name: %s\n", name)
				result += fmt.Sprintf("Owner: %s\n", owner)
				result += fmt.Sprintf("GRASP servers: %v\n", graspServers)
				result += fmt.Sprintf("\n⚠️  Warning: Failed to push initial commit after retries: %s\n", pushErr.Error())
				result += "Repository is synced to relays but initial commit may not be published. Run 'nak git push' manually to push commits."
				result += "\n📝 This is normal for new repositories as relays may need time to set up the remote repository."
				return mcp.NewToolResultText(result), nil
			}

			result := fmt.Sprintf("Successfully created and synced NIP-34 repository '%s' in directory '%s'\n", identifier, directory)
			result += fmt.Sprintf("Repository name: %s\n", name)
			result += fmt.Sprintf("Owner: %s\n", owner)
			result += fmt.Sprintf("GRASP servers: %v\n", graspServers)
			result += fmt.Sprintf("Gitworkshop Link: https://gitworkshop.dev/%s/%s\n", owner, identifier)
			result += "\n✅ Repository is now published to relays and ready for use!"
			result += "\n✅ Initial commit with README.md has been pushed to the network!"

			return mcp.NewToolResultText(result), nil
		})

		s.AddTool(mcp.NewTool("clone_git_repo",
			mcp.WithDescription("Clone a NIP-34 repository from a nostr:// URI or address"),
			mcp.WithString("repository", mcp.Description("Repository address (format: <npub/hex/nprofile/nip05>/<identifier>, nostr://<npub>/<relay>/<identifier>, or naddr1...)"), mcp.Required()),
			mcp.WithString("directory", mcp.Description("Target directory (defaults to repository identifier)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			repoAddr := required[string](r, "repository")
			targetDir, _ := optional[string](r, "directory")

			// Parse repository address
			owner, identifier, relayHints, err := parseRepositoryAddress(ctx, repoAddr)
			if err != nil {
				return mcp.NewToolResultError("failed to parse repository address: " + err.Error()), nil
			}

			// Fetch repository metadata and state
			repo, _, state, err := fetchRepositoryAndState(ctx, owner, identifier, relayHints)
			if err != nil {
				return mcp.NewToolResultError("failed to fetch repository: " + err.Error()), nil
			}

			// Determine target directory
			if targetDir == "" {
				targetDir = repo.ID
			}
			if targetDir == "" {
				targetDir = identifier
			}

			// Check if target directory exists and is non-empty
			if fi, err := os.Stat(targetDir); err == nil && fi.IsDir() {
				entries, err := os.ReadDir(targetDir)
				if err == nil && len(entries) > 0 {
					return mcp.NewToolResultError("target directory '" + targetDir + "' already exists and is not empty"), nil
				}
			}

			// Create directory
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				return mcp.NewToolResultError("failed to create directory: " + err.Error()), nil
			}

			// Initialize git inside the directory
			initCmd := exec.Command("git", "init")
			initCmd.Dir = targetDir
			if output, err := initCmd.CombinedOutput(); err != nil {
				return mcp.NewToolResultError("failed to initialize git repository: " + string(output)), nil
			}

			// Write nip34.json
			localConfig := RepositoryToConfig(repo)
			if err := writeNip34ConfigFile(targetDir, localConfig); err != nil {
				return mcp.NewToolResultError("failed to write nip34.json: " + err.Error()), nil
			}

			// Add to git exclude
			excludeNip34ConfigFile(targetDir)

			// Setup git remotes
			gitSetupRemotes(ctx, targetDir, repo)

			// Fetch from remotes
			fetchFromRemotes(ctx, targetDir, repo)

			// Reset to HEAD if available
			if state != nil && state.HEAD != "" {
				if headCommit, ok := state.Branches[state.HEAD]; ok {
					checkCmd := exec.Command("git", "-C", targetDir, "cat-file", "-e", headCommit)
					if err := checkCmd.Run(); err == nil {
						resetCmd := exec.Command("git", "-C", targetDir, "reset", "--hard", headCommit)
						resetCmd.Run() // Ignore errors, reset is optional
					}
				}
			}

			// Update refs from state
			if state != nil {
				gitUpdateRefs(ctx, targetDir, *state)
			}

			result := fmt.Sprintf("Successfully cloned repository '%s' into directory '%s'\n", repo.ID, targetDir)
			result += fmt.Sprintf("Repository name: %s\n", repo.Name)
			result += fmt.Sprintf("Owner: %s\n", nip19.EncodeNpub(repo.PubKey))
			result += fmt.Sprintf("Description: %s\n", repo.Description)

			return mcp.NewToolResultText(result), nil
		})

		s.AddTool(mcp.NewTool("sync_git_repo",
			mcp.WithDescription("Sync repository metadata and state with Nostr relays"),
			mcp.WithString("directory", mcp.Description("Directory to sync (defaults to current directory)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			directory, _ := optional[string](r, "directory")

			if directory == "" {
				var err error
				directory, err = os.Getwd()
				if err != nil {
					return mcp.NewToolResultError("failed to get current directory: " + err.Error()), nil
				}
			}

			// Change to directory for git operations
			originalDir, _ := os.Getwd()
			defer os.Chdir(originalDir)
			os.Chdir(directory)

			repo, state, err := gitSync(ctx, keyer)
			if err != nil {
				return mcp.NewToolResultError("failed to sync repository: " + err.Error()), nil
			}

			result := fmt.Sprintf("Successfully synced repository '%s'\n", repo.ID)
			result += fmt.Sprintf("Repository name: %s\n", repo.Name)
			result += fmt.Sprintf("Owner: %s\n", nip19.EncodeNpub(repo.PubKey))
			
			if state != nil {
				result += fmt.Sprintf("Current HEAD: %s\n", state.HEAD)
				result += "Branches:\n"
				for branch, commit := range state.Branches {
					result += fmt.Sprintf("  %s: %s\n", branch, commit[:8])
				}
			}

			return mcp.NewToolResultText(result), nil
		})

		// Git Operations Tools
		s.AddTool(mcp.NewTool("git_push",
			mcp.WithDescription("Push git changes to Nostr relays"),
			mcp.WithString("branch", mcp.Description("Branch to push (defaults to current branch)")),
			mcp.WithString("remote_branch", mcp.Description("Remote branch name (defaults to local branch name)")),
			mcp.WithBoolean("force", mcp.Description("Force push even if not fast-forward")),
			mcp.WithString("directory", mcp.Description("Directory to push from (defaults to current directory)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			branch, _ := optional[string](r, "branch")
			remoteBranch, _ := optional[string](r, "remote_branch")
			force, _ := optional[bool](r, "force")
			directory, _ := optional[string](r, "directory")

			if directory == "" {
				var err error
				directory, err = os.Getwd()
				if err != nil {
					return mcp.NewToolResultError("failed to get current directory: " + err.Error()), nil
				}
			}

			// Change to directory for git operations
			originalDir, _ := os.Getwd()
			defer os.Chdir(originalDir)
			os.Chdir(directory)

			// Sync first to get latest state
			repo, state, err := gitSync(ctx, keyer)
			if err != nil {
				return mcp.NewToolResultError("failed to sync repository: " + err.Error()), nil
			}

			// Check if current user is allowed to push
			currentPk, err := keyer.GetPublicKey(ctx)
			if err != nil {
				return mcp.NewToolResultError("failed to get current public key: " + err.Error()), nil
			}

			if currentPk != repo.Event.PubKey && !slices.Contains(repo.Maintainers, currentPk) {
				return mcp.NewToolResultError("current user is not allowed to push to this repository"), nil
			}

			// Determine branches
			if branch == "" {
				cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
				output, err := cmd.Output()
				if err != nil {
					return mcp.NewToolResultError("failed to get current branch: " + err.Error()), nil
				}
				branch = strings.TrimSpace(string(output))
			}

			if remoteBranch == "" {
				remoteBranch = branch
			}

			// Get current commit
			cmd := exec.Command("git", "rev-parse", branch)
			output, err := cmd.Output()
			if err != nil {
				return mcp.NewToolResultError("failed to get branch commit: " + err.Error()), nil
			}
			currentCommit := strings.TrimSpace(string(output))

			// Create or update state
			if state == nil {
				state = &nip34.RepositoryState{
					ID:       repo.ID,
					Branches: make(map[string]string),
					Tags:     make(map[string]string),
				}
			}

			// Check fast-forward if not force
			if !force {
				if prevCommit, exists := state.Branches[remoteBranch]; exists {
					cmd := exec.Command("git", "merge-base", "--is-ancestor", prevCommit, currentCommit)
					if err := cmd.Run(); err != nil {
						return mcp.NewToolResultError("non-fast-forward push not allowed, use force=true to override"), nil
					}
				}
			}

			// Update state
			state.Branches[remoteBranch] = currentCommit
			if state.HEAD == "" {
				state.HEAD = remoteBranch
			}

			// Sign and publish state event
			newStateEvent := state.ToEvent()
			if err := keyer.SignEvent(ctx, &newStateEvent); err != nil {
				return mcp.NewToolResultError("failed to sign state event: " + err.Error()), nil
			}

			// Publish to relays
			publishCount := 0
			for res := range sys.Pool.PublishMany(ctx, repo.Relays, newStateEvent) {
				if res.Error != nil {
					continue
				}
				publishCount++
			}

			if publishCount == 0 {
				return mcp.NewToolResultError("failed to publish state event to any relay"), nil
			}

			// Push to grasp remotes
			pushSuccesses := 0
			for _, relay := range repo.Relays {
				remoteName := gitRemoteName(relay)
				pushArgs := []string{"push", remoteName, fmt.Sprintf("%s:refs/heads/%s", branch, remoteBranch)}
				if force {
					pushArgs = append(pushArgs, "--force")
				}
				pushCmd := exec.Command("git", pushArgs...)
				if err := pushCmd.Run(); err == nil {
					pushSuccesses++
				}
			}

			result := fmt.Sprintf("Successfully pushed branch '%s' to remote branch '%s'\n", branch, remoteBranch)
			result += fmt.Sprintf("Commit: %s\n", currentCommit)
			result += fmt.Sprintf("Published to %d relays, pushed to %d grasp servers\n", publishCount, pushSuccesses)

			return mcp.NewToolResultText(result), nil
		})

		s.AddTool(mcp.NewTool("git_pull",
			mcp.WithDescription("Pull git changes from Nostr relays"),
			mcp.WithString("branch", mcp.Description("Branch to pull (defaults to current branch)")),
			mcp.WithString("remote_branch", mcp.Description("Remote branch name (defaults to local branch name)")),
			mcp.WithString("strategy", mcp.Description("Merge strategy: merge, rebase, ff-only")),
			mcp.WithString("directory", mcp.Description("Directory to pull into (defaults to current directory)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			branch, _ := optional[string](r, "branch")
			remoteBranch, _ := optional[string](r, "remote_branch")
			strategy, _ := optional[string](r, "strategy")
			directory, _ := optional[string](r, "directory")

			if directory == "" {
				var err error
				directory, err = os.Getwd()
				if err != nil {
					return mcp.NewToolResultError("failed to get current directory: " + err.Error()), nil
				}
			}

			// Change to directory for git operations
			originalDir, _ := os.Getwd()
			defer os.Chdir(originalDir)
			os.Chdir(directory)

			// Sync to get latest state
			_, state, err := gitSync(ctx, nil)
			if err != nil {
				return mcp.NewToolResultError("failed to sync repository: " + err.Error()), nil
			}

			if state == nil || state.Event.ID == nostr.ZeroID {
				return mcp.NewToolResultError("no repository state found"), nil
			}

			// Determine branches
			if branch == "" {
				cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
				output, err := cmd.Output()
				if err != nil {
					return mcp.NewToolResultError("failed to get current branch: " + err.Error()), nil
				}
				branch = strings.TrimSpace(string(output))
			}

			if remoteBranch == "" {
				remoteBranch = branch
			}

			// Get target commit from state
			targetCommit, ok := state.Branches[remoteBranch]
			if !ok {
				return mcp.NewToolResultError("branch '" + remoteBranch + "' not found in repository state"), nil
			}

			// Check if commit exists locally
			cmd := exec.Command("git", "cat-file", "-e", targetCommit)
			if err := cmd.Run(); err != nil {
				return mcp.NewToolResultError("commit " + targetCommit + " not found locally, try git_fetch first"), nil
			}

			// Determine strategy
			if strategy == "" {
				strategy = "merge"
			}

			// Execute the merge/rebase
			switch strategy {
			case "rebase":
				cmd = exec.Command("git", "rebase", targetCommit)
			case "ff-only":
				cmd = exec.Command("git", "merge", "--ff-only", targetCommit)
			default:
				cmd = exec.Command("git", "merge", targetCommit)
			}

			if output, err := cmd.CombinedOutput(); err != nil {
				return mcp.NewToolResultError("pull failed: " + string(output)), nil
			}

			result := fmt.Sprintf("Successfully pulled changes for branch '%s'\n", branch)
			result += fmt.Sprintf("Remote branch: %s\n", remoteBranch)
			result += fmt.Sprintf("Target commit: %s\n", targetCommit)
			result += fmt.Sprintf("Strategy: %s\n", strategy)

			return mcp.NewToolResultText(result), nil
		})

		s.AddTool(mcp.NewTool("git_fetch",
			mcp.WithDescription("Fetch git data from Nostr relays"),
			mcp.WithString("directory", mcp.Description("Directory to fetch into (defaults to current directory)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			directory, _ := optional[string](r, "directory")

			if directory == "" {
				var err error
				directory, err = os.Getwd()
				if err != nil {
					return mcp.NewToolResultError("failed to get current directory: " + err.Error()), nil
				}
			}

			// Change to directory for git operations
			originalDir, _ := os.Getwd()
			defer os.Chdir(originalDir)
			os.Chdir(directory)

			repo, _, err := gitSync(ctx, nil)
			if err != nil {
				return mcp.NewToolResultError("failed to sync repository: " + err.Error()), nil
			}

			result := fmt.Sprintf("Successfully fetched repository '%s'\n", repo.ID)
			result += fmt.Sprintf("Repository name: %s\n", repo.Name)
			result += fmt.Sprintf("GRASP servers: %v\n", repo.Relays)

			return mcp.NewToolResultText(result), nil
		})

		// Pull Request Tools
		// IMPORTANT: When creating pull requests, remember to:
		// 1. Create a new branch BEFORE making changes
		// 2. Add your changes and commit to the new branch
		// 3. Push the new branch
		// 4. Create PR from new branch to base branch
		s.AddTool(mcp.NewTool("create_pull_request",
			mcp.WithDescription("Create a pull request (kind 1618)"),
			mcp.WithString("base_repository", mcp.Description("Base repository address (format: <npub>/<identifier> or leave empty for internal PR)"), mcp.Required()),
			mcp.WithString("base_branch", mcp.Description("Base branch to merge into (defaults to 'master')")),
			mcp.WithString("head_branch", mcp.Description("Head branch to merge from (required)"), mcp.Required()),
			mcp.WithString("subject", mcp.Description("Pull request title/description (required)"), mcp.Required()),
			mcp.WithString("relay", mcp.Description("Relay to publish PR to (will use configured relays if not specified)")),
			mcp.WithString("directory", mcp.Description("Directory to create PR from (defaults to current directory)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			baseRepo := required[string](r, "base_repository")
			baseBranch, _ := optional[string](r, "base_branch")
			headBranch := required[string](r, "head_branch")
			subject := required[string](r, "subject")
			relay, _ := optional[string](r, "relay")
			directory, _ := optional[string](r, "directory")

			if directory == "" {
				var err error
				directory, err = os.Getwd()
				if err != nil {
					return mcp.NewToolResultError("failed to get current directory: " + err.Error()), nil
				}
			}

			// Change to directory for git operations
			originalDir, _ := os.Getwd()
			defer os.Chdir(originalDir)
			os.Chdir(directory)

			if baseBranch == "" {
				baseBranch = "master"
			}

			prNevent, err := createPullRequestWithSigner(ctx, keyer, baseRepo, baseBranch, headBranch, subject, relay)
			if err != nil {
				return mcp.NewToolResultError("failed to create pull request: " + err.Error()), nil
			}

			result := fmt.Sprintf("Successfully created pull request\n")
			result += fmt.Sprintf("Base branch: %s\n", baseBranch)
			result += fmt.Sprintf("Head branch: %s\n", headBranch)
			result += fmt.Sprintf("Subject: %s\n", subject)
			if prNevent != nil {
				result += fmt.Sprintf("Pull Request ID: %s\n", *prNevent)
				result += fmt.Sprintf("Gitworkshop Link: https://gitworkshop.dev/%s\n", *prNevent)
			}

			return mcp.NewToolResultText(result), nil
		})

		s.AddTool(mcp.NewTool("update_pull_request",
			mcp.WithDescription("Update an existing pull request (kind 1619)"),
			mcp.WithString("pr_id", mcp.Description("ID of the PR event to update (required)"), mcp.Required()),
			mcp.WithString("head_branch", mcp.Description("New head branch with updated commits (required)"), mcp.Required()),
			mcp.WithString("subject", mcp.Description("Updated PR title/description (required)"), mcp.Required()),
			mcp.WithString("relay", mcp.Description("Relay to publish PR update to (will use configured relays if not specified)")),
			mcp.WithString("directory", mcp.Description("Directory to update PR from (defaults to current directory)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			prID := required[string](r, "pr_id")
			headBranch := required[string](r, "head_branch")
			subject := required[string](r, "subject")
			relay, _ := optional[string](r, "relay")
			directory, _ := optional[string](r, "directory")

			if directory == "" {
				var err error
				directory, err = os.Getwd()
				if err != nil {
					return mcp.NewToolResultError("failed to get current directory: " + err.Error()), nil
				}
			}

			// Change to directory for git operations
			originalDir, _ := os.Getwd()
			defer os.Chdir(originalDir)
			os.Chdir(directory)

			if err := updatePullRequestWithSigner(ctx, keyer, prID, headBranch, subject, relay); err != nil {
				return mcp.NewToolResultError("failed to update pull request: " + err.Error()), nil
			}

			result := fmt.Sprintf("Successfully updated pull request\n")
			result += fmt.Sprintf("PR ID: %s\n", prID)
			result += fmt.Sprintf("New head branch: %s\n", headBranch)
			result += fmt.Sprintf("Subject: %s\n", subject)

			return mcp.NewToolResultText(result), nil
		})

		// Repository Information Tools
		s.AddTool(mcp.NewTool("get_git_repo_info",
			mcp.WithDescription("Get repository metadata and status"),
			mcp.WithString("directory", mcp.Description("Directory to get info for (defaults to current directory)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			directory, _ := optional[string](r, "directory")

			if directory == "" {
				var err error
				directory, err = os.Getwd()
				if err != nil {
					return mcp.NewToolResultError("failed to get current directory: " + err.Error()), nil
				}
			}

			// Change to directory for git operations
			originalDir, _ := os.Getwd()
			defer os.Chdir(originalDir)
			os.Chdir(directory)

			// Read local config
			localConfig, err := readNip34ConfigFile("")
			if err != nil {
				return mcp.NewToolResultError("failed to read repository config: " + err.Error()), nil
			}

			// Fetch latest from relays
			owner, err := parsePubKey(localConfig.Owner)
			if err != nil {
				return mcp.NewToolResultError("failed to parse owner pubkey: " + err.Error()), nil
			}

			repo, _, state, err := fetchRepositoryAndState(ctx, owner, localConfig.Identifier, localConfig.GraspServers)
			if err != nil {
				return mcp.NewToolResultError("failed to fetch repository: " + err.Error()), nil
			}

			// Get current git branch
			currentBranch, _ := getCurrentGitBranch("")

			result := fmt.Sprintf("Repository Information\n")
			result += fmt.Sprintf("===================\n")
			result += fmt.Sprintf("ID: %s\n", repo.ID)
			result += fmt.Sprintf("Name: %s\n", repo.Name)
			result += fmt.Sprintf("Description: %s\n", repo.Description)
			result += fmt.Sprintf("Owner: %s\n", nip19.EncodeNpub(repo.PubKey))
			result += fmt.Sprintf("Web URLs: %v\n", repo.Web)
			result += fmt.Sprintf("GRASP Servers: %v\n", repo.Relays)
			result += fmt.Sprintf("Maintainers: %v\n", repo.Maintainers)
			result += fmt.Sprintf("Earliest Commit: %s\n", repo.EarliestUniqueCommitID)
			result += fmt.Sprintf("Current Git Branch: %s\n", currentBranch)

			if state != nil {
				result += fmt.Sprintf("\nRepository State\n")
				result += fmt.Sprintf("==============\n")
				result += fmt.Sprintf("HEAD: %s\n", state.HEAD)
				result += "Branches:\n"
				for branch, commit := range state.Branches {
					current := ""
					if branch == currentBranch {
						current = " (current)"
					}
					result += fmt.Sprintf("  %s: %s%s\n", branch, commit[:8], current)
				}
			}

			return mcp.NewToolResultText(result), nil
		})

		s.AddTool(mcp.NewTool("list_git_branches",
			mcp.WithDescription("List available branches in the repository"),
			mcp.WithString("directory", mcp.Description("Directory to list branches for (defaults to current directory)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			directory, _ := optional[string](r, "directory")

			if directory == "" {
				var err error
				directory, err = os.Getwd()
				if err != nil {
					return mcp.NewToolResultError("failed to get current directory: " + err.Error()), nil
				}
			}

			// Change to directory for git operations
			originalDir, _ := os.Getwd()
			defer os.Chdir(originalDir)
			os.Chdir(directory)

			// Get current branch
			currentBranch, err := getCurrentGitBranch("")
			if err != nil {
				return mcp.NewToolResultError("failed to get current branch: " + err.Error()), nil
			}

			// Get local branches
			cmd := exec.Command("git", "branch", "-a")
			output, err := cmd.Output()
			if err != nil {
				return mcp.NewToolResultError("failed to list branches: " + err.Error()), nil
			}

			result := fmt.Sprintf("Repository Branches\n")
			result += fmt.Sprintf("==================\n")
			result += fmt.Sprintf("Current branch: %s\n\n", currentBranch)
			result += "All branches:\n"

			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "* ") {
					line = strings.TrimPrefix(line, "* ")
					result += fmt.Sprintf("  * %s (current)\n", line)
				} else {
					line = strings.TrimPrefix(line, "  ")
					result += fmt.Sprintf("    %s\n", line)
				}
			}

			// Also get remote state branches if available
			_, state, err := gitSync(ctx, nil)
			if err == nil && state != nil {
				result += "\nRemote state branches:\n"
				for branch, commit := range state.Branches {
					result += fmt.Sprintf("  %s: %s\n", branch, commit[:8])
				}
			}

			return mcp.NewToolResultText(result), nil
		})

		s.AddTool(mcp.NewTool("get_git_status",
			mcp.WithDescription("Get current git repository status"),
			mcp.WithString("directory", mcp.Description("Directory to get status for (defaults to current directory)")),
		), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			directory, _ := optional[string](r, "directory")

			if directory == "" {
				var err error
				directory, err = os.Getwd()
				if err != nil {
					return mcp.NewToolResultError("failed to get current directory: " + err.Error()), nil
				}
			}

			// Get git status
			cmd := exec.Command("git", "-C", directory, "status", "--porcelain", "-b")
			output, err := cmd.Output()
			if err != nil {
				return mcp.NewToolResultError("failed to get git status: " + err.Error()), nil
			}

			// Get current commit
			cmd = exec.Command("git", "-C", directory, "rev-parse", "HEAD")
			commitOutput, _ := cmd.Output()
			currentCommit := strings.TrimSpace(string(commitOutput))

			result := fmt.Sprintf("Git Status\n")
			result += fmt.Sprintf("===========\n")
			result += fmt.Sprintf("Directory: %s\n", directory)
			result += fmt.Sprintf("Current commit: %s\n", currentCommit)
			result += "\nStatus output:\n"
			result += string(output)

			return mcp.NewToolResultText(result), nil
		})
		return server.ServeStdio(s)
	},
}

// retryInitialPush attempts to push the initial commit with retry logic for new repositories
// This handles the case where relays need time to set up the remote repository
func retryInitialPush(ctx context.Context, signer nostr.Keyer, maxRetries int, retryInterval time.Duration) error {
	fmt.Fprintf(os.Stderr, "Starting initial push retry loop (max %d retries, %v interval)...\n", maxRetries, retryInterval)
	
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			fmt.Fprintf(os.Stderr, "Waiting %v before attempt %d...\n", retryInterval, attempt+1)
			select {
			case <-time.After(retryInterval):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		
		fmt.Fprintf(os.Stderr, "Attempt %d/%d: pushing initial commit...\n", attempt+1, maxRetries)
		
		if err := gitPush(ctx, signer, "", "master", false); err != nil {
			fmt.Fprintf(os.Stderr, "Attempt %d failed: %v\n", attempt+1, err)
			
			// Check if this is the last attempt
			if attempt == maxRetries-1 {
				return fmt.Errorf("failed to push initial commit after %d attempts: %w", maxRetries, err)
			}
			continue
		}
		
		fmt.Fprintf(os.Stderr, "Initial push successful on attempt %d!\n", attempt+1)
		return nil
	}
	
	return fmt.Errorf("unexpected: retry loop completed without success or failure")
}

func required[T comparable](r mcp.CallToolRequest, p string) T {
	var zero T
	if _, ok := r.Params.Arguments[p]; !ok {
		return zero
	}
	if _, ok := r.Params.Arguments[p].(T); !ok {
		return zero
	}
	if r.Params.Arguments[p].(T) == zero {
		return zero
	}
	return r.Params.Arguments[p].(T)
}

func optional[T any](r mcp.CallToolRequest, p string) (T, bool) {
	var zero T
	if _, ok := r.Params.Arguments[p]; !ok {
		return zero, false
	}
	if _, ok := r.Params.Arguments[p].(T); !ok {
		return zero, false
	}
	return r.Params.Arguments[p].(T), true
}
