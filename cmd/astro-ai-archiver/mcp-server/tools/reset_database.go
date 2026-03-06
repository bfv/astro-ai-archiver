package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

func RegisterResetDatabase(s *mcp.Server, db Database) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "reset_database",
		Description: "Delete records from the FITS archive database. When 'year' is provided only records for that observation year are removed; otherwise all records are deleted. The schema is preserved. A rescan is required afterwards to repopulate.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"year": map[string]interface{}{
					"type":        "integer",
					"description": "Optional: four-digit observation year to delete (e.g. 2023). When omitted all records are deleted.",
				},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "reset_database").Interface("params", args).Msg("Tool called")

		var n int64
		var err error
		var scope string

		if yearVal, ok := args["year"]; ok && yearVal != nil {
			// JSON numbers arrive as float64
			yearFloat, ok := yearVal.(float64)
			if !ok {
				return nil, nil, fmt.Errorf("year must be an integer")
			}
			year := int(yearFloat)
			if year < 1900 || year > 2100 {
				return nil, nil, fmt.Errorf("year %d is out of range (1900-2100)", year)
			}
			n, err = db.DeleteFilesByYear(year)
			scope = fmt.Sprintf("year %d", year)
		} else {
			n, err = db.DeleteAllFiles()
			scope = "all years"
		}

		if err != nil {
			log.Error().Err(err).Msg("reset_database failed")
			return nil, nil, fmt.Errorf("reset_database: %w", err)
		}

		result := map[string]interface{}{
			"status":          "ok",
			"records_deleted": n,
			"scope":           scope,
			"message":         fmt.Sprintf("Deleted %d record(s) for %s. Use rescan_fits_directory to repopulate.", n, scope),
		}

		log.Trace().Str("tool", "reset_database").Interface("response", result).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Database reset (%s): %d record(s) deleted.", scope, n)},
			},
		}, result, nil
	})
}
