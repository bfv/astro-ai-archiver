package main

import (
	"context"
	"fmt"
	"os"
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
}

func runMCPServer(cmd *cobra.Command, args []string) {
	configFile, _ := cmd.Flags().GetString("config")

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

	// Initialize database
	db, err := NewDatabase(cfg.Database.Path, cfg.Scan.Directory)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer db.Close()

	log.Info().Str("database", db.filePath).Msg("Database initialized")

	// Start initial scan in background if configured
	if cfg.Scan.OnStartup {
		go func() {
			log.Info().Msg("Starting initial scan in background")
			scanner := NewScanner(db, cfg.Scan.Directory, cfg.Scan.Recursive, false)
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
		Description: "Search FITS files with flexible filters (target, filter, telescope, date range, etc.)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		log.Info().Str("tool", "query_fits_archive").Interface("params", args).Msg("Tool called")

		limit := 100
		offset := 0
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
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

			scanner := NewScanner(db, cfg.Scan.Directory, cfg.Scan.Recursive, force)
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
			"scan_directory":  cfg.Scan.Directory,
			"recursive":       cfg.Scan.Recursive,
			"scan_on_startup": cfg.Scan.OnStartup,
			"database_path":   db.filePath,
			"log_level":       cfg.Logging.Level,
			"log_format":      cfg.Logging.Format,
		}

		log.Trace().Str("tool", "get_configuration").Interface("response", config).Msg("Tool response")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Config loaded from: %s", cfg.Scan.Directory)},
			},
		}, config, nil
	})

	log.Info().Int("tools", 6).Msg("MCP tools registered")
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
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}
