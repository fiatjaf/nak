//go:build integration

package nip05nmc

import (
	"context"
	"encoding/hex"
	"testing"
	"time"
)

// Live tests that hit a real ElectrumX server. Gated behind the
// "integration" build tag because they require network access and the
// Namecoin ecosystem's public ElectrumX servers to be available.
//
// Run with: go test -tags=integration ./nip05nmc -run Integration -v
//
// Known-good fixtures captured against electrumx.testls.space:50002:
//   testls.bit     → 460c25e682fda7832b52d1f22d3d22b3176d972f60dcdc3212ed8c92ef85065c
//   m@testls.bit   → 6cdebccabda1dfa058ab85352a79509b592b2bdfa0370325e28ec1cb4f18667d

const (
	testlsRootPubkey = "460c25e682fda7832b52d1f22d3d22b3176d972f60dcdc3212ed8c92ef85065c"
	testlsMPubkey    = "6cdebccabda1dfa058ab85352a79509b592b2bdfa0370325e28ec1cb4f18667d"
)

func TestIntegration_ResolveTestlsBit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pp, err := QueryIdentifier(ctx, "testls.bit")
	if err != nil {
		t.Fatalf("QueryIdentifier(testls.bit): %v", err)
	}
	if got := hex.EncodeToString(pp.PublicKey[:]); got != testlsRootPubkey {
		t.Errorf("testls.bit pubkey = %s, want %s", got, testlsRootPubkey)
	}
}

func TestIntegration_ResolveAtTestlsBit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pp, err := QueryIdentifier(ctx, "m@testls.bit")
	if err != nil {
		t.Fatalf("QueryIdentifier(m@testls.bit): %v", err)
	}
	if got := hex.EncodeToString(pp.PublicKey[:]); got != testlsMPubkey {
		t.Errorf("m@testls.bit pubkey = %s, want %s", got, testlsMPubkey)
	}
}
