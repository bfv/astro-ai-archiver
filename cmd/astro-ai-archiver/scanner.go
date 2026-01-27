package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
}

// NewScanner creates a new scanner instance
// Note: directories should already be expanded (wildcards resolved) and absolute paths
func NewScanner(db *Database, directories []string, recursive, force bool) *Scanner {
	log.Info().Strs("base_directories", directories).Msg("Scanner initialized with directories")

	return &Scanner{
		db:        db,
		baseDirs:  directories,
		recursive: recursive,
		force:     force,
		errors:    make([]string, 0),
		limit:     -1, // Limit to 25 files for testing
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
	const numWorkers = 4
	fileChan := make(chan string, 100)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < numWorkers; i++ {
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

	return nil
}

// isFITSFile checks if a file has a FITS extension
func (s *Scanner) isFITSFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".fits" || ext == ".fit" || ext == ".fts"
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

		// Skip if file hasn't been modified
		if existing != nil && existing.FileModTime == fileInfo.ModTime().Unix() {
			s.skipped.Add(1)
			log.Debug().Str("file", relPath).Msg("Skipped (unchanged)")
			return
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

	// Insert or update in database
	err = s.db.InsertOrUpdateFile(fitsFile)
	if err != nil {
		s.addError(fmt.Sprintf("failed to save %s to database: %v", relPath, err))
		return
	}

	// Determine if this was an add or update
	existing, _ := s.db.GetFileByPath(relPath)
	if existing != nil && existing.FileModTime != fileInfo.ModTime().Unix() {
		s.updated.Add(1)
		log.Debug().Str("file", relPath).Msg("Updated")
	} else {
		s.added.Add(1)
		log.Debug().Str("file", relPath).Msg("Added")
	}
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
		log.Warn().Str("file", relPath).Err(err).Msg("Failed to calculate hash")
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
	file.Object = s.getStringHeader(header, "OBJECT", "TARGET", "OBJNAME")

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
		if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
			file.UTCTime = sql.NullString{String: t.Format(time.RFC3339), Valid: true}
		} else if t, err := time.Parse("2006-01-02T15:04:05", dateStr); err == nil {
			file.UTCTime = sql.NullString{String: t.Format(time.RFC3339), Valid: true}
		}
	}

	// Local time
	if localStr := s.getStringHeader(header, "DATE-LOC", "DATE_LOCAL", "LOCTIME"); localStr != "" {
		if t, err := time.Parse(time.RFC3339, localStr); err == nil {
			file.LocalTime = sql.NullString{String: t.Format(time.RFC3339), Valid: true}
		}
	}

	// Julian date
	file.JulianDate = s.getFloatHeader(header, "MJD-OBS", "JD", "JULIAN", "JD-OBS")

	// Calculate observation date from Julian date
	if file.JulianDate != nil {
		log.Debug().Float64("julian_date", *file.JulianDate).Msg("Found Julian date, converting to observation date")

		// Convert Julian date to Gregorian date
		// Julian Date Number is days since January 1, 4713 BCE in proleptic Julian calendar
		// Modified Julian Date (MJD) = JD - 2400000.5
		// We assume the julian_date field contains Modified Julian Date (MJD) as that's common in FITS
		jd := *file.JulianDate + 2400000.5

		// Convert Julian Day Number to Gregorian date
		// Algorithm from Meeus, "Astronomical Algorithms"
		a := int(jd + 0.5)
		var b int
		if a >= 2299161 {
			alpha := int((float64(a) - 1867216.25) / 36524.25)
			b = a + 1 + alpha - alpha/4
		} else {
			b = a
		}
		c := b + 1524
		d := int((float64(c) - 122.1) / 365.25)
		e := int(365.25 * float64(d))
		f := int((float64(c) - float64(e)) / 30.6001)

		day := c - e - int(30.6001*float64(f))
		var month int
		if f <= 13 {
			month = f - 1
		} else {
			month = f - 13
		}
		var year int
		if month > 2 {
			year = d - 4716
		} else {
			year = d - 4715
		}

		// Format as YYYY-MM-DD
		dateStr := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
		file.ObservationDate = sql.NullString{String: dateStr, Valid: true}
		log.Debug().Str("observation_date", dateStr).Msg("Calculated observation date")
	} else {
		log.Debug().Msg("No Julian date found, trying to derive observation_date from utc_time")
		// If no Julian date, try to derive observation date from UTC time
		if file.UTCTime.Valid {
			// Parse UTC time and extract just the date part
			if t, err := time.Parse(time.RFC3339, file.UTCTime.String); err == nil {
				dateStr := t.Format("2006-01-02")
				file.ObservationDate = sql.NullString{String: dateStr, Valid: true}
				log.Debug().Str("observation_date", dateStr).Msg("Calculated observation date from UTC time")
			} else {
				log.Debug().Err(err).Str("utc_time", file.UTCTime.String).Msg("Failed to parse UTC time for observation date")
			}
		} else {
			log.Debug().Msg("No UTC time available either, trying to parse DATE-OBS directly")
			// Try to get date directly from headers in various formats
			if dateStr := s.getStringHeader(header, "DATE-OBS", "DATE_OBS", "DATEOBS"); dateStr != "" {
				// Try various date formats
				formats := []string{
					"2006-01-02T15:04:05",
					"2006-01-02T15:04:05.000",
					"2006-01-02T15:04:05Z",
					"2006-01-02T15:04:05.000Z",
					"2006-01-02 15:04:05",
					"2006-01-02",
				}

				for _, format := range formats {
					if t, err := time.Parse(format, dateStr); err == nil {
						obsDate := t.Format("2006-01-02")
						file.ObservationDate = sql.NullString{String: obsDate, Valid: true}
						log.Debug().Str("observation_date", obsDate).Str("format", format).Msg("Calculated observation date from DATE-OBS")
						break
					}
				}

				if !file.ObservationDate.Valid {
					log.Debug().Str("date_obs", dateStr).Msg("Could not parse DATE-OBS in any known format")
				}
			} else {
				log.Debug().Msg("No date information found, observation_date will be null")
			}
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

// addError adds an error message to the error list (thread-safe)
func (s *Scanner) addError(msg string) {
	s.errorsMu.Lock()
	defer s.errorsMu.Unlock()
	s.errors = append(s.errors, msg)
	log.Error().Msg(msg)
}
