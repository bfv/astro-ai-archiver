package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

func RegisterQueryFitsArchive(s *mcp.Server, db Database) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "query_fits_archive",
		Description: "Search and retrieve individual FITS file records with flexible filters (target, filter, telescope, date range, etc.). Returns paginated file details. For counting, statistics, or aggregations use execute_sql_query instead.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"target": map[string]interface{}{
					"type":        "string",
					"description": "Target object name (partial match, e.g., 'NGC2903', 'M31')",
				},
				"filter": map[string]interface{}{
					"type":        "string",
					"description": "Filter name (exact match, e.g., 'Ha', 'OIII', 'L', 'Red')",
				},
				"telescope": map[string]interface{}{
					"type":        "string",
					"description": "Telescope identifier (partial match)",
				},
				"camera": map[string]interface{}{
					"type":        "string",
					"description": "Camera identifier (partial match)",
				},
				"software": map[string]interface{}{
					"type":        "string",
					"description": "Acquisition software (partial match)",
				},
				"date_from": map[string]interface{}{
					"type":        "string",
					"description": "Start date in ISO8601 format (YYYY-MM-DD or YYYY-MM-DD HH:MM:SS)",
				},
				"date_to": map[string]interface{}{
					"type":        "string",
					"description": "End date in ISO8601 format (YYYY-MM-DD or YYYY-MM-DD HH:MM:SS)",
				},
				"min_exposure": map[string]interface{}{
					"type":        "number",
					"description": "Minimum exposure time in seconds",
				},
				"max_exposure": map[string]interface{}{
					"type":        "number",
					"description": "Maximum exposure time in seconds",
				},
				"gain_min": map[string]interface{}{
					"type":        "number",
					"description": "Minimum gain value",
				},
				"gain_max": map[string]interface{}{
					"type":        "number",
					"description": "Maximum gain value",
				},
				"limit": map[string]interface{}{
					"type":        "number",
					"description": "Maximum number of results to return (default: 100, max: 1000)",
					"default":     100,
				},
				"offset": map[string]interface{}{
					"type":        "number",
					"description": "Number of results to skip for pagination (default: 0)",
					"default":     0,
				},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "query_fits_archive").Interface("params", args).Msg("Tool called")

		limit := 100
		offset := 0
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
			if limit > 1000 {
				limit = 1000
			}
		}
		if o, ok := args["offset"].(float64); ok {
			offset = int(o)
		}

		files, err := db.QueryFiles(args, limit, offset)
		if err != nil {
			log.Error().Err(err).Str("tool", "query_fits_archive").Msg("Tool failed")
			return nil, nil, fmt.Errorf("query failed: %w", err)
		}

		// Get count from the result (files can be a slice of any type)
		var count int
		if filesSlice, ok := files.([]interface{}); ok {
			count = len(filesSlice)
		} else {
			// Try to get length via reflection as fallback
			if v := fmt.Sprintf("%v", files); v != "" {
				count = 0 // Default if we can't determine
			}
		}

		response := map[string]interface{}{
			"files":  files,
			"count":  count,
			"limit":  limit,
			"offset": offset,
		}

		log.Trace().Str("tool", "query_fits_archive").Interface("response", response).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Found %d files", count)},
			},
		}, response, nil
	})
}
