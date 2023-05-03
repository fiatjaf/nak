package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "nak",
		Usage: "the nostr army knife command-line tool",
		Commands: []*cli.Command{
			req,
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("failed to run cli: %s", err)
	}
}
