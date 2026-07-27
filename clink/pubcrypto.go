package clink

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"fiatjaf.com/nostr"
	"github.com/btcsuite/btcd/btcec/v2"
	"golang.org/x/crypto/chacha20"
)

// Lightning.Pub kind-21000 uses a custom "nip44 v1" scheme (not NIP-44 v2).

func pubSharedSecret(sk nostr.SecretKey, pub nostr.PubKey) ([32]byte, error) {
	var out [32]byte
	priv, _ := btcec.PrivKeyFromBytes(sk[:])
	peer, err := btcec.ParsePubKey(append([]byte{0x02}, pub[:]...))
	if err != nil {
		return out, fmt.Errorf("parse pubkey: %w", err)
	}
	x := btcec.GenerateSharedSecret(priv, peer)
	return sha256.Sum256(x), nil
}

func encryptPubV1(plaintext string, shared [32]byte) (string, error) {
	nonce := make([]byte, chacha20.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	cipher, err := chacha20.NewUnauthenticatedCipher(shared[:], nonce)
	if err != nil {
		return "", err
	}
	pt := []byte(plaintext)
	ct := make([]byte, len(pt))
	cipher.XORKeyStream(ct, pt)

	payload := make([]byte, 1+len(nonce)+len(ct))
	payload[0] = 1
	copy(payload[1:], nonce)
	copy(payload[1+len(nonce):], ct)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func decryptPubV1(content string, shared [32]byte) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return "", err
	}
	if len(raw) < 1+chacha20.NonceSizeX {
		return "", fmt.Errorf("ciphertext too short")
	}
	if raw[0] != 1 {
		return "", fmt.Errorf("unsupported encryption version %d", raw[0])
	}
	nonce := raw[1 : 1+chacha20.NonceSizeX]
	ct := raw[1+chacha20.NonceSizeX:]
	cipher, err := chacha20.NewUnauthenticatedCipher(shared[:], nonce)
	if err != nil {
		return "", err
	}
	pt := make([]byte, len(ct))
	cipher.XORKeyStream(pt, ct)
	return string(pt), nil
}
