package main

import (
	"bytes"
	"io"
	"os"

	"github.com/urfave/cli/v2"
)

func getStdin() string {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		read := bytes.NewBuffer(make([]byte, 0, 1000))
		_, err := io.Copy(read, os.Stdin)
		if err == nil {
			return read.String()
		}
	}
	return ""
}

func getStdinOrFirstArgument(c *cli.Context) string {
	target := c.Args().First()
	if target != "" {
		return target
	}
	return getStdin()
}
