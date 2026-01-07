package main

import (
	"os"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:   "astro-ai-archiver",
	Short: "MCP server for FITS file archival and querying",
	Long: `Astro AI Archiver (AAA) scans directories for FITS files, extracts metadata,
stores it in a local database, and provides an MCP server interface for AI assistants
to query astronomical observation data.

All data remains local on your computer. The MCP server communicates via stdio
and can be integrated with Claude Desktop for natural language queries.`,
	Version: version,
}

func main() {
	// Set up basic logging for startup
	initLogging("info", "console")

	if err := rootCmd.Execute(); err != nil {
		log.Error().Err(err).Msg("Command failed")
		os.Exit(1)
	}
}

func init() {
	// Add commands
	rootCmd.AddCommand(mcpServerCmd)

	// Global flags (if needed)
	rootCmd.PersistentFlags().StringP("log-level", "l", "info", "Log level (debug, info, warn, error, trace)")
}
