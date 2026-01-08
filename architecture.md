# Astro AI Archiver - Architecture

## System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Claude Desktop / AI Client               │
│                         (MCP Host Application)                  │
└────────────────────────────┬────────────────────────────────────┘
                             │ stdio
                             │ (MCP Protocol)
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Astro AI Archiver (AAA)                     │
│                         MCP Server                              │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                    MCP Interface Layer                    │  │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐ │  │
│  │  │  Resources   │  │    Tools     │  │    Commands      │ │  │
│  │  │  (metadata)  │  │  (queries)   │  │    (scan)        │ │  │
│  │  └──────┬───────┘  └───────┬──────┘  └────────┬─────────┘ │  │
│  └─────────┼──────────────────┼──────────────────┼───────────┘  │
│            │                  │                  │              │
│  ┌─────────▼──────────────────▼──────────────────▼───────────┐  │
│  │                  Business Logic Layer                     │  │
│  │  ┌──────────────┐  ┌──────────────┐  ┌─────────────────┐  │  │
│  │  │   Scanner    │  │   Database   │  │    Logging      │  │  │
│  │  │   (FITS)     │  │   Handler    │  │                 │  │  │
│  │  └──────┬───────┘  └───────┬──────┘  └─────────────────┘  │  │
│  └─────────┼──────────────────┼──────────────────────────────┘  │
│            │                  │                                 │
└────────────┼──────────────────┼─────────────────────────────────┘
             │                  │
             ▼                  ▼
    ┌─────────────────┐  ┌─────────────────┐
    │  FITS Files     │  │  SQLite DB      │
    │  (Filesystem)   │  │  (Local)        │
    │                 │  │  - fits_files   │
    │  *.fits         │  │  - scan_history │
    │  *.fit          │  │                 │
    └─────────────────┘  └─────────────────┘
```

## Component Details

### 1. MCP Server Interface
- **Protocol**: Model Context Protocol over stdio
- **Transport**: Standard input/output
- **Purpose**: Enables AI assistants to interact with FITS archive

### 2. MCP Resources
Expose metadata about the archive:
- `fits://archive/stats` - Statistics about indexed files
- `fits://archive/summary` - Summary of observations

### 3. MCP Tools
Provide query capabilities:
- `query_fits_files` - Search and filter FITS files
- `get_fits_metadata` - Retrieve detailed metadata
- `scan_directory` - Trigger new scan
- `list_observations` - List unique observations

### 4. Scanner Component
- Recursively scans directories for FITS files
- Extracts metadata from FITS headers
- Detects new, modified, and deleted files
- Can run on startup or on-demand

### 5. Database Handler
- SQLite database for local storage
- Tables:
  - `fits_files`: File metadata and headers
  - `scan_history`: Scan operation logs
- Provides query interface for tools

### 6. Configuration
```yaml
scan:
  directory: "<path-to-fits-files>"
  recursive: true
  on_startup: true

database:
  path: ""  # Default: ~/.astro-ai-archiver/archive.db

logging:
  level: "info"
  format: "console"
```

