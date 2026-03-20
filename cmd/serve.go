package cmd

import (
	"flag"
	"fmt"
	"io"
	"strings"

	pitmcp "github.com/dusk-network/pituitary/internal/mcp"
)

func runServe(args []string, stdout, stderr io.Writer) int {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Fprintln(stdout, "pituitary serve: run the optional MCP server transport")
			fmt.Fprintln(stdout, "usage: pituitary serve [--config pituitary.toml] [--transport stdio]")
			return 0
		}
	}

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		configPath string
		transport  string
	)
	fs.StringVar(&configPath, "config", "pituitary.toml", "path to workspace config")
	fs.StringVar(&transport, "transport", "stdio", "server transport")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "pituitary serve: %s\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "pituitary serve: unexpected positional arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}
	if transport != "stdio" {
		fmt.Fprintf(stderr, "pituitary serve: unsupported transport %q\n", transport)
		return 2
	}

	if err := pitmcp.ServeStdio(pitmcp.Options{ConfigPath: strings.TrimSpace(configPath)}); err != nil {
		fmt.Fprintf(stderr, "pituitary serve: %s\n", err)
		return 2
	}
	return 0
}
