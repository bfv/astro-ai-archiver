package mcpserver

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/astrogo/fitsio"
	"github.com/rs/zerolog/log"
)

// Scanner handles scanning directories for FITS files
type Scanner struct {
	db        *Database
	baseDirs  []string // Changed from single baseDir to multiple baseDirs
	recursive bool
	force     bool
	workers   int

	// Statistics
	scanned  atomic.Int64
	added    atomic.Int64
	updated  atomic.Int64
	skipped  atomic.Int64
	errors   []string
	errorsMu sync.Mutex

	// Limit for testing
	limit     int64
	processed atomic.Int64

	// Batch processing
	batchSize int
	batch     []*FITSFile
	batchMu   sync.Mutex
}

// NewScanner creates a new scanner instance
// Note: directories should already be expanded (wildcards resolved) and absolute paths
func NewScanner(db *Database, directories []string, recursive, force bool, workers int) *Scanner {
	// Default to runtime.NumCPU() if not specified or invalid
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	log.Info().
		Strs("base_directories", directories).
		Int("workers", workers).
		Msg("Scanner initialized with directories")

	return &Scanner{
		db:        db,
		baseDirs:  directories,
		recursive: recursive,
		force:     force,
		workers:   workers,
		errors:    make([]string, 0),
		limit:     -1, // Limit to 25 files for testing
		batchSize: 50, // Process in batches of 50 for better performance
		batch:     make([]*FITSFile, 0, 50),
	}
}

// Scan performs the directory scan across all configured directories
func (s *Scanner) Scan() (*ScanResult, error) {
	startTime := time.Now()

	log.Info().
		Strs("directories", s.baseDirs).
		Bool("recursive", s.recursive).
		Bool("force", s.force).
		Msg("Starting FITS file scan")

	if len(s.baseDirs) == 0 {
		return nil, fmt.Errorf("no directories configured to scan")
	}

	// Walk all directories
	for _, baseDir := range s.baseDirs {
		// Check if directory exists
		if _, err := os.Stat(baseDir); os.IsNotExist(err) {
			log.Warn().Str("directory", baseDir).Msg("Directory does not exist, skipping")
			s.addError(fmt.Sprintf("directory does not exist: %s", baseDir))
			continue
		}

		log.Info().Str("directory", baseDir).Msg("Scanning directory")
		err := s.walkDirectory(baseDir)
		if err != nil {
			log.Error().Err(err).Str("directory", baseDir).Msg("Failed to scan directory")
			s.addError(fmt.Sprintf("failed to scan %s: %v", baseDir, err))
		}
	}

	duration := time.Since(startTime)

	result := &ScanResult{
		FilesScanned: int(s.scanned.Load()),
		FilesAdded:   int(s.added.Load()),
		FilesUpdated: int(s.updated.Load()),
		FilesSkipped: int(s.skipped.Load()),
		Errors:       s.errors,
		Duration:     duration,
	}

	log.Info().
		Int("scanned", result.FilesScanned).
		Int("added", result.FilesAdded).
		Int("updated", result.FilesUpdated).
		Int("skipped", result.FilesSkipped).
		Int("errors", len(result.Errors)).
		Dur("duration", duration).
		Msg("Scan completed")

	return result, nil
}

// walkDirectory recursively walks the directory tree
func (s *Scanner) walkDirectory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		s.addError(fmt.Sprintf("failed to read directory %s: %v", dir, err))
		return nil // Continue scanning other directories
	}

	// Process files in parallel using worker pool
	fileChan := make(chan string, 100)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < s.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range fileChan {
				s.processFile(filePath)
			}
		}()
	}

	// Queue files
	for _, entry := range entries {
		// Check if we've reached the limit
		if s.limit > 0 && s.processed.Load() >= s.limit {
			log.Info().Int64("limit", s.limit).Msg("Reached file limit, stopping scan")
			break
		}

		fullPath := filepath.Join(dir, entry.Name())

		if entry.IsDir() {
			if s.recursive && entry.Name() != "." && entry.Name() != ".." {
				s.walkDirectory(fullPath)
			}
		} else if s.isFITSFile(entry.Name()) {
			s.processed.Add(1)
			fileChan <- fullPath
		}
	}

	close(fileChan)
	wg.Wait()

	// Flush any remaining batched files
	if err := s.flushBatch(); err != nil {
		s.addError(fmt.Sprintf("failed to flush batch: %v", err))
	}

	return nil
}

// isFITSFile checks if a file has a FITS extension
func (s *Scanner) isFITSFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return fitsExtensions[ext]
}

// processFile processes a single FITS file
func (s *Scanner) processFile(filePath string) {
	s.scanned.Add(1)

	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		s.addError(fmt.Sprintf("failed to stat %s: %v", filePath, err))
		return
	}

	// Get relative path
	relPath, err := s.db.GetRelativePath(filePath)
	if err != nil {
		s.addError(fmt.Sprintf("failed to get relative path for %s: %v", filePath, err))
		return
	}

	// Check if file already exists in database
	if !s.force {
		existing, err := s.db.GetFileByPath(relPath)
		if err != nil {
			s.addError(fmt.Sprintf("database error for %s: %v", relPath, err))
			return
		}

		// Skip if file hasn't been modified (need type assertion since GetFileByPath returns interface{})
		if existing != nil {
			if existingFile, ok := existing.(*FITSFile); ok && existingFile.FileModTime == fileInfo.ModTime().Unix() {
				s.skipped.Add(1)
				log.Debug().Str("file", relPath).Msg("Skipped (unchanged)")
				return
			}
		}
	}

	// Extract FITS metadata
	fitsFile, err := s.extractMetadata(filePath, relPath, fileInfo)
	if err != nil {
		s.addError(fmt.Sprintf("failed to extract metadata from %s: %v", relPath, err))
		return
	}

	// Skip non-LIGHT frames (calibration files)
	if fitsFile.ImageType != "LIGHT" {
		s.skipped.Add(1)
		log.Debug().Str("file", relPath).Str("type", fitsFile.ImageType).Msg("Skipped (not a light frame)")
		return
	}

	// Check required fields
	if fitsFile.Object == "" {
		s.addError(fmt.Sprintf("missing required field OBJECT in %s", relPath))
		s.skipped.Add(1)
		return
	}
	if fitsFile.Telescope == "" {
		s.addError(fmt.Sprintf("missing required field TELESCOPE in %s", relPath))
		s.skipped.Add(1)
		return
	}
	if fitsFile.Filter == "" {
		s.addError(fmt.Sprintf("missing required field FILTER in %s", relPath))
		s.skipped.Add(1)
		return
	}
	if fitsFile.Exposure == 0 {
		s.addError(fmt.Sprintf("missing required field EXPOSURE in %s", relPath))
		s.skipped.Add(1)
		return
	}

	// Add to batch instead of immediate insert
	if err := s.addToBatch(fitsFile); err != nil {
		s.addError(fmt.Sprintf("failed to batch %s: %v", relPath, err))
		return
	}

	// Track statistics (simplified - assume new for now)
	s.added.Add(1)
	log.Debug().Str("file", relPath).Msg("Batched for insert")
}

// extractMetadata reads FITS file and extracts metadata
func (s *Scanner) extractMetadata(filePath, relPath string, fileInfo os.FileInfo) (*FITSFile, error) {
	// Open file first
	osFile, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer osFile.Close()

	// Open FITS file
	f, err := fitsio.Open(osFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open FITS file: %w", err)
	}
	defer f.Close()

	// Get primary HDU
	hdu := f.HDU(0)
	header := hdu.Header()

	// Calculate file hash
	hash, err := CalculateFileHash(filePath)
	if err != nil {
		log.Debug().Str("file", relPath).Err(err).Msg("Failed to calculate hash")
		hash = ""
	}

	file := &FITSFile{
		RelativePath: relPath,
		Hash:         hash,
		FileModTime:  fileInfo.ModTime().Unix(),
	}

	// Extract metadata using header keywords
	// Try multiple common variations for each field

	// Object/Target
	file.Object = s.normalizeTarget(s.getStringHeader(header, "OBJECT", "TARGET", "OBJNAME"))

	// RA/DEC
	file.RA = s.getFloatHeader(header, "RA", "OBJCTRA", "RA_OBJ")
	file.Dec = s.getFloatHeader(header, "DEC", "OBJCTDEC", "DEC_OBJ")

	// Telescope
	file.Telescope = s.getStringHeader(header, "TELESCOP", "TELESCOPE", "SCOPE")

	// Focal length
	file.FocalLength = s.getFloatHeader(header, "FOCALLEN", "FOCAL", "FOCAL_LENGTH")

	// Exposure
	if exp := s.getFloatHeader(header, "EXPTIME", "EXPOSURE", "EXPOS"); exp != nil {
		file.Exposure = *exp
	}

	// Date/Time
	if dateStr := s.getStringHeader(header, "DATE-OBS", "DATE_OBS", "DATEOBS"); dateStr != "" {
		if t, ok := parseDateTimeString(dateStr); ok {
			file.UTCTime = sql.NullString{String: t.UTC().Format("2006-01-02T15:04:05"), Valid: true}
		}
	}

	// Local time
	if localStr := s.getStringHeader(header, "DATE-LOC", "DATE_LOCAL", "LOCTIME"); localStr != "" {
		if t, ok := parseDateTimeString(localStr); ok {
			file.LocalTime = sql.NullString{String: t.Format("2006-01-02T15:04:05"), Valid: true}
		}
	}

	// Julian date (stored as MJD-OBS)
	file.JulianDate = s.getFloatHeader(header, "MJD-OBS", "JD", "JULIAN", "JD-OBS")

	// Observation date: astrophotography convention is UTC - 12h so that images taken
	// after midnight are attributed to the evening session they belong to.
	if file.JulianDate != nil {
		// Convert MJD to time.Time (MJD epoch: 1858-11-17 00:00:00 UTC)
		mjdEpoch := time.Date(1858, 11, 17, 0, 0, 0, 0, time.UTC)
		obsTime := mjdEpoch.Add(time.Duration(float64(time.Hour) * 24 * (*file.JulianDate)))
		dateStr := observationDateFromTime(obsTime)
		file.ObservationDate = sql.NullString{String: dateStr, Valid: true}
		log.Debug().Float64("julian_date", *file.JulianDate).Str("observation_date", dateStr).Msg("Calculated observation date from MJD")
	} else if file.UTCTime.Valid {
		if t, ok := parseDateTimeString(file.UTCTime.String); ok {
			dateStr := observationDateFromTime(t)
			file.ObservationDate = sql.NullString{String: dateStr, Valid: true}
			log.Debug().Str("observation_date", dateStr).Msg("Calculated observation date from UTC time")
		}
	} else {
		// Last resort: re-read DATE-OBS header directly
		if rawDate := s.getStringHeader(header, "DATE-OBS", "DATE_OBS", "DATEOBS"); rawDate != "" {
			if t, ok := parseDateTimeString(rawDate); ok {
				dateStr := observationDateFromTime(t)
				file.ObservationDate = sql.NullString{String: dateStr, Valid: true}
				log.Debug().Str("observation_date", dateStr).Msg("Calculated observation date from DATE-OBS header")
			} else {
				log.Debug().Str("date_obs", rawDate).Msg("Could not parse DATE-OBS in any known format")
			}
		} else {
			log.Debug().Msg("No date information found, observation_date will be null")
		}
	}

	// Software/Platform
	file.Software = s.getStringHeader(header, "SWCREATE", "SOFTWARE", "PROGRAM", "CREATOR")

	// Camera/Instrument
	file.Camera = s.getStringHeader(header, "INSTRUME", "CAMERA", "DETECTOR")

	// Gain
	file.Gain = s.getFloatHeader(header, "GAIN", "EGAIN")

	// Offset
	if offset := s.getIntHeader(header, "OFFSET", "PEDESTAL"); offset != nil {
		file.Offset = offset
	}

	// Filter
	file.Filter = s.getStringHeader(header, "FILTER", "FILT", "FILTNAME")

	// Image Type - only process LIGHT frames
	file.ImageType = s.getStringHeader(header, "IMAGETYP", "IMAGTYP", "FRAME")
	if file.ImageType == "" {
		file.ImageType = "LIGHT" // Default to LIGHT if not specified
	}

	return file, nil
}

// getStringHeader tries multiple header keywords and returns first found value
func (s *Scanner) getStringHeader(header *fitsio.Header, keys ...string) string {
	for _, key := range keys {
		if card := header.Get(key); card != nil {
			if val, ok := card.Value.(string); ok && val != "" {
				return strings.TrimSpace(val)
			}
		}
	}
	return ""
}

// getFloatHeader tries multiple header keywords and returns first found value
func (s *Scanner) getFloatHeader(header *fitsio.Header, keys ...string) *float64 {
	for _, key := range keys {
		if card := header.Get(key); card != nil {
			switch v := card.Value.(type) {
			case float64:
				return &v
			case float32:
				f := float64(v)
				return &f
			case int:
				f := float64(v)
				return &f
			case int64:
				f := float64(v)
				return &f
			}
		}
	}
	return nil
}

// getIntHeader tries multiple header keywords and returns first found value
func (s *Scanner) getIntHeader(header *fitsio.Header, keys ...string) *int {
	for _, key := range keys {
		if card := header.Get(key); card != nil {
			switch v := card.Value.(type) {
			case int:
				return &v
			case int64:
				i := int(v)
				return &i
			case float64:
				i := int(v)
				return &i
			}
		}
	}
	return nil
}

// parseDateTimeString parses a datetime string in formats used by FITS capture software
// (N.I.N.A., ASI Air, Seestar, Dwarf 3, etc.). For inputs without an explicit timezone,
// UTC is assumed. Returns the parsed time and true on success.
func parseDateTimeString(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	// Try formats with explicit timezone first.
	// RFC3339Nano covers both with and without fractional seconds when tz is present.
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	// Formats without timezone — treat as UTC.
	// "2006-01-02T15:04:05.999999999" matches any number of fractional digits (0–9),
	// so it also matches the plain "2006-01-02T15:04:05" case.
	noTZLayouts := []string{
		"2006-01-02T15:04:05.999999999", // T-separator, optional fractional seconds (N.I.N.A., most software)
		"2006-01-02 15:04:05",           // space separator
		"2006-01-02",                    // date only
	}
	for _, layout := range noTZLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC), true
		}
	}
	return time.Time{}, false
}

// observationDateFromTime returns the astrophotography observation date for t.
// The convention is UTC - 12h: images taken after midnight belong to the previous
// calendar day (i.e. the evening session that started it).
func observationDateFromTime(t time.Time) string {
	return t.UTC().Add(-12 * time.Hour).Format("2006-01-02")
}

// addError adds an error message to the error list (thread-safe)
func (s *Scanner) addError(msg string) {
	s.errorsMu.Lock()
	defer s.errorsMu.Unlock()
	s.errors = append(s.errors, msg)
	log.Error().Msg(msg)
}

// addToBatch adds a file to the batch and flushes if needed
func (s *Scanner) addToBatch(file *FITSFile) error {
	s.batchMu.Lock()
	defer s.batchMu.Unlock()

	s.batch = append(s.batch, file)

	if len(s.batch) >= s.batchSize {
		return s.flushBatchLocked()
	}
	return nil
}

// flushBatchLocked flushes the current batch (must be called with batchMu held)
func (s *Scanner) flushBatchLocked() error {
	if len(s.batch) == 0 {
		return nil
	}

	// Process batch
	for _, file := range s.batch {
		if err := s.db.InsertOrUpdateFile(file); err != nil {
			return err
		}
	}

	// Clear batch
	s.batch = s.batch[:0]
	return nil
}

// flushBatch flushes any remaining files in the batch
func (s *Scanner) flushBatch() error {
	s.batchMu.Lock()
	defer s.batchMu.Unlock()
	return s.flushBatchLocked()
}

// normalizeTarget normalizes astronomical target names
// - Uppercases catalog prefixes (M, NGC, IC, SH2, etc.)
// - Removes space between catalog prefix and number (M 31 -> M31, NGC 7822 -> NGC7822)
// - Special case: SH2 uses dash (SH2 159 -> SH2-159)
// - Replaces remaining spaces with underscores
func (s *Scanner) normalizeTarget(target string) string {
	if target == "" {
		return ""
	}

	// Handle catalog prefixes with regex (case-insensitive)
	target = catalogPrefixRegex.ReplaceAllStringFunc(target, func(match string) string {
		parts := strings.Fields(match)
		if len(parts) == 2 {
			prefix := strings.ToUpper(parts[0])
			if prefix == "SH2" {
				return prefix + "-" + parts[1]
			}
			return prefix + parts[1]
		}
		return match
	})

	// Replace remaining spaces with underscores (single pass)
	target = multiSpaceRegex.ReplaceAllString(target, "_")

	return target
}

// Pre-compile regex patterns and create lookup maps
var (
	catalogPrefixRegex = regexp.MustCompile(`(?i)\b(M|NGC|IC|SH2)\s+(\d+)`)
	multiSpaceRegex    = regexp.MustCompile(`\s+`)
	fitsExtensions     = map[string]bool{
		".fits": true,
		".fit":  true,
		".fts":  true,
	}
)
