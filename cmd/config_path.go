package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultConfigName  = "pituitary.toml"
	localConfigDirName = ".pituitary"
	configEnvVar       = "PITUITARY_CONFIG"
)

type cliGlobalOptions struct {
	ConfigPath string
}

type cliConfigPathContextKey struct{}

func parseGlobalCLIOptions(args []string) (cliGlobalOptions, []string, error) {
	fs := flag.NewFlagSet("pituitary", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var options cliGlobalOptions
	fs.StringVar(&options.ConfigPath, "config", "", "path to workspace config")

	if err := fs.Parse(args); err != nil {
		return cliGlobalOptions{}, nil, err
	}

	options.ConfigPath = strings.TrimSpace(options.ConfigPath)
	return options, fs.Args(), nil
}

func withCLIConfigPath(ctx context.Context, configPath string) context.Context {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return ctx
	}
	return context.WithValue(ctx, cliConfigPathContextKey{}, configPath)
}

func cliConfigPathFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(cliConfigPathContextKey{}).(string)
	return strings.TrimSpace(value)
}

func resolveCommandConfigPath(ctx context.Context, localConfigPath string) (string, error) {
	return resolveCLIConfigPath(localConfigPath, cliConfigPathFromContext(ctx))
}

func resolveCLIConfigPath(explicitPaths ...string) (string, error) {
	for _, explicitPath := range explicitPaths {
		explicitPath = strings.TrimSpace(explicitPath)
		if explicitPath != "" {
			return explicitPath, nil
		}
	}

	envPath := strings.TrimSpace(os.Getenv(configEnvVar))
	if envPath != "" {
		return envPath, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}

	if discoveredPath, ok := discoverCLIConfigPath(cwd); ok {
		return discoveredPath, nil
	}

	return "", fmt.Errorf(
		"no config found; set --config, set %s, or add %s or %s in %s or a parent directory",
		configEnvVar,
		filepath.ToSlash(filepath.Join(localConfigDirName, defaultConfigName)),
		defaultConfigName,
		filepath.ToSlash(cwd),
	)
}

func discoverCLIConfigPath(startDir string) (string, bool) {
	dir := filepath.Clean(startDir)
	for {
		for _, candidate := range []string{
			filepath.Join(dir, localConfigDirName, defaultConfigName),
			filepath.Join(dir, defaultConfigName),
		} {
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() {
				return candidate, true
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
