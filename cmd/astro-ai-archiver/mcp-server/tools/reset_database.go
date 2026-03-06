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
		Description: "Delete all records from the FITS archive database. The database schema is preserved. A rescan is required afterwards to repopulate the archive.",
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"properties":           map[string]interface{}{},
			"additionalProperties": false,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "reset_database").Msg("Tool called")

		n, err := db.DeleteAllFiles()
		if err != nil {
			log.Error().Err(err).Msg("reset_database failed")
			return nil, nil, fmt.Errorf("reset_database: %w", err)
		}

		result := map[string]interface{}{
			"status":          "ok",
			"records_deleted": n,
			"message":         fmt.Sprintf("Deleted %d record(s). Use rescan_fits_directory to repopulate.", n),
		}

		log.Trace().Str("tool", "reset_database").Interface("response", result).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Database reset: %d record(s) deleted.", n)},
			},
		}, result, nil
	})
}
