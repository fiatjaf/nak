package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"fiatjaf.com/nostr"
	"github.com/urfave/cli/v3"
	"golang.org/x/exp/slices"
)

var curlFlags []string

var curl = &cli.Command{
	Name:                      "curl",
	Usage:                     "calls curl but with a nip98 header",
	Description:               "accepts all flags and arguments exactly as they would be passed to curl.",
	Flags:                     defaultKeyFlags,
	DisableSliceFlagSeparator: true,
	Action: func(ctx context.Context, c *cli.Command) error {
		kr, _, err := gatherKeyerFromArguments(ctx, c)
		if err != nil {
			return err
		}

		// cowboy parsing of curl flags to get the data we need for nip98
		var url string
		var method string
		var presumedMethod string

		curlBodyBuildingFlags := []string{
			"-d",
			"--data",
			"--data-binary",
			"--data-ascii",
			"--data-raw",
			"--data-urlencode",
			"-F",
			"--form",
			"--form-string",
			"--form-escape",
			"--upload-file",
		}

		nextIsMethod := false
		for _, f := range curlFlags {
			if nextIsMethod {
				method = f
				method, _ = strings.CutPrefix(method, `"`)
				method, _ = strings.CutSuffix(method, `"`)
				method = strings.ToUpper(method)
			} else if strings.HasPrefix(f, "https://") || strings.HasPrefix(f, "http://") {
				url = f
			} else if f == "--request" || f == "-X" {
				nextIsMethod = true
				continue
			} else if slices.Contains(curlBodyBuildingFlags, f) ||
				slices.ContainsFunc(curlBodyBuildingFlags, func(s string) bool {
					return strings.HasPrefix(f, s)
				}) {
				presumedMethod = "POST"
			}
			nextIsMethod = false
		}

		if url == "" {
			return fmt.Errorf("can't create nip98 event: target url is empty")
		}

		if method == "" {
			if presumedMethod != "" {
				method = presumedMethod
			} else {
				method = "GET"
			}
		}

		// make and sign event
		evt := nostr.Event{
			Kind:      27235,
			CreatedAt: nostr.Now(),
			Tags: nostr.Tags{
				{"u", url},
				{"method", method},
			},
		}
		if err := kr.SignEvent(ctx, &evt); err != nil {
			return err
		}

		// the first 2 indexes of curlFlags were reserved for this
		curlFlags[0] = "-H"
		curlFlags[1] = fmt.Sprintf("Authorization: Nostr %s", base64.StdEncoding.EncodeToString([]byte(evt.String())))

		// call curl
		cmd := exec.Command("curl", curlFlags...)
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Run()
		return nil
	},
}

func realCurl() error {
	curlFlags = make([]string, 2, max(len(os.Args)-4, 2))
	keyFlags := make([]string, 0, 5)

	for i := 0; i < len(os.Args[2:]); i++ {
		arg := os.Args[i+2]
		if slices.ContainsFunc(defaultKeyFlags, func(f cli.Flag) bool {
			bareArg, _ := strings.CutPrefix(arg, "-")
			bareArg, _ = strings.CutPrefix(bareArg, "-")
			return slices.Contains(f.Names(), bareArg)
		}) {
			keyFlags = append(keyFlags, arg)
			if arg != "--prompt-sec" {
				i++
				val := os.Args[i+2]
				keyFlags = append(keyFlags, val)
			}
		} else {
			curlFlags = append(curlFlags, arg)
		}
	}

	return curl.Run(context.Background(), keyFlags)
}
