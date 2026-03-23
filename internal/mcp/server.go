package mcp

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const (
	defaultConfigPath = "pituitary.toml"
	serverName        = "Pituitary"
)

// serverVersion defaults to "dev" and can be overridden at build time with -ldflags.
var serverVersion = "dev"

var sqliteReadyCheck = index.CheckSQLiteReady

// Options configures the MCP transport.
type Options struct {
	ConfigPath string
}

func (o Options) normalized() Options {
	if o.ConfigPath == "" {
		o.ConfigPath = defaultConfigPath
	}
	return o
}

// Tools returns the registered MCP tools for the configured workspace.
func Tools(options Options) []mcpserver.ServerTool {
	options = options.normalized()
	return []mcpserver.ServerTool{
		searchSpecsTool(options),
		checkOverlapTool(options),
		compareSpecsTool(options),
		analyzeImpactTool(options),
		checkDocDriftTool(options),
		reviewSpecTool(options),
	}
}

// NewServer constructs the MCP server with the registered tools.
func NewServer(options Options) *mcpserver.MCPServer {
	options = options.normalized()

	server := mcpserver.NewMCPServer(
		serverName,
		serverVersion,
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithRecovery(),
	)
	server.AddTools(Tools(options)...)
	return server
}

// ServeStdio runs the MCP server over stdio.
func ServeStdio(options Options) error {
	if err := validateStartup(options); err != nil {
		return err
	}
	return mcpserver.ServeStdio(NewServer(options))
}

func validateStartup(options Options) error {
	options = options.normalized()

	cfg, err := config.Load(options.ConfigPath)
	if err != nil {
		return fmt.Errorf("load config %s: %w", options.ConfigPath, err)
	}

	embedder, err := index.NewEmbedder(cfg.Runtime.Embedder)
	if err != nil {
		return err
	}
	if err := sqliteReadyCheck(); err != nil {
		return fmt.Errorf("sqlite readiness check failed: %w", err)
	}

	if _, err := os.Stat(cfg.Workspace.ResolvedIndexPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("index %s does not exist; run `pituitary index --rebuild`", cfg.Workspace.ResolvedIndexPath)
		}
		return fmt.Errorf("stat index %s: %w", cfg.Workspace.ResolvedIndexPath, err)
	}

	db, err := index.OpenReadOnly(cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return fmt.Errorf("open index %s: %w", cfg.Workspace.ResolvedIndexPath, err)
	}
	defer db.Close()

	dimension, err := embedder.Dimension(context.Background())
	if err != nil {
		return err
	}
	if err := validateIndexReady(db, embedder.Fingerprint(), dimension); err != nil {
		return err
	}
	return nil
}

func validateIndexReady(db *sql.DB, configuredFingerprint string, configuredDimension int) error {
	var raw string
	err := db.QueryRow(`SELECT value FROM metadata WHERE key = 'embedder_dimension'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return fmt.Errorf("index metadata is missing embedder_dimension; run `pituitary index --rebuild`")
	}
	if err != nil {
		return fmt.Errorf("read index metadata: %w", err)
	}

	stored, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("parse embedder_dimension %q: %w", raw, err)
	}
	if stored != configuredDimension {
		return fmt.Errorf("index embedder dimension %d does not match configured embedder dimension %d; run `pituitary index --rebuild`", stored, configuredDimension)
	}

	if strings.TrimSpace(configuredFingerprint) == "" {
		return nil
	}

	var storedFingerprint string
	err = db.QueryRow(`SELECT value FROM metadata WHERE key = 'embedder_fingerprint'`).Scan(&storedFingerprint)
	if err == sql.ErrNoRows {
		return fmt.Errorf("index metadata is missing embedder_fingerprint; run `pituitary index --rebuild`")
	}
	if err != nil {
		return fmt.Errorf("read index metadata: %w", err)
	}
	if storedFingerprint != configuredFingerprint {
		return fmt.Errorf("index embedder fingerprint %q does not match configured embedder fingerprint %q; run `pituitary index --rebuild`", storedFingerprint, configuredFingerprint)
	}
	return nil
}
