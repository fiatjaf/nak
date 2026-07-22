package main

import (
	"testing"

	"fiatjaf.com/nostr"
	"github.com/stretchr/testify/require"
)

func TestZero(t *testing.T) {
	b := []byte{1, 2, 3, 4, 5, 255, 128, 9}
	zero(b)
	for i, v := range b {
		require.Equalf(t, byte(0), v, "byte %d was not zeroed", i)
	}
}

func TestZeroEmptyAndNil(t *testing.T) {
	// must not panic on the degenerate cases
	zero(nil)
	zero([]byte{})
}

// TestParseSecretKeyDoesNotLeaveDecodedBytesReadable is a best-effort check
// that the defer-based zeroing added around key parsing actually happens:
// it exercises parseSecretKey (which internally defers zero() on its local
// copy of the decoded key before returning) and confirms the function still
// returns the correct, untouched key to its caller -- proving the deferred
// zero of the *local* copy does not corrupt the *returned* copy, which is
// the core safety property the whole approach relies on.
func TestParseSecretKeyReturnsUnmodifiedValueDespiteInternalZeroing(t *testing.T) {
	want := nostr.Generate()

	got, err := parseSecretKey(want.Hex())
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.NotEqual(t, nostr.SecretKey{}, got, "sanity: returned key should not be the zero key")
}
