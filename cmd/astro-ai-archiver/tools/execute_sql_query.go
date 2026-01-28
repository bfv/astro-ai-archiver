package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

func RegisterExecuteSqlQuery(s *mcp.Server, db Database) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "execute_sql_query",
		Description: "Execute a custom SQL query on the FITS archive database. Only SELECT queries are allowed. Use this for counting, statistics, aggregations, or complex queries not covered by other tools.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "SQL SELECT query to execute. Only SELECT statements are allowed for safety. Table name is 'fits_files'. Use the database schema resource for column names.",
				},
			},
			"required": []string{"query"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "execute_sql_query").Interface("params", args).Msg("Tool called")

		query, ok := args["query"].(string)
		if !ok {
			log.Error().Str("tool", "execute_sql_query").Msg("Tool failed: query required")
			return nil, nil, fmt.Errorf("query parameter is required")
		}

		results, err := db.ExecuteReadOnlyQuery(query)
		if err != nil {
			log.Error().Err(err).Str("tool", "execute_sql_query").Msg("Tool failed")
			return nil, nil, fmt.Errorf("query failed: %w", err)
		}

		response := map[string]interface{}{
			"results": results,
			"count":   len(results),
			"query":   query,
		}

		log.Trace().Str("tool", "execute_sql_query").Interface("response", response).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Query returned %d rows", len(results))},
			},
		}, response, nil
	})
}
