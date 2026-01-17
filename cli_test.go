package main

import (
	"encoding/hex"
	stdjson "encoding/json"
	"fmt"
	"strings"
	"testing"

	"fiatjaf.com/nostr"
	"github.com/stretchr/testify/require"
)

// these tests are tricky because commands and flags are declared as globals and values set in one call may persist
// to the next. for example, if in the first test we set --limit 2 then doesn't specify --limit in the second then
// it will still return true for cmd.IsSet("limit") and then we will set .LimitZero = true

func call(t *testing.T, cmd string) string {
	var output strings.Builder
	stdout = func(a ...any) {
		output.WriteString(fmt.Sprint(a...))
		output.WriteString("\n")
	}
	err := app.Run(t.Context(), strings.Split(cmd, " "))
	require.NoError(t, err)

	return strings.TrimSpace(output.String())
}

func TestEventBasic(t *testing.T) {
	output := call(t, "nak event --ts 1699485669")

	var evt nostr.Event
	err := stdjson.Unmarshal([]byte(output), &evt)
	require.NoError(t, err)

	require.Equal(t, nostr.Kind(1), evt.Kind)
	require.Equal(t, nostr.Timestamp(1699485669), evt.CreatedAt)
	require.Equal(t, "hello from the nostr army knife", evt.Content)
	require.Equal(t, "36d88cf5fcc449f2390a424907023eda7a74278120eebab8d02797cd92e7e29c", evt.ID.Hex())
	require.Equal(t, "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798", evt.PubKey.Hex())
	require.Equal(t, "68e71a192e8abcf8582a222434ac823ecc50607450ebe8cc4c145eb047794cc382dc3f888ce879d2f404f5ba6085a47601360a0fa2dd4b50d317bd0c6197c2c2", hex.EncodeToString(evt.Sig[:]))
}

func TestEventComplex(t *testing.T) {
	output := call(t, "nak event --ts 1699485669 -k 11 -c skjdbaskd --sec 17 -t t=spam -e 36d88cf5fcc449f2390a424907023eda7a74278120eebab8d02797cd92e7e29c -t r=https://abc.def?name=foobar;nothing")

	var evt nostr.Event
	err := stdjson.Unmarshal([]byte(output), &evt)
	require.NoError(t, err)

	require.Equal(t, nostr.Kind(11), evt.Kind)
	require.Equal(t, nostr.Timestamp(1699485669), evt.CreatedAt)
	require.Equal(t, "skjdbaskd", evt.Content)
	require.Equal(t, "19aba166dcf354bf5ef64f4afe69ada1eb851495001ee05e07d393ee8c8ea179", evt.ID.Hex())
	require.Equal(t, "2fa2104d6b38d11b0230010559879124e42ab8dfeff5ff29dc9cdadd4ecacc3f", evt.PubKey.Hex())
	require.Equal(t, "cf452def4a68341c897c3fc96fa34dc6895a5b8cc266d4c041bcdf758ec992ec5adb8b0179e98552aaaf9450526a26d7e62e413b15b1c57e0cfc8db6b29215d7", hex.EncodeToString(evt.Sig[:]))

	require.Len(t, evt.Tags, 3)
	require.Equal(t, nostr.Tag{"t", "spam"}, evt.Tags[0])
	require.Equal(t, nostr.Tag{"r", "https://abc.def?name=foobar", "nothing"}, evt.Tags[1])
	require.Equal(t, nostr.Tag{"e", "36d88cf5fcc449f2390a424907023eda7a74278120eebab8d02797cd92e7e29c"}, evt.Tags[2])
}

func TestEncode(t *testing.T) {
	require.Equal(t,
		"npub156n8a7wuhwk9tgrzjh8gwzc8q2dlekedec5djk0js9d3d7qhnq3qjpdq28",
		call(t, "nak encode npub a6a67ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f8179822"),
	)
	require.Equal(t,
		`nprofile1qqs2dfn7l8wthtz45p3ftn58pvrs9xlumvkuu2xet8egzkcklqtesgspz9mhxue69uhk27rpd4cxcefwvdhk6fl5jug
nprofile1qqs22kfpwwt4mmvlsd4f2uh23vg60ctvadnyvntx659jw93l0upe6tqpz9mhxue69uhk27rpd4cxcefwvdhk64h265a`,
		call(t, "nak encode nprofile -r wss://example.com a6a67ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f8179822 a5592173975ded9f836a9572ea8b11a7e16ceb66464d66d50b27163f7f039d2c"),
	)
}

func TestDecodeNaddr(t *testing.T) {
	output := call(t, "nak decode naddr1qqyrgcmyxe3kvefhqyxhwumn8ghj7mn0wvhxcmmvqgs9kqvr4dkruv3t7n2pc6e6a7v9v2s5fprmwjv4gde8c4fe5y29v0srqsqqql9ngrt6tu")

	var result map[string]interface{}
	err := stdjson.Unmarshal([]byte(output), &result)
	require.NoError(t, err)

	require.Equal(t, "5b0183ab6c3e322bf4d41c6b3aef98562a144847b7499543727c5539a114563e", result["pubkey"])
	require.Equal(t, float64(31923), result["kind"])
	require.Equal(t, "4cd6cfe7", result["identifier"])
	require.Equal(t, []interface{}{"wss://nos.lol"}, result["relays"])
}

func TestDecodePubkey(t *testing.T) {
	output := call(t, "nak decode -p npub10xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqpkge6d npub1ccz8l9zpa47k6vz9gphftsrumpw80rjt3nhnefat4symjhrsnmjs38mnyd")

	expected := "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798\nc6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5"
	require.Equal(t, expected, output)
}

func TestDecodeMultipleNpubs(t *testing.T) {
	output := call(t, "nak decode npub1l2vyh47mk2p0qlsku7hg0vn29faehy9hy34ygaclpn66ukqp3afqutajft npub10000003zmk89narqpczy4ff6rnuht2wu05na7kpnh3mak7z2tqzsv8vwqk")
	require.Len(t, strings.Split(output, "\n"), 2)
}

func TestDecodeEventId(t *testing.T) {
	output := call(t, "nak decode -e nevent1qyd8wumn8ghj7urewfsk66ty9enxjct5dfskvtnrdakj7qgmwaehxw309aex2mrp0yh8wetnw3jhymnzw33jucm0d5hszxthwden5te0wfjkccte9eekummjwsh8xmmrd9skctcpzamhxue69uhkzarvv9ejumn0wd68ytnvv9hxgtcqyqllp5v5j0nxr74fptqxkhvfv0h3uj870qpk3ln8a58agyxl3fka296ewr8 nevent1qqswh48lurxs8u0pll9qj2rzctvjncwhstpzlstq59rdtzlty79awns5hl5uf")

	expected := "3ff0d19493e661faa90ac06b5d8963ef1e48fe780368fe67ed0fd410df8a6dd5\nebd4ffe0cd03f1e1ffca092862c2d929e1d782c22fc160a146d58beb278bd74e"
	require.Equal(t, expected, output)
}

func TestReq(t *testing.T) {
	output := call(t, "nak req -k 1 -l 18 -a 2fa2104d6b38d11b0230010559879124e42ab8dfeff5ff29dc9cdadd4ecacc3f -e aec4de6d051a7c2b6ca2d087903d42051a31e07fb742f1240970084822de10a6")

	var result []interface{}
	err := stdjson.Unmarshal([]byte(output), &result)
	require.NoError(t, err)

	require.Equal(t, "REQ", result[0])
	require.Equal(t, "nak", result[1])

	filter := result[2].(map[string]interface{})
	require.Equal(t, []interface{}{float64(1)}, filter["kinds"])
	require.Equal(t, []interface{}{"2fa2104d6b38d11b0230010559879124e42ab8dfeff5ff29dc9cdadd4ecacc3f"}, filter["authors"])
	require.Equal(t, float64(18), filter["limit"])
	require.Equal(t, []interface{}{"aec4de6d051a7c2b6ca2d087903d42051a31e07fb742f1240970084822de10a6"}, filter["#e"])
}

func TestMultipleFetch(t *testing.T) {
	output := call(t, "nak fetch naddr1qqyrgcmyxe3kvefhqyxhwumn8ghj7mn0wvhxcmmvqgs9kqvr4dkruv3t7n2pc6e6a7v9v2s5fprmwjv4gde8c4fe5y29v0srqsqqql9ngrt6tu nevent1qyd8wumn8ghj7urewfsk66ty9enxjct5dfskvtnrdakj7qgmwaehxw309aex2mrp0yh8wetnw3jhymnzw33jucm0d5hszxthwden5te0wfjkccte9eekummjwsh8xmmrd9skctcpzamhxue69uhkzarvv9ejumn0wd68ytnvv9hxgtcqyqllp5v5j0nxr74fptqxkhvfv0h3uj870qpk3ln8a58agyxl3fka296ewr8")

	var events []nostr.Event
	for _, line := range strings.Split(output, "\n") {
		var evt nostr.Event
		err := stdjson.Unmarshal([]byte(line), &evt)
		require.NoError(t, err)
		events = append(events, evt)
	}

	require.Len(t, events, 2)

	// first event validation
	require.Equal(t, nostr.Kind(31923), events[0].Kind)
	require.Equal(t, "9ae5014573fc75ced00b343868d2cd9343ebcbbae50591c6fa8ae1cd99568f05", events[0].ID.Hex())
	require.Equal(t, "5b0183ab6c3e322bf4d41c6b3aef98562a144847b7499543727c5539a114563e", events[0].PubKey.Hex())
	require.Equal(t, nostr.Timestamp(1707764605), events[0].CreatedAt)

	// second event validation
	require.Equal(t, nostr.Kind(1), events[1].Kind)
	require.Equal(t, "3ff0d19493e661faa90ac06b5d8963ef1e48fe780368fe67ed0fd410df8a6dd5", events[1].ID.Hex())
	require.Equal(t, "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d", events[1].PubKey.Hex())
	require.Equal(t, nostr.Timestamp(1710759386), events[1].CreatedAt)
}

func TestKeyPublic(t *testing.T) {
	output := call(t, "nak key public 3ff0d19493e661faa90ac06b5d8963ef1e48fe780368fe67ed0fd410df8a6dd5 3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d")

	expected := "70f7120d065870513a6bddb61c8d400ad1e43449b1900ffdb5551e4c421375c8\n718d756f60cf5179ef35b39dc6db3ff58f04c0734f81f6d4410f0b047ddf9029"
	require.Equal(t, expected, output)
}

func TestKeyDecrypt(t *testing.T) {
	output := call(t, "nak key decrypt ncryptsec1qgg2gx2a7hxpsse2zulrv7m8qwccvl3mh8e9k8vtz3wpyrwuuclaq73gz7ddt5kpa93qyfhfjakguuf8uhw90jn6mszh7kqeh9mxzlyw8hy75fluzx4h75frwmu2yngsq7hx7w32d0vdyxyns5g6rqft banana")
	require.Equal(t, "718d756f60cf5179ef35b39dc6db3ff58f04c0734f81f6d4410f0b047ddf9029", output)
}

func TestReqIdFromRelay(t *testing.T) {
	output := call(t, "nak req -i 20a6606ed548fe7107533cf3416ce1aa5e957c315c2a40249e12bd9873dca7da --limit 1 nos.lol")

	var evt nostr.Event
	err := stdjson.Unmarshal([]byte(output), &evt)
	require.NoError(t, err)

	require.Equal(t, nostr.Kind(1), evt.Kind)
	require.Equal(t, "20a6606ed548fe7107533cf3416ce1aa5e957c315c2a40249e12bd9873dca7da", evt.ID.Hex())
	require.Equal(t, "dd664d5e4016433a8cd69f005ae1480804351789b59de5af06276de65633d319", evt.PubKey.Hex())
	require.Equal(t, nostr.Timestamp(1720972243), evt.CreatedAt)
	require.Equal(t, "Yeah, so bizarre, but I guess most people are meant to be serfs.", evt.Content)
}

func TestReqWithFlagsAfter1(t *testing.T) {
	output := call(t, "nak req nos.lol -i 20a6606ed548fe7107533cf3416ce1aa5e957c315c2a40249e12bd9873dca7da --limit 1")

	var evt nostr.Event
	err := stdjson.Unmarshal([]byte(output), &evt)
	require.NoError(t, err)

	require.Equal(t, nostr.Kind(1), evt.Kind)
	require.Equal(t, "20a6606ed548fe7107533cf3416ce1aa5e957c315c2a40249e12bd9873dca7da", evt.ID.Hex())
	require.Equal(t, "dd664d5e4016433a8cd69f005ae1480804351789b59de5af06276de65633d319", evt.PubKey.Hex())
	require.Equal(t, nostr.Timestamp(1720972243), evt.CreatedAt)
	require.Equal(t, "Yeah, so bizarre, but I guess most people are meant to be serfs.", evt.Content)
}

func TestReqWithFlagsAfter2(t *testing.T) {
	output := call(t, "nak req -e 893d4c10f1c230240812c6bdf9ad877eed1e29e87029d153820c24680bb183b1 nostr.mom --author 2a7dcf382bcc96a393ada5c975f500393b3f7be6e466bff220aa161ad6b15eb6 --limit 1 -k 7")

	var evt nostr.Event
	err := stdjson.Unmarshal([]byte(output), &evt)
	require.NoError(t, err)

	require.Equal(t, nostr.Kind(7), evt.Kind)
	require.Equal(t, "9b4868b068ea34ae51092807586c4541b3569d9efc23862aea48ef13de275857", evt.ID.Hex())
	require.Equal(t, "2a7dcf382bcc96a393ada5c975f500393b3f7be6e466bff220aa161ad6b15eb6", evt.PubKey.Hex())
	require.Equal(t, nostr.Timestamp(1720987327), evt.CreatedAt)
	require.Equal(t, "❤️", evt.Content)
}

func TestReqWithFlagsAfter3(t *testing.T) {
	output := call(t, "nak req --limit 1 pyramid.fiatjaf.com -a 3f770d65d3a764a9c5cb503ae123e62ec7598ad035d836e2a810f3877a745b24 -qp 3f770d65d3a764a9c5cb503ae123e62ec7598ad035d836e2a810f3877a745b24 -e 9f3c1121c96edf17d84b9194f74d66d012b28c4e25b3ef190582c76b8546a188")

	var evt nostr.Event
	err := stdjson.Unmarshal([]byte(output), &evt)
	require.NoError(t, err)

	require.Equal(t, nostr.Kind(1), evt.Kind)
	require.Equal(t, "101572c80ebdc963dab8440f6307387a3023b6d90f7e495d6c5ee1ef77045a67", evt.ID.Hex())
	require.Equal(t, "3f770d65d3a764a9c5cb503ae123e62ec7598ad035d836e2a810f3877a745b24", evt.PubKey.Hex())
	require.Equal(t, nostr.Timestamp(1720987305), evt.CreatedAt)
	require.Equal(t, "Nope. I grew up playing in the woods. Never once saw a bear in the woods. If I did, I'd probably shiy my pants, then scream at it like I was a crazy person with my arms above my head to make me seem huge.", evt.Content)
}

func TestNaturalTimestamps(t *testing.T) {
	output := call(t, "nak event -t plu=pla -e 3f770d65d3a764a9c5cb503ae123e62ec7598ad035d836e2a810f3877a745b24 --ts '2018-May-19T03:37:19' -c nn")

	var evt nostr.Event
	err := stdjson.Unmarshal([]byte(output), &evt)
	require.NoError(t, err)

	require.Equal(t, nostr.Kind(1), evt.Kind)
	require.Equal(t, "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798", evt.PubKey.Hex())
	require.Equal(t, nostr.Timestamp(1526711839), evt.CreatedAt)
	require.Equal(t, "nn", evt.Content)
}
