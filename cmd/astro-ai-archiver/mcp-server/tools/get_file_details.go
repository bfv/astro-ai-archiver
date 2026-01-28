package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

func RegisterGetFileDetails(s *mcp.Server, db Database) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_file_details",
		Description: "Get complete metadata for a specific FITS file by path",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file_path": map[string]interface{}{
					"type":        "string",
					"description": "Full path to the FITS file",
				},
			},
			"required": []string{"file_path"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "get_file_details").Interface("params", args).Msg("Tool called")

		filePath, ok := args["file_path"].(string)
		if !ok {
			log.Error().Str("tool", "get_file_details").Msg("Tool failed: file_path required")
			return nil, nil, fmt.Errorf("file_path is required")
		}

		file, err := db.GetFileByPath(filePath)
		if err != nil {
			log.Error().Err(err).Str("tool", "get_file_details").Msg("Tool failed")
			return nil, nil, fmt.Errorf("failed to get file: %w", err)
		}
		if file == nil {
			log.Error().Str("tool", "get_file_details").Str("path", filePath).Msg("Tool failed: file not found")
			return nil, nil, fmt.Errorf("file not found: %s", filePath)
		}

		response := map[string]interface{}{
			"file": file,
			"path": filePath,
		}

		log.Trace().Str("tool", "get_file_details").Interface("response", response).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "File details retrieved"},
			},
		}, response, nil
	})
}
