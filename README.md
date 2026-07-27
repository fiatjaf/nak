# clink-go

Go **CLINK SDK** and **`clinkctl`** CLI for [CLINK](https://github.com/shocknet/CLINK) — enroll on a Pub, create offers, request invoices, and pay via Nostr.

No browser or PWA required.

| | |
|--|--|
| **Module** | [`github.com/shocknet/clink-go`](https://github.com/shocknet/clink-go) |
| **SDK** | `import "github.com/shocknet/clink-go/clink"` |
| **CLI** | `clinkctl` |
| **Spec** | [shocknet/CLINK](https://github.com/shocknet/CLINK) ([Enroll](https://github.com/shocknet/CLINK/blob/master/specs/clink-enroll.md), Offers, Debits, Manage) |
| **Reference server** | [Lightning.Pub](https://github.com/shocknet/Lightning.Pub) |

Based on a fork of [nak](https://github.com/fiatjaf/nak) (extra Nostr tooling remains in the same binary).

---

## Install

```sh
git clone https://github.com/shocknet/clink-go.git
cd clink-go
go build -o clinkctl .
sudo mv clinkctl /usr/local/bin/   # or somewhere on your PATH
```

Or:

```sh
go install github.com/shocknet/clink-go@latest
mv "$(go env GOPATH)/bin/clink-go" "$(go env GOPATH)/bin/clinkctl"
```

---

## Quick start (`clinkctl`)

Pure CLINK path — enroll, then offer / invoice / pay with kinds 21001–21004:

```sh
export NOSTR_SECRET_KEY=your-hex-or-nsec

# bind this key on the Pub; print default noffer / ndebit / nmanage
clinkctl enroll

# create an offer as the account owner (nmanage; owner key needs no third-party grant)
clinkctl offer --label tips --sats 0

# request a BOLT11 from any noffer1…
NOFFER=noffer1...   # from enroll or offer
clinkctl invoice "$NOFFER" --amount 21

# pay an invoice via ndebit (owner key self-debit)
clinkctl pay "$NOFFER" --amount 21

clinkctl decode-noffer "$NOFFER"
```

### Flags

| Flag | Default | Meaning |
|------|---------|---------|
| `--sec` / `NOSTR_SECRET_KEY` | machine default key | signing key (hex or nsec) |
| `--pub` | hosted bootstrap Pub | node service pubkey |
| `--relay` / `-r` | `wss://relay.lightning.pub` | relay the service listens on |

Bootstrap Pub: `76ed45f00cea7bac59d8d0b7d204848f5319d7b96c140ffb6fcbaaab0a13d44e`.

### Commands

| Command | Wire | Role |
|---------|------|------|
| `enroll` | CLINK kind **21004** | ensure account for this key; return `pointer`, `noffer`, `ndebit`, `nmanage` |
| `offer` | kind **21003** Manage | list default / create offer (`--label`) as owner |
| `invoice` | kind **21001** Offers | BOLT11 from a `noffer1…` |
| `pay` | **21001** then **21002** Debit | get invoice, settle via owner `ndebit` |
| `info` | (optional) Pub RPC | balance / debug; not required for the CLINK path |
| `decode-noffer` | local | TLV decode |

**Owner policy:** the key that enrolled is the account principal. That same key MAY `nmanage` its own offers and `ndebit` its own balance without a third-party authorization. Marketplace keys still need explicit Manage/Debit grants.

---

## Go SDK

```go
package main

import (
	"context"

	"fiatjaf.com/nostr"
	"github.com/shocknet/clink-go/clink"
)

func main() {
	pool := nostr.NewPool(context.Background())
	client := clink.NewClient(pool, secretKey)

	// Enroll on a service (kind 21004)
	acct, err := client.Enroll(ctx, destPub, []string{relay})
	// acct.Noffer, acct.Ndebit, acct.Nmanage, acct.Pointer

	offer, err := clink.DecodeNoffer(acct.Noffer)
	bolt11, err := client.Noffer(ctx, offer, 21, "coffee")

	// pay via ndebit (owner self-debit)
	_, err = client.NdebitPay(ctx, acct.Ndebit, bolt11)

	// create offers via nmanage (owner)
	noffer, err := client.NmanageCreateOffer(ctx, acct.Nmanage, clink.OfferCreate{
		Label:     "tips",
		PriceSats: 0,
	})
}
```

### Package surface

- `Client.Enroll` — kind 21004 account bootstrap
- `DecodeNoffer` / `EncodeNoffer` / `OfferPointer`
- `Client.Noffer` — kind 21001
- `Client.Ndebit` / `NdebitPay` — kind 21002
- `Client.Nmanage` / `NmanageCreateOffer` — kind 21003
- `Client.Pub` — optional Lightning.Pub kind-21000 helpers (balance, legacy RPC)

---

## Protocol map

| Kind | Spec | In this repo |
|------|------|----------------|
| 21004 | [CLINK Enroll](https://github.com/shocknet/CLINK/blob/master/specs/clink-enroll.md) | `enroll` / `Client.Enroll` |
| 21001 | [CLINK Offers](https://github.com/shocknet/CLINK/blob/master/specs/clink-offers.md) | `invoice` / `Client.Noffer` |
| 21002 | [CLINK Debits](https://github.com/shocknet/CLINK/blob/master/specs/clink-debits.md) | `pay` / `Client.NdebitPay` |
| 21003 | [CLINK Manage](https://github.com/shocknet/CLINK/blob/master/specs/clink-manage.md) | `offer` / `Client.Nmanage*` |

---

## License

Same as upstream nak (Unlicense) unless noted otherwise in source files.
