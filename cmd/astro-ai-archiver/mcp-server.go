package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/yourusername/astro-ai-archiver/cmd/astro-ai-archiver/tools"
)

var (
	mcpServer *mcp.Server
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
			tools.SetLastScanTime(time.Now())
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
	tools.RegisterAll(mcpServer, db, cfg, expandedDirs, cfg.Scan.Recursive)

	// Start server with configured transport
	log.Info().
		Str("transport", getTransportType(cfg)).
		Str("details", getTransportDetails(cfg)).
		Msg("MCP server ready")

	if err := startMCPServer(mcpServer, cfg); err != nil {
		log.Fatal().Err(err).Msg("Server error")
	}
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
