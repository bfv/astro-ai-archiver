package prompts

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

// Database interface for prompts (same as tools)
type Database interface {
	GetFilePath() string
}

// RegisterAll registers all MCP prompts
func RegisterAll(s *mcp.Server, db Database) {
	RegisterYTD(s, db)

	log.Info().Int("prompts", 1).Msg("MCP prompts registered")
}
