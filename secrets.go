package main

// zero overwrites b with zero bytes. It is used to wipe sensitive data (raw
// private key bytes, decrypted secret keys) from memory as soon as we are
// done using it, so it doesn't linger around for longer than necessary.
//
// Callers should invoke this via `defer zero(sk[:])` right after obtaining a
// nostr.SecretKey they own (as a local variable, parameter, or a value about
// to be returned -- deferred calls run after the return value has already
// been copied out, so this never changes what gets returned) or, inside a
// loop, with a direct call as soon as that iteration's copy is no longer
// needed.
//
// This is a best-effort hygiene measure, not a hard security guarantee: Go
// copies values freely (by assignment, by being boxed into an interface, by
// being handed to other libraries that keep their own copy) and this
// function can only reach the specific copy it's given. See the "known
// limitations" note in the PR/commit that introduced this file for a
// rundown of the copies that are out of nak's reach.
//
//go:noinline
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
