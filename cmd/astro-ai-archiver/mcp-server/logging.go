package mcpserver

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func InitLogging(level, format string) {
	// Configure output format
	if format == "json" {
		// JSON output for production
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	} else {
		// Human-friendly console output without colors
		output := zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
			NoColor:    true,
		}
		log.Logger = zerolog.New(output).With().Timestamp().Logger()
	}

	// Set log level
	setLogLevel(level)
}

func setLogLevel(levelStr string) {
	switch strings.ToLower(levelStr) {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn", "warning":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case "trace":
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	default:
		log.Warn().Str("level", levelStr).Msg("Unknown log level, using info")
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

func setLogLevelFromCmd(cmd *cobra.Command) {
	levelStr, _ := cmd.Flags().GetString("log-level")
	setLogLevel(levelStr)
}
