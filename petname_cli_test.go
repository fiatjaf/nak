package main

import (
	"context"
	"io"
	"testing"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/sdk"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestPetnameCLI(t *testing.T) {
	me := keyFromSeed(t, 1)
	erin := keyFromSeed(t, 2)
	david := keyFromSeed(t, 3)
	abs := "~" + nip19.EncodeNpub(me.Public()) + "/erin"
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"secret before flag", []string{"--sec", me.Hex(), "check", "--pubkey", "~/erin"}},
		{"secret after flag", []string{"check", "--pubkey", "~/erin", "--sec", me.Hex()}},
		{"relative chain", []string{"check", "--pubkey", "~erin/david", "--sec", me.Hex()}},
		{"nostr prefix", []string{"check", "--pubkey", "nostr:~/erin", "--sec", me.Hex()}},
		{"environment key", []string{"check", "--pubkey", "~/erin"}},
		{"absolute without signer", []string{"check", "--pubkey", abs, "--sec", "invalid"}},
		{"npub without signer", []string{"check", "--pubkey", nip19.EncodeNpub(erin.Public()), "--sec", "invalid"}},
		{"positional", []string{"check", "~/erin", "--sec", me.Hex()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NOSTR_SECRET_KEY", me.Hex())
			setupPetnameSystem(t, map[nostr.SecretKey][]sdk.ProfileRef{
				me:   {{Pubkey: erin.Public(), Petname: "erin"}},
				erin: {{Pubkey: david.Public(), Petname: "david"}},
			})
			called := false
			cmd := &cli.Command{
				Name: "nak", Writer: io.Discard, ErrWriter: io.Discard,
				Flags: []cli.Flag{&cli.StringFlag{Name: "sec", Sources: cli.EnvVars("NOSTR_SECRET_KEY")}},
				Commands: []*cli.Command{{
					Name:  "check",
					Flags: []cli.Flag{&PubKeyFlag{Name: "pubkey"}},
					Action: func(ctx context.Context, c *cli.Command) error {
						called = true
						var got nostr.PubKey
						if c.Args().Present() {
							var err error
							got, err = parsePubKeyForCommand(ctx, c, c.Args().First())
							require.NoError(t, err)
						} else {
							got = getPubKey(c, "pubkey")
						}
						want := erin.Public()
						if tc.name == "relative chain" {
							want = david.Public()
						}
						require.Equal(t, want, got)
						return nil
					},
				}},
			}
			deferPetnameFlags(cmd)
			require.NoError(t, cmd.Run(t.Context(), append([]string{"nak"}, tc.args...)))
			require.True(t, called)
		})
	}
}

func TestPetnameCLIRepeatedFlags(t *testing.T) {
	me := keyFromSeed(t, 1)
	erin := keyFromSeed(t, 2)
	david := keyFromSeed(t, 3)
	setupPetnameSystem(t, map[nostr.SecretKey][]sdk.ProfileRef{
		me: {{Pubkey: erin.Public(), Petname: "erin"}},
	})
	addr := "30023:" + david.Public().Hex() + ":post"
	cmd := &cli.Command{
		Name: "nak", Writer: io.Discard, ErrWriter: io.Discard,
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "sec"},
			&PubKeySliceFlag{Name: "keys"},
			&PubKeyOrAddressFlag{Name: "author", Aliases: []string{"a"}},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			require.Equal(t, []nostr.PubKey{erin.Public(), david.Public(), erin.Public()}, getPubKeySlice(c, "keys"))
			authors := getPubKeyOrAddressSlice(c, "author")
			require.Len(t, authors, 3)
			require.Equal(t, erin.Public(), authors[0].PubKey)
			require.Equal(t, david.Public(), authors[1].PubKey)
			require.NotNil(t, authors[2].Addr)
			require.Equal(t, addr, authors[2].Addr.AsTagReference())
			return nil
		},
	}
	deferPetnameFlags(cmd)
	require.NoError(t, cmd.Run(t.Context(), []string{"nak",
		"--keys", "~/erin", "--keys", david.Public().Hex(), "--keys", "~/erin",
		"-a", "~/erin", "--author", david.Public().Hex(), "-a", addr, "--sec", me.Hex(),
	}))
}

func TestPetnameCLIInvalidIdentity(t *testing.T) {
	setupPetnameSystem(t, nil)
	cmd := &cli.Command{
		Name: "nak", Writer: io.Discard, ErrWriter: io.Discard,
		Flags: []cli.Flag{&cli.StringFlag{Name: "sec"}, &PubKeyFlag{Name: "pubkey"}},
		Action: func(context.Context, *cli.Command) error {
			t.Fatal("action must not run after a failed petname lookup")
			return nil
		},
	}
	deferPetnameFlags(cmd)
	err := cmd.Run(t.Context(), []string{"nak", "--pubkey", "~/erin", "--sec", "invalid"})
	require.ErrorContains(t, err, "failed to get petname identity")
}
