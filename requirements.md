# Astro AI Archiver 
The Astro AI Archiver is called AAA from now on.

## Goal
The goal of this project is to build an MCP server which discloses a database containing all the data on .fits files found in a given directory (and beneath). All data which is stored in the database comes from either the FITS headers or maybe, in the future, from additional meta data sources. The aim for the MCP server is to provide the means to search database for what targets are shot when and all sort of combinations. AI assistants like Claude Desktop should be able to query the MCP server without the need to expose your to anything outside your own computer. The aim is to create an executable which can run independantly and a Docker container doing the same thing.

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
  - Scanning is recursive (includes subdirectories)
  - Scanning happens on startup in a separate goroutine (non-blocking)
  - Scanning can be triggered via MCP tool called `rescan_fits_directory`
- Meta data we encounter in the .fits file is stored in SQLite database
- Database supports complex queries with many-to-many relationships:
  - Files ↔ Targets
  - Files ↔ Filters  
  - Files ↔ Telescopes
  - Files ↔ [other astronomical entities]
- MCP server provides tools for querying:
  - Search by target name
  - Search by date range
  - Search by filter type
  - Search by telescope
  - Complex combinations of above
- All data remains local - no external network calls except MCP stdio communication
- Handle incremental scans (detect new/modified/deleted files since last scan)

## Configuration
- Configuration file: `config.yaml` (via viper)
- Required configuration:
  - `scan.directory`: Path to FITS files directory
  - `scan.recursive`: Enable/disable recursive scanning (default: true)
  - `scan.on_startup`: Enable/disable startup scan (default: true)
  - `database.path`: Path to SQLite database file (default: ${scan.directory/.aaa-db})
  - `logging.level`: Log level (debug, info, warn, error, debug, trace)
  - `logging.format`: Log format (json, console)

## FITS header to store
There are a few platform which may store info in different FITS headers. The plaform are:
- N.I.N.A.
- ASI Air
- Seestar (S30/S50)
- Dwarf 3

If there are no values in the columns right of NINA, the values are identical.
If a column contains `--` it means the particular system does not register a value.

| db attr | NINA | Dwarf | Seestar | ASI Air |
|---------|------|-------|---------|---------|
| object | OBJECT | | | |
| ra | RA | | | |
| dec | DEC | | | |
| telescope | TELESCOP |  |  |  |
| focal_length | FOCALLEN |  |  |  |
| exposure | EXPOSURE | EXPTIME |  |  |
| utc_time | DATE-OBS |  |  |  |
| local_time | DATE-LOC | -- |  |  |
| julian_date | MJD-OBS | -- | -- | -- |

to be continued...
