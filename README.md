# Astro AI Archiver

A Model Context Protocol (MCP) server for scanning, indexing, and querying FITS astronomical image files.

## Features

- 🔭 Scans directories for FITS files recursively
- 📊 Extracts metadata from FITS headers
- 💾 Stores metadata in local SQLite database
- 🤖 Exposes MCP server for AI assistants (Claude Desktop)
- 🔒 All data stays local on your computer
- 🚀 Cross-platform: Windows, Linux, macOS (x64 & ARM64)

## Quick Start

### 1. Build

```bash
make deps    # Download dependencies
make build   # Build for current platform
```

Or build for all platforms:

```bash
make build-all
```

### 2. Configure

Copy the example config:

```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml`:

```yaml
scan:
  directory: "C:\\path\\to\\your\\fits\\files"
  recursive: true
  on_startup: true

database:
  path: ""  # Leave empty for default location

logging:
  level: "info"
  format: "console"
```

### 3. Run

```bash
./bin/astro-ai-archiver mcp-server --config config.yaml
```

## MCP Tools

The server exposes these tools for AI assistants:

### Query Tools

- **`query_fits_archive`** - Search with flexible filters (target, filter, telescope, dates, etc.)
- **`get_file_details`** - Get complete metadata for a specific file
- **`get_archive_summary`** - Statistics and lists of unique targets, filters, telescopes, cameras

### Maintenance Tools

- **`rescan_fits_directory`** - Trigger a rescan to find new/updated files
- **`get_scan_status`** - Check current scan progress
- **`get_configuration`** - Show current configuration

## Integrating with Claude Desktop

Add to your Claude Desktop MCP configuration:

**Windows**: `%APPDATA%\Claude\claude_desktop_config.json`  
**macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`  
**Linux**: `~/.config/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "astro-ai-archiver": {
      "command": "C:\\path\\to\\astro-ai-archiver.exe",
      "args": ["mcp-server", "--config", "C:\\path\\to\\config.yaml"]
    }
  }
}
```

Restart Claude Desktop, and you can now ask:

- "What targets did I shoot last month?"
- "Show me all Ha filter images of M31"
- "What's in my archive?"

## FITS Headers Supported

The scanner extracts these common FITS headers:

- Object/Target name
- RA/DEC coordinates
- Telescope
- Focal length
- Exposure time
- Date/time (UTC, local, Julian)
- Software/platform
- Camera/instrument
- Gain, offset
- Filter

See `requirements.md` for detailed header mapping.

## Development

### Structure

```
cmd/astro-ai-archiver/
├── main.go          # Entry point, Cobra CLI
├── mcp-server.go    # MCP server & tools
├── scanner.go       # FITS file scanning
├── database.go      # SQLite operations
├── models.go        # Data structures
└── logging.go       # Logging setup
```

### Testing

```bash
make test
```

### Building for specific platforms

```bash
make windows        # Windows x64
make windows-arm    # Windows ARM64
make linux          # Linux x64
make linux-arm      # Linux ARM64
make darwin         # macOS x64
make darwin-arm     # macOS ARM64 (Apple Silicon)
```

## License

See LICENSE file.

## Requirements

- Go 1.25.5 or later
- FITS files with standard headers

## Contributing

Contributions welcome! Please see requirements.md for technical details 
