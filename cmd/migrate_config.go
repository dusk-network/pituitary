package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
)

type migrateConfigRequest struct {
	Path  string `json:"path"`
	Write bool   `json:"write,omitempty"`
}

type migrateConfigResult struct {
	ConfigPath          string   `json:"config_path"`
	DetectedSchema      string   `json:"detected_schema"`
	TargetSchemaVersion int      `json:"target_schema_version"`
	WroteConfig         bool     `json:"wrote_config"`
	Notes               []string `json:"notes,omitempty"`
	Config              string   `json:"config"`
}

func runMigrateConfig(args []string, stdout, stderr io.Writer) int {
	return runMigrateConfigContext(context.Background(), args, stdout, stderr)
}

func runMigrateConfigContext(_ context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("migrate-config", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newStandaloneCommandHelp("migrate-config", "pituitary migrate-config [--path PATH] [--write] [--format FORMAT]")

	var (
		path   string
		write  bool
		format string
	)
	fs.StringVar(&path, "path", defaultConfigName, "path to the config file to migrate")
	fs.BoolVar(&write, "write", false, "rewrite the config in place")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "migrate-config", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "migrate-config", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}
	if err := validateCLIFormat("migrate-config", format); err != nil {
		return writeCLIError(stdout, stderr, format, "migrate-config", migrateConfigRequest{Path: path, Write: write}, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	request := migrateConfigRequest{
		Path:  strings.TrimSpace(path),
		Write: write,
	}
	if request.Path == "" {
		request.Path = defaultConfigName
	}

	migration, err := config.MigrateFile(request.Path)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "migrate-config", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}
	rendered, err := config.Render(migration.Config)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "migrate-config", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	if write {
		// #nosec G306 -- migrated config files are normal repo files intended to remain readable by standard tooling.
		if err := os.WriteFile(migration.Config.ConfigPath, []byte(rendered), 0o644); err != nil {
			return writeCLIError(stdout, stderr, format, "migrate-config", request, cliIssue{
				Code:    "config_error",
				Message: fmt.Sprintf("write migrated config: %v", err),
			}, 2)
		}
	}

	return writeCLISuccess(stdout, stderr, format, "migrate-config", request, &migrateConfigResult{
		ConfigPath:          migration.Config.ConfigPath,
		DetectedSchema:      migration.DetectedSchema,
		TargetSchemaVersion: config.CurrentSchemaVersion,
		WroteConfig:         write,
		Notes:               migration.Notes,
		Config:              rendered,
	}, nil)
}
