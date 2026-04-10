package cmd

import (
	"flag"
	"fmt"
	"io"
	pathpkg "path"
	"strings"
)

type commandHelp struct {
	Name       string
	Usage      string
	UsesConfig bool
}

func newCommandHelp(name, usage string) commandHelp {
	return commandHelp{Name: name, Usage: usage, UsesConfig: true}
}

func newStandaloneCommandHelp(name, usage string) commandHelp {
	return commandHelp{Name: name, Usage: usage}
}

func parseCommandFlags(fs *flag.FlagSet, args []string, stdout io.Writer, help commandHelp) (bool, error) {
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			printCommandHelp(stdout, fs, help)
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	seen := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			seen = true
		}
	})
	return seen
}

func printCommandHelp(w io.Writer, fs *flag.FlagSet, help commandHelp) {
	description := commandDescription(help.Name)
	if description == "" {
		fmt.Fprintf(w, "pituitary %s\n", help.Name)
	} else {
		fmt.Fprintf(w, "pituitary %s: %s\n", help.Name, description)
	}
	fmt.Fprintf(w, "usage: %s\n", help.Usage)
	if help.UsesConfig {
		fmt.Fprintln(w)
		printSharedConfigResolution(w)
	}
	fmt.Fprintln(w)
	printCommandDebugTip(w, help.Name)
	fmt.Fprintln(w, "flags:")
	printFlagSetDefaults(w, fs)
}

func printCommandDebugTip(w io.Writer, name string) {
	switch name {
	case "preview-sources", "check-doc-drift", "check-compliance", "check-terminology", "compile":
		fmt.Fprintln(w, "debug tip:")
		fmt.Fprintln(w, "  if a file looks unexpectedly included, excluded, or misclassified, run `pituitary explain-file PATH` first")
		fmt.Fprintln(w)
	}
}

func printSharedConfigResolution(w io.Writer) {
	fmt.Fprintln(w, "shared config resolution:")
	fmt.Fprintln(w, "  the first match wins:")
	fmt.Fprintln(w, "  - command-local --config PATH")
	fmt.Fprintln(w, "  - global --config PATH before the command")
	fmt.Fprintf(w, "  - %s\n", configEnvVar)
	fmt.Fprintf(w, "  - %s or %s in the working directory or a parent directory\n", pathpkg.Join(localConfigDirName, defaultConfigName), defaultConfigName)
}

func printFlagSetDefaults(w io.Writer, fs *flag.FlagSet) {
	fs.VisitAll(func(f *flag.Flag) {
		placeholder := flagPlaceholder(f)
		label := "--" + f.Name
		if placeholder != "" {
			label += " " + placeholder
		}

		line := fmt.Sprintf("  %-24s %s", label, f.Usage)
		if suffix := flagDefaultSuffix(f); suffix != "" {
			line += suffix
		}
		fmt.Fprintln(w, line)
	})
}

func flagPlaceholder(f *flag.Flag) string {
	if boolFlag, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && boolFlag.IsBoolFlag() {
		return ""
	}

	name, _ := flag.UnquoteUsage(f)
	switch strings.ToLower(name) {
	case "", "string":
		return "VALUE"
	case "int":
		return "N"
	default:
		return strings.ToUpper(name)
	}
}

func flagDefaultSuffix(f *flag.Flag) string {
	switch f.DefValue {
	case "", "false":
		return ""
	default:
		return fmt.Sprintf(" (default %s)", f.DefValue)
	}
}
