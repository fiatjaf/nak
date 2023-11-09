package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

var app = &cli.App{
	Name:  "nak",
	Usage: "the nostr army knife command-line tool",
	Commands: []*cli.Command{
		req,
		count,
		fetch,
		event,
		decode,
		encode,
		verify,
	},
}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
