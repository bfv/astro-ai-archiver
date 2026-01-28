package tools

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

func RegisterRescanFitsDirectory(s *mcp.Server, db Database, scanDirs []string, recursive bool) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "rescan_fits_directory",
		Description: "Trigger a rescan of the FITS directory to find new/updated/deleted files",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"force": map[string]interface{}{
					"type":        "boolean",
					"description": "Force full rescan ignoring modification times",
					"default":     false,
				},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "rescan_fits_directory").Interface("params", args).Msg("Tool called")

		force := false
		if f, ok := args["force"].(bool); ok {
			force = f
		}

		scanMutex.Lock()
		if scanState.Scanning {
			scanMutex.Unlock()
			result := map[string]interface{}{
				"status":  "already_running",
				"message": "A scan is already in progress",
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Scan already in progress"},
				},
			}, result, nil
		}
		scanState.Scanning = true
		scanMutex.Unlock()

		go func() {
			defer func() {
				scanMutex.Lock()
				scanState.Scanning = false
				scanState.LastScanTime = time.Now()
				scanMutex.Unlock()
			}()

			scanner := db.NewScanner(scanDirs, recursive, force)
			// Use type assertion since we're working across packages
			type scannable interface {
				Scan() (*ScanResult, error)
			}
			result, err := scanner.(scannable).Scan()
			if err != nil {
				log.Error().Err(err).Msg("Scan failed")
			} else {
				log.Info().
					Int("added", result.FilesAdded).
					Int("updated", result.FilesUpdated).
					Msg("Scan completed")
			}
		}()

		result := map[string]interface{}{
			"status":  "started",
			"message": "Scan started in background",
			"force":   force,
		}

		log.Trace().Str("tool", "rescan_fits_directory").Interface("response", result).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Scan started"},
			},
		}, result, nil
	})
}
