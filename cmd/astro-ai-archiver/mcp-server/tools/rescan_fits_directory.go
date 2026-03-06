package tools

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

func RegisterRescanFitsDirectory(s *mcp.Server, db Database, scanDirs []string, recursive bool) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "rescan_fits_directory",
		Description: "Trigger a rescan of the configured FITS directories to find new/updated files. Optionally limit the rescan to a single directory by matching its last path component (e.g. \"_2022\" matches \"/app/data/_2022\").",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"force": map[string]interface{}{
					"type":        "boolean",
					"description": "Force full rescan ignoring modification times",
					"default":     false,
				},
				"directory": map[string]interface{}{
					"type":        "string",
					"description": "Optional: last path component of the directory to rescan (e.g. \"_2022\"). When omitted all configured directories are scanned.",
				},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "rescan_fits_directory").Interface("params", args).Msg("Tool called")

		force := false
		if f, ok := args["force"].(bool); ok {
			force = f
		}

		// Resolve which directories to scan
		dirsToScan := scanDirs
		if dirFilter, ok := args["directory"].(string); ok && dirFilter != "" {
			var matched []string
			for _, d := range scanDirs {
				if filepath.Base(d) == dirFilter {
					matched = append(matched, d)
				}
			}
			if len(matched) == 0 {
				msg := fmt.Sprintf("no configured directory whose last component matches %q; configured directories: %v", dirFilter, scanDirs)
				log.Warn().Str("tool", "rescan_fits_directory").Str("filter", dirFilter).Msg("No matching directory")
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: msg},
					},
				}, map[string]interface{}{"status": "no_match", "message": msg}, nil
			}
			dirsToScan = matched
		}

		// Try to begin scan
		if !BeginScan() {
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

		go func() {
			defer EndScan()

			scanner := db.NewScanner(dirsToScan, recursive, force)
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
			"status":      "started",
			"message":     "Scan started in background",
			"force":       force,
			"directories": dirsToScan,
		}

		log.Trace().Str("tool", "rescan_fits_directory").Interface("response", result).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Scan started for %v", dirsToScan)},
			},
		}, result, nil
	})
}
