package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

func RegisterGetDatabaseSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_database_schema",
		Description: "Get the complete database schema including table structure, columns, types, and example queries",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "get_database_schema").Msg("Tool called")

		schema := map[string]interface{}{
			"table": "fits_files",
			"columns": []map[string]interface{}{
				{"name": "id", "type": "INTEGER", "description": "Unique file identifier"},
				{"name": "relative_path", "type": "TEXT", "description": "Path relative to scan directory"},
				{"name": "hash", "type": "TEXT", "description": "SHA256 hash of file content"},
				{"name": "file_mod_time", "type": "INTEGER", "description": "File modification time (Unix timestamp)"},
				{"name": "row_mod_time", "type": "INTEGER", "description": "Database row modification time (Unix timestamp)"},
				{"name": "object", "type": "TEXT", "description": "Target object name (e.g., 'NGC2903', 'M31')"},
				{"name": "ra", "type": "REAL", "description": "Right Ascension in degrees"},
				{"name": "dec", "type": "REAL", "description": "Declination in degrees"},
				{"name": "telescope", "type": "TEXT", "description": "Telescope name/model"},
				{"name": "focal_length", "type": "REAL", "description": "Focal length in mm"},
				{"name": "exposure", "type": "REAL", "description": "Exposure time in seconds"},
				{"name": "utc_time", "type": "TEXT", "description": "UTC timestamp (ISO8601)"},
				{"name": "local_time", "type": "TEXT", "description": "Local timestamp (ISO8601)"},
				{"name": "julian_date", "type": "REAL", "description": "Julian date (MJD-OBS)"},
				{"name": "observation_date", "type": "TEXT", "description": "Observation date (YYYY-MM-DD)"},
				{"name": "software", "type": "TEXT", "description": "Capture software"},
				{"name": "camera", "type": "TEXT", "description": "Camera name/model"},
				{"name": "gain", "type": "REAL", "description": "Camera gain setting"},
				{"name": "offset", "type": "INTEGER", "description": "Camera offset setting"},
				{"name": "filter", "type": "TEXT", "description": "Filter name (e.g., 'L', 'Ha', 'OIII')"},
				{"name": "image_type", "type": "TEXT", "description": "Image type (typically 'LIGHT')"},
			},
			"indexes": []string{"object", "filter", "telescope", "utc_time", "hash", "relative_path"},
		}

		log.Trace().Str("tool", "get_database_schema").Interface("response", schema).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Database schema retrieved"},
			},
		}, schema, nil
	})
}
