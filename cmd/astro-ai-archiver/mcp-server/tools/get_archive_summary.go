package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

func RegisterGetArchiveSummary(s *mcp.Server, db Database) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_archive_summary",
		Description: "Get summary statistics and lists of unique targets, filters, telescopes, cameras",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "get_archive_summary").Msg("Tool called")

		summary, err := db.GetArchiveSummary()
		if err != nil {
			log.Error().Err(err).Str("tool", "get_archive_summary").Msg("Tool failed")
			return nil, nil, fmt.Errorf("failed to get summary: %w", err)
		}

		response := map[string]interface{}{
			"summary": summary,
		}

		log.Trace().Str("tool", "get_archive_summary").Interface("response", response).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Archive contains %d files", summary.TotalFiles)},
			},
		}, response, nil
	})
}
