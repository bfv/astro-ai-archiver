package main

import (
	"github.com/spf13/cobra"

	"github.com/yourusername/astro-ai-archiver/cmd/astro-ai-archiver/mcp-server"
)

var mcpServerCmd = &cobra.Command{
	Use:   "mcp-server",
	Short: "Start the MCP server",
	Long: `Start the Model Context Protocol server that exposes FITS archive data
to AI assistants like Claude Desktop via stdio transport.`,
	Run: mcpserver.RunMCPServer,
}

func init() {
	mcpServerCmd.Flags().StringP("config", "c", "config.yaml", "Path to configuration file")
	mcpServerCmd.Flags().Bool("force-scan", false, "Force scan all files on startup, ignoring modification times")
}
