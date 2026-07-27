// Package clink is the Go CLINK SDK (offers / Pub RPC over Nostr).
// Import from other Go programs; the clinkctl CLI in this module uses the same package.
package clink

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"

	"fiatjaf.com/nostr"
	"github.com/btcsuite/btcd/btcutil/bech32"
)

const (
	PriceFixed       = 0
	PriceVariable    = 1
	PriceSpontaneous = 2
)

// OfferPointer is a decoded CLINK static offer (noffer1…).
type OfferPointer struct {
	Pubkey    nostr.PubKey
	Relay     string
	Offer     string
	PriceType int
	Price     uint32
	HasPrice  bool
}

func DecodeNoffer(s string) (OfferPointer, error) {
	if !looksLikeNoffer(s) {
		return OfferPointer{}, fmt.Errorf("need a real noffer1… string, not a placeholder")
	}
	hrp, bits5, err := bech32.DecodeNoLimit(s)
	if err != nil {
		return OfferPointer{}, fmt.Errorf("invalid noffer bech32 %q: %w", truncate(s, 48), err)
	}
	if hrp != "noffer" {
		return OfferPointer{}, fmt.Errorf("expected noffer prefix, got %s", hrp)
	}
	data, err := bech32.ConvertBits(bits5, 5, 8, false)
	if err != nil {
		return OfferPointer{}, err
	}
	tlv, err := parseTLV(data)
	if err != nil {
		return OfferPointer{}, err
	}
	return offerFromTLV(tlv)
}

func EncodeNoffer(o OfferPointer) (string, error) {
	tlv := map[byte][][]byte{
		0: {o.Pubkey[:]},
		1: {[]byte(o.Relay)},
		2: {[]byte(o.Offer)},
		3: {{byte(o.PriceType)}},
	}
	if o.HasPrice {
		var price [4]byte
		binary.BigEndian.PutUint32(price[:], o.Price)
		tlv[4] = [][]byte{price[:]}
	}
	bits5, err := bech32.ConvertBits(encodeTLV(tlv), 8, 5, true)
	if err != nil {
		return "", err
	}
	return bech32.Encode("noffer", bits5)
}

func PriceTypeName(t int) string {
	switch t {
	case PriceFixed:
		return "Fixed"
	case PriceVariable:
		return "Variable"
	case PriceSpontaneous:
		return "Spontaneous"
	default:
		return fmt.Sprintf("Unknown(%d)", t)
	}
}

func offerFromTLV(tlv map[byte][][]byte) (OfferPointer, error) {
	pkBytes, err := requireTLV(tlv, 0, 32)
	if err != nil {
		return OfferPointer{}, err
	}
	relay, err := requireTLV(tlv, 1, -1)
	if err != nil {
		return OfferPointer{}, err
	}
	offer, err := requireTLV(tlv, 2, -1)
	if err != nil {
		return OfferPointer{}, err
	}
	priceType, err := requireTLV(tlv, 3, 1)
	if err != nil {
		return OfferPointer{}, err
	}
	pk, err := nostr.PubKeyFromHex(hex.EncodeToString(pkBytes))
	if err != nil {
		return OfferPointer{}, err
	}
	out := OfferPointer{
		Pubkey:    pk,
		Relay:     string(relay),
		Offer:     string(offer),
		PriceType: int(priceType[0]),
	}
	if price, ok := tlv[4]; ok && len(price) > 0 && len(price[0]) == 4 {
		out.HasPrice = true
		out.Price = binary.BigEndian.Uint32(price[0])
	}
	return out, nil
}

func requireTLV(tlv map[byte][][]byte, t byte, wantLen int) ([]byte, error) {
	vs, ok := tlv[t]
	if !ok || len(vs) == 0 {
		return nil, fmt.Errorf("missing TLV %d", t)
	}
	if wantLen >= 0 && len(vs[0]) != wantLen {
		return nil, fmt.Errorf("TLV %d should be %d bytes", t, wantLen)
	}
	return vs[0], nil
}

func parseTLV(data []byte) (map[byte][][]byte, error) {
	out := map[byte][][]byte{}
	rest := data
	for len(rest) > 0 {
		if len(rest) < 2 {
			return nil, fmt.Errorf("truncated TLV")
		}
		t, l := rest[0], int(rest[1])
		if len(rest) < 2+l {
			return nil, fmt.Errorf("not enough data for TLV %d", t)
		}
		v := append([]byte{}, rest[2:2+l]...)
		out[t] = append(out[t], v)
		rest = rest[2+l:]
	}
	return out, nil
}

func encodeTLV(tlv map[byte][][]byte) []byte {
	keys := make([]byte, 0, len(tlv))
	for t := range tlv {
		keys = append(keys, t)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] > keys[j] })
	var buf []byte
	for _, k := range keys {
		for _, v := range tlv[k] {
			buf = append(buf, k, byte(len(v)))
			buf = append(buf, v...)
		}
	}
	return buf
}

func looksLikeNoffer(s string) bool {
	if len(s) < 20 || s[:7] != "noffer1" {
		return false
	}
	const charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
	for _, r := range s[7:] {
		if r > 127 || !containsRune(charset, r) {
			return false
		}
	}
	return true
}

func containsRune(set string, r rune) bool {
	for _, c := range set {
		if c == r {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
