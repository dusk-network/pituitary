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
	fmt.Fprintf(&builder, "index_path = %s\n", strconv.Quote(cfg.Workspace.IndexPath))

	builder.WriteString("\n[runtime.embedder]\n")
	fmt.Fprintf(&builder, "provider = %s\n", strconv.Quote(cfg.Runtime.Embedder.Provider))
	fmt.Fprintf(&builder, "model = %s\n", strconv.Quote(cfg.Runtime.Embedder.Model))
	if cfg.Runtime.Embedder.Endpoint != "" {
		fmt.Fprintf(&builder, "endpoint = %s\n", strconv.Quote(cfg.Runtime.Embedder.Endpoint))
	}
	if cfg.Runtime.Embedder.APIKeyEnv != "" {
		fmt.Fprintf(&builder, "api_key_env = %s\n", strconv.Quote(cfg.Runtime.Embedder.APIKeyEnv))
	}
	fmt.Fprintf(&builder, "timeout_ms = %d\n", cfg.Runtime.Embedder.TimeoutMS)
	fmt.Fprintf(&builder, "max_retries = %d\n", cfg.Runtime.Embedder.MaxRetries)

	if provider := strings.TrimSpace(cfg.Runtime.Analysis.Provider); provider != "" && provider != "disabled" {
		builder.WriteString("\n[runtime.analysis]\n")
		fmt.Fprintf(&builder, "provider = %s\n", strconv.Quote(provider))
		if cfg.Runtime.Analysis.Model != "" {
			fmt.Fprintf(&builder, "model = %s\n", strconv.Quote(cfg.Runtime.Analysis.Model))
		}
		if cfg.Runtime.Analysis.Endpoint != "" {
			fmt.Fprintf(&builder, "endpoint = %s\n", strconv.Quote(cfg.Runtime.Analysis.Endpoint))
		}
		if cfg.Runtime.Analysis.APIKeyEnv != "" {
			fmt.Fprintf(&builder, "api_key_env = %s\n", strconv.Quote(cfg.Runtime.Analysis.APIKeyEnv))
		}
		fmt.Fprintf(&builder, "timeout_ms = %d\n", cfg.Runtime.Analysis.TimeoutMS)
		fmt.Fprintf(&builder, "max_retries = %d\n", cfg.Runtime.Analysis.MaxRetries)
	}

	for _, source := range cfg.Sources {
		builder.WriteString("\n[[sources]]\n")
		fmt.Fprintf(&builder, "name = %s\n", strconv.Quote(source.Name))
		fmt.Fprintf(&builder, "adapter = %s\n", strconv.Quote(source.Adapter))
		fmt.Fprintf(&builder, "kind = %s\n", strconv.Quote(source.Kind))
		if source.Role != "" {
			fmt.Fprintf(&builder, "role = %s\n", strconv.Quote(source.Role))
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
