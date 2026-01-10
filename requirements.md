# Astro AI Archiver 
The Astro AI Archiver is called AAA from now on.

## Goal
The goal of this project is to build an MCP server which discloses a database containing all the data on .fits files found in a given directory (and beneath). All data which is stored in the database comes from either the FITS headers or maybe, in the future, from additional meta data sources. The aim for the MCP server is to provide the means to search database for what targets are shot when and all sort of combinations. AI assistants like Claude Desktop should be able to query the MCP server without the need to expose your to anything outside your own computer. The aim is to create an executable which can run independantly and on a Docker container doing the same thing (later).

## technical stack
- we use the latest Go 1.25 version (1.25.5 at the time of writing)
- as modules we use
  - viper - configuration
    - `go get github.com/spf13/viper`
  - cobra - cli parameters
    - `go get -u github.com/spf13/cobra@latest`
  - zerolog - logging
    - `go get -u github.com/rs/zerolog/log`
  - we use the Model Context Protocol Go SDK for the MCP server
    - `go get github.com/modelcontextprotocol/go-sdk`
  - fitsio for extracting .FITS data
    - `go get github.com/astrogo/fitsio`
  - we use sqlite as an embedded database
    - `go get modernc.org/sqlite`
  - for future database updates: goose
    - `go get github.com/pressly/goose/v3`
- We use `make` for building
  - the Makefile should work under all OS's, Windows, Linux, MacOS
- output is for windows and linux and MacOS (all: either ARM of x64)
- the sources are in `cmd/astro-ai-archiver`, the default structure for Go CLI tools
- main.go drives the command. For now there's just the `mcp-server` command. Put the command in separate `.go` file. 
- data structures in `models.go`
- logging related in `logging.go` 
- split up code in logical files 

## functional requirements
- AAA should expose an MCP server
- AAA should be able to scan a given set of directories for .fits files
  - **Multiple directories supported**: Can scan multiple base directories simultaneously
  - **Wildcard support**: Directory patterns with wildcards (e.g., `C:\astro\_20*`) are expanded to match multiple directories
  - Wildcards are expanded using `filepath.Glob()` at startup and before each scan
  - Scanning is recursive (includes subdirectories)
  - Scanning happens on startup in a separate goroutine (non-blocking)
  - Scanning can be triggered via MCP tool called `rescan_fits_directory`
  - **Only FITS files with IMAGETYP='LIGHT' are processed** (calibration files like DARK, FLAT, BIAS are ignored)
- Meta data we encounter in the .fits file is stored in SQLite database
- Relative paths are calculated based on the appropriate base directory the file belongs to
- Files are correctly identified across all configured base directories
- MCP server provides tools for querying:
  - Search by target name
  - Search by date range
  - Search by filter type
  - Search by telescope
  - Complex combinations of above
- AAA scans for files with the `.fit` or `.fits` extension
- All data remains local - no external network calls except MCP stdio communication
- Handle incremental scans (detect new/modified/deleted files since last scan)
- Errors should not interfere with scanning. Errors can be logged.
- if required headers are missing, log the event skip the .fits
- if something happens which prevents proceeding, log the event and quit.

## Configuration
- Configuration file: `config.yaml` (via viper)
- Required configuration:
  - `scan.directory`: Path(s) to FITS files directory
    - Can be a single string: `"C:\\path\\to\\fits"`
    - Or an array of strings: `["C:\\path1", "C:\\path2"]`
    - **Supports wildcards**: Use glob patterns like `"C:\\astro\\_20*"` to match multiple directories
    - Wildcards are expanded at startup and before each scan
    - Non-matching patterns are logged as warnings
  - `scan.recursive`: Enable/disable recursive scanning (default: true)
  - `scan.on_startup`: Enable/disable startup scan (default: true)
  - `scan.threads`: the amount of Go routines used for scanning the dirs
  - `database.path`: Path to SQLite database file (default: ${scan.directory/.aaa-db})
  - `logging.level`: Log level (debug, info, warn, error, debug, trace)
    - `info`: Logs MCP tool calls (name + parameters) and errors only
    - `trace`: Additionally logs full tool responses
  - `logging.format`: Log format (json, console)

## FITS header to store
There are a few platform which may store info in different FITS headers. The plaform are:
- N.I.N.A.
- ASI Air
- Seestar (S30/S50)
- Dwarf 3

If there are no values in the columns right of NINA, the values are identical.
If a column contains `--` it means the particular system does not register a value. These are columns in the `fits_files` table. 

| db column | NINA | Dwarf | Seestar | ASI Air |
|-----------|------|-------|---------|---------|
| object | OBJECT | | | |
| ra | RA | | | |
| dec | DEC | | | |
| telescope | TELESCOP |  |  |  |
| focal_length | FOCALLEN |  |  |  |
| exposure | EXPOSURE | EXPTIME | tbd | tbd |
| utc_time | DATE-OBS |  |  |  |
| local_time | DATE-LOC | -- | tbd | tbd |
| julian_date | MJD-OBS | -- | -- | -- |
| software | SWCREATE | ORIGIN | tbd | tbd |
| camera | INSTRUME |  |  |  |
| gain | GAIN |  |  |  |
| offset | OFFSET | -- | tbd | tbd |
| filter | FILTER |  |  |  |

For every .fits file these value are recorded. Normalization of the data is a concern for later.
Required values are:
- object
- ra
- dec
- telescope
- focal_length
- exposure
- filter

## Other columns
more column for `fits_files`.
- relative_path
- hash (sha256)
- file_mod_time
- row_mod_time (default: now)
## Performance
For scanning we use goroutines, so the scanning is scalable. Use the `scan.threads` setting for the amount of goroutines.

## MCP tools

### Resources

#### `fits://schema`
MCP Resource that provides the complete database schema including:
- Table structure (fits_files)
- All column definitions with types and descriptions
- Index information
- Example queries
- Query best practices

Claude can read this resource to understand the database structure before constructing queries.

### Query Tools

#### `query_fits_archive`
Flexible search with multiple optional filters (target, filter, telescope, date range, etc.)

#### `get_database_schema`
Get the complete database schema as a tool response. Returns:
- Table structure (fits_files)
- All column definitions with types and descriptions
- Index information
- Example queries for common use cases
- Query best practices

This tool provides the same information as the `fits://schema` resource but as a direct tool call.
Use this before constructing custom SQL queries with `execute_sql_query`.

**Parameters:**
- None

**Returns:**
- Complete schema documentation as structured text
- Includes table definitions, column descriptions, indexes, and example queries

#### `execute_sql_query`
Execute custom SQL queries on the FITS archive database. Provides maximum flexibility for:
- Complex filtering and aggregations
- Custom grouping and sorting
- Statistical queries (COUNT, AVG, SUM, etc.)
- Date/time calculations
- Multi-column queries

**Security:**
- Only SELECT queries are allowed (read-only)
- Forbidden keywords: INSERT, UPDATE, DELETE, DROP, CREATE, ALTER, TRUNCATE, REPLACE, ATTACH, DETACH, PRAGMA
- Query validation happens before execution
- Full query logging at debug level

**Parameters:**
- `query` (string, required): SQL SELECT query to execute

**Returns:**
- Array of row objects (maps with column names as keys)
- Row count
- Executed query (for verification)

**Example queries:**
```sql
-- Files by month
SELECT strftime('%Y-%m', utc_time) as month, COUNT(*) as count 
FROM fits_files 
GROUP BY month 
ORDER BY month DESC

-- Average exposure per filter
SELECT filter, AVG(exposure) as avg_exp, COUNT(*) as count
FROM fits_files 
GROUP BY filter

-- Files from specific date range
SELECT object, filter, exposure, utc_time
FROM fits_files
WHERE utc_time BETWEEN '2025-05-01' AND '2025-06-30'
ORDER BY utc_time
```

#### `get_file_details`
Complete metadata for a specific file by path or ID

#### `get_archive_summary`
Statistics + lists all unique: targets, filters, telescopes, cameras
(Replaces: list_targets, list_filters, list_telescopes, list_cameras, get_statistics)

### Maintenance Tools

#### `rescan_fits_directory`
Trigger rescan (with optional force parameter)

#### `get_scan_status`
Check current scan progress

#### `get_configuration`
Show current AAA configuration (scan directory, database path, etc.)

## Database Schema

### fits_files table
Single flat table containing all FITS file metadata:

**Columns:**
- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `relative_path` TEXT NOT NULL UNIQUE
- `hash` TEXT (SHA256 of file)
- `file_mod_time` INTEGER (Unix timestamp)
- `row_mod_time` INTEGER DEFAULT (strftime('%s', 'now'))
- `object` TEXT (required)
- `ra` REAL (required)
- `dec` REAL (required)
- `telescope` TEXT (required)
- `focal_length` REAL (required)
- `exposure` REAL (required, in seconds)
- `utc_time` TEXT (ISO8601 format)
- `local_time` TEXT (ISO8601 format, nullable)
- `julian_date` REAL (nullable)
- `software` TEXT
- `camera` TEXT
- `gain` REAL (nullable)
- `offset` INTEGER (nullable)
- `filter` TEXT (required)

**Indexes:**
- `idx_object` on `object`
- `idx_filter` on `filter`
- `idx_telescope` on `telescope`
- `idx_utc_time` on `utc_time`
- `idx_hash` on `hash`

Note: Many-to-many junction tables (files ↔ targets, filters, telescopes) can be added in future schema migrations.