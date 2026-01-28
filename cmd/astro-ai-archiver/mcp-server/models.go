package mcpserver

import (
	"database/sql"
	"fmt"
	"reflect"
	"time"
)

// DirectoryConfig can be either a single string or an array of strings
type DirectoryConfig []string

// UnmarshalYAML implements custom unmarshaling for directory field
func (d *DirectoryConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try unmarshaling as a string first
	var single string
	if err := unmarshal(&single); err == nil {
		*d = DirectoryConfig{single}
		return nil
	}

	// Try unmarshaling as a slice of strings
	var multiple []string
	if err := unmarshal(&multiple); err == nil {
		*d = DirectoryConfig(multiple)
		return nil
	}

	return fmt.Errorf("directory must be a string or array of strings")
}

// DecodeHook for viper/mapstructure to handle DirectoryConfig
func StringOrSliceHookFunc() func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		// Only process if target is DirectoryConfig
		if t != reflect.TypeOf(DirectoryConfig{}) {
			return data, nil
		}

		// Handle string input
		if f.Kind() == reflect.String {
			return DirectoryConfig{data.(string)}, nil
		}

		// Handle slice input
		if f.Kind() == reflect.Slice {
			v := reflect.ValueOf(data)
			result := make(DirectoryConfig, v.Len())
			for i := 0; i < v.Len(); i++ {
				result[i] = v.Index(i).Interface().(string)
			}
			return result, nil
		}

		return data, nil
	}
}

// FITSFile represents a FITS file record in the database
type FITSFile struct {
	ID              int64          `db:"id"`
	RelativePath    string         `db:"relative_path"`
	Hash            string         `db:"hash"`
	FileModTime     int64          `db:"file_mod_time"`
	RowModTime      int64          `db:"row_mod_time"`
	Object          string         `db:"object"`
	RA              *float64       `db:"ra"`
	Dec             *float64       `db:"dec"`
	Telescope       string         `db:"telescope"`
	FocalLength     *float64       `db:"focal_length"`
	Exposure        float64        `db:"exposure"`
	UTCTime         sql.NullString `db:"utc_time"`
	LocalTime       sql.NullString `db:"local_time"`
	JulianDate      *float64       `db:"julian_date"`
	ObservationDate sql.NullString `db:"observation_date"`
	Software        string         `db:"software"`
	Camera          string         `db:"camera"`
	Gain            *float64       `db:"gain"`
	Offset          *int           `db:"offset"`
	Filter          string         `db:"filter"`
	ImageType       string         `db:"image_type"`
}

// Config represents the application configuration
type Config struct {
	Scan struct {
		Directory DirectoryConfig `yaml:"directory" mapstructure:"directory"`
		Recursive bool            `yaml:"recursive" mapstructure:"recursive"`
		OnStartup bool            `yaml:"on_startup" mapstructure:"on_startup"`
		Workers   int             `yaml:"workers" mapstructure:"workers"`
	} `yaml:"scan" mapstructure:"scan"`
	Database struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"database"`
	Transport struct {
		Type string `yaml:"type" mapstructure:"type"` // "stdio" or "http"
		HTTP struct {
			Host string `yaml:"host" mapstructure:"host"`
			Port int    `yaml:"port" mapstructure:"port"`
		} `yaml:"http" mapstructure:"http"`
	} `yaml:"transport" mapstructure:"transport"`
	Logging struct {
		Level  string `mapstructure:"level"`
		Format string `mapstructure:"format"`
	} `mapstructure:"logging"`
}

// Config interface implementation (for tools package)
func (c *Config) GetScanDirectories() []string {
	return c.Scan.Directory
}

func (c *Config) GetScanRecursive() bool {
	return c.Scan.Recursive
}

func (c *Config) GetScanOnStartup() bool {
	return c.Scan.OnStartup
}

func (c *Config) GetDatabasePath() string {
	return c.Database.Path
}

func (c *Config) GetLoggingLevel() string {
	return c.Logging.Level
}

func (c *Config) GetLoggingFormat() string {
	return c.Logging.Format
}

// ScanResult represents the result of a directory scan
type ScanResult struct {
	FilesScanned int
	FilesAdded   int
	FilesUpdated int
	FilesSkipped int
	FilesDeleted int
	Errors       []string
	Duration     time.Duration
}
