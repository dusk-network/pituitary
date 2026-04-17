package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dusk-network/pituitary/internal/app"
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

func runMigrateConfigContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var (
		path  string
		write bool
	)

	return runCommand[migrateConfigRequest, migrateConfigResult](
		ctx, args, stdout, stderr,
		commandRun[migrateConfigRequest, migrateConfigResult]{
			Name:  "migrate-config",
			Usage: "pituitary migrate-config [--path PATH] [--write] [--format FORMAT]",
			Options: commandRunOptions{
				Standalone: true,
			},
			BindFlags: func(fs *flag.FlagSet) {
				fs.StringVar(&path, "path", defaultConfigName, "path to the config file to migrate")
				fs.BoolVar(&write, "write", false, "rewrite the config in place")
			},
			BuildRequest: func(_ context.Context, _ *config.Config, _ string, _ []string) (migrateConfigRequest, error) {
				trimmedPath := strings.TrimSpace(path)
				if trimmedPath == "" {
					trimmedPath = defaultConfigName
				}
				return migrateConfigRequest{
					Path:  trimmedPath,
					Write: write,
				}, nil
			},
			Execute: func(_ context.Context, _ string, req migrateConfigRequest, _ string) (migrateConfigRequest, *migrateConfigResult, *app.Issue) {
				migration, err := config.MigrateFile(req.Path)
				if err != nil {
					return req, nil, plainIssue(err, "config_error")
				}
				rendered, err := config.Render(migration.Config)
				if err != nil {
					return req, nil, plainIssue(err, "config_error")
				}
				if req.Write {
					// #nosec G306 -- migrated config files are normal repo files intended to remain readable by standard tooling.
					if err := os.WriteFile(migration.Config.ConfigPath, []byte(rendered), 0o644); err != nil {
						return req, nil, plainIssue(fmt.Errorf("write migrated config: %w", err), "config_error")
					}
				}
				return req, &migrateConfigResult{
					ConfigPath:          migration.Config.ConfigPath,
					DetectedSchema:      migration.DetectedSchema,
					TargetSchemaVersion: config.CurrentSchemaVersion,
					WroteConfig:         req.Write,
					Notes:               migration.Notes,
					Config:              rendered,
				}, nil
			},
		},
	)
}
