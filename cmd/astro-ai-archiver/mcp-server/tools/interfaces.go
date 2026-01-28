package tools

import (
	"time"
)

// Database interface defines methods needed by tools
type Database interface {
	QueryFiles(filters map[string]interface{}, limit, offset int) (interface{}, error)
	GetFileByPath(path string) (interface{}, error)
	GetArchiveSummary() (*ArchiveSummary, error)
	ExecuteReadOnlyQuery(query string) ([]map[string]interface{}, error)
	GetFilePath() string
	NewScanner(directories []string, recursive, force bool) interface{}
}

// Config interface defines methods needed by tools
type Config interface {
	GetScanDirectories() []string
	GetScanRecursive() bool
	GetScanOnStartup() bool
	GetDatabasePath() string
	GetLoggingLevel() string
	GetLoggingFormat() string
}

// ArchiveSummary holds summary statistics
type ArchiveSummary struct {
	TotalFiles       int       `json:"total_files"`
	TotalExposure    float64   `json:"total_exposure_seconds"`
	OldestDate       time.Time `json:"oldest_observation"`
	NewestDate       time.Time `json:"newest_observation"`
	UniqueTargets    []string  `json:"unique_targets"`
	UniqueFilters    []string  `json:"unique_filters"`
	UniqueTelescopes []string  `json:"unique_telescopes"`
	UniqueCameras    []string  `json:"unique_cameras"`
}

// ScanResult holds scan operation results
type ScanResult struct {
	FilesAdded   int
	FilesUpdated int
	FilesDeleted int
	Errors       []error
}

// ScanState holds the current scanning state
type ScanState struct {
	Scanning     bool
	LastScanTime time.Time
}
