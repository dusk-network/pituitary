package config

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const legacyProjectSchemaName = "project.v1"

type legacyProjectConfig struct {
	projectID   string
	projectName string
	specsDir    string
	docsDir     string
}

type MigrationResult struct {
	Config         *Config
	DetectedSchema string
	Notes          []string
}

func MigrateFile(path string) (*MigrationResult, error) {
	configPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	// #nosec G304 -- configPath is the explicit config file selected for migration.
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", configPath, err)
	}

	if legacy, ok, err := detectLegacyProjectConfig(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("%s: %w", configPath, err)
	} else if ok {
		cfg, notes, err := migrateLegacyProjectConfig(configPath, legacy)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", configPath, err)
		}
		return &MigrationResult{
			Config:         cfg,
			DetectedSchema: legacyProjectSchemaName,
			Notes:          notes,
		}, nil
	}

	raw, err := parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", configPath, err)
	}
	cfg, err := buildFromRaw(configPath, raw, false)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", configPath, err)
	}

	detectedSchema := fmt.Sprintf("current.v%d", CurrentSchemaVersion)
	var notes []string
	switch raw.SchemaVersion {
	case 0:
		detectedSchema = "current.unversioned"
		notes = append(notes, fmt.Sprintf("added explicit schema_version = %d", CurrentSchemaVersion))
	case CurrentSchemaVersion:
		notes = append(notes, fmt.Sprintf("config already uses schema_version = %d; output is normalized", CurrentSchemaVersion))
	default:
		return nil, fmt.Errorf("cannot migrate unsupported schema_version %d automatically", raw.SchemaVersion)
	}

	return &MigrationResult{
		Config:         cfg,
		DetectedSchema: detectedSchema,
		Notes:          notes,
	}, nil
}

func detectLegacyProjectConfig(reader io.Reader) (*legacyProjectConfig, bool, error) {
	var (
		legacy           legacyProjectConfig
		section          string
		sawProject       bool
		sawCurrentSchema bool
	)

	scanner := bufio.NewScanner(reader)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]"):
			if sawProject {
				return nil, false, fmt.Errorf("line %d: unsupported array section %q in legacy [project] config", lineNo, strings.TrimSpace(line[2:len(line)-2]))
			}
			sawCurrentSchema = true
			continue
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			section = strings.TrimSpace(line[1 : len(line)-1])
			switch section {
			case "project":
				sawProject = true
			case "workspace", "runtime.embedder", "runtime.analysis":
				sawCurrentSchema = true
			}
			continue
		}

		if section == "" && strings.HasPrefix(line, "schema_version") {
			sawCurrentSchema = true
			continue
		}

		if section != "project" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, false, fmt.Errorf("line %d: expected key = value", lineNo)
		}
		key = strings.TrimSpace(key)
		parsed, err := parseQuotedString(strings.TrimSpace(value))
		if err != nil {
			return nil, false, fmt.Errorf("line %d: project.%s: %w", lineNo, key, err)
		}
		switch key {
		case "id":
			legacy.projectID = parsed
		case "name":
			legacy.projectName = parsed
		case "specs_dir":
			legacy.specsDir = parsed
		case "docs_dir":
			legacy.docsDir = parsed
		default:
			return nil, false, fmt.Errorf("line %d: unsupported project field %q", lineNo, key)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, false, fmt.Errorf("read config: %w", err)
	}
	if sawProject && sawCurrentSchema {
		return nil, false, fmt.Errorf("cannot mix legacy [project] config with current schema sections")
	}
	if !sawProject {
		return nil, false, nil
	}
	if strings.TrimSpace(legacy.specsDir) == "" {
		return nil, false, fmt.Errorf("legacy [project] config is missing required field \"specs_dir\"")
	}
	return &legacy, true, nil
}

func migrateLegacyProjectConfig(configPath string, legacy *legacyProjectConfig) (*Config, []string, error) {
	if legacy == nil {
		return nil, nil, fmt.Errorf("legacy config is required")
	}

	cfg := &Config{
		SchemaVersion: CurrentSchemaVersion,
		ConfigPath:    configPath,
		ConfigDir:     configBaseDir(configPath),
		Workspace: Workspace{
			Root:      ".",
			IndexPath: filepath.ToSlash(filepath.Join(".pituitary", "pituitary.db")),
		},
		Runtime: Runtime{
			Embedder: RuntimeProvider{
				Provider:   RuntimeProviderFixture,
				Model:      "fixture-8d",
				TimeoutMS:  1000,
				MaxRetries: 0,
			},
			Analysis: RuntimeProvider{
				Provider:   RuntimeProviderDisabled,
				TimeoutMS:  1000,
				MaxRetries: 0,
			},
		},
		Sources: []Source{
			{
				Name:    "specs",
				Adapter: AdapterFilesystem,
				Kind:    SourceKindSpecBundle,
				Path:    legacy.specsDir,
			},
		},
	}
	if strings.TrimSpace(legacy.docsDir) != "" {
		cfg.Sources = append(cfg.Sources, Source{
			Name:    "docs",
			Adapter: AdapterFilesystem,
			Kind:    SourceKindMarkdownDocs,
			Path:    legacy.docsDir,
		})
	}

	if err := validate(cfg); err != nil {
		return nil, nil, err
	}

	notes := []string{
		fmt.Sprintf("migrated legacy [project] config to schema_version = %d", CurrentSchemaVersion),
	}
	if strings.TrimSpace(legacy.projectID) != "" || strings.TrimSpace(legacy.projectName) != "" {
		notes = append(notes, "legacy project.id and project.name are informational only and are not represented in the current schema")
	}
	if strings.TrimSpace(legacy.docsDir) == "" {
		notes = append(notes, "legacy config did not declare docs; add markdown docs sources manually or run `pituitary discover` if needed")
	}
	return cfg, notes, nil
}

func legacyConfigLoadMessage(configPath string, legacy *legacyProjectConfig) string {
	if legacy == nil {
		return fmt.Sprintf("legacy config schema detected; run `pituitary migrate-config --path %s --write`", filepath.ToSlash(configPath))
	}
	return fmt.Sprintf(
		"legacy config schema detected: [project] with specs_dir = %q; run `pituitary migrate-config --path %s --write`",
		legacy.specsDir,
		filepath.ToSlash(configPath),
	)
}
