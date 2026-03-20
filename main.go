package main

import (
	"os"

	pitcmd "github.com/dusk-network/pituitary/cmd"
)

func main() {
	os.Exit(pitcmd.Run(os.Args[1:], os.Stdout, os.Stderr))
}
