package main

import (
	"fmt"
	"os"
	"sort"
)

var commands = map[string]string{
	"index":           "rebuild the local Pituitary index",
	"search-specs":    "search spec sections semantically",
	"check-overlap":   "find overlapping specs",
	"compare-specs":   "compare design tradeoffs across specs",
	"analyze-impact":  "report affected specs and docs",
	"check-doc-drift": "find docs that drift from specs",
	"review-spec":     "run the common spec-review workflow",
	"help":            "show available commands",
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	cmd := os.Args[1]
	if cmd == "help" || cmd == "--help" || cmd == "-h" {
		printHelp()
		return
	}

	description, ok := commands[cmd]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		printHelp()
		os.Exit(1)
	}

	fmt.Printf("pituitary %s: %s\n", cmd, description)
	fmt.Println("status: bootstrap only, not implemented yet")
}

func printHelp() {
	fmt.Println("pituitary bootstrap CLI")
	fmt.Println()
	fmt.Println("available commands:")
	names := make([]string, 0, len(commands)-1)
	for name := range commands {
		if name != "help" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		description := commands[name]
		fmt.Printf("  %-16s %s\n", name, description)
	}
	fmt.Println()
	fmt.Println("run `pituitary help` for this message")
}
