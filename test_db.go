package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

func main() {
	// Open database
	db, err := sql.Open("sqlite", "config/.aaa/archive.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Check if observation_date column exists
	rows, err := db.Query("PRAGMA table_info(fits_files)")
	if err != nil {
		log.Fatal("Error getting table info:", err)
	}
	defer rows.Close()

	fmt.Println("Table columns:")
	for rows.Next() {
		var cid, name, type_, notnull, dflt_value, pk interface{}
		err := rows.Scan(&cid, &name, &type_, &notnull, &dflt_value, &pk)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  %v: %v (%v)\n", name, type_, cid)
	}

	// Check data samples
	fmt.Println("\nSample data:")
	rows, err = db.Query(`
		SELECT relative_path, julian_date, utc_time, observation_date 
		FROM fits_files 
		LIMIT 3
	`)
	if err != nil {
		log.Fatal("Error querying data:", err)
	}
	defer rows.Close()

	for rows.Next() {
		var path string
		var julian_date *float64
		var utc_time, observation_date *string
		err := rows.Scan(&path, &julian_date, &utc_time, &observation_date)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("File: %s\n", path)
		if julian_date != nil {
			fmt.Printf("  Julian Date: %f\n", *julian_date)
		} else {
			fmt.Printf("  Julian Date: NULL\n")
		}
		if utc_time != nil {
			fmt.Printf("  UTC Time: %s\n", *utc_time)
		} else {
			fmt.Printf("  UTC Time: NULL\n")
		}
		if observation_date != nil {
			fmt.Printf("  Observation Date: %s\n", *observation_date)
		} else {
			fmt.Printf("  Observation Date: NULL\n")
		}
		fmt.Println()
	}

	// Count statistics
	var total, hasJulian, hasUTC, hasObsDate int
	err = db.QueryRow(`
		SELECT 
			COUNT(*) as total,
			COUNT(julian_date) as has_julian,
			COUNT(utc_time) as has_utc,
			COUNT(observation_date) as has_obs_date
		FROM fits_files
	`).Scan(&total, &hasJulian, &hasUTC, &hasObsDate)
	if err != nil {
		log.Fatal("Error getting statistics:", err)
	}

	fmt.Printf("Statistics:\n")
	fmt.Printf("  Total files: %d\n", total)
	fmt.Printf("  Have Julian Date: %d\n", hasJulian)
	fmt.Printf("  Have UTC Time: %d\n", hasUTC)
	fmt.Printf("  Have Observation Date: %d\n", hasObsDate)
}
