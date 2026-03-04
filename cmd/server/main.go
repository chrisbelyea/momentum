package main

import (
"crypto/tls"
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
port := getEnv("PORT", "8443")
tlsCert := os.Getenv("TLS_CERT")
tlsKey := os.Getenv("TLS_KEY")
httpRedirectPort := os.Getenv("HTTP_REDIRECT_PORT")

// TLS is required; fail fast if cert/key are not provided.
if tlsCert == "" || tlsKey == "" {
log.Fatalf(
"TLS configuration is required.\n" +
"Set TLS_CERT and TLS_KEY environment variables to the paths of your\n" +
"certificate and private key files.\n" +
"See docs/tls-setup.md for dev and production setup instructions.",
)
}

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

// Optionally start an HTTP redirect server that sends plain-HTTP clients to HTTPS.
if httpRedirectPort != "" {
redirectAddr := fmt.Sprintf(":%s", httpRedirectPort)
log.Printf("Starting HTTP redirect server on %s -> HTTPS port %s", redirectAddr, port)
go func() {
redirectMux := http.NewServeMux()
redirectMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
target := "https://" + r.Host + r.URL.RequestURI()
http.Redirect(w, r, target, http.StatusMovedPermanently)
})
if err := http.ListenAndServe(redirectAddr, redirectMux); err != nil {
log.Printf("HTTP redirect server error: %v", err)
}
}()
}

// Start TLS server — TLS 1.3 minimum, strong cipher suites enforced by Go's crypto/tls.
addr := fmt.Sprintf(":%s", port)
log.Printf("Starting Momentum server on %s (TLS)", addr)
log.Printf("Database: %s", dbPath)

server := &http.Server{
Addr:    addr,
Handler: mux,
TLSConfig: &tls.Config{
MinVersion: tls.VersionTLS13,
},
}
if err := server.ListenAndServeTLS(tlsCert, tlsKey); err != nil {
log.Fatalf("Server failed to start: %v", err)
}
}

func getEnv(key, defaultValue string) string {
if value := os.Getenv(key); value != "" {
return value
}
return defaultValue
}
