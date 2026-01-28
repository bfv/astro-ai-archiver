package tools

import (
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

var (
	scanMutex sync.Mutex
	scanState = &ScanState{}
)

// RegisterAll registers all MCP tools
func RegisterAll(s *mcp.Server, db Database, cfg Config, scanDirs []string, recursive bool, version string) {
	RegisterQueryFitsArchive(s, db)
	RegisterGetFileDetails(s, db)
	RegisterGetArchiveSummary(s, db)
	RegisterRescanFitsDirectory(s, db, scanDirs, recursive)
	RegisterGetScanStatus(s)
	RegisterGetConfiguration(s, cfg, db.GetFilePath(), version)
	RegisterExecuteSqlQuery(s, db)
	RegisterGetDatabaseSchema(s)

	log.Info().Int("tools", 8).Msg("MCP tools registered")
}

// GetScanState returns the current scan state (for external access)
func GetScanState() *ScanState {
	scanMutex.Lock()
	defer scanMutex.Unlock()
	return &ScanState{
		Scanning:     scanState.Scanning,
		LastScanTime: scanState.LastScanTime,
	}
}

// SetLastScanTime sets the last scan time (for initialization)
func SetLastScanTime(t time.Time) {
	scanMutex.Lock()
	defer scanMutex.Unlock()
	scanState.LastScanTime = t
}
