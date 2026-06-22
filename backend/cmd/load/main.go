// Command load ingests a query,count CSV into the SQLite store.
//
//	go run ./cmd/load -data data/queries.csv -db data/typeahead.db
package main

import (
	"log"

	"searchtypeahead/internal/config"
	"searchtypeahead/internal/store"
)

func main() {
	cfg := config.Parse()

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	log.Printf("loading %s → %s ...", cfg.DataPath, cfg.DBPath)
	n, err := st.BulkLoadCSV(cfg.DataPath)
	if err != nil {
		log.Fatalf("load: %v", err)
	}
	total, err := st.Count()
	if err != nil {
		log.Fatalf("count: %v", err)
	}
	log.Printf("applied %d rows; %d distinct queries now in store", n, total)
}
