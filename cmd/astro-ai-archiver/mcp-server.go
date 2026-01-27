package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	mcpServer    *mcp.Server
	scanMutex    sync.Mutex
	scanningNow  bool
	lastScanTime time.Time
)

var mcpServerCmd = &cobra.Command{
	Use:   "mcp-server",
	Short: "Start the MCP server",
	Long: `Start the Model Context Protocol server that exposes FITS archive data
to AI assistants like Claude Desktop via stdio transport.`,
	Run: runMCPServer,
}

func init() {
	mcpServerCmd.Flags().StringP("config", "c", "config.yaml", "Path to configuration file")
	mcpServerCmd.Flags().Bool("force-scan", false, "Force scan all files on startup, ignoring modification times")
}

func runMCPServer(cmd *cobra.Command, args []string) {
	configFile, _ := cmd.Flags().GetString("config")
	forceScan, _ := cmd.Flags().GetBool("force-scan")

	// Load configuration
	cfg, err := loadConfig(configFile)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Initialize logging from config
	initLogging(cfg.Logging.Level, cfg.Logging.Format)

	log.Info().
		Str("version", "0.1.0").
		Str("config", configFile).
		Msg("Starting Astro AI Archiver MCP Server")

	// Expand wildcards in scan directories
	expandedDirs := expandDirectories(cfg.Scan.Directory)
	if len(expandedDirs) == 0 {
		log.Fatal().Msg("No valid scan directories found after wildcard expansion")
	}
	log.Info().Strs("directories", expandedDirs).Msg("Scan directories configured")

	// Initialize database
	db, err := NewDatabase(cfg.Database.Path, expandedDirs)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer db.Close()

	log.Info().Str("database", db.filePath).Msg("Database initialized")

	// Start initial scan in background if configured
	if cfg.Scan.OnStartup {
		go func() {
			log.Info().Bool("force", forceScan).Msg("Starting initial scan in background")
			scanner := NewScanner(db, expandedDirs, cfg.Scan.Recursive, forceScan)
			result, err := scanner.Scan()
			if err != nil {
				log.Error().Err(err).Msg("Initial scan failed")
			} else {
				log.Info().
					Int("added", result.FilesAdded).
					Int("updated", result.FilesUpdated).
					Int("errors", len(result.Errors)).
					Msg("Initial scan completed")
			}
			lastScanTime = time.Now()
		}()
	}

	// Create MCP server
	mcpServer = mcp.NewServer(&mcp.Implementation{
		Name:    "astro-ai-archiver",
		Version: "0.1.0",
	}, nil)

	// Register resources
	registerResources(mcpServer, db)

	// Register tools
	registerTools(mcpServer, db, cfg)

	// Start server (stdio transport)
	log.Info().Msg("MCP server ready, listening on stdio")
	if err := mcpServer.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal().Err(err).Msg("Server error")
	}
}

// registerTools registers all MCP tools
func registerTools(s *mcp.Server, db *Database, cfg *Config) {
	// query_fits_archive
	mcp.AddTool(s, &mcp.Tool{
		Name:        "query_fits_archive",
		Description: "Search and retrieve individual FITS file records with flexible filters (target, filter, telescope, date range, etc.). Returns paginated file details. For counting, statistics, or aggregations use execute_sql_query instead.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"target": map[string]interface{}{
					"type":        "string",
					"description": "Target object name (partial match, e.g., 'NGC2903', 'M31')",
				},
				"filter": map[string]interface{}{
					"type":        "string",
					"description": "Filter name (exact match, e.g., 'Ha', 'OIII', 'L', 'Red')",
				},
				"telescope": map[string]interface{}{
					"type":        "string",
					"description": "Telescope identifier (partial match)",
				},
				"camera": map[string]interface{}{
					"type":        "string",
					"description": "Camera identifier (partial match)",
				},
				"software": map[string]interface{}{
					"type":        "string",
					"description": "Acquisition software (partial match)",
				},
				"date_from": map[string]interface{}{
					"type":        "string",
					"description": "Start date in ISO8601 format (YYYY-MM-DD or YYYY-MM-DD HH:MM:SS)",
				},
				"date_to": map[string]interface{}{
					"type":        "string",
					"description": "End date in ISO8601 format (YYYY-MM-DD or YYYY-MM-DD HH:MM:SS)",
				},
				"min_exposure": map[string]interface{}{
					"type":        "number",
					"description": "Minimum exposure time in seconds",
				},
				"max_exposure": map[string]interface{}{
					"type":        "number",
					"description": "Maximum exposure time in seconds",
				},
				"gain_min": map[string]interface{}{
					"type":        "number",
					"description": "Minimum gain value",
				},
				"gain_max": map[string]interface{}{
					"type":        "number",
					"description": "Maximum gain value",
				},
				"limit": map[string]interface{}{
					"type":        "number",
					"description": "Maximum number of results to return (default: 100, max: 1000)",
					"default":     100,
				},
				"offset": map[string]interface{}{
					"type":        "number",
					"description": "Number of results to skip for pagination (default: 0)",
					"default":     0,
				},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "query_fits_archive").Interface("params", args).Msg("Tool called")

		limit := 100
		offset := 0
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
			// Cap at 1000 for performance
			if limit > 1000 {
				limit = 1000
			}
		}
		if o, ok := args["offset"].(float64); ok {
			offset = int(o)
		}

		files, err := db.QueryFiles(args, limit, offset)
		if err != nil {
			log.Error().Err(err).Str("tool", "query_fits_archive").Msg("Tool failed")
			return nil, nil, fmt.Errorf("query failed: %w", err)
		}

		response := map[string]interface{}{
			"files":  files,
			"count":  len(files),
			"limit":  limit,
			"offset": offset,
		}

		log.Trace().Str("tool", "query_fits_archive").Interface("response", response).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Found %d files", len(files))},
			},
		}, response, nil
	})

	// get_file_details
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_file_details",
		Description: "Get complete metadata for a specific FITS file by path",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "get_file_details").Interface("params", args).Msg("Tool called")

		filePath, ok := args["file_path"].(string)
		if !ok {
			log.Error().Str("tool", "get_file_details").Msg("Tool failed: file_path required")
			return nil, nil, fmt.Errorf("file_path is required")
		}

		file, err := db.GetFileByPath(filePath)
		if err != nil {
			log.Error().Err(err).Str("tool", "get_file_details").Msg("Tool failed")
			return nil, nil, fmt.Errorf("failed to get file: %w", err)
		}
		if file == nil {
			log.Error().Str("tool", "get_file_details").Str("path", filePath).Msg("Tool failed: file not found")
			return nil, nil, fmt.Errorf("file not found: %s", filePath)
		}

		response := map[string]interface{}{
			"file": file,
			"path": filePath,
		}

		log.Trace().Str("tool", "get_file_details").Interface("response", response).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "File details retrieved"},
			},
		}, response, nil
	})

	// get_archive_summary
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_archive_summary",
		Description: "Get summary statistics and lists of unique targets, filters, telescopes, cameras",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "get_archive_summary").Msg("Tool called")

		summary, err := db.GetArchiveSummary()
		if err != nil {
			log.Error().Err(err).Str("tool", "get_archive_summary").Msg("Tool failed")
			return nil, nil, fmt.Errorf("failed to get summary: %w", err)
		}

		response := map[string]interface{}{
			"summary": summary,
		}

		log.Trace().Str("tool", "get_archive_summary").Interface("response", response).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Archive contains %d files", summary.TotalFiles)},
			},
		}, response, nil
	})

	// rescan_fits_directory
	mcp.AddTool(s, &mcp.Tool{
		Name:        "rescan_fits_directory",
		Description: "Trigger a rescan of the FITS directory to find new/updated/deleted files",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "rescan_fits_directory").Interface("params", args).Msg("Tool called")

		force := false
		if f, ok := args["force"].(bool); ok {
			force = f
		}

		// Check if already scanning
		scanMutex.Lock()
		if scanningNow {
			scanMutex.Unlock()
			result := map[string]interface{}{
				"status":  "already_running",
				"message": "A scan is already in progress",
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Scan already in progress"},
				},
			}, result, nil
		}
		scanningNow = true
		scanMutex.Unlock()

		// Run scan in background
		go func() {
			defer func() {
				scanMutex.Lock()
				scanningNow = false
				lastScanTime = time.Now()
				scanMutex.Unlock()
			}()

			// Expand directories again in case filesystem changed
			currentDirs := expandDirectories(cfg.Scan.Directory)
			scanner := NewScanner(db, currentDirs, cfg.Scan.Recursive, force)
			result, err := scanner.Scan()
			if err != nil {
				log.Error().Err(err).Msg("Scan failed")
			} else {
				log.Info().
					Int("added", result.FilesAdded).
					Int("updated", result.FilesUpdated).
					Msg("Scan completed")
			}
		}()

		result := map[string]interface{}{
			"status":  "started",
			"message": "Scan started in background",
			"force":   force,
		}

		log.Trace().Str("tool", "rescan_fits_directory").Interface("response", result).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Scan started"},
			},
		}, result, nil
	})

	// get_scan_status
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_scan_status",
		Description: "Check the current status of any running scan operation",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "get_scan_status").Msg("Tool called")

		scanMutex.Lock()
		defer scanMutex.Unlock()

		status := map[string]interface{}{
			"scanning": scanningNow,
		}

		if !lastScanTime.IsZero() {
			status["last_scan"] = lastScanTime.Format(time.RFC3339)
			status["time_since_last_scan"] = time.Since(lastScanTime).String()
		}

		log.Trace().Str("tool", "get_scan_status").Interface("response", status).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Scanning: %v", scanningNow)},
			},
		}, status, nil
	})

	// get_configuration
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_configuration",
		Description: "Show current AAA configuration (scan directory, database path, etc.)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "get_configuration").Msg("Tool called")

		config := map[string]interface{}{
			"scan_directories": cfg.Scan.Directory, // Now an array
			"recursive":        cfg.Scan.Recursive,
			"scan_on_startup":  cfg.Scan.OnStartup,
			"database_path":    db.filePath,
			"log_level":        cfg.Logging.Level,
			"log_format":       cfg.Logging.Format,
		}

		log.Trace().Str("tool", "get_configuration").Interface("response", config).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Configuration: %d scan director%s", len(cfg.Scan.Directory), map[bool]string{true: "y", false: "ies"}[len(cfg.Scan.Directory) == 1])},
			},
		}, config, nil
	})

	// execute_sql_query
	mcp.AddTool(s, &mcp.Tool{
		Name:        "execute_sql_query",
		Description: "Execute a custom SQL query on the FITS archive. Only SELECT queries are allowed. Use this for: counting files (COUNT), statistics (AVG, SUM, MIN, MAX), grouping (GROUP BY), aggregations, and any custom filtering. For retrieving individual file records use query_fits_archive.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "execute_sql_query").Interface("params", args).Msg("Tool called")

		sqlQuery, ok := args["query"].(string)
		if !ok || sqlQuery == "" {
			log.Error().Str("tool", "execute_sql_query").Msg("Tool failed: query required")
			return nil, nil, fmt.Errorf("query parameter is required")
		}

		// Validate and execute query
		result, err := db.ExecuteReadOnlyQuery(sqlQuery)
		if err != nil {
			log.Error().Err(err).Str("tool", "execute_sql_query").Str("query", sqlQuery).Msg("Tool failed")
			return nil, nil, fmt.Errorf("query execution failed: %w", err)
		}

		response := map[string]interface{}{
			"rows":      result,
			"row_count": len(result),
			"query":     sqlQuery,
		}

		// Build a text summary of results
		textResult := fmt.Sprintf("Query returned %d row(s)\n\n", len(result))

		// For small result sets (≤10 rows), include the actual data in text form
		if len(result) > 0 && len(result) <= 10 {
			// Get column names from first row
			firstRow := result[0]
			var columns []string
			for col := range firstRow {
				columns = append(columns, col)
			}

			// Add each row
			for i, row := range result {
				textResult += fmt.Sprintf("Row %d:\n", i+1)
				for _, col := range columns {
					textResult += fmt.Sprintf("  %s: %v\n", col, row[col])
				}
				if i < len(result)-1 {
					textResult += "\n"
				}
			}
		} else if len(result) > 10 {
			textResult += fmt.Sprintf("(First 3 rows shown)\n\n")
			for i := 0; i < 3 && i < len(result); i++ {
				textResult += fmt.Sprintf("Row %d: %v\n", i+1, result[i])
			}
			textResult += fmt.Sprintf("\n...and %d more rows", len(result)-3)
		}

		log.Trace().Str("tool", "execute_sql_query").Interface("response", response).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: textResult},
			},
		}, response, nil
	})

	// get_database_schema
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_database_schema",
		Description: "Get the complete database schema including table structure, columns with types and descriptions, indexes, and example queries. Use this before constructing custom SQL queries.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "get_database_schema").Msg("Tool called")

		schema := `DATABASE SCHEMA - FITS Archive
========================================

TABLE: fits_files
-----------------
Primary table containing all FITS file metadata.

COLUMNS:
  id              INTEGER PRIMARY KEY AUTOINCREMENT
  relative_path   TEXT NOT NULL UNIQUE           - Path relative to scan directory
  hash            TEXT                             - SHA256 hash of file content
  file_mod_time   INTEGER                          - File modification time (Unix timestamp)
  row_mod_time    INTEGER DEFAULT (strftime('%s', 'now')) - Database row update time
  
  object          TEXT NOT NULL                    - Target object name (e.g., "NGC2903", "M31")
  ra              REAL NOT NULL                    - Right Ascension in degrees
  dec             REAL NOT NULL                    - Declination in degrees
  telescope       TEXT NOT NULL                    - Telescope identifier
  focal_length    REAL NOT NULL                    - Focal length in mm
  exposure        REAL NOT NULL                    - Exposure time in seconds
  utc_time        TEXT                             - Observation time in UTC (ISO8601 format)
  local_time      TEXT                             - Local observation time (ISO8601 format, nullable)
  julian_date     REAL                             - Julian date (nullable)
  observation_date TEXT                            - Gregorian calendar date (YYYY-MM-DD format, derived from julian_date)
  software        TEXT                             - Acquisition software (e.g., "N.I.N.A", "Dwarf3")
  camera          TEXT                             - Camera identifier
  gain            REAL                             - Camera gain (nullable)
  offset          INTEGER                          - Camera offset (nullable)
  filter          TEXT NOT NULL                    - Filter name (e.g., "Ha", "OIII", "Lum", "Red")
  image_type      TEXT                             - Image type (typically "LIGHT")

INDEXES:
  idx_object      ON object
  idx_filter      ON filter
  idx_telescope   ON telescope
  idx_utc_time    ON utc_time
  idx_hash        ON hash

NOTES:
- All date/time fields use ISO8601 format (YYYY-MM-DD HH:MM:SS)
- Text searches should use LIKE with wildcards: WHERE object LIKE '%NGC%'
- Date filtering: WHERE utc_time BETWEEN '2025-01-01' AND '2025-12-31'
- Use strftime() for date grouping: strftime('%Y-%m', utc_time)

EXAMPLE QUERIES:
----------------

1. Files by target:
   SELECT object, COUNT(*) as count, SUM(exposure) as total_exposure
   FROM fits_files
   GROUP BY object
   ORDER BY count DESC

2. Monthly observation summary:
   SELECT strftime('%Y-%m', utc_time) as month, 
          COUNT(*) as files,
          COUNT(DISTINCT object) as unique_targets,
          SUM(exposure)/3600.0 as hours
   FROM fits_files
   GROUP BY month
   ORDER BY month DESC

3. Filter usage statistics:
   SELECT filter, 
          COUNT(*) as count,
          AVG(exposure) as avg_exposure,
          SUM(exposure)/3600.0 as total_hours
   FROM fits_files
   GROUP BY filter
   ORDER BY count DESC

4. Specific target observations:
   SELECT utc_time, filter, exposure, telescope, camera
   FROM fits_files
   WHERE object LIKE '%M31%'
   ORDER BY utc_time DESC

5. Date range with multiple filters:
   SELECT object, filter, COUNT(*) as subs, SUM(exposure) as total_exp
   FROM fits_files
   WHERE utc_time BETWEEN '2025-05-01' AND '2025-06-30'
     AND filter IN ('Ha', 'OIII', 'SII')
   GROUP BY object, filter
   ORDER BY object, filter
`

		response := map[string]interface{}{
			"schema": schema,
		}

		log.Trace().Str("tool", "get_database_schema").Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Database schema retrieved successfully"},
			},
		}, response, nil
	})

	log.Info().Int("tools", 8).Msg("MCP tools registered")
}

// registerResources registers MCP resources
func registerResources(s *mcp.Server, db *Database) {
	// Database schema resource
	s.AddResource(&mcp.Resource{
		URI:         "fits://schema",
		Name:        "FITS Archive Database Schema",
		Description: "Complete database schema including table structure, column types, and indexes for the FITS archive",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		log.Info().Str("resource", "fits://schema").Msg("Resource requested")

		schema := `# FITS Archive Database Schema

## Table: fits_files
Main table containing all FITS file metadata.

### Columns:
- id (INTEGER PRIMARY KEY) - Unique file identifier
- relative_path (TEXT NOT NULL UNIQUE) - Path relative to scan directory
- hash (TEXT) - SHA256 hash of file content
- file_mod_time (INTEGER) - File modification time (Unix timestamp)
- row_mod_time (INTEGER) - Database row modification time (Unix timestamp)
- object (TEXT NOT NULL) - Target object name (e.g., 'NGC2903', 'M31')
- ra (REAL) - Right Ascension in degrees
- dec (REAL) - Declination in degrees
- telescope (TEXT NOT NULL) - Telescope name/model
- focal_length (REAL) - Focal length in mm
- exposure (REAL NOT NULL) - Exposure time in seconds
- utc_time (TEXT) - UTC timestamp (ISO8601 format: YYYY-MM-DD HH:MM:SS)
- local_time (TEXT) - Local timestamp (ISO8601 format)
- julian_date (REAL) - Julian date (MJD-OBS)
- observation_date (TEXT) - Gregorian calendar date (YYYY-MM-DD format, derived from julian_date)
- software (TEXT) - Capture software (e.g., 'N.I.N.A.', 'Dwarf Lab')
- camera (TEXT) - Camera name/model
- gain (REAL) - Camera gain setting
- offset (INTEGER) - Camera offset setting
- filter (TEXT NOT NULL) - Filter name (e.g., 'L', 'Ha', 'OIII', 'Red')
- image_type (TEXT) - Image type (typically 'LIGHT')

### Indexes:
- idx_object ON object
- idx_filter ON filter
- idx_telescope ON telescope
- idx_utc_time ON utc_time
- idx_hash ON hash
- idx_relative_path ON relative_path

### Notes:
- All dates should be queried in ISO8601 format
- Use LIKE for partial text matching on object, telescope, camera
- Use BETWEEN for date ranges on utc_time
- Only LIGHT frames are stored (no DARK, FLAT, BIAS calibration frames)

### Example Queries:
-- Files by month
SELECT strftime('%Y-%m', utc_time) as month, COUNT(*) as count 
FROM fits_files 
GROUP BY month 
ORDER BY month DESC;

-- Average exposure per filter
SELECT filter, AVG(exposure) as avg_exp, COUNT(*) as count
FROM fits_files 
GROUP BY filter;

-- Specific target with date range
SELECT object, filter, exposure, utc_time, relative_path
FROM fits_files
WHERE object LIKE '%NGC2903%' 
  AND utc_time BETWEEN '2025-05-01' AND '2025-06-30'
ORDER BY utc_time;
`

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      "fits://schema",
					MIMEType: "text/plain",
					Text:     schema,
				},
			},
		}, nil
	})

	log.Info().Int("resources", 1).Msg("MCP resources registered")
}

// loadConfig loads configuration from file using viper
func loadConfig(configFile string) (*Config, error) {
	// Check if config file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", configFile)
	}

	v := viper.New()
	v.SetConfigFile(configFile)

	// Set defaults
	v.SetDefault("scan.recursive", true)
	v.SetDefault("scan.on_startup", true)
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "console")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	// Use custom decode hook to handle DirectoryConfig
	if err := v.Unmarshal(&cfg, viper.DecodeHook(StringOrSliceHookFunc())); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// expandDirectories expands wildcards in directory patterns and returns absolute paths
func expandDirectories(patterns []string) []string {
	expandedDirs := make([]string, 0)
	for _, pattern := range patterns {
		// Clean the path to ensure consistent separators
		cleanPattern := filepath.Clean(pattern)

		matches, err := filepath.Glob(cleanPattern)
		if err != nil {
			log.Warn().Err(err).Str("pattern", cleanPattern).Msg("Invalid glob pattern")
			continue
		}
		if len(matches) == 0 {
			// No matches, use original path if it's an existing directory
			if info, err := os.Stat(cleanPattern); err == nil && info.IsDir() {
				absPath, _ := filepath.Abs(cleanPattern)
				expandedDirs = append(expandedDirs, absPath)
				log.Info().Str("directory", absPath).Msg("Using literal directory path")
			} else {
				log.Warn().Str("pattern", cleanPattern).Msg("No matches for pattern and path does not exist")
			}
		} else {
			// Add all matches that are directories
			for _, match := range matches {
				if info, err := os.Stat(match); err == nil && info.IsDir() {
					// Get absolute path to ensure consistency
					absMatch, err := filepath.Abs(match)
					if err != nil {
						log.Warn().Err(err).Str("path", match).Msg("Failed to get absolute path")
						absMatch = match
					}
					expandedDirs = append(expandedDirs, absMatch)
					log.Info().Str("pattern", cleanPattern).Str("matched", absMatch).Msg("Wildcard expanded")
				}
			}
		}
	}
	return expandedDirs
}
