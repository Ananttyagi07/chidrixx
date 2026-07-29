package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", ":8090", "address to serve the ingest API and dashboard on")
	dbPath := flag.String("db", "controlplane.db", "path to the SQLite store")
	flag.Parse()

	store, err := OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/ingest", handleIngest(store))
	mux.HandleFunc("/api/v1/findings", handleFindingsAPI(store))
	mux.HandleFunc("/", handleDashboard(store))

	log.Printf("chidrixx control plane listening on %s (store: %s)", *addr, *dbPath)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
