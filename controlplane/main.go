// SPDX-License-Identifier: Apache-2.0
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
)

func main() {
	addr := flag.String("addr", ":8090", "address to serve the ingest API and dashboard on")
	dbPath := flag.String("db", "controlplane.db", "path to the SQLite store")
	flag.Parse()

	// Read from the environment, not a flag: flags show up in `ps`/process
	// listings and container specs; a shared secret shouldn't.
	token := os.Getenv("CHIDRIXX_AUTH_TOKEN")
	if token == "" {
		log.Println("WARNING: CHIDRIXX_AUTH_TOKEN is not set — this control plane is unauthenticated. Fine for local dev, not for anything reachable beyond localhost.")
	}

	store, err := OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/ingest", handleIngest(store))
	mux.HandleFunc("/api/v1/findings", handleFindingsAPI(store))
	mux.HandleFunc("/api/v1/dashboard-summary", handleDashboardSummary(store))
	mux.Handle("/", webAssetsHandler())

	log.Printf("chidrixx control plane listening on %s (store: %s)", *addr, *dbPath)
	log.Fatal(http.ListenAndServe(*addr, requireToken(token, mux)))
}
