package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Render serializes a validated config into the restricted TOML subset that
// the bootstrap parser accepts.
func Render(cfg *Config) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("config is required")
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "schema_version = %d\n\n", CurrentSchemaVersion)
	fmt.Fprintf(&builder, "[workspace]\n")
	fmt.Fprintf(&builder, "root = %s\n", strconv.Quote(cfg.Workspace.Root))
	if repoID := strings.TrimSpace(cfg.Workspace.RepoID); repoID != "" {
		fmt.Fprintf(&builder, "repo_id = %s\n", strconv.Quote(repoID))
	}
	fmt.Fprintf(&builder, "index_path = %s\n", strconv.Quote(cfg.Workspace.IndexPath))
	for _, repo := range cfg.Workspace.Repos {
		builder.WriteString("\n[[workspace.repos]]\n")
		fmt.Fprintf(&builder, "id = %s\n", strconv.Quote(repo.ID))
		fmt.Fprintf(&builder, "root = %s\n", strconv.Quote(repo.Root))
	}

	profileNames := make([]string, 0, len(cfg.Runtime.Profiles))
	for name := range cfg.Runtime.Profiles {
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)
	for _, name := range profileNames {
		builder.WriteString("\n[runtime.profiles.")
		builder.WriteString(name)
		builder.WriteString("]\n")
		writeRuntimeProviderConfig(&builder, cfg.Runtime.Profiles[name], nil)
	}

	builder.WriteString("\n[runtime.embedder]\n")
	writeRuntimeProviderConfig(&builder, cfg.Runtime.Embedder, runtimeProfileBase(cfg.Runtime.Profiles, cfg.Runtime.Embedder.Profile))

	if provider := strings.TrimSpace(cfg.Runtime.Analysis.Provider); provider != "" && (provider != RuntimeProviderDisabled || strings.TrimSpace(cfg.Runtime.Analysis.Profile) != "") {
		builder.WriteString("\n[runtime.analysis]\n")
		writeRuntimeProviderConfig(&builder, cfg.Runtime.Analysis, runtimeProfileBase(cfg.Runtime.Profiles, cfg.Runtime.Analysis.Profile))
	}

	for _, source := range cfg.Sources {
		builder.WriteString("\n[[sources]]\n")
		fmt.Fprintf(&builder, "name = %s\n", strconv.Quote(source.Name))
		fmt.Fprintf(&builder, "adapter = %s\n", strconv.Quote(source.Adapter))
		fmt.Fprintf(&builder, "kind = %s\n", strconv.Quote(source.Kind))
		if source.Role != "" {
			fmt.Fprintf(&builder, "role = %s\n", strconv.Quote(source.Role))
		}
		if source.Repo != "" {
			fmt.Fprintf(&builder, "repo = %s\n", strconv.Quote(source.Repo))
		}
		if source.Path != "" {
			fmt.Fprintf(&builder, "path = %s\n", strconv.Quote(source.Path))
		}
		writeQuotedArray(&builder, "files", source.Files)
		writeQuotedArray(&builder, "include", source.Include)
		writeQuotedArray(&builder, "exclude", source.Exclude)
		if err := writeOptionsTables(&builder, "sources.options", source.Options); err != nil {
			return "", fmt.Errorf("render source %q options: %w", source.Name, err)
		}
	}

	return builder.String(), nil
}

func runtimeProfileBase(profiles map[string]RuntimeProvider, name string) *RuntimeProvider {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	profile, ok := profiles[name]
	if !ok {
		return nil
	}
	return &profile
}

func writeRuntimeProviderConfig(builder *strings.Builder, provider RuntimeProvider, base *RuntimeProvider) {
	if profile := strings.TrimSpace(provider.Profile); profile != "" {
		fmt.Fprintf(builder, "profile = %s\n", strconv.Quote(profile))
	}
	if base == nil || provider.Provider != base.Provider {
		fmt.Fprintf(builder, "provider = %s\n", strconv.Quote(provider.Provider))
	}
	if provider.Model != "" && (base == nil || provider.Model != base.Model) {
		fmt.Fprintf(builder, "model = %s\n", strconv.Quote(provider.Model))
	}
	if provider.Endpoint != "" && (base == nil || provider.Endpoint != base.Endpoint) {
		fmt.Fprintf(builder, "endpoint = %s\n", strconv.Quote(provider.Endpoint))
	}
	if provider.APIKeyEnv != "" && (base == nil || provider.APIKeyEnv != base.APIKeyEnv) {
		fmt.Fprintf(builder, "api_key_env = %s\n", strconv.Quote(provider.APIKeyEnv))
	}
	if base == nil || provider.TimeoutMS != base.TimeoutMS {
		fmt.Fprintf(builder, "timeout_ms = %d\n", provider.TimeoutMS)
	}
	if base == nil || provider.MaxRetries != base.MaxRetries {
		fmt.Fprintf(builder, "max_retries = %d\n", provider.MaxRetries)
	}
}

func writeQuotedArray(builder *strings.Builder, key string, values []string) {
	if len(values) == 0 {
		return
	}

	builder.WriteString(key)
	builder.WriteString(" = [\n")
	for _, value := range values {
		builder.WriteString("  ")
		builder.WriteString(strconv.Quote(value))
		builder.WriteString(",\n")
	}
	builder.WriteString("]\n")
}

func writeOptionsTables(builder *strings.Builder, table string, options map[string]any) error {
	if len(options) == 0 {
		return nil
	}

	scalarKeys := make([]string, 0, len(options))
	nestedKeys := make([]string, 0, len(options))
	for key, value := range options {
		switch value.(type) {
		case map[string]any:
			nestedKeys = append(nestedKeys, key)
		default:
			scalarKeys = append(scalarKeys, key)
		}
	}
	sort.Strings(scalarKeys)
	sort.Strings(nestedKeys)

	builder.WriteString("\n[")
	builder.WriteString(table)
	builder.WriteString("]\n")
	for _, key := range scalarKeys {
		rendered, err := renderOptionValue(options[key])
		if err != nil {
			return fmt.Errorf("%s.%s: %w", table, key, err)
		}
		builder.WriteString(key)
		builder.WriteString(" = ")
		builder.WriteString(rendered)
		builder.WriteString("\n")
	}
	for _, key := range nestedKeys {
		nested, _ := options[key].(map[string]any)
		if err := writeOptionsTables(builder, table+"."+key, nested); err != nil {
			return err
		}
	}
	return nil
}

func renderOptionValue(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return strconv.Quote(typed), nil
	case bool:
		return strconv.FormatBool(typed), nil
	case int:
		return strconv.Itoa(typed), nil
	case int8:
		return strconv.FormatInt(int64(typed), 10), nil
	case int16:
		return strconv.FormatInt(int64(typed), 10), nil
	case int32:
		return strconv.FormatInt(int64(typed), 10), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case uint:
		return strconv.FormatUint(uint64(typed), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(typed), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(typed), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(typed), 10), nil
	case uint64:
		return strconv.FormatUint(typed, 10), nil
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32), nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case []string:
		values := make([]any, len(typed))
		for i := range typed {
			values[i] = typed[i]
		}
		return renderOptionArray(values)
	case []any:
		return renderOptionArray(typed)
	case map[string]any:
		return "", fmt.Errorf("nested maps must be rendered as subtables")
	default:
		return "", fmt.Errorf("unsupported value type %T", value)
	}
}

func renderOptionArray(values []any) (string, error) {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		rendered, err := renderOptionValue(value)
		if err != nil {
			return "", err
		}
		parts = append(parts, rendered)
	}
	return "[" + strings.Join(parts, ", ") + "]", nil
}
