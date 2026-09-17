package main

import (
	"strings"
	"testing"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/sdk"
	cache_memory "fiatjaf.com/nostr/sdk/cache/memory"
	"github.com/stretchr/testify/require"
)

// makeFollowListEvent builds a signed kind-3 event for user with a set of ["p", pubkey, relay, petname] tags.
func makeFollowListEvent(t *testing.T, sec nostr.SecretKey, ts nostr.Timestamp, refs ...sdk.ProfileRef) nostr.Event {
	t.Helper()

	tags := make(nostr.Tags, 0, len(refs))
	for _, ref := range refs {
		tags = append(tags, nostr.Tag{"p", ref.Pubkey.Hex(), ref.Relay, ref.Petname})
	}

	evt := nostr.Event{
		PubKey:    sec.Public(),
		CreatedAt: ts,
		Kind:      nostr.KindFollowList,
		Tags:      tags,
	}
	require.NoError(t, evt.Sign(sec))
	return evt
}

// setupPetnameSystem initializes a hermetic global sys by pre-seeding the follow list cache with
// kind-3 events for the given identities, so no network or store access ever happens.
func setupPetnameSystem(t *testing.T, lists map[nostr.SecretKey][]sdk.ProfileRef) {
	t.Helper()

	sys = sdk.NewSystem()
	sys.FollowListCache = newSettledFollowListCache(t, lists)

	t.Cleanup(func() {
		sys = nil
		rootSec = ""
	})
}

// newSettledFollowListCache builds a follow list cache with the given entries already visible.
// ristretto Set/SetWithTTL are asynchronous, so entries are read back until they stick.
func newSettledFollowListCache(t *testing.T, lists map[nostr.SecretKey][]sdk.ProfileRef) *cache_memory.RistrettoCache[sdk.GenericList[nostr.PubKey, sdk.ProfileRef]] {
	t.Helper()
	cache := cache_memory.New[sdk.GenericList[nostr.PubKey, sdk.ProfileRef]](1000)
	for sk, refs := range lists {
		pubkey := sk.Public()
		v := sdk.GenericList[nostr.PubKey, sdk.ProfileRef]{PubKey: pubkey}
		if refs != nil {
			// nil refs means "no kind-3 event found at all"; empty refs means a tagless event
			evt := makeFollowListEvent(t, sk, 1000, refs...)
			v.Event = &evt
			v.Items = refs
		}

		deadline := time.Now().Add(5 * time.Second)
		for {
			if cache.SetWithTTL(pubkey, v, time.Hour) {
				if _, ok := cache.Get(pubkey); ok {
					break
				}
			}
			require.Less(t, time.Now(), deadline, "cache entry did not settle")
			time.Sleep(time.Millisecond)
		}
	}
	return cache
}

func keyFromSeed(t *testing.T, seed byte) nostr.SecretKey {
	t.Helper()
	sk := nostr.SecretKey{}
	for i := range sk {
		sk[i] = seed + byte(i)
	}
	return sk
}

func TestResolvePetnamePathChain(t *testing.T) {
	me := keyFromSeed(t, 1)
	erin := keyFromSeed(t, 2)
	david := keyFromSeed(t, 3)
	frank := keyFromSeed(t, 4)

	setupPetnameSystem(t, map[nostr.SecretKey][]sdk.ProfileRef{
		me:    {{Pubkey: erin.Public(), Petname: "erin"}},
		erin:  {{Pubkey: david.Public(), Petname: "david"}},
		david: {{Pubkey: frank.Public(), Petname: "frank"}},
	})
	rootSec = me.Hex()

	pk, err := resolvePetnamePath("~erin/david/frank")
	require.NoError(t, err)
	require.Equal(t, frank.Public(), pk)
}

func TestResolvePetnamePathSingleHop(t *testing.T) {
	me := keyFromSeed(t, 1)
	erin := keyFromSeed(t, 2)

	setupPetnameSystem(t, map[nostr.SecretKey][]sdk.ProfileRef{
		me: {{Pubkey: erin.Public(), Petname: "erin"}},
	})
	rootSec = me.Hex()

	pk, err := resolvePetnamePath("~erin")
	require.NoError(t, err)
	require.Equal(t, erin.Public(), pk)
}

func TestResolvePetnamePathCurrentUser(t *testing.T) {
	me := keyFromSeed(t, 1)
	erin := keyFromSeed(t, 2)

	setupPetnameSystem(t, map[nostr.SecretKey][]sdk.ProfileRef{
		me: {{Pubkey: erin.Public(), Petname: "erin"}},
	})
	rootSec = me.Hex()

	pk, err := resolvePetnamePath("~/erin")
	require.NoError(t, err)
	require.Equal(t, erin.Public(), pk)
}

func TestResolvePetnamePathDirectRootWithoutIdentity(t *testing.T) {
	me := keyFromSeed(t, 1)
	erin := keyFromSeed(t, 2)

	setupPetnameSystem(t, map[nostr.SecretKey][]sdk.ProfileRef{
		me: {{Pubkey: erin.Public(), Petname: "erin"}},
	})

	pk, err := resolvePetnamePath("~" + nip19.EncodeNpub(me.Public()) + "/erin")
	require.NoError(t, err)
	require.Equal(t, erin.Public(), pk)
}

func TestResolvePetnamePathNip05Root(t *testing.T) {
	erin := keyFromSeed(t, 2)
	david := keyFromSeed(t, 3)

	setupPetnameSystem(t, map[nostr.SecretKey][]sdk.ProfileRef{
		erin: {{Pubkey: david.Public(), Petname: "david"}},
	})

	// the first segment must be routed to nip05 (which will fail to connect here, as nothing
	// listens on 127.0.0.1:443) instead of being treated as a petname
	_, err := resolvePetnamePath("~erin@127.0.0.1/david")
	require.Error(t, err)
	require.Contains(t, err.Error(), "request failed")
	require.NotContains(t, err.Error(), "follow list")
}

func TestResolvePetnamePathErrors(t *testing.T) {
	me := keyFromSeed(t, 1)
	erin := keyFromSeed(t, 2)
	david := keyFromSeed(t, 3)

	setupPetnameSystem(t, map[nostr.SecretKey][]sdk.ProfileRef{
		me:   {{Pubkey: erin.Public(), Petname: "erin"}},
		erin: {{Pubkey: david.Public(), Petname: "david"}},
	})
	rootSec = me.Hex()

	// unknown petname at root
	_, err := resolvePetnamePath("~carol")
	require.ErrorContains(t, err, "no one in your follow list")

	// unknown petname mid-chain
	_, err = resolvePetnamePath("~erin/carol")
	require.ErrorContains(t, err, "doesn't follow anyone named")

	// empty reference
	_, err = resolvePetnamePath("~")
	require.Error(t, err)

	// empty segment
	_, err = resolvePetnamePath("~erin//frank")
	require.Error(t, err)

	// invalid petname characters
	_, err = resolvePetnamePath("~er\tin")
	require.Error(t, err)

	// no identity
	rootSec = ""
	_, err = resolvePetnamePath("~erin")
	require.ErrorContains(t, err, "no secret key")
}

func TestResolvePetnameMissingFollowList(t *testing.T) {
	me := keyFromSeed(t, 1)
	erin := keyFromSeed(t, 2)

	// erin has a cached follow list record with no event, i.e. her kind-3 was never found
	sys = sdk.NewSystem()
	sys.FollowListCache = newSettledFollowListCache(t, map[nostr.SecretKey][]sdk.ProfileRef{
		me:   {{Pubkey: erin.Public(), Petname: "erin"}},
		erin: nil,
	})
	t.Cleanup(func() { sys = nil; rootSec = "" })
	rootSec = me.Hex()

	_, err := resolvePetnamePath("~erin/frank")
	require.ErrorContains(t, err, "couldn't get follow list")
}

func TestParsePubKeyUnaffectedInputs(t *testing.T) {
	me := keyFromSeed(t, 1)
	erin := keyFromSeed(t, 2)

	setupPetnameSystem(t, map[nostr.SecretKey][]sdk.ProfileRef{
		me: {{Pubkey: erin.Public(), Petname: "erin"}},
	})
	rootSec = me.Hex()

	// plain npub keeps working through parsePubKey
	pk, err := parsePubKey(nip19.EncodeNpub(erin.Public()), nostr.ZeroPK)
	require.NoError(t, err)
	require.Equal(t, erin.Public(), pk)

	// hex keeps working
	pk, err = parsePubKey(erin.Public().Hex(), nostr.ZeroPK)
	require.NoError(t, err)
	require.Equal(t, erin.Public(), pk)

	// without a system initialized, petname resolution fails cleanly
	sys = nil
	_, err = parsePubKey("~erin", nostr.ZeroPK)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "petname") || strings.Contains(err.Error(), "system"))
}
