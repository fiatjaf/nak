package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

func onSocketConnect(ctx context.Context, c *cli.Command) chan *url.URL {
	res := make(chan *url.URL)

	socketPath := getSocketPath(c)
	if _, err := os.Stat(socketPath); err == nil {
		// file exists, we must delete it (or not)
		os.Remove(socketPath)
	} else if !os.IsNotExist(err) {
		log(color.RedString("failed to check on unix socket: %w\n", err))
		return res
	}

	// start unix socket listener
	os.MkdirAll(filepath.Dir(socketPath), 0755)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log(color.RedString("failed to listen on unix socket: %w\n", err))
		return res
	}

	// handle unix socket connections in background
	go func() {
		defer listener.Close()

		// clean up socket file on exit
		// (irrelevant, as we clean it on startup, but just to keep the user filesystem sane)
		defer os.Remove(socketPath)

		for {
			conn, err := listener.Accept()
			if err != nil {
				continue
			}

			defer conn.Close()
			buf := make([]byte, 4096)

			for {
				n, err := conn.Read(buf)
				if err != nil {
					break
				}

				uri, err := url.Parse(string(buf[:n]))
				if err == nil && uri.Scheme == "nostrconnect" {
					res <- uri
				}
			}
		}
	}()

	return res
}

func sendToSocket(c *cli.Command, value string) error {
	socketPath := getSocketPath(c)

	// connect to unix socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to bunker unix socket at %s: %w", socketPath, err)
	}
	defer conn.Close()

	// send the uri
	_, err = conn.Write([]byte(value))
	if err != nil {
		return fmt.Errorf("failed to send uri to bunker: %w", err)
	}
	return nil
}

func getSocketPath(c *cli.Command) string {
	profile := "any"
	if c.Bool("persist") || c.IsSet("profile") {
		profile = c.String("profile")
	}
	return filepath.Join(c.String("config-path"), "bunkerconn", profile+".sock")
}
