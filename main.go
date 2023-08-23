package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "nak",
		Usage: "the nostr army knife command-line tool",
		Commands: []*cli.Command{
			req,
			count,
			event,
			decode,
			encode,
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
