package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

func RegisterGetScanStatus(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_scan_status",
		Description: "Check the current status of any running scan operation",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "get_scan_status").Msg("Tool called")

		scanMutex.Lock()
		defer scanMutex.Unlock()

		status := map[string]interface{}{
			"scanning": scanState.Scanning,
		}

		if !scanState.LastScanTime.IsZero() {
			status["last_scan"] = scanState.LastScanTime.Format(time.RFC3339)
			status["time_since_last_scan"] = time.Since(scanState.LastScanTime).String()
		}

		log.Trace().Str("tool", "get_scan_status").Interface("response", status).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Scanning: %v", scanState.Scanning)},
			},
		}, status, nil
	})
}
