package main

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	_ "modernc.org/sqlite"
)

const schemaVersion = 1

const schema = `
CREATE TABLE IF NOT EXISTS fits_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    relative_path TEXT NOT NULL UNIQUE,
    hash TEXT,
    file_mod_time INTEGER,
    row_mod_time INTEGER DEFAULT (strftime('%s', 'now')),
    object TEXT NOT NULL,
    ra REAL,
    dec REAL,
    telescope TEXT NOT NULL,
    focal_length REAL,
    exposure REAL NOT NULL,
    utc_time TEXT,
    local_time TEXT,
    julian_date REAL,
    observation_date TEXT,
    software TEXT,
    camera TEXT,
    gain REAL,
    offset INTEGER,
    filter TEXT NOT NULL,
    image_type TEXT
);

CREATE INDEX IF NOT EXISTS idx_object ON fits_files(object);
CREATE INDEX IF NOT EXISTS idx_filter ON fits_files(filter);
CREATE INDEX IF NOT EXISTS idx_telescope ON fits_files(telescope);
CREATE INDEX IF NOT EXISTS idx_utc_time ON fits_files(utc_time);
CREATE INDEX IF NOT EXISTS idx_hash ON fits_files(hash);
CREATE INDEX IF NOT EXISTS idx_relative_path ON fits_files(relative_path);

CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER NOT NULL,
    applied_at INTEGER DEFAULT (strftime('%s', 'now'))
);
`

// Database handles all database operations
type Database struct {
	db       *sql.DB
	baseDirs []string // Changed to support multiple base directories
	filePath string
	writeMu  sync.Mutex
}

// NewDatabase creates a new database connection and initializes schema
func NewDatabase(dbPath string, scanDirs []string) (*Database, error) {
	// If dbPath is empty, use default location in first scan directory
	if dbPath == "" {
		if len(scanDirs) == 0 {
			return nil, fmt.Errorf("no scan directories configured")
		}
		dbPath = filepath.Join(scanDirs[0], ".aaa", "archive.db")
	}

	log.Info().Str("path", dbPath).Msg("Opening database")

	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure SQLite for better performance and concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		return nil, fmt.Errorf("failed to set synchronous mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}
	// Set busy timeout to 10 seconds for concurrent writes
	if _, err := db.Exec("PRAGMA busy_timeout=10000"); err != nil {
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	database := &Database{
		db:       db,
		baseDirs: scanDirs,
		filePath: dbPath,
	}

	// Initialize schema
	if err := database.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return database, nil
}

// initSchema creates the database schema if it doesn't exist
func (d *Database) initSchema() error {
	log.Debug().Msg("Initializing database schema")

	// Execute schema
	if _, err := d.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Check and insert schema version
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check schema version: %w", err)
	}

	if count == 0 {
		_, err := d.db.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion)
		if err != nil {
			return fmt.Errorf("failed to insert schema version: %w", err)
		}
		log.Info().Int("version", schemaVersion).Msg("Database schema created")
	} else {
		var version int
		err := d.db.QueryRow("SELECT version FROM schema_version ORDER BY applied_at DESC LIMIT 1").Scan(&version)
		if err != nil {
			return fmt.Errorf("failed to read schema version: %w", err)
		}
		log.Debug().Int("version", version).Msg("Database schema version")
	}

	return nil
}

// validateReadOnlyQuery ensures the query is safe (SELECT only)
func validateReadOnlyQuery(query string) error {
	// Trim whitespace and convert to uppercase for checking
	normalized := strings.TrimSpace(strings.ToUpper(query))

	// Must start with SELECT
	if !strings.HasPrefix(normalized, "SELECT") {
		return fmt.Errorf("only SELECT queries are allowed")
	}

	// Forbidden keywords that could modify data
	forbidden := []string{
		"INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER",
		"TRUNCATE", "REPLACE", "ATTACH", "DETACH", "PRAGMA",
	}

	for _, keyword := range forbidden {
		if strings.Contains(normalized, keyword) {
			return fmt.Errorf("forbidden keyword in query: %s", keyword)
		}
	}

	return nil
}

// Close closes the database connection
func (d *Database) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// InsertOrUpdateFile inserts or updates a FITS file record
func (d *Database) InsertOrUpdateFile(file *FITSFile) error {
	// Serialize writes to prevent database locks
	d.writeMu.Lock()
	defer d.writeMu.Unlock()

	query := `
		INSERT INTO fits_files (
			relative_path, hash, file_mod_time, object, ra, dec, telescope,
			focal_length, exposure, utc_time, local_time, julian_date, observation_date,
			software, camera, gain, offset, filter, image_type
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(relative_path) DO UPDATE SET
			hash = excluded.hash,
			file_mod_time = excluded.file_mod_time,
			row_mod_time = strftime('%s', 'now'),
			object = excluded.object,
			ra = excluded.ra,
			dec = excluded.dec,
			telescope = excluded.telescope,
			focal_length = excluded.focal_length,
			exposure = excluded.exposure,
			utc_time = excluded.utc_time,
			local_time = excluded.local_time,
			julian_date = excluded.julian_date,
			observation_date = excluded.observation_date,
			software = excluded.software,
			camera = excluded.camera,
			gain = excluded.gain,
			offset = excluded.offset,
			filter = excluded.filter,
			image_type = excluded.image_type
	`

	_, err := d.db.Exec(query,
		file.RelativePath, file.Hash, file.FileModTime, file.Object,
		file.RA, file.Dec, file.Telescope, file.FocalLength, file.Exposure,
		file.UTCTime, file.LocalTime, file.JulianDate, file.ObservationDate, file.Software,
		file.Camera, file.Gain, file.Offset, file.Filter, file.ImageType,
	)

	return err
}

// GetFileByPath retrieves a file by its relative path
func (d *Database) GetFileByPath(relativePath string) (*FITSFile, error) {
	query := `
		SELECT id, relative_path, hash, file_mod_time, row_mod_time,
			   object, ra, dec, telescope, focal_length, exposure,
			   utc_time, local_time, julian_date, observation_date, software, camera,
			   gain, offset, filter, image_type
		FROM fits_files
		WHERE relative_path = ?
	`

	file := &FITSFile{}
	err := d.db.QueryRow(query, relativePath).Scan(
		&file.ID, &file.RelativePath, &file.Hash, &file.FileModTime,
		&file.RowModTime, &file.Object, &file.RA, &file.Dec, &file.Telescope,
		&file.FocalLength, &file.Exposure, &file.UTCTime, &file.LocalTime,
		&file.JulianDate, &file.ObservationDate, &file.Software, &file.Camera, &file.Gain,
		&file.Offset, &file.Filter, &file.ImageType,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return file, nil
}

// QueryFiles performs a flexible query on FITS files
func (d *Database) QueryFiles(filters map[string]interface{}, limit, offset int) ([]*FITSFile, error) {
	query := `
		SELECT id, relative_path, hash, file_mod_time, row_mod_time,
			   object, ra, dec, telescope, focal_length, exposure,
			   utc_time, local_time, julian_date, observation_date, software, camera,
			   gain, offset, filter, image_type
		FROM fits_files
		WHERE 1=1
	`
	args := []interface{}{}

	// Build dynamic WHERE clause
	if target, ok := filters["target"].(string); ok && target != "" {
		query += " AND object LIKE ?"
		args = append(args, "%"+target+"%")
	}
	if filter, ok := filters["filter"].(string); ok && filter != "" {
		query += " AND filter = ?"
		args = append(args, filter)
	}
	if telescope, ok := filters["telescope"].(string); ok && telescope != "" {
		query += " AND telescope LIKE ?"
		args = append(args, "%"+telescope+"%")
	}
	if camera, ok := filters["camera"].(string); ok && camera != "" {
		query += " AND camera LIKE ?"
		args = append(args, "%"+camera+"%")
	}
	if software, ok := filters["software"].(string); ok && software != "" {
		query += " AND software LIKE ?"
		args = append(args, "%"+software+"%")
	}
	if dateFrom, ok := filters["date_from"].(string); ok && dateFrom != "" {
		query += " AND utc_time >= ?"
		args = append(args, dateFrom)
	}
	if dateTo, ok := filters["date_to"].(string); ok && dateTo != "" {
		query += " AND utc_time <= ?"
		args = append(args, dateTo)
	}
	if minExp, ok := filters["min_exposure"].(float64); ok {
		query += " AND exposure >= ?"
		args = append(args, minExp)
	}
	if maxExp, ok := filters["max_exposure"].(float64); ok {
		query += " AND exposure <= ?"
		args = append(args, maxExp)
	}
	if minGain, ok := filters["gain_min"].(float64); ok {
		query += " AND gain >= ?"
		args = append(args, minGain)
	}
	if maxGain, ok := filters["gain_max"].(float64); ok {
		query += " AND gain <= ?"
		args = append(args, maxGain)
	}

	query += " ORDER BY utc_time DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	log.Debug().
		Str("query", query).
		Interface("args", args).
		Msg("Executing query")

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := []*FITSFile{}
	for rows.Next() {
		file := &FITSFile{}
		err := rows.Scan(
			&file.ID, &file.RelativePath, &file.Hash, &file.FileModTime,
			&file.RowModTime, &file.Object, &file.RA, &file.Dec, &file.Telescope,
			&file.FocalLength, &file.Exposure, &file.UTCTime, &file.LocalTime,
			&file.JulianDate, &file.ObservationDate, &file.Software, &file.Camera, &file.Gain,
			&file.Offset, &file.Filter, &file.ImageType,
		)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

// GetArchiveSummary returns statistics about the archive
func (d *Database) GetArchiveSummary() (*ArchiveSummary, error) {
	summary := &ArchiveSummary{}

	// Total files and exposure
	err := d.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(exposure), 0)
		FROM fits_files
	`).Scan(&summary.TotalFiles, &summary.TotalExposure)
	if err != nil {
		return nil, err
	}

	// Date range
	var minDate, maxDate sql.NullString
	err = d.db.QueryRow(`
		SELECT MIN(utc_time), MAX(utc_time)
		FROM fits_files
		WHERE utc_time IS NOT NULL AND utc_time != ''
	`).Scan(&minDate, &maxDate)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// Convert to time.Time if valid
	if minDate.Valid {
		if t, err := time.Parse(time.RFC3339, minDate.String); err == nil {
			summary.OldestDate = t
		}
	}
	if maxDate.Valid {
		if t, err := time.Parse(time.RFC3339, maxDate.String); err == nil {
			summary.NewestDate = t
		}
	}

	// Unique targets
	rows, err := d.db.Query("SELECT DISTINCT object FROM fits_files WHERE object != '' ORDER BY object")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var target string
		if err := rows.Scan(&target); err != nil {
			return nil, err
		}
		summary.UniqueTargets = append(summary.UniqueTargets, target)
	}

	// Unique filters
	rows, err = d.db.Query("SELECT DISTINCT filter FROM fits_files WHERE filter != '' ORDER BY filter")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var filter string
		if err := rows.Scan(&filter); err != nil {
			return nil, err
		}
		summary.UniqueFilters = append(summary.UniqueFilters, filter)
	}

	// Unique telescopes
	rows, err = d.db.Query("SELECT DISTINCT telescope FROM fits_files WHERE telescope != '' ORDER BY telescope")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var telescope string
		if err := rows.Scan(&telescope); err != nil {
			return nil, err
		}
		summary.UniqueTelescopes = append(summary.UniqueTelescopes, telescope)
	}

	// Unique cameras
	rows, err = d.db.Query("SELECT DISTINCT camera FROM fits_files WHERE camera != '' ORDER BY camera")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var camera string
		if err := rows.Scan(&camera); err != nil {
			return nil, err
		}
		summary.UniqueCameras = append(summary.UniqueCameras, camera)
	}

	return summary, nil
}

// DeleteFile removes a file record by relative path
func (d *Database) DeleteFile(relativePath string) error {
	_, err := d.db.Exec("DELETE FROM fits_files WHERE relative_path = ?", relativePath)
	return err
}

// CalculateFileHash computes SHA256 hash of a file
func CalculateFileHash(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// GetRelativePath converts an absolute path to relative path from the appropriate base directory
func (d *Database) GetRelativePath(absPath string) (string, error) {
	// Ensure we're working with clean absolute path
	cleanAbsPath, err := filepath.Abs(absPath)
	if err != nil {
		cleanAbsPath = filepath.Clean(absPath)
	}

	// Find which base directory this file belongs to
	for _, baseDir := range d.baseDirs {
		// Also ensure base directory is clean and absolute
		cleanBaseDir, err := filepath.Abs(baseDir)
		if err != nil {
			cleanBaseDir = filepath.Clean(baseDir)
		}

		rel, err := filepath.Rel(cleanBaseDir, cleanAbsPath)
		if err == nil && !strings.HasPrefix(rel, "..") {
			// This base directory is a parent of the file
			// Convert to forward slashes for consistency across platforms
			return filepath.ToSlash(rel), nil
		}
	}

	// If no base directory matches, log debug info and return error
	log.Debug().
		Str("file", cleanAbsPath).
		Strs("base_dirs", d.baseDirs).
		Msg("File not found in any base directory")
	return "", fmt.Errorf("file %s is not within any configured base directory", cleanAbsPath)
}

// GetAbsolutePath converts a relative path to absolute path
// Searches through all base directories to find where the file exists
func (d *Database) GetAbsolutePath(relPath string) string {
	// Convert from forward slashes to OS-specific
	osPath := filepath.FromSlash(relPath)

	// Try each base directory until we find where the file exists
	for _, baseDir := range d.baseDirs {
		absPath := filepath.Join(baseDir, osPath)
		if _, err := os.Stat(absPath); err == nil {
			return absPath
		}
	}

	// If not found, return path with first base directory as fallback
	if len(d.baseDirs) > 0 {
		return filepath.Join(d.baseDirs[0], osPath)
	}
	return osPath
}

// ExecuteReadOnlyQuery executes a custom SQL query with strict validation
// Only SELECT queries are allowed, no modifications permitted
func (d *Database) ExecuteReadOnlyQuery(query string) ([]map[string]interface{}, error) {
	// Validate query is read-only
	if err := validateReadOnlyQuery(query); err != nil {
		return nil, err
	}

	log.Debug().
		Str("query", query).
		Msg("Executing custom SQL query")

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Prepare result slice
	result := []map[string]interface{}{}

	// Scan all rows
	for rows.Next() {
		// Create a slice of interface{} to hold the values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Create map for this row
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]

			// Convert byte slices to strings for better JSON serialization
			if b, ok := val.([]byte); ok {
				rowMap[col] = string(b)
			} else {
				rowMap[col] = val
			}
		}

		result = append(result, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	log.Debug().
		Int("rows", len(result)).
		Int("columns", len(columns)).
		Msg("Query executed successfully")

	return result, nil
}
