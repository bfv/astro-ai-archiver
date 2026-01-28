package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
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

	// Start server with configured transport
	log.Info().
		Str("transport", getTransportType(cfg)).
		Str("details", getTransportDetails(cfg)).
		Msg("MCP server ready")

	if err := startMCPServer(mcpServer, cfg); err != nil {
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

	// get_database_schema
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_database_schema",
		Description: "Get the complete database schema including table structure, columns, types, and example queries",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "get_database_schema").Msg("Tool called")

		schema := map[string]interface{}{
			"table": "fits_files",
			"columns": []map[string]interface{}{
				{"name": "id", "type": "INTEGER", "description": "Unique file identifier"},
				{"name": "relative_path", "type": "TEXT", "description": "Path relative to scan directory"},
				{"name": "hash", "type": "TEXT", "description": "SHA256 hash of file content"},
				{"name": "file_mod_time", "type": "INTEGER", "description": "File modification time (Unix timestamp)"},
				{"name": "row_mod_time", "type": "INTEGER", "description": "Database row modification time (Unix timestamp)"},
				{"name": "object", "type": "TEXT", "description": "Target object name (e.g., 'NGC2903', 'M31')"},
				{"name": "ra", "type": "REAL", "description": "Right Ascension in degrees"},
				{"name": "dec", "type": "REAL", "description": "Declination in degrees"},
				{"name": "telescope", "type": "TEXT", "description": "Telescope name/model"},
				{"name": "focal_length", "type": "REAL", "description": "Focal length in mm"},
				{"name": "exposure", "type": "REAL", "description": "Exposure time in seconds"},
				{"name": "utc_time", "type": "TEXT", "description": "UTC timestamp (ISO8601)"},
				{"name": "local_time", "type": "TEXT", "description": "Local timestamp (ISO8601)"},
				{"name": "julian_date", "type": "REAL", "description": "Julian date (MJD-OBS)"},
				{"name": "observation_date", "type": "TEXT", "description": "Observation date (YYYY-MM-DD)"},
				{"name": "software", "type": "TEXT", "description": "Capture software"},
				{"name": "camera", "type": "TEXT", "description": "Camera name/model"},
				{"name": "gain", "type": "REAL", "description": "Camera gain setting"},
				{"name": "offset", "type": "INTEGER", "description": "Camera offset setting"},
				{"name": "filter", "type": "TEXT", "description": "Filter name (e.g., 'L', 'Ha', 'OIII')"},
				{"name": "image_type", "type": "TEXT", "description": "Image type (typically 'LIGHT')"},
			},
			"indexes": []string{"object", "filter", "telescope", "utc_time", "hash", "relative_path"},
		}

		log.Trace().Str("tool", "get_database_schema").Interface("response", schema).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Database schema retrieved"},
			},
		}, schema, nil
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

// startMCPServer starts the MCP server with the configured transport
func startMCPServer(server *mcp.Server, cfg *Config) error {
	transportType := getTransportType(cfg)

	switch transportType {
	case "stdio":
		return server.Run(context.Background(), &mcp.StdioTransport{})

	case "http":
		return startHTTPServer(server, cfg)

	default:
		return fmt.Errorf("unsupported transport type: %s (supported: stdio, http)", transportType)
	}
}

// startHTTPServer starts the HTTP server using StreamableHTTPHandler
func startHTTPServer(server *mcp.Server, cfg *Config) error {
	// Get HTTP configuration
	host := cfg.Transport.HTTP.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Transport.HTTP.Port
	if port == 0 {
		port = 8080
	}

	// Create HTTP handler
	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server
	}, nil)

	// Create HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", host, port),
		Handler: handler,
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		log.Info().Str("addr", httpServer.Addr).Msg("Starting HTTP server")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("HTTP server error")
			cancel()
		}
	}()

	// Wait for shutdown signal
	select {
	case <-sigChan:
		log.Info().Msg("Shutdown signal received")
	case <-ctx.Done():
		log.Info().Msg("Context cancelled")
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	log.Info().Msg("Shutting down HTTP server...")
	return httpServer.Shutdown(shutdownCtx)
}

// getTransportType returns the transport type with stdio as default
func getTransportType(cfg *Config) string {
	if cfg.Transport.Type == "" {
		return "stdio"
	}
	return cfg.Transport.Type
}

// getTransportDetails returns a string with transport details for logging
func getTransportDetails(cfg *Config) string {
	transportType := getTransportType(cfg)

	switch transportType {
	case "stdio":
		return "stdio transport"
	case "http":
		host := cfg.Transport.HTTP.Host
		if host == "" {
			host = "localhost"
		}
		port := cfg.Transport.HTTP.Port
		if port == 0 {
			port = 8080
		}
		return fmt.Sprintf("http transport on %s:%d", host, port)
	default:
		return "unknown transport"
	}
}
