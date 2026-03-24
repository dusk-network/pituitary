package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/source"
)

const (
	freshnessStateMissing      = "missing"
	freshnessStateFresh        = "fresh"
	freshnessStateStale        = "stale"
	freshnessStateIncompatible = "incompatible"
)

// FreshnessIssue describes one stale or incompatible index signal.
type FreshnessIssue struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Indexed string `json:"indexed,omitempty"`
	Current string `json:"current,omitempty"`
}

// FreshnessStatus reports whether the current workspace can safely reuse the configured index.
type FreshnessStatus struct {
	IndexPath string           `json:"index_path"`
	State     string           `json:"state"`
	Action    string           `json:"action,omitempty"`
	Issues    []FreshnessIssue `json:"issues,omitempty"`
}

// StaleIndexError reports that the configured index is present but not safe to reuse.
type StaleIndexError struct {
	Status *FreshnessStatus
}

func (e *StaleIndexError) Error() string {
	if e == nil || e.Status == nil {
		return "index is stale; run `pituitary index --rebuild`"
	}
	parts := make([]string, 0, len(e.Status.Issues))
	for _, issue := range e.Status.Issues {
		if strings.TrimSpace(issue.Message) != "" {
			parts = append(parts, issue.Message)
		}
	}
	switch {
	case len(parts) == 0 && strings.TrimSpace(e.Status.Action) == "":
		return "index is stale; run `pituitary index --rebuild`"
	case len(parts) == 0:
		return fmt.Sprintf("index is %s; %s", e.Status.State, e.Status.Action)
	default:
		message := fmt.Sprintf("index is %s: %s", e.Status.State, strings.Join(parts, "; "))
		if strings.TrimSpace(e.Status.Action) != "" {
			message = fmt.Sprintf("%s; %s", message, e.Status.Action)
		}
		return message
	}
}

// IsStaleIndex reports whether err wraps a stale or incompatible index failure.
func IsStaleIndex(err error) bool {
	var target *StaleIndexError
	return errors.As(err, &target)
}

// InspectFreshnessContext compares the configured workspace against the stored index metadata.
func InspectFreshnessContext(ctx context.Context, cfg *config.Config) (*FreshnessStatus, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	status := &FreshnessStatus{
		IndexPath: cfg.Workspace.ResolvedIndexPath,
		State:     freshnessStateMissing,
		Action:    "run `pituitary index --rebuild`",
	}

	info, err := os.Stat(cfg.Workspace.ResolvedIndexPath)
	switch {
	case os.IsNotExist(err):
		return status, nil
	case err != nil:
		return nil, fmt.Errorf("stat index %s: %w", cfg.Workspace.ResolvedIndexPath, err)
	case info.IsDir():
		return nil, fmt.Errorf("index path %s is a directory", cfg.Workspace.ResolvedIndexPath)
	}

	configuredEmbedderFingerprint, err := configuredEmbedderFingerprint(cfg.Runtime.Embedder)
	if err != nil {
		return nil, err
	}

	currentSourceFingerprint := sourceFingerprint(cfg)

	db, err := OpenReadOnlyContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return nil, fmt.Errorf("open index %s: %w", cfg.Workspace.ResolvedIndexPath, err)
	}
	defer db.Close()

	metadata, err := readMetadataContext(ctx, db,
		"schema_version",
		"embedder_fingerprint",
		"source_fingerprint",
		"content_fingerprint",
	)
	if err != nil {
		return nil, err
	}

	issues := make([]FreshnessIssue, 0, 4)
	switch stored := strings.TrimSpace(metadata["schema_version"]); stored {
	case "":
		issues = append(issues, FreshnessIssue{
			Kind:    "missing_schema_version",
			Message: "index metadata is missing schema_version",
		})
	case fmt.Sprintf("%d", schemaVersion):
	default:
		issues = append(issues, FreshnessIssue{
			Kind:    "schema_version_mismatch",
			Message: fmt.Sprintf("index schema_version %q does not match expected schema_version %d", stored, schemaVersion),
			Indexed: stored,
			Current: fmt.Sprintf("%d", schemaVersion),
		})
	}

	switch stored := strings.TrimSpace(metadata["embedder_fingerprint"]); {
	case stored == "":
		issues = append(issues, FreshnessIssue{
			Kind:    "missing_embedder_fingerprint",
			Message: "index metadata is missing embedder_fingerprint",
		})
	case configuredEmbedderFingerprint != "" && stored != configuredEmbedderFingerprint:
		issues = append(issues, FreshnessIssue{
			Kind:    "embedder_fingerprint_mismatch",
			Message: fmt.Sprintf("index embedder fingerprint %q does not match configured embedder fingerprint %q", stored, configuredEmbedderFingerprint),
			Indexed: stored,
			Current: configuredEmbedderFingerprint,
		})
	}

	switch stored := strings.TrimSpace(metadata["source_fingerprint"]); {
	case stored == "":
		issues = append(issues, FreshnessIssue{
			Kind:    "missing_source_fingerprint",
			Message: "index metadata is missing source_fingerprint",
		})
	case stored != currentSourceFingerprint:
		issues = append(issues, FreshnessIssue{
			Kind:    "source_fingerprint_mismatch",
			Message: fmt.Sprintf("index source fingerprint %q does not match current configured source fingerprint %q", stored, currentSourceFingerprint),
			Indexed: stored,
			Current: currentSourceFingerprint,
		})
	}

	if len(issues) != 0 {
		status.Issues = issues
		status.State = deriveFreshnessState(issues)
		if status.State == freshnessStateFresh {
			status.Action = ""
		}
		return status, nil
	}

	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("load current sources for freshness check: %w", err)
	}
	currentContentFingerprint := contentFingerprint(records)

	switch stored := strings.TrimSpace(metadata["content_fingerprint"]); {
	case stored == "":
		issues = append(issues, FreshnessIssue{
			Kind:    "missing_content_fingerprint",
			Message: "index metadata is missing content_fingerprint",
		})
	case stored != currentContentFingerprint:
		issues = append(issues, FreshnessIssue{
			Kind:    "content_fingerprint_mismatch",
			Message: fmt.Sprintf("index content fingerprint %q does not match current workspace content fingerprint %q", stored, currentContentFingerprint),
			Indexed: stored,
			Current: currentContentFingerprint,
		})
	}

	status.Issues = issues
	status.State = deriveFreshnessState(issues)
	if status.State == freshnessStateFresh {
		status.Action = ""
	}
	return status, nil
}

func configuredEmbedderFingerprint(provider config.RuntimeProvider) (string, error) {
	switch provider.Provider {
	case "", config.RuntimeProviderFixture:
		if _, err := fixtureDimension(provider.Model); err != nil {
			return "", err
		}
		return embedderFingerprint(config.RuntimeProviderFixture, provider.Model, embeddingStrategyPlain), nil
	case config.RuntimeProviderOpenAI:
		return embedderFingerprint(config.RuntimeProviderOpenAI, provider.Model, embeddingStrategyForModel(provider.Model)), nil
	default:
		return "", fmt.Errorf(
			"runtime.embedder.provider %q is not supported; supported providers are %q and %q",
			provider.Provider,
			config.RuntimeProviderFixture,
			config.RuntimeProviderOpenAI,
		)
	}
}

// ValidateFreshnessContext returns a stale-index error when the current workspace no longer matches the index metadata.
func ValidateFreshnessContext(ctx context.Context, cfg *config.Config) error {
	status, err := InspectFreshnessContext(ctx, cfg)
	if err != nil {
		return err
	}
	switch status.State {
	case freshnessStateFresh, freshnessStateMissing:
		return nil
	default:
		return &StaleIndexError{Status: status}
	}
}

func deriveFreshnessState(issues []FreshnessIssue) string {
	if len(issues) == 0 {
		return freshnessStateFresh
	}
	for _, issue := range issues {
		if issue.Kind == "content_fingerprint_mismatch" {
			continue
		}
		return freshnessStateIncompatible
	}
	return freshnessStateStale
}

func contentFingerprint(records *source.LoadResult) string {
	if records == nil {
		return ""
	}
	parts := make([]string, 0, len(records.Specs)+len(records.Docs))
	for _, spec := range records.Specs {
		parts = append(parts, spec.Ref+":"+spec.ContentHash)
	}
	for _, doc := range records.Docs {
		parts = append(parts, doc.Ref+":"+doc.ContentHash)
	}
	return fingerprint(parts)
}

func sourceFingerprint(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	parts := make([]string, 0, len(cfg.Sources))
	for _, src := range cfg.Sources {
		files := append([]string(nil), src.Files...)
		include := append([]string(nil), src.Include...)
		exclude := append([]string(nil), src.Exclude...)
		sort.Strings(files)
		sort.Strings(include)
		sort.Strings(exclude)

		part := strings.Join([]string{
			src.Name,
			src.Adapter,
			src.Kind,
			filepath.ToSlash(src.Path),
			filepath.ToSlash(src.ResolvedPath),
			strings.Join(files, ","),
			strings.Join(include, ","),
			strings.Join(exclude, ","),
		}, "|")
		parts = append(parts, part)
	}
	return fingerprint(parts)
}

func readMetadataContext(ctx context.Context, db *sql.DB, keys ...string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	if len(keys) == 0 {
		return result, nil
	}

	query := fmt.Sprintf(`SELECT key, value FROM metadata WHERE key IN (%s)`, placeholders(len(keys)))
	args := make([]any, 0, len(keys))
	for _, key := range keys {
		args = append(args, key)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read index metadata: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var key string
		var value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan index metadata: %w", err)
		}
		result[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate index metadata: %w", err)
	}
	return result, nil
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	parts := make([]string, 0, count)
	for i := 0; i < count; i++ {
		parts = append(parts, "?")
	}
	return strings.Join(parts, ", ")
}
