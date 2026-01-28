package main

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
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
