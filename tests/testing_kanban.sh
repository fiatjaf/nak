#!/bin/bash

# DevOps Workflow Script using nak kanban commands
# Replicates devops_workflow.sh functionality using only nak kanban commands

set -e
# Define nak executable path
NAK="/home/shepherd/Nextcloud/Projects/lab/nak/nak"
DEBUG_MODE=false
if [ "$1" = "--debug" ]; then
    DEBUG_MODE=true
fi

export config="config.yaml"
export RELAY="wss://relay.damus.io"

echo "Step 1: Check for existing keys, generate only if missing"
if [ $(yq eval -r '.nostr.identity.private_key.nsec' $config || echo "null") != "null" ]; then
    [ "$DEBUG_MODE" = true ] && echo "✓ Using existing identity"
else
    echo "generating keys"
    SECRET_KEY_HEX=$(nak key generate)
    PUBKEY_HEX=$(nak key public "$SECRET_KEY_HEX")
    NSEC=$(nak encode nsec "$SECRET_KEY_HEX")
    NPUB=$(nak encode npub "$PUBKEY_HEX")
    echo "$NPUB"
    echo "$NSEC"

    # Save to YAML file
    cat > $config << EOF
nostr:
identity:
    private_key:
    nsec: "$NSEC"
    generated_at: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    public_key:
    npub: "$NPUB"
    generated_at: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
EOF

fi

[ "$DEBUG_MODE" = true ] && echo $NPUB:$NSEC
export NPUB_DECODED=$($NAK decode "$NPUB" --pubkey)
export PUBKEY=$NPUB_DECODED
export QUERY_PUBKEY=$NPUB_DECODED
export CONSISTENT_PUBKEY=$NPUB_DECODED

# Retry function for rate limiting
retry_command() {
    local cmd="$1"
    local max_retries=3
    local retry_count=0
    
    while [ $retry_count -lt $max_retries ]; do
        [ "$DEBUG_MODE" = true ] && echo "Attempt $((retry_count + 1))/$max_retries: $cmd"
        RESULT=$(eval "$cmd" 2>&1)
        local exit_code=$?
        
        if [ $exit_code -eq 0 ]; then
            echo "$RESULT"
            return 0
        fi
        
        # Check for rate limiting
        if echo "$RESULT" | grep -q "rate-limited.*noting too much"; then
            echo "Rate limited detected, waiting 5 seconds before retry..."
            sleep 5
        else
            echo "Command failed with error:"
            echo "$RESULT"
            return $exit_code
        fi
        
        retry_count=$((retry_count + 1))
    done
    
    echo "Max retries exceeded for command: $cmd"
    return 1
}
echo ""
echo "Step 2: Check if board exists, create only if missing"
if [ $(yq eval -r '.nostr.board.naddr' $config || echo "null") != "null" ]; then
    echo "✓ Using existing board"
    export BOARD_ID=$(yq eval -r '.nostr.board.id' $config)
    export BOARD_PUBKEY=$CONSISTENT_PUBKEY
else
    
    BOARD_RESULT=$(retry_command "$NAK kanban create-board \
        --title \"DevOps Workflow Board\" \
        --description \"DevOps workflow management board\" \
        --sec \"$NSEC\" \
        --relay \"$RELAY\"")
    
    if [ $? -ne 0 ]; then
        echo "❌ Failed to create board after retries"
        exit 1
    fi
    
    [ "$DEBUG_MODE" = true ] && echo "Board creation result: $BOARD_RESULT"
    
    BOARD_ID=$(echo "$BOARD_RESULT" | grep "Board created:" | sed 's/.*Board created: //' | awk '{print $1}')
    EVENT_ID=$(echo "$BOARD_RESULT" | grep "Event ID:" | sed 's/.*Event ID: id:://' | awk '{print $1}')
    
    if [ -z "$BOARD_ID" ]; then
        echo "❌ Failed to extract board ID from result"
        echo "Full result: $BOARD_RESULT"
        exit 1
    fi
    
    echo "✓ Board created: $BOARD_ID"
    export BOARD_PUBKEY=$CONSISTENT_PUBKEY
    
    # Update config with board info
    NADDR=$($NAK encode naddr --kind 30301 --pubkey "$CONSISTENT_PUBKEY" --identifier "$BOARD_ID")
    
    if [ -z "$NADDR" ]; then
        echo "⚠️  Warning: Failed to generate NADDR"
        NADDR="naddr1qq9xgetkdac8xwf5x56syg8makapkkjpwqdd8r33tyty6mnxq3vleft9ga4deesw5rgewhvqg5psgqqqwewse3z643"
    fi
    
    KANBANSTR_URL="https://www.kanbanstr.com/#/board/$CONSISTENT_PUBKEY/$BOARD_ID"
    
    [ "$DEBUG_MODE" = true ] && echo "✓ Generated NADDR: $NADDR"
    [ "$DEBUG_MODE" = true ] && echo  "✓ Kanbanstr URL: $KANBANSTR_URL"
    
    # Save to config
    yq e ".nostr.board.id = \"$BOARD_ID\"" -i $config
    yq e ".nostr.board.naddr = \"$NADDR\"" -i $config
    yq e ".nostr.board.event_id = \"$EVENT_ID\"" -i $config
fi
echo ""
echo "Step 3: Verify board (debug only)"
if [ "$DEBUG_MODE" = true ]; then
    echo "Querying for board events..."
    BOARD_QUERY=$($NAK req --kind 30301 -d "$BOARD_ID" $RELAY)
    echo "✓ Board query result:"
    echo "$BOARD_QUERY"
fi
echo ""
echo "Step 4: Check if cards exist, create if missing"
if [ $(yq eval -r '.nostr.board.cards_created' $config || echo "null") != "null" ]; then
    echo "✓ Using existing cards"
else
    # echo "Creating sample cards..."
    
    # echo "Creating 'Task 1: Setup Database' card..."
    CARD1_RESULT=$(retry_command "$NAK kanban create-card \
        --title \"Task 1: Setup Database\" \
        --description \"Initialize PostgreSQL database with required schemas\" \
        --board-id \"$BOARD_ID\" \
        --board-pubkey \"$BOARD_PUBKEY\" \
        --column \"Ideas\" \
        --priority \"high\" \
        --sec \"$NSEC\" \
        --relay \"$RELAY\"")
    
    if [ $? -ne 0 ]; then
        echo "❌ Failed to create Task 1 card after retries"
        exit 1
    fi
    
    echo "✓ Task 1 created"
    [ "$DEBUG_MODE" = true ] && echo "Task 1 creation result: $CARD1_RESULT"
    
    #echo "Creating 'Task 2: Create API Endpoints' card..."
    CARD2_RESULT=$(retry_command "$NAK kanban create-card \
        --title \"Task 2: Create API Endpoints\" \
        --description \"Develop REST API endpoints for user management\" \
        --board-id \"$BOARD_ID\" \
        --board-pubkey \"$BOARD_PUBKEY\" \
        --column \"Ideas\" \
        --priority \"medium\" \
        --sec \"$NSEC\" \
        --relay \"$RELAY\"")
    
    if [ $? -ne 0 ]; then
        echo "❌ Failed to create Task 2 card after retries"
        exit 1
    fi
    
    echo "✓ Task 2 created"
    [ "$DEBUG_MODE" = true ] && echo "Task 2 creation result: $CARD2_RESULT"
    
    echo "✓ Card creation completed"
    
    # Mark cards as created in config
    yq e '.nostr.board.cards_created = true' -i $config
fi

echo ""
echo "Step 5: Verify cards (debug only)"
if [ "$DEBUG_MODE" = true ]; then
    echo "Querying for card events..."
    CARD_QUERY=$($NAK req --kind 30302 --author "$BOARD_PUBKEY" --limit 10 $RELAY)
    echo "✓ Card query result:"
    echo "$CARD_QUERY"
fi
echo ""
echo "Step 6: List cards on board"
BOARD_INFO=$($NAK kanban board-info \
    --board-id "$BOARD_ID" \
    --board-pubkey "$BOARD_PUBKEY" \
    --relay "$RELAY")
if [ "$DEBUG_MODE" = true ]; then 
    echo "$BOARD_INFO" | grep -E "(Board:|Description:|Board URL:)" || echo "$BOARD_INFO"
fi
CARD_LIST=$($NAK kanban list-cards \
    --board-id "$BOARD_ID" \
    --board-pubkey "$BOARD_PUBKEY" \
    --limit 10 \
    --relay "$RELAY")

if echo "$CARD_LIST" | grep -q "Found [0-9] cards:"; then
    [ "$DEBUG_MODE" = true ] && echo "$CARD_LIST"
else
    echo "No cards found or error listing cards"
fi

echo ""
echo "Step 7: Move a card to In Progress"
KANBANSTR_URL="https://www.kanbanstr.com/#/board/$BOARD_PUBKEY/$BOARD_ID"
echo "📋 View board: $KANBANSTR_URL"
read -p "Press Enter to move 'Task 2: Create API Endpoints' to 'In Progress' (or Ctrl+C to cancel)..."

echo "Moving card..."
MOVE_RESULT=$(retry_command "$NAK kanban move-card \
    --card-title \"Task 2: Create API Endpoints\" \
    --board-id \"$BOARD_ID\" \
    --board-pubkey \"$BOARD_PUBKEY\" \
    --new-column \"In Progress\" \
    --sec \"$NSEC\" \
    --relay \"$RELAY\"")
echo here
if [ $? -ne 0 ]; then
    echo "⚠️  Card move failed after retries"
else
    echo "✓ Card moved successfully"
fi
[ "$DEBUG_MODE" = true ] && echo "Move result: $MOVE_RESULT"
echo ""
echo "Step 8: Verify card movement"
UPDATED_CARDS=$($NAK kanban list-cards \
    --board-id "$BOARD_ID" \
    --board-pubkey "$BOARD_PUBKEY" \
    --limit 10 \
    --relay "$RELAY")

if echo "$UPDATED_CARDS" | grep -q "Found [0-9] cards:"; then
    [ "$DEBUG_MODE" = true ] && echo "$UPDATED_CARDS"
else
    echo "No cards found or error listing cards"
fi

# Generate URLs for debugging
if [ "$DEBUG_MODE" = true ]; then
    echo ""
    echo "=== DEBUG URLS ==="
    
    # Board Highlighter URL
    BOARD_NADDR=$($NAK encode naddr --kind 30301 --pubkey "$BOARD_PUBKEY" --identifier "$BOARD_ID")
    echo "Board Highlighter: https://highlighter.com/a/$BOARD_NADDR"
    
    # Card Highlighter URLs
    echo ""
    echo "=== INDIVIDUAL CARD URLS ==="
    
    TASK1_EVENT_ID=$(echo "$CARD1_RESULT" | grep "Event ID:" | sed 's/.*Event ID: id:://' | awk '{print $1}')
    if [ -n "$TASK1_EVENT_ID" ]; then
        echo "Task 1 Card (Event ID: $TASK1_EVENT_ID):"
        NEVENT=$($NAK encode nevent "$TASK1_EVENT_ID" --author "$BOARD_PUBKEY" 2>/dev/null || echo "")
        if [ -n "$NEVENT" ]; then
            echo "https://highlighter.com/a/$NEVENT"
        else
            echo "Failed to generate Highlighter URL for Task 1"
        fi
    fi
    
    echo ""
    
    TASK2_EVENT_ID=$(echo "$CARD2_RESULT" | grep "Event ID:" | sed 's/.*Event ID: id:://' | awk '{print $1}')
    if [ -n "$TASK2_EVENT_ID" ]; then
        echo "Task 2 Card (Event ID: $TASK2_EVENT_ID):"
        NEVENT=$($NAK encode nevent "$TASK2_EVENT_ID" --author "$BOARD_PUBKEY" 2>/dev/null || echo "")
        if [ -n "$NEVENT" ]; then
            echo "https://highlighter.com/a/$NEVENT"
        else
            echo "Failed to generate Highlighter URL for Task 2"
        fi
    fi
    
    echo ""
    
    MOVED_EVENT_ID=$(echo "$MOVE_RESULT" | grep "Event ID:" | sed 's/.*Event ID: id:://' | awk '{print $1}')
    if [ -n "$MOVED_EVENT_ID" ]; then
        echo "Moved Card (Event ID: $MOVED_EVENT_ID):"
        NEVENT=$($NAK encode nevent "$MOVED_EVENT_ID" --author "$BOARD_PUBKEY" 2>/dev/null || echo "")
        if [ -n "$NEVENT" ]; then
            echo "https://highlighter.com/a/$NEVENT"
        else
            echo "Failed to generate Highlighter URL for moved card"
        fi
    fi
fi

echo ""
echo "=== SUMMARY ==="
echo "✓ Kanban workflow completed"
echo "✓ Board ID: $BOARD_ID"
echo "✓ Board URL: $KANBANSTR_URL"
echo "✓ Cards created and moved"
if [ "$DEBUG_MODE" = true ]; then
    echo "✓ Debug mode: verbose output enabled"
else
    echo "✓ Run with --debug for detailed output"
fi
echo ""
echo "All operations completed successfully!"
