package config

import (
	"fmt"
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
		fmt.Fprintf(&builder, "path = %s\n", strconv.Quote(source.Path))
		writeQuotedArray(&builder, "files", source.Files)
		writeQuotedArray(&builder, "include", source.Include)
		writeQuotedArray(&builder, "exclude", source.Exclude)
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
