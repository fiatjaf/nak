package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fiatjaf.com/nostr"
	"github.com/urfave/cli/v3"
	"github.com/mark3labs/mcp-go/mcp"
)

var kanban = &cli.Command{
	Name:  "kanban",
	Usage: "kanban board operations",
	Description: `create and manage kanban boards using Nostr events (kinds 30301 for boards, 30302 for cards)`,
	Commands: []*cli.Command{
		{
			Name:  "create-board",
			Usage: "create a new kanban board",
			Flags: append(defaultKeyFlags,
				&cli.StringFlag{
					Name:     "title",
					Usage:    "board title",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "description", 
					Usage:    "board description",
				},
				&cli.StringSliceFlag{
					Name:     "relay",
					Usage:    "relay URLs to publish to",
				},
				&cli.StringFlag{
					Name:     "board-id",
					Usage:    "board identifier (auto-generated if not provided)",
				},
				&cli.BoolFlag{
					Name:     "debug",
					Usage:    "show Highlighter URLs for debugging",
				},
			),
			Action: createBoardCLI,
		},
		{
			Name:  "create-card",
			Usage: "create a new kanban card",
			Flags: append(defaultKeyFlags,
				&cli.StringFlag{
					Name:     "title",
					Usage:    "card title",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "description",
					Usage:    "card description",
				},
				&cli.StringFlag{
					Name:     "board-id",
					Usage:    "board identifier",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "board-pubkey",
					Usage:    "board owner public key",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "column",
					Usage:    "column name",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "priority",
					Usage:    "card priority (low, medium, high)",
					Value:    "medium",
				},
				&cli.StringSliceFlag{
					Name:     "relay",
					Usage:    "relay URLs to publish to",
				},
				&cli.BoolFlag{
					Name:     "debug",
					Usage:    "show Highlighter URLs for debugging",
				},
			),
			Action: createCardCLI,
		},
		{
			Name:  "update-card",
			Usage: "update any field of a card (title, description, column, priority)",
			Flags: append(defaultKeyFlags,
				&cli.StringFlag{
					Name:     "card-title",
					Usage:    "card title to search for",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "board-id",
					Usage:    "board identifier",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "board-pubkey",
					Usage:    "board owner public key",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "new-title",
					Usage:    "new card title",
				},
				&cli.StringFlag{
					Name:     "new-description",
					Usage:    "new card description",
				},
				&cli.StringFlag{
					Name:     "new-column",
					Usage:    "new column name",
				},
				&cli.StringFlag{
					Name:     "new-priority",
					Usage:    "new card priority (low, medium, high)",
				},
				&cli.StringSliceFlag{
					Name:     "relay",
					Usage:    "relay URLs to publish to",
				},
				&cli.BoolFlag{
					Name:     "debug",
					Usage:    "show Highlighter URLs for debugging",
				},
			),
			Action: updateCardCLI,
		},
		{
			Name:  "list-cards",
			Usage: "list cards on a board",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "board-id",
					Usage:    "board identifier",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "board-pubkey",
					Usage:    "board owner public key",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "column",
					Usage:    "filter by column",
				},
				&cli.IntFlag{
					Name:     "limit",
					Usage:    "maximum number of cards to return",
					Value:    50,
				},
				&cli.StringSliceFlag{
					Name:     "relay",
					Usage:    "relay URLs to query",
				},
				&cli.BoolFlag{
					Name:     "debug",
					Usage:    "show Highlighter URLs for debugging",
				},
			},
			Action: listCardsCLI,
		},
		{
			Name:  "board-info",
			Usage: "show board information",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "board-id",
					Usage:    "board identifier",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "board-pubkey",
					Usage:    "board owner public key",
					Required: true,
				},
				&cli.StringSliceFlag{
					Name:     "relay",
					Usage:    "relay URLs to query",
				},
				&cli.BoolFlag{
					Name:     "debug",
					Usage:    "show Highlighter URLs for debugging",
				},
			},
			Action: boardInfoCLI,
		},
	},
}

// CLI handlers
func createBoardCLI(ctx context.Context, c *cli.Command) error {
	keyer, _, err := gatherKeyerFromArguments(ctx, c)
	if err != nil {
		return err
	}

	title := c.String("title")
	description := c.String("description")
	boardID := c.String("board-id")
	relays := c.StringSlice("relay")

	boardID = generateUUID()

	result, err := createBoard(ctx, keyer, title, description, boardID, relays)
	if err != nil {
		return err
	}

	fmt.Printf("✓ Board created: %s\n", result.BoardID)
	fmt.Printf("✓ Event ID: %s\n", result.EventID)
	fmt.Printf("✓ Board URL: %s\n", result.KanbanstrURL)
	
	// Only show Highlighter URL if debug flag is provided
	if c.Bool("debug") {
		fmt.Printf("✓ Highlighter: https://highlighter.com/a/%s\n", result.Naddr)
	}

	return nil
}

func createCardCLI(ctx context.Context, c *cli.Command) error {
	keyer, _, err := gatherKeyerFromArguments(ctx, c)
	if err != nil {
		return err
	}

	title := c.String("title")
	description := c.String("description")
	boardID := c.String("board-id")
	boardPubkey := c.String("board-pubkey")
	column := c.String("column")
	priority := c.String("priority")
	relays := c.StringSlice("relay")

	result, err := createCard(ctx, keyer, title, description, boardID, boardPubkey, column, priority, relays)
	if err != nil {
		return err
	}

	fmt.Printf("✓ Card created: %s\n", title)
	fmt.Printf("✓ Event ID: %s\n", result.EventID)
	
	// Only show Highlighter URL if debug flag is provided
	if c.Bool("debug") {
		fmt.Printf("✓ Card Highlighter: https://highlighter.com/a/%s\n", result.Naddr)
	}

	return nil
}

func updateCardCLI(ctx context.Context, c *cli.Command) error {
	keyer, _, err := gatherKeyerFromArguments(ctx, c)
	if err != nil {
		return err
	}

	cardTitle := c.String("card-title")
	boardID := c.String("board-id")
	boardPubkey := c.String("board-pubkey")
	newTitle := c.String("new-title")
	newDescription := c.String("new-description")
	newColumn := c.String("new-column")
	newPriority := c.String("new-priority")
	relays := c.StringSlice("relay")

	result, err := updateCard(ctx, keyer, cardTitle, boardID, boardPubkey, newTitle, newDescription, newColumn, newPriority, relays)
	if err != nil {
		return err
	}

	fmt.Printf("✓ Card '%s' updated successfully\n", cardTitle)
	fmt.Printf("✓ Event ID: %s\n", result.EventID)
	
	// Only show Highlighter URL if debug flag is provided
	if c.Bool("debug") {
		fmt.Printf("✓ Updated Card Highlighter: https://highlighter.com/a/%s\n", result.Naddr)
	}

	return nil
}

func listCardsCLI(ctx context.Context, c *cli.Command) error {
	boardID := c.String("board-id")
	boardPubkey := c.String("board-pubkey")
	column := c.String("column")
	limit := c.Int("limit")
	relays := c.StringSlice("relay")

	cards, err := listCards(ctx, boardID, boardPubkey, column, int64(limit), relays)
	if err != nil {
		return err
	}

	if len(cards) == 0 {
		fmt.Println("No cards found")
		return nil
	}

	fmt.Printf("Found %d cards:\n\n", len(cards))
	for i, card := range cards {
		fmt.Printf("%d. %s\n", i+1, card.Title)
		fmt.Printf("   Status: %s\n", card.Status)
		fmt.Printf("   Priority: %s\n", card.Priority)
		if card.Description != "" {
			fmt.Printf("   Description: %s\n", card.Description)
		}
		fmt.Printf("   Event ID: %s\n", card.EventID)
		if c.Bool("debug") {
			fmt.Printf("   Card Highlighter: https://highlighter.com/a/%s\n", card.Naddr)
		}
		fmt.Println()
	}

	return nil
}

func boardInfoCLI(ctx context.Context, c *cli.Command) error {
	boardID := c.String("board-id")
	boardPubkey := c.String("board-pubkey")
	relays := c.StringSlice("relay")

	boardInfo, err := getBoardInfo(ctx, boardID, boardPubkey, relays)
	if err != nil {
		return err
	}

	fmt.Printf("Board: %s\n", boardInfo.Title)
	fmt.Printf("Description: %s\n", boardInfo.Description)
	fmt.Printf("Event ID: %s\n", boardInfo.EventID)
	fmt.Printf("Board URL: %s\n", boardInfo.KanbanstrURL)
	if c.Bool("debug") {
		fmt.Printf("Board Highlighter: https://highlighter.com/a/%s\n", boardInfo.Naddr)
	}
	fmt.Println("\nColumns:")
	for _, col := range boardInfo.Columns {
		fmt.Printf("  - %s (ID: %s)\n", col.Name, col.UUID)
	}

	return nil
}

// Core data structures
type BoardResult struct {
	BoardID      string
	EventID      string
	Naddr        string
	KanbanstrURL string
}

type CardResult struct {
	EventID string
	Naddr   string
}

type BoardInfo struct {
	BoardID      string
	EventID      string
	Title        string
	Description  string
	Columns      []Column
	KanbanstrURL string
	Naddr        string
}

type Column struct {
	UUID string
	Name string
	Order int
}

type Card struct {
	EventID    string
	Title      string
	Status     string
	Priority   string
	ColumnUUID string
	Description string
	Naddr      string
}

// Core functions shared by CLI and MCP
func createBoard(ctx context.Context, keyer nostr.Keyer, title, description, boardID string, relays []string) (*BoardResult, error) {
	if len(relays) == 0 {
		relays = []string{"wss://relay.damus.io", "wss://nos.lol"}
	}

	pubkey, err := keyer.GetPublicKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	// Generate column UUIDs and names
	columns := []struct {
		UUID string
		Name string
		Order int
	}{
		{generateUUID(), "Ideas", 0},
		{generateUUID(), "Backlog", 1},
		{generateUUID(), "In Progress", 2},
		{generateUUID(), "Testing", 3},
		{generateUUID(), "Review", 4},
		{generateUUID(), "Done", 5},
	}

	// Create event tags with d tag for board identifier
	tags := nostr.Tags{
		{"d", boardID}, // d tag with UUID identifier
		{"title", title},
		{"alt", fmt.Sprintf("A board titled %s", title)},
	}

	if description != "" {
		tags = append(tags, []string{"description", description})
	}

	for _, col := range columns {
		tags = append(tags, []string{"col", col.UUID, col.Name, fmt.Sprintf("%d", col.Order)})
	}

	// Create board event (kind 30301)
	event := nostr.Event{
		Kind:      30301,
		CreatedAt: nostr.Now(),
		Tags:      tags,
		Content:   "",
	}

	if err := keyer.SignEvent(ctx, &event); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}

	// Publish to relays
	for res := range sys.Pool.PublishMany(ctx, relays, event) {
		if res.Error != nil {
			log("Error publishing to %s: %v\n", res.RelayURL, res.Error)
		} else {
			log("Published to %s\n", res.RelayURL)
		}
	}

	// Generate proper naddr for board
	naddr, err := generateNaddr(event.ID.String(), pubkey.Hex(), "30301", boardID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate naddr: %w", err)
	}

	// Generate kanbanstr URL
	kanbanstrURL := fmt.Sprintf("https://www.kanbanstr.com/#/board/%s/%s", pubkey.Hex(), boardID)

	return &BoardResult{
		BoardID:      boardID,
		EventID:      event.ID.String(),
		Naddr:        naddr,
		KanbanstrURL: kanbanstrURL,
	}, nil
}

func createCard(ctx context.Context, keyer nostr.Keyer, title, description, boardID, boardPubkey, column, priority string, relays []string) (*CardResult, error) {
	if len(relays) == 0 {
		relays = []string{"wss://relay.damus.io", "wss://nos.lol"}
	}

	// Parse board pubkey
	pk, err := nostr.PubKeyFromHex(boardPubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid board pubkey: %w", err)
	}

	// Get board info to find column UUID
	boardInfo, err := getBoardInfo(ctx, boardID, pk.Hex(), relays)
	if err != nil {
		return nil, fmt.Errorf("failed to get board info: %w", err)
	}

	var columnUUID string
	for _, col := range boardInfo.Columns {
		if col.Name == column {
			columnUUID = col.UUID
			break
		}
	}
	if columnUUID == "" {
		return nil, fmt.Errorf("column '%s' not found on board", column)
	}

	// Generate card UUID for d tag
	cardUUID := generateUUID()

	// Create card tags matching the example format exactly
	tags := nostr.Tags{
		{"d", cardUUID}, // d tag with UUID identifier (like the example)
		{"title", title},
		{"description", description},
		{"alt", fmt.Sprintf("A card titled %s", title)},
		{"rank", "0"},
		{"a", fmt.Sprintf("30301:%s:%s", pk.Hex(), boardID)}, // Link to board (like the example)
		{"s", column}, // Status/column name
	}

	// Create card event (kind 30302)
	event := nostr.Event{
		Kind:      30302,
		CreatedAt: nostr.Now(),
		Tags:      tags,
		Content:   description,
	}

	if err := keyer.SignEvent(ctx, &event); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}

	// Publish to relays
	for res := range sys.Pool.PublishMany(ctx, relays, event) {
		if res.Error != nil {
			log("Error publishing to %s: %v\n", res.RelayURL, res.Error)
		} else {
			log("Published to %s\n", res.RelayURL)
		}
	}

	// Generate proper naddr for card using card UUID as identifier
	naddr, err := generateNaddr(event.ID.String(), pk.Hex(), "30302", cardUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate naddr: %w", err)
	}

	return &CardResult{
		EventID: event.ID.String(),
		Naddr:   naddr,
	}, nil
}

func updateCard(ctx context.Context, keyer nostr.Keyer, cardTitle, boardID, boardPubkey, newTitle, newDescription, newColumn, newPriority string, relays []string) (*CardResult, error) {
	if len(relays) == 0 {
		relays = []string{"wss://relay.damus.io", "wss://nos.lol"}
	}

	// Check if at least one field is being updated
	if newTitle == "" && newDescription == "" && newColumn == "" && newPriority == "" {
		return nil, fmt.Errorf("at least one field must be updated (title, description, column, or priority)")
	}

	// Parse board pubkey
	pk, err := nostr.PubKeyFromHex(boardPubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid board pubkey: %w", err)
	}

	// Find card
	card, err := findCardByTitle(ctx, boardID, pk.Hex(), cardTitle, relays)
	if err != nil {
		return nil, fmt.Errorf("failed to find card: %w", err)
	}

	// Get board info if updating column
	var boardInfo *BoardInfo
	if newColumn != "" {
		boardInfo, err = getBoardInfo(ctx, boardID, pk.Hex(), relays)
		if err != nil {
			return nil, fmt.Errorf("failed to get board info: %w", err)
		}
	}

	// Update card tags based on provided parameters
	tags := make(nostr.Tags, len(card.Tags))
	copy(tags, card.Tags)

	// Update title if provided
	if newTitle != "" {
		for i, tag := range tags {
			if len(tag) >= 2 && tag[0] == "title" {
				tags[i] = []string{"title", newTitle}
				break
			}
		}
		// Also update alt tag if present
		for i, tag := range tags {
			if len(tag) >= 2 && tag[0] == "alt" {
				tags[i] = []string{"alt", fmt.Sprintf("A card titled %s", newTitle)}
				break
			}
		}
	}

	// Update description tag if provided
	if newDescription != "" {
		for i, tag := range tags {
			if len(tag) >= 2 && tag[0] == "description" {
				tags[i] = []string{"description", newDescription}
				break
			}
		}
	}

	// Update column if provided
	if newColumn != "" {
		var newColumnUUID string
		for _, col := range boardInfo.Columns {
			if col.Name == newColumn {
				newColumnUUID = col.UUID
				break
			}
		}
		if newColumnUUID == "" {
			return nil, fmt.Errorf("column '%s' not found on board", newColumn)
		}
		for i, tag := range tags {
			if len(tag) > 0 && tag[0] == "s" {
				tags[i] = []string{"s", newColumn}
				break
			}
		}
	}

	// Update priority if provided
	if newPriority != "" {
		for i, tag := range tags {
			if len(tag) >= 2 && tag[0] == "priority" {
				tags[i] = []string{"priority", newPriority}
				break
			}
		}
	}

	// Determine content for the new card
	content := card.Content
	if newDescription != "" {
		content = newDescription
	}

	// Create updated card event
	event := nostr.Event{
		Kind:      30302,
		CreatedAt: nostr.Now(),
		Tags:      tags,
		Content:   content,
	}

	if err := keyer.SignEvent(ctx, &event); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}

	// Publish to relays
	for res := range sys.Pool.PublishMany(ctx, relays, event) {
		if res.Error != nil {
			log("Error publishing to %s: %v\n", res.RelayURL, res.Error)
		} else {
			log("Published to %s\n", res.RelayURL)
		}
	}

	// Extract card UUID from d tag for naddr generation
	var cardUUID string
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == "d" {
			cardUUID = tag[1]
			break
		}
	}

	// Generate proper naddr for card
	naddr, err := generateNaddr(event.ID.String(), pk.Hex(), "30302", cardUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate naddr: %w", err)
	}

	return &CardResult{
		EventID: event.ID.String(),
		Naddr:   naddr,
	}, nil
}

// Keep moveCard for backward compatibility but have it call updateCard
func moveCard(ctx context.Context, keyer nostr.Keyer, cardTitle, boardID, boardPubkey, newColumn string, relays []string) (*CardResult, error) {
	return updateCard(ctx, keyer, cardTitle, boardID, boardPubkey, "", "", newColumn, "", relays)
}

func listCards(ctx context.Context, boardID, boardPubkey, column string, limit int64, relays []string) ([]Card, error) {
	if len(relays) == 0 {
		relays = []string{"wss://relay.damus.io", "wss://nos.lol"}
	}

	// Parse board pubkey
	pk, err := nostr.PubKeyFromHex(boardPubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid board pubkey: %w", err)
	}

	// Query for card events
	filter := nostr.Filter{
		Kinds:   []nostr.Kind{30302},
		Authors: []nostr.PubKey{pk},
		Tags:    nostr.TagMap{"a": []string{fmt.Sprintf("30301:%s:%s", pk.Hex(), boardID)}},
		Limit:   int(limit),
	}

	var cards []Card
	for ie := range sys.Pool.FetchMany(ctx, relays, filter, nostr.SubscriptionOptions{}) {
		var title, status, priority, columnUUID, cardUUID, description string

		// Extract card info from tags
		for _, tag := range ie.Event.Tags {
			if len(tag) >= 2 {
				switch tag[0] {
				case "title":
					title = tag[1]
				case "s":
					status = tag[1]
				case "priority":
					priority = tag[1]
				case "col":
					columnUUID = tag[1]
				case "d":
					cardUUID = tag[1]
				case "description":
					description = tag[1]
				}
			}
		}

		// Filter by column if specified
		if column != "" && status != column {
			continue
		}

		// Generate naddr for this card
		naddr, _ := generateNaddr(ie.Event.ID.String(), pk.Hex(), "30302", cardUUID)

		cards = append(cards, Card{
			EventID:    ie.Event.ID.String(),
			Title:      title,
			Status:     status,
			Priority:   priority,
			ColumnUUID: columnUUID,
			Description: description,
			Naddr:      naddr,
		})
	}

	return cards, nil
}

func getBoardInfo(ctx context.Context, boardID, boardPubkey string, relays []string) (*BoardInfo, error) {
	if len(relays) == 0 {
		relays = []string{"wss://relay.damus.io", "wss://nos.lol"}
	}

	// Parse board pubkey
	pk, err := nostr.PubKeyFromHex(boardPubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid board pubkey: %w", err)
	}

	// Query for board event
	filter := nostr.Filter{
		Kinds:   []nostr.Kind{30301},
		Authors: []nostr.PubKey{pk},
		Tags:    nostr.TagMap{"d": []string{boardID}},
		Limit:   1,
	}

	var boardEvent *nostr.Event
	for ie := range sys.Pool.FetchMany(ctx, relays, filter, nostr.SubscriptionOptions{}) {
		boardEvent = &ie.Event
		break
	}

	if boardEvent == nil {
		return nil, fmt.Errorf("board not found")
	}

	// Extract board info
	var title, description string
	var columns []Column

	for _, tag := range boardEvent.Tags {
		if len(tag) >= 2 {
			switch tag[0] {
			case "title":
				title = tag[1]
			case "description":
				description = tag[1]
			case "col":
				if len(tag) >= 3 {
					// Column tag format: ["col", UUID, "Name", "Order"]
					columns = append(columns, Column{
						UUID: tag[1],
						Name: tag[2],
						Order: 0, // Would need to parse order from tag[3]
					})
				}
			}
		}
	}

	// Generate kanbanstr URL
	kanbanstrURL := fmt.Sprintf("https://www.kanbanstr.com/#/board/%s/%s", pk.Hex(), boardID)

	// Generate naddr for board
	naddr, err := generateNaddr(boardEvent.ID.String(), pk.Hex(), "30301", boardID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate naddr: %w", err)
	}

	return &BoardInfo{
		BoardID:      boardID,
		EventID:      boardEvent.ID.String(),
		Title:        title,
		Description:  description,
		Columns:      columns,
		KanbanstrURL: kanbanstrURL,
		Naddr:        naddr,
	}, nil
}

func findCardByTitle(ctx context.Context, boardID, boardPubkey, cardTitle string, relays []string) (*nostr.Event, error) {
	// Query for card events to get full event data
	pk, err := nostr.PubKeyFromHex(boardPubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid board pubkey: %w", err)
	}

	filter := nostr.Filter{
		Kinds:   []nostr.Kind{30302},
		Authors: []nostr.PubKey{pk},
		Tags:    nostr.TagMap{"a": []string{fmt.Sprintf("30301:%s:%s", pk.Hex(), boardID)}},
		Limit:   50,
	}

	for ie := range sys.Pool.FetchMany(ctx, relays, filter, nostr.SubscriptionOptions{}) {
		var title string
		for _, tag := range ie.Event.Tags {
			if len(tag) >= 2 && tag[0] == "title" {
				title = tag[1]
				break
			}
		}
		if title == cardTitle {
			return &ie.Event, nil
		}
	}

	return nil, fmt.Errorf("card '%s' not found", cardTitle)
}

// Utility functions
func generateUUID() string {
	// Generate a proper UUID-like identifier using timestamp and nanoseconds
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func generateNaddr(eventID, pubkey, kind, identifier string) (string, error) {
	// For now, return the event ID directly as a simple highlighter URL
	// The proper naddr encoding will need to be implemented later
	return eventID, nil
}

// MCP tool handlers (will be added to mcp.go)
func createBoardMCP(ctx context.Context, keyer nostr.Keyer, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title := required[string](r, "title")
	description, _ := optional[string](r, "description")
	relays, _ := optional[[]string](r, "relay_urls")
	
	// Always generate a UUID for board ID - never allow empty
	boardID := generateUUID()
	
	result, err := createBoard(ctx, keyer, title, description, boardID, relays)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create board: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Board created successfully:\nID: %s\nEvent ID: %s\nURL: %s\nHighlighter: https://highlighter.com/a/%s", 
		result.BoardID, result.EventID, result.KanbanstrURL, result.Naddr)), nil
}

func createCardMCP(ctx context.Context, keyer nostr.Keyer, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title := required[string](r, "title")
	description, _ := optional[string](r, "description")
	column := required[string](r, "column")
	boardID := required[string](r, "board_id")
	boardPubkey := required[string](r, "board_pubkey")
	priority, _ := optional[string](r, "priority")
	if priority == "" {
		priority = "medium"
	}
	relays, _ := optional[[]string](r, "relay_urls")

	result, err := createCard(ctx, keyer, title, description, boardID, boardPubkey, column, priority, relays)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create card: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Card '%s' created successfully with event ID: %s\nCard Highlighter: https://highlighter.com/a/%s", title, result.EventID, result.Naddr)), nil
}

func updateCardMCP(ctx context.Context, keyer nostr.Keyer, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cardTitle := required[string](r, "card_title")
	boardID := required[string](r, "board_id")
	boardPubkey := required[string](r, "board_pubkey")
	newTitle, _ := optional[string](r, "new_title")
	newDescription, _ := optional[string](r, "new_description")
	newColumn, _ := optional[string](r, "new_column")
	newPriority, _ := optional[string](r, "new_priority")
	relays, _ := optional[[]string](r, "relay_urls")

	result, err := updateCard(ctx, keyer, cardTitle, boardID, boardPubkey, newTitle, newDescription, newColumn, newPriority, relays)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update card: %v", err)), nil
	}

	// Build update summary
	var updates []string
	if newTitle != "" {
		updates = append(updates, fmt.Sprintf("title to '%s'", newTitle))
	}
	if newDescription != "" {
		updates = append(updates, "description")
	}
	if newColumn != "" {
		updates = append(updates, fmt.Sprintf("column to '%s'", newColumn))
	}
	if newPriority != "" {
		updates = append(updates, fmt.Sprintf("priority to '%s'", newPriority))
	}

	updateText := strings.Join(updates, ", ")
	return mcp.NewToolResultText(fmt.Sprintf("Card '%s' updated successfully (%s) with event ID: %s\nCard Highlighter: https://highlighter.com/a/%s", cardTitle, updateText, result.EventID, result.Naddr)), nil
}

// Keep moveCardMCP for backward compatibility
func moveCardMCP(ctx context.Context, keyer nostr.Keyer, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cardTitle := required[string](r, "card_title")
	newColumn := required[string](r, "new_column")
	boardID := required[string](r, "board_id")
	boardPubkey := required[string](r, "board_pubkey")
	relays, _ := optional[[]string](r, "relay_urls")

	result, err := moveCard(ctx, keyer, cardTitle, boardID, boardPubkey, newColumn, relays)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to move card: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Card '%s' moved to '%s' successfully with event ID: %s\nCard Highlighter: https://highlighter.com/a/%s", cardTitle, newColumn, result.EventID, result.Naddr)), nil
}

func listCardsMCP(ctx context.Context, keyer nostr.Keyer, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	boardID := required[string](r, "board_id")
	boardPubkey := required[string](r, "board_pubkey")
	column, _ := optional[string](r, "column")
	limit, hasLimit := optional[float64](r, "limit")
	if !hasLimit {
		limit = 50
	}
	relays, _ := optional[[]string](r, "relay_urls")

	cards, err := listCards(ctx, boardID, boardPubkey, column, int64(limit), relays)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list cards: %v", err)), nil
	}

	if len(cards) == 0 {
		return mcp.NewToolResultText("No cards found"), nil
	}

	result := fmt.Sprintf("Found %d cards:\n\n", len(cards))
	for i, card := range cards {
		result += fmt.Sprintf("%d. %s\n", i+1, card.Title)
		result += fmt.Sprintf("   Status: %s\n", card.Status)
		result += fmt.Sprintf("   Priority: %s\n", card.Priority)
		result += fmt.Sprintf("   Event ID: %s\n", card.EventID)
		result += fmt.Sprintf("   Card Highlighter: https://highlighter.com/a/%s\n\n", card.Naddr)
	}

	return mcp.NewToolResultText(result), nil
}

func getBoardInfoMCP(ctx context.Context, keyer nostr.Keyer, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	boardID := required[string](r, "board_id")
	boardPubkey := required[string](r, "board_pubkey")
	relays, _ := optional[[]string](r, "relay_urls")

	boardInfo, err := getBoardInfo(ctx, boardID, boardPubkey, relays)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get board info: %v", err)), nil
	}

	result := fmt.Sprintf("Board: %s\n", boardInfo.Title)
	result += fmt.Sprintf("Description: %s\n", boardInfo.Description)
	result += fmt.Sprintf("Event ID: %s\n", boardInfo.EventID)
	result += fmt.Sprintf("Board URL: %s\n", boardInfo.KanbanstrURL)
	result += fmt.Sprintf("Board Highlighter: https://highlighter.com/a/%s\n", boardInfo.Naddr)
	result += "\nColumns:\n"
	for _, col := range boardInfo.Columns {
		result += fmt.Sprintf("  - %s (ID: %s)\n", col.Name, col.UUID)
	}

	return mcp.NewToolResultText(result), nil
}
