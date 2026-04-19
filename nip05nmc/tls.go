package nip05nmc

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// buildPinnedCertPool parses the pinned PEM certs into an *x509.CertPool.
// Malformed entries are skipped so that a bad paste doesn't break the
// whole bundle.
func buildPinnedCertPool() *x509.CertPool {
	pool := x509.NewCertPool()
	for _, pemBlock := range PinnedElectrumXCerts {
		block, _ := pem.Decode([]byte(pemBlock))
		if block == nil {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		pool.AddCert(cert)
	}
	return pool
}

// pinnedFingerprints returns the SHA-256 fingerprints of the pinned
// certificates. We use this as an additional match path in the custom
// VerifyPeerCertificate callback so that cert-chain mismatches from the
// standard library (e.g. expired leaf re-used by operator) still succeed
// when the operator publishes an updated cert with a known fingerprint.
func pinnedFingerprints() [][32]byte {
	out := make([][32]byte, 0, len(PinnedElectrumXCerts))
	for _, pemBlock := range PinnedElectrumXCerts {
		block, _ := pem.Decode([]byte(pemBlock))
		if block == nil {
			continue
		}
		out = append(out, sha256.Sum256(block.Bytes))
	}
	return out
}

// tlsConfigFor returns a *tls.Config suitable for an ElectrumX connection
// to the given server. When `usePinned` is false, the returned config is
// the stdlib default (system roots only). When true, the config trusts
// either the system roots *or* one of the pinned self-signed certs —
// verified by both full chain against the pinned pool and by leaf SHA-256
// fingerprint, which matches what the Kotlin reference does on Android.
func tlsConfigFor(server ElectrumxServer) *tls.Config {
	if !server.UsePinnedTrustStore {
		return &tls.Config{
			ServerName: server.Host,
			MinVersion: tls.VersionTLS12,
		}
	}

	pinnedPool := buildPinnedCertPool()
	fingerprints := pinnedFingerprints()

	cfg := &tls.Config{
		ServerName: server.Host,
		MinVersion: tls.VersionTLS12,
		// InsecureSkipVerify disables the default chain verification. We
		// run our own in VerifyPeerCertificate below — trying the system
		// roots first, then the pinned bundle, then raw SHA-256 match.
		// This is NOT a trust-all: the handshake still fails unless at
		// least one of those paths succeeds.
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("nip05nmc: no peer certificates presented")
			}
			certs := make([]*x509.Certificate, 0, len(rawCerts))
			for _, raw := range rawCerts {
				c, err := x509.ParseCertificate(raw)
				if err != nil {
					return fmt.Errorf("nip05nmc: parse peer cert: %w", err)
				}
				certs = append(certs, c)
			}
			leaf := certs[0]
			intermediates := x509.NewCertPool()
			for _, c := range certs[1:] {
				intermediates.AddCert(c)
			}

			// 1. Try the system trust store with proper hostname binding.
			if _, err := leaf.Verify(x509.VerifyOptions{
				DNSName:       server.Host,
				Intermediates: intermediates,
			}); err == nil {
				return nil
			}

			// 2. Try the pinned pool. The pinned certs are self-signed, so
			//    we let them act as their own root. Hostname match is
			//    intentionally not required here — some operators share a
			//    cert across multiple hostnames (testls clear + onion).
			if _, err := leaf.Verify(x509.VerifyOptions{
				Roots:         pinnedPool,
				Intermediates: intermediates,
			}); err == nil {
				return nil
			}

			// 3. Last-chance: raw SHA-256 fingerprint match.
			leafFP := sha256.Sum256(leaf.Raw)
			for _, fp := range fingerprints {
				if fp == leafFP {
					return nil
				}
			}

			return fmt.Errorf("nip05nmc: peer cert for %s is not in system or pinned trust store", server.Host)
		},
	}

	return cfg
}
