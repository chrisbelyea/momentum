package main

import (
"database/sql"
"fmt"
"log"
"net/http"
"os"

"github.com/chrisbelyea/momentum/internal/backend"
"github.com/chrisbelyea/momentum/internal/caldav"
"github.com/chrisbelyea/momentum/internal/crypto"
"github.com/chrisbelyea/momentum/internal/db"
"github.com/chrisbelyea/momentum/internal/web"
_ "github.com/mattn/go-sqlite3"
)

func main() {
// Get configuration from environment
dbPath := getEnv("DB_PATH", "momentum.db")
port := getEnv("PORT", "8080")

// Initialize encryption
if err := crypto.LoadEncryptionKeyFromEnv(); err != nil {
log.Printf("Warning: Encryption key not set. Backend credentials will not be encrypted.")
log.Printf("Set MOMENTUM_ENCRYPTION_KEY environment variable for production use.")
// Initialize with a default key for development (not secure for production)
if err := crypto.InitializeEncryption("dev-default-key-change-in-production"); err != nil {
log.Fatalf("Failed to initialize encryption: %v", err)
}
}

// Initialize database
database, err := sql.Open("sqlite3", dbPath)
if err != nil {
log.Fatalf("Failed to open database: %v", err)
}
defer database.Close()

// Verify database connection
if err := database.Ping(); err != nil {
log.Fatalf("Failed to connect to database: %v", err)
}

// Initialize repositories
taskRepo := db.NewTaskRepository(database)
backendRepo := db.NewBackendRepository(database)

// Initialize handlers
caldavHandler := caldav.NewHandler(taskRepo)
backendHandler := backend.NewHandler(backendRepo)

// Initialize Web handler
webHandler := web.NewHandler(taskRepo)

// Setup routes
mux := http.NewServeMux()

// Static file serving (PWA assets: manifest.json, icons, service worker)
mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

// Web UI routes
mux.HandleFunc("/", webHandler.HandleIndex)
mux.HandleFunc("/api/tasks/", webHandler.HandleUpdateStatus)

// CalDAV routes
mux.HandleFunc("/caldav/tasks", caldavHandler.HandleTasks)
mux.HandleFunc("/caldav/tasks/", caldavHandler.HandleTask)

// Backend configuration routes
mux.HandleFunc("/backends", backendHandler.HandleBackends)
mux.HandleFunc("/backends/", backendHandler.HandleBackend)
mux.HandleFunc("/backends/validate", backendHandler.HandleValidateConnection)

// Health check endpoint
mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusOK)
fmt.Fprintf(w, "OK")
})

// Start server
addr := fmt.Sprintf(":%s", port)
log.Printf("Starting Momentum CalDAV server on %s", addr)
log.Printf("Database: %s", dbPath)

if err := http.ListenAndServe(addr, mux); err != nil {
log.Fatalf("Server failed to start: %v", err)
}
}

func getEnv(key, defaultValue string) string {
if value := os.Getenv(key); value != "" {
return value
}
return defaultValue
}
