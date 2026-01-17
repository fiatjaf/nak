package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/urfave/cli/v3"
)

type nipInfo struct {
	nip, desc, link string
}

var nip = &cli.Command{
	Name:  "nip",
	Usage: "list NIPs or get the description of a NIP from its number",
	Description: `lists NIPs, fetches and displays NIP text, or opens a NIP page in the browser.

examples:
  nak nip          # list all NIPs
  nak nip 29       # shows nip29 details
  nak nip open 29  # opens nip29 in browser`,
	ArgsUsage: "[NIP number]",
	Commands: []*cli.Command{
		{
			Name:  "open",
			Usage: "open the NIP page in the browser",
			Action: func(ctx context.Context, c *cli.Command) error {
				reqNum := c.Args().First()
				if reqNum == "" {
					return fmt.Errorf("missing NIP number")
				}

				normalize := func(s string) string {
					s = strings.ToLower(s)
					s = strings.TrimPrefix(s, "nip-")
					s = strings.TrimLeft(s, "0")
					if s == "" {
						s = "0"
					}
					return s
				}

				reqNum = normalize(reqNum)

				foundLink := ""
				for info := range listnips() {
					nipNum := normalize(info.nip)
					if nipNum == reqNum {
						foundLink = info.link
						break
					}
				}

				if foundLink == "" {
					return fmt.Errorf("NIP-%s not found", strings.ToUpper(reqNum))
				}

				url := "https://github.com/nostr-protocol/nips/blob/master/" + foundLink
				fmt.Println("Opening " + url)

				var cmd *exec.Cmd
				switch runtime.GOOS {
				case "darwin":
					cmd = exec.Command("open", url)
				case "windows":
					cmd = exec.Command("cmd", "/c", "start", url)
				default:
					cmd = exec.Command("xdg-open", url)
				}

				return cmd.Start()
			},
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		reqNum := c.Args().First()
		if reqNum == "" {
			// list all NIPs
			for info := range listnips() {
				stdout(info.nip + ": " + info.desc)
			}
			return nil
		}

		normalize := func(s string) string {
			s = strings.ToLower(s)
			s = strings.TrimPrefix(s, "nip-")
			s = strings.TrimLeft(s, "0")
			if s == "" {
				s = "0"
			}
			return s
		}

		reqNum = normalize(reqNum)

		var foundLink string
		for info := range listnips() {
			nipNum := normalize(info.nip)

			if nipNum == reqNum {
				foundLink = info.link
				break
			}
		}

		if foundLink == "" {
			return fmt.Errorf("NIP-%s not found", strings.ToUpper(reqNum))
		}

		// fetch the NIP markdown
		url := "https://raw.githubusercontent.com/nostr-protocol/nips/master/" + foundLink
		resp, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("failed to fetch NIP: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read NIP: %w", err)
		}

		// render markdown
		rendered, err := glamour.Render(string(body), "auto")
		if err != nil {
			return fmt.Errorf("failed to render markdown: %w", err)
		}

		fmt.Print(rendered)
		return nil
	},
}

func listnips() <-chan nipInfo {
	ch := make(chan nipInfo)
	go func() {
		defer close(ch)
		resp, err := http.Get("https://raw.githubusercontent.com/nostr-protocol/nips/master/README.md")
		if err != nil {
			// TODO: handle error? but since chan, maybe send error somehow, but for now, just close
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return
		}
		bodyStr := string(body)
		epoch := strings.Index(bodyStr, "## List")
		if epoch == -1 {
			return
		}

		lines := strings.SplitSeq(bodyStr[epoch+8:], "\n")
		for line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "##") {
				break
			}
			if !strings.HasPrefix(line, "- [NIP-") {
				continue
			}

			start := strings.Index(line, "[")
			end := strings.Index(line, "]")
			if start == -1 || end == -1 || end < start {
				continue
			}

			content := line[start+1 : end]

			parts := strings.SplitN(content, ":", 2)
			if len(parts) != 2 {
				continue
			}

			nipPart := parts[0]
			descPart := parts[1]

			rest := line[end+1:]
			linkStart := strings.Index(rest, "(")
			linkEnd := strings.Index(rest, ")")
			link := ""
			if linkStart != -1 && linkEnd != -1 && linkEnd > linkStart {
				link = rest[linkStart+1 : linkEnd]
			}

			ch <- nipInfo{nipPart, strings.TrimSpace(descPart), link}
		}
	}()
	return ch
}
