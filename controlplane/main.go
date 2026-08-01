// SPDX-License-Identifier: Apache-2.0
package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	// `controlplane create-tenant --name X --admin-user Y --admin-password Z`
	// provisions a brand-new isolated tenant. This is deliberately a CLI
	// subcommand run by the operator, not a public signup form -- chidrixx
	// is still a self-hosted tool, and a web-facing "create your own
	// tenant" flow is a much bigger, riskier surface (abuse, verification,
	// billing) than what was actually asked for here, which is real data
	// isolation between tenants once they exist.
	if len(os.Args) > 1 && os.Args[1] == "create-tenant" {
		runCreateTenant(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "create-user" {
		runCreateUser(os.Args[2:])
		return
	}

	addr := flag.String("addr", ":8090", "address to serve the ingest API and dashboard on")
	dbPath := flag.String("db", "controlplane.db", "path to the SQLite store")
	flag.Parse()

	store, err := OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	bootstrapDefaultTenant(store)

	// The static SPA shell carries no secrets (no data is embedded at
	// build time — everything real is fetched from the API below), so it
	// serves publicly. The actual data endpoints stay behind real auth:
	// requireAPIToken for agent ingest, requireSession for everything a
	// logged-in browser reads.
	api := http.NewServeMux()
	api.HandleFunc("/api/v1/ingest", requireAPIToken(store, handleIngest(store)))
	api.HandleFunc("/api/v1/findings", requireSession(store, handleFindingsAPI(store)))
	api.HandleFunc("/api/v1/dashboard-summary", requireSession(store, handleDashboardSummary(store)))
	api.HandleFunc("/api/v1/budget", requireSession(store, budgetRoute(store)))
	api.HandleFunc("/api/v1/auth/me", requireSession(store, handleMe))

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/login", handleLogin(store))
	mux.HandleFunc("/api/v1/auth/logout", handleLogout(store))
	mux.Handle("/api/", api)
	mux.Handle("/", webAssetsHandler())

	log.Printf("chidrixx control plane listening on %s (store: %s)", *addr, *dbPath)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// budgetRoute applies requireAdmin only to the write side of handleBudget
// -- a viewer can still see the configured budget, just not change it.
func budgetRoute(store *Store) http.HandlerFunc {
	get := handleBudget(store)
	adminOnly := requireAdmin(handleBudget(store))
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			adminOnly(w, r)
			return
		}
		get(w, r)
	}
}

// bootstrapDefaultTenant creates tenant 1 + its admin user + its ingest
// API token on first startup, so an upgrade from the single-shared-token
// era (or a brand-new install) always ends up with something you can
// actually log in and ship data to, instead of a control plane nobody can
// reach. Idempotent: if any tenant already exists, this does nothing --
// it never runs twice against the same database.
func bootstrapDefaultTenant(store *Store) {
	count, err := store.TenantCount()
	if err != nil {
		log.Fatalf("bootstrap: count tenants: %v", err)
	}
	if count > 0 {
		return
	}

	adminUser := os.Getenv("CHIDRIXX_ADMIN_USER")
	if adminUser == "" {
		adminUser = "admin"
	}

	adminPassword := os.Getenv("CHIDRIXX_ADMIN_PASSWORD")
	generatedPassword := adminPassword == ""
	if generatedPassword {
		var err error
		adminPassword, err = randomPassword()
		if err != nil {
			log.Fatalf("bootstrap: generate admin password: %v", err)
		}
	}

	tenantName := os.Getenv("CHIDRIXX_TENANT_NAME")
	if tenantName == "" {
		tenantName = "default"
	}

	tenantID, apiToken, err := store.CreateTenant(tenantName, adminUser, adminPassword)
	if err != nil {
		log.Fatalf("bootstrap: create default tenant: %v", err)
	}

	log.Printf("bootstrap: created tenant %q (id=%d)", tenantName, tenantID)
	log.Printf("bootstrap: admin login — username=%q", adminUser)
	if generatedPassword {
		log.Printf("bootstrap: admin password (generated, shown once): %s", adminPassword)
	}
	log.Printf("bootstrap: agent ingest token (shown once, put it in CHIDRIXX_AUTH_TOKEN on every agent): %s", apiToken)
}

func randomPassword() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random password: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func runCreateTenant(args []string) {
	fs := flag.NewFlagSet("create-tenant", flag.ExitOnError)
	dbPath := fs.String("db", "controlplane.db", "path to the SQLite store")
	name := fs.String("name", "", "tenant name (required)")
	adminUser := fs.String("admin-user", "", "admin username (required)")
	adminPassword := fs.String("admin-password", "", "admin password (required)")
	fs.Parse(args)

	if *name == "" || *adminUser == "" || *adminPassword == "" {
		fmt.Fprintln(os.Stderr, "usage: controlplane create-tenant --name X --admin-user Y --admin-password Z [--db path]")
		os.Exit(2)
	}

	store, err := OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	tenantID, apiToken, err := store.CreateTenant(*name, *adminUser, *adminPassword)
	if err != nil {
		log.Fatalf("create tenant: %v", err)
	}

	fmt.Printf("Created tenant %q (id=%d)\n", *name, tenantID)
	fmt.Printf("Admin login: username=%s\n", *adminUser)
	fmt.Printf("Agent ingest token (shown once, put it in CHIDRIXX_AUTH_TOKEN on this tenant's agents): %s\n", apiToken)
}

func runCreateUser(args []string) {
	fs := flag.NewFlagSet("create-user", flag.ExitOnError)
	dbPath := fs.String("db", "controlplane.db", "path to the SQLite store")
	tenantID := fs.Int64("tenant-id", 0, "existing tenant ID to add this user to (required)")
	username := fs.String("username", "", "login username (required)")
	password := fs.String("password", "", "login password (required)")
	role := fs.String("role", RoleViewer, "role: admin or viewer")
	fs.Parse(args)

	if *tenantID == 0 || *username == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "usage: controlplane create-user --tenant-id N --username Y --password Z [--role admin|viewer] [--db path]")
		os.Exit(2)
	}

	store, err := OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.CreateUser(*tenantID, *username, *password, *role); err != nil {
		log.Fatalf("create user: %v", err)
	}

	fmt.Printf("Created %s user %q on tenant %d\n", *role, *username, *tenantID)
}
