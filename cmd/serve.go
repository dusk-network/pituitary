package cmd

import (
	"flag"
	"fmt"
	"io"
	"strings"

	pitmcp "github.com/dusk-network/pituitary/internal/mcp"
)

func runServe(args []string, stdout, stderr io.Writer) int {
	return runServeWithConfig("", args, stdout, stderr)
}

func runServeWithConfig(globalConfigPath string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("serve", "pituitary [--config PATH] serve [--transport stdio]")

	var (
		configPath string
		transport  string
	)
	fs.StringVar(&configPath, "config", "", "path to workspace config")
	fs.StringVar(&transport, "transport", "stdio", "server transport")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		fmt.Fprintf(stderr, "pituitary serve: %s\n", err)
		return 2
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "pituitary serve: unexpected positional arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}
	if transport != "stdio" {
		fmt.Fprintf(stderr, "pituitary serve: unsupported transport %q\n", transport)
		return 2
	}

	resolvedConfigPath, err := resolveCLIConfigPath(strings.TrimSpace(configPath), globalConfigPath)
	if err != nil {
		fmt.Fprintf(stderr, "pituitary serve: %s\n", err)
		return 2
	}

	if err := pitmcp.ServeStdio(pitmcp.Options{ConfigPath: resolvedConfigPath}); err != nil {
		fmt.Fprintf(stderr, "pituitary serve: %s\n", err)
		return 2
	}
	return 0
}
