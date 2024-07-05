module github.com/fiatjaf/nak

go 1.21

toolchain go1.21.0

require (
	github.com/btcsuite/btcd/btcec/v2 v2.3.3
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.3.0
	github.com/fatih/color v1.16.0
	github.com/mailru/easyjson v0.7.7
	github.com/nbd-wtf/go-nostr v0.34.0
	github.com/nbd-wtf/nostr-sdk v0.0.5
	github.com/urfave/cli/v3 v3.0.0-alpha9
	golang.org/x/exp v0.0.0-20240506185415-9bf2ced13842
)

require (
	github.com/btcsuite/btcd/btcutil v1.1.3 // indirect
	github.com/btcsuite/btcd/chaincfg/chainhash v1.1.0 // indirect
	github.com/chzyer/logex v1.1.10 // indirect
	github.com/chzyer/test v0.0.0-20180213035817-a1ea475d72b1 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.0.1 // indirect
	github.com/fiatjaf/eventstore v0.2.16 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/puzpuzpuz/xsync/v3 v3.1.0 // indirect
	github.com/tidwall/gjson v1.17.1 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	golang.org/x/crypto v0.7.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
	golang.org/x/text v0.8.0 // indirect
)

replace github.com/urfave/cli/v3 => github.com/fiatjaf/cli/v3 v3.0.0-20240626022047-0fc2565ea728
