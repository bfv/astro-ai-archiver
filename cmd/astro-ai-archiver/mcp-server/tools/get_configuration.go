package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

func RegisterGetConfiguration(s *mcp.Server, cfg Config, dbPath string, version string) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_configuration",
		Description: "Show current AAA configuration (scan directory, database path, etc.)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "get_configuration").Msg("Tool called")

		config := map[string]interface{}{
			"version":          version,
			"scan_directories": cfg.GetScanDirectories(),
			"recursive":        cfg.GetScanRecursive(),
			"scan_on_startup":  cfg.GetScanOnStartup(),
			"database_path":    dbPath,
			"log_level":        cfg.GetLoggingLevel(),
			"log_format":       cfg.GetLoggingFormat(),
		}

		log.Trace().Str("tool", "get_configuration").Interface("response", config).Msg("Tool response")

		scanDirs := cfg.GetScanDirectories()
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Configuration: %d scan director%s", len(scanDirs), map[bool]string{true: "y", false: "ies"}[len(scanDirs) == 1])},
			},
		}, config, nil
	})
}
