package main

import (
	"crypto/tls"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/chrisbelyea/momentum/internal/backend"
	"github.com/chrisbelyea/momentum/internal/caldav"
	"github.com/chrisbelyea/momentum/internal/crypto"
	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/web"
	_ "github.com/mattn/go-sqlite3"
)

// version is set at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	// Get configuration from environment
	dbPath := getEnv("DB_PATH", "momentum.db")
	port := getEnv("PORT", "8443")
	tlsCert := os.Getenv("TLS_CERT")
	tlsKey := os.Getenv("TLS_KEY")
	httpRedirectPort := os.Getenv("HTTP_REDIRECT_PORT")
	// EXTERNAL_HOST is used to build safe redirect URLs. Defaults to localhost:<port>.
	// Set this to your public hostname (e.g., "example.com" or "example.com:8443") in
	// production to ensure the HTTP redirect target is always your own server.
	externalHost := getEnv("EXTERNAL_HOST", "localhost:"+port)

	// TLS is required. If cert/key are not provided, auto-generate a self-signed
	// development certificate so the server works out-of-the-box.
	if tlsCert == "" || tlsKey == "" {
		log.Println("WARNING: No TLS certificates provided (TLS_CERT/TLS_KEY not set)")
		certDir := crypto.DevCertDir()
		var certErr error
		tlsCert, tlsKey, certErr = crypto.GetOrCreateDevCert(certDir)
		if certErr != nil {
			log.Fatalf("Failed to generate development certificate: %v", certErr)
		}
		log.Printf("Using development certificate from: %s", certDir)
		log.Println()
		log.Println("WARNING: DEVELOPMENT MODE: Using auto-generated self-signed certificate")
		log.Println("   Your browser will show security warnings. This is expected.")
		log.Println("   For production, set TLS_CERT and TLS_KEY environment variables.")
		log.Println("   See: docs/tls-setup.md")
		log.Println()
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

	// Static file serving (PWA assets: manifest.json, icons, service worker).
	// Directory listings are disabled; individual files are cached for one year.
	staticFileServer := http.FileServer(http.Dir("web/static"))
	mux.Handle("/static/", http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Disable directory listings
		if r.URL.Path == "" || r.URL.Path[len(r.URL.Path)-1] == '/' {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		staticFileServer.ServeHTTP(w, r)
	})))

	// Web UI routes
	mux.HandleFunc("/", webHandler.HandleIndex)
	mux.HandleFunc("/list", webHandler.HandleList)
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
	// The redirect target is built from EXTERNAL_HOST (not the client-supplied Host
	// header) to prevent host-header injection / open redirect attacks.
	if httpRedirectPort != "" {
		// Build the HTTPS base URL using the trusted external host. Include the port only
		// when it is not the standard HTTPS port (443).
		httpsBase := "https://" + externalHost
		if port != "443" {
			// If externalHost already contains a port (operator explicitly set it), use
			// it as-is; otherwise append our port.
			hasPort := false
			for i := len(externalHost) - 1; i >= 0; i-- {
				if externalHost[i] == ':' {
					hasPort = true
					break
				}
				if externalHost[i] == ']' {
					// IPv6 address with no port
					break
				}
			}
			if !hasPort {
				httpsBase = "https://" + externalHost + ":" + port
			}
		}

		redirectAddr := fmt.Sprintf(":%s", httpRedirectPort)
		go func() {
			ln, err := net.Listen("tcp", redirectAddr)
			if err != nil {
				log.Fatalf("HTTP redirect server failed to bind on %s: %v", redirectAddr, err)
			}
			log.Printf("Starting HTTP redirect server on %s -> %s", redirectAddr, httpsBase)
			redirectMux := http.NewServeMux()
			redirectMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				target := httpsBase + r.URL.RequestURI()
				http.Redirect(w, r, target, http.StatusMovedPermanently)
			})
			redirectServer := &http.Server{
				Addr:              redirectAddr,
				Handler:           redirectMux,
				ReadHeaderTimeout: 5 * time.Second,
				ReadTimeout:       10 * time.Second,
				WriteTimeout:      10 * time.Second,
			}
			if err := redirectServer.Serve(ln); err != nil {
				log.Printf("HTTP redirect server error: %v", err)
			}
		}()
	}

	// Start TLS server — TLS 1.3 minimum, strong cipher suites enforced by Go's crypto/tls.
	addr := fmt.Sprintf(":%s", port)
	log.Printf("Starting Momentum %s on %s (TLS)", version, addr)
	log.Printf("Listening on https://localhost:%s", port)
	log.Printf("Database: %s", dbPath)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
		},
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
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
