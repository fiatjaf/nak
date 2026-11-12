package nip05nmc

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Protocol version negotiated with the server. Matches the Kotlin reference.
const electrumProtocolVersion = "1.4"

// NameExpireDepth is the number of blocks after which a Namecoin name
// expires if not re-registered (≈ 250 days at 10 min/block). Sourced from
// chainparams.cpp → consensus.nNameExpirationDepth.
const NameExpireDepth = 36000

// NameShowResult is the structured outcome of a name_show lookup.
type NameShowResult struct {
	Name      string
	Value     string
	TxID      string
	Height    int
	ExpiresIn int // blocks until expiry; 0 if unknown
}

// Errors we surface to callers. NameNotFound and NameExpired are
// "definitive" (blockchain said so) — no point retrying other servers.
// ServersUnreachable is returned when every candidate server failed
// with a transport-level error.
var (
	ErrNameNotFound       = errors.New("nip05nmc: name not found on Namecoin blockchain")
	ErrNameExpired        = errors.New("nip05nmc: namecoin name has expired")
	ErrServersUnreachable = errors.New("nip05nmc: all ElectrumX servers unreachable")
)

// ElectrumClient is a minimal, query-only Namecoin ElectrumX client.
// It opens a short-lived TCP/TLS socket per request, which is plenty
// for interactive CLI use.
type ElectrumClient struct {
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	requestID      atomic.Int64
}

// NewElectrumClient returns a client with sensible defaults.
func NewElectrumClient() *ElectrumClient {
	return &ElectrumClient{
		ConnectTimeout: 10 * time.Second,
		ReadTimeout:    15 * time.Second,
	}
}

// NameShow queries a single server and returns the current value for
// `identifier` (e.g. "d/example"). Returns nil + ErrNameNotFound when
// the name is provably absent, and a generic error for transport
// failures.
func (c *ElectrumClient) NameShow(ctx context.Context, identifier string, server ElectrumxServer) (*NameShowResult, error) {
	conn, err := c.dial(ctx, server)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(c.ReadTimeout))

	reader := bufio.NewReader(conn)

	// 1. Negotiate protocol version. The response is consumed and
	//    discarded — we only care that the socket is alive.
	if err := c.sendRPC(conn, "server.version", []any{"nak-nip05nmc/0.1", electrumProtocolVersion}); err != nil {
		return nil, err
	}
	if _, err := reader.ReadString('\n'); err != nil {
		return nil, fmt.Errorf("nip05nmc: read version response: %w", err)
	}

	// 2. Compute the name-index scripthash.
	script := buildNameIndexScript([]byte(identifier))
	scriptHash := electrumScriptHash(script)

	// 3. Fetch transaction history for that scripthash.
	if err := c.sendRPC(conn, "blockchain.scripthash.get_history", []any{scriptHash}); err != nil {
		return nil, err
	}
	histLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("nip05nmc: read history response: %w", err)
	}
	entries, err := parseHistoryResponse(histLine)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, ErrNameNotFound
	}

	// Most recent transaction = last entry. The Kotlin reference does
	// the same.
	latest := entries[len(entries)-1]

	// 4. Fetch the verbose transaction.
	if err := c.sendRPC(conn, "blockchain.transaction.get", []any{latest.TxHash, true}); err != nil {
		return nil, err
	}
	txLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("nip05nmc: read transaction response: %w", err)
	}

	// 5. Get the current block height so we can compute expiry.
	if err := c.sendRPC(conn, "blockchain.headers.subscribe", []any{}); err != nil {
		return nil, err
	}
	headerLine, _ := reader.ReadString('\n')
	currentHeight := parseBlockHeight(headerLine)

	if currentHeight > 0 && latest.Height > 0 {
		if currentHeight-latest.Height >= NameExpireDepth {
			return nil, ErrNameExpired
		}
	}

	result, err := parseNameFromTransaction(identifier, latest.TxHash, latest.Height, txLine)
	if err != nil {
		return nil, err
	}
	if result != nil && currentHeight > 0 && latest.Height > 0 {
		result.ExpiresIn = NameExpireDepth - (currentHeight - latest.Height)
	}
	return result, nil
}

// NameShowWithFallback tries each server in order until one returns a
// result. Definitive errors (NameNotFound, NameExpired) are propagated
// immediately; transport errors are swallowed and the next server is
// tried.
func (c *ElectrumClient) NameShowWithFallback(ctx context.Context, identifier string, servers []ElectrumxServer) (*NameShowResult, error) {
	var lastErr error
	for _, srv := range servers {
		result, err := c.NameShow(ctx, identifier, srv)
		if err == nil {
			return result, nil
		}
		if errors.Is(err, ErrNameNotFound) || errors.Is(err, ErrNameExpired) {
			return nil, err
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrServersUnreachable
	}
	return nil, fmt.Errorf("%w: last error: %v", ErrServersUnreachable, lastErr)
}

// dial opens a TCP connection to the server, upgrading to TLS when
// UseSSL is set. Honours both context cancellation and our connect
// timeout, whichever fires first.
func (c *ElectrumClient) dial(ctx context.Context, server ElectrumxServer) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: c.ConnectTimeout}
	address := net.JoinHostPort(server.Host, strconv.Itoa(server.Port))

	raw, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("nip05nmc: dial %s: %w", address, err)
	}

	if !server.UseSSL {
		return raw, nil
	}

	cfg := tlsConfigFor(server)
	tlsConn := tls.Client(raw, cfg)

	// Run the handshake with a deadline so a silent server doesn't hang
	// the call beyond what the caller asked for.
	if deadline, ok := ctx.Deadline(); ok {
		_ = tlsConn.SetDeadline(deadline)
	} else {
		_ = tlsConn.SetDeadline(time.Now().Add(c.ConnectTimeout))
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, fmt.Errorf("nip05nmc: TLS handshake with %s: %w", address, err)
	}
	// Clear the handshake deadline — the caller sets its own read
	// deadline afterwards.
	_ = tlsConn.SetDeadline(time.Time{})
	return tlsConn, nil
}

// sendRPC writes a JSON-RPC 2.0 request (newline-terminated, per
// Electrum's line-delimited protocol) to the connection.
func (c *ElectrumClient) sendRPC(w net.Conn, method string, params []any) error {
	id := c.requestID.Add(1)
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("nip05nmc: marshal rpc request: %w", err)
	}
	encoded = append(encoded, '\n')
	if _, err := w.Write(encoded); err != nil {
		return fmt.Errorf("nip05nmc: write rpc request: %w", err)
	}
	return nil
}

// historyEntry is one row of `blockchain.scripthash.get_history`.
type historyEntry struct {
	TxHash string
	Height int
}

// parseHistoryResponse extracts (tx_hash, height) pairs from a
// get_history response. An error response (non-null `error` field)
// yields an empty slice so callers can treat it as "no data".
func parseHistoryResponse(raw string) ([]historyEntry, error) {
	var envelope struct {
		Result []struct {
			TxHash string `json:"tx_hash"`
			Height int    `json:"height"`
		} `json:"result"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil, fmt.Errorf("nip05nmc: parse history response: %w", err)
	}
	if len(envelope.Error) > 0 && !isJSONNull(envelope.Error) {
		return nil, nil
	}
	out := make([]historyEntry, 0, len(envelope.Result))
	for _, e := range envelope.Result {
		out = append(out, historyEntry{TxHash: e.TxHash, Height: e.Height})
	}
	return out, nil
}

// parseBlockHeight extracts `result.height` from a
// `blockchain.headers.subscribe` response, or returns 0 on any error.
func parseBlockHeight(raw string) int {
	if raw == "" {
		return 0
	}
	var envelope struct {
		Result struct {
			Height int `json:"height"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return 0
	}
	return envelope.Result.Height
}

// parseNameFromTransaction walks the verbose transaction's vouts
// looking for a NAME_UPDATE output that matches `identifier`.
// Returns (nil, nil) if no matching output exists.
func parseNameFromTransaction(identifier, txHash string, height int, raw string) (*NameShowResult, error) {
	var envelope struct {
		Result struct {
			Vout []struct {
				ScriptPubKey struct {
					Hex string `json:"hex"`
				} `json:"scriptPubKey"`
			} `json:"vout"`
		} `json:"result"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil, fmt.Errorf("nip05nmc: parse transaction response: %w", err)
	}
	if len(envelope.Error) > 0 && !isJSONNull(envelope.Error) {
		return nil, nil
	}
	for _, vout := range envelope.Result.Vout {
		hexScript := vout.ScriptPubKey.Hex
		// NAME_UPDATE scripts start with OP_3 (0x53). Skip anything
		// else without the cost of a hex decode.
		if !strings.HasPrefix(hexScript, "53") {
			continue
		}
		scriptBytes, err := hex.DecodeString(hexScript)
		if err != nil {
			continue
		}
		name, value, err := parseNameScript(scriptBytes)
		if err != nil {
			continue
		}
		if name == identifier {
			return &NameShowResult{
				Name:   name,
				Value:  value,
				TxID:   txHash,
				Height: height,
			}, nil
		}
	}
	return nil, nil
}

func isJSONNull(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s == "" || s == "null"
}
