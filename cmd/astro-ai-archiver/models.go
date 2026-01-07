package main

import (
	"database/sql"
	"time"
)

// FITSFile represents a FITS file record in the database
type FITSFile struct {
	ID           int64          `db:"id"`
	RelativePath string         `db:"relative_path"`
	Hash         string         `db:"hash"`
	FileModTime  int64          `db:"file_mod_time"`
	RowModTime   int64          `db:"row_mod_time"`
	Object       string         `db:"object"`
	RA           *float64       `db:"ra"`
	Dec          *float64       `db:"dec"`
	Telescope    string         `db:"telescope"`
	FocalLength  *float64       `db:"focal_length"`
	Exposure     float64        `db:"exposure"`
	UTCTime      sql.NullString `db:"utc_time"`
	LocalTime    sql.NullString `db:"local_time"`
	JulianDate   *float64       `db:"julian_date"`
	Software     string         `db:"software"`
	Camera       string         `db:"camera"`
	Gain         *float64       `db:"gain"`
	Offset       *int           `db:"offset"`
	Filter       string         `db:"filter"`
	ImageType    string         `db:"image_type"`
}

// Config represents the application configuration
type Config struct {
	Scan struct {
		Directory string `mapstructure:"directory"`
		Recursive bool   `mapstructure:"recursive"`
		OnStartup bool   `mapstructure:"on_startup"`
	} `mapstructure:"scan"`
	Database struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"database"`
	Logging struct {
		Level  string `mapstructure:"level"`
		Format string `mapstructure:"format"`
	} `mapstructure:"logging"`
}

// ScanResult represents the result of a directory scan
type ScanResult struct {
	FilesScanned int
	FilesAdded   int
	FilesUpdated int
	FilesSkipped int
	FilesDeleted int
	Errors       []string
	Duration     time.Duration
}

// ArchiveSummary provides statistics about the archive
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
