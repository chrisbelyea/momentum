package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chrisbelyea/momentum/internal/auth"
	"github.com/chrisbelyea/momentum/internal/backend"
	"github.com/chrisbelyea/momentum/internal/caldav"
	"github.com/chrisbelyea/momentum/internal/config"
	"github.com/chrisbelyea/momentum/internal/crypto"
	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/web"
	webassets "github.com/chrisbelyea/momentum/web"
	_ "github.com/mattn/go-sqlite3"
)

// version is set at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	if err := runAsPlatform(); err != nil {
		log.Fatal(err)
	}
}

// runServer contains the application lifecycle shared by interactive and
// Windows-service launches. A nil stop channel installs the normal console
// signal handlers; the Windows service adapter supplies its own stop channel.
func runServer(stop <-chan os.Signal) {
	// Get configuration from environment
	dbPath, err := config.DatabasePath()
	if err != nil {
		log.Fatalf("Failed to determine database path: %v", err)
	}
	port := getEnv("PORT", "8443")
	addr, err := configuredListenAddress(port)
	if err != nil {
		log.Fatalf("Invalid listen configuration: %v", err)
	}
	listenHost, _, _ := net.SplitHostPort(addr)
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
		if os.Getenv("MOMENTUM_DEV_MODE") != "1" {
			log.Fatalf("MOMENTUM_ENCRYPTION_KEY is required (set MOMENTUM_DEV_MODE=1 only for local development): %v", err)
		}
		log.Printf("WARNING: MOMENTUM_DEV_MODE=1; using ephemeral development encryption key")
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

	// Initialize schema on first run (no-op when schema already exists)
	if err := db.InitializeSchema(database); err != nil {
		log.Fatalf("Failed to initialize database schema: %v", err)
	}

	// Initialize repositories
	taskRepo := db.NewTaskRepository(database)
	backendRepo := db.NewBackendRepository(database)
	authService := auth.NewService(database)

	// Initialize handlers
	caldavHandler := caldav.NewHandler(taskRepo)
	backendHandler := backend.NewHandler(backendRepo)

	// Initialize Web handler
	webHandler := web.NewHandler(taskRepo, backendRepo)
	webHandler.SetSyncRepository(db.NewSyncRepository(database))

	// Setup routes
	mux := http.NewServeMux()
	mux.Handle("/auth/", authService.Routes())

	// Static file serving (PWA assets: manifest.json, icons, service worker).
	// Directory listings are disabled; individual files are cached for one year.
	staticFiles, err := fs.Sub(webassets.Files, "static")
	if err != nil {
		log.Fatalf("Failed to load embedded static assets: %v", err)
	}
	staticFileServer := http.FileServer(http.FS(staticFiles))
	mux.Handle("/static/", http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Disable directory listings
		if r.URL.Path == "" || r.URL.Path[len(r.URL.Path)-1] == '/' {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		staticFileServer.ServeHTTP(w, r)
	})))
	// The worker must be served from the origin root so it can control the
	// application pages. Keep it out of the immutable static-asset policy and
	// allow prompt updates when a new binary is installed.
	mux.HandleFunc("/sw.js", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		script, err := fs.ReadFile(staticFiles, "sw.js")
		if err != nil {
			http.Error(w, "service worker unavailable", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Service-Worker-Allowed", "/")
		_, _ = w.Write(script)
	})

	// Web UI routes
	mux.Handle("/", authService.Require(http.HandlerFunc(webHandler.HandleIndex)))
	mux.Handle("/list", authService.Require(http.HandlerFunc(webHandler.HandleList)))
	mux.Handle("/api/tasks", authService.Require(http.HandlerFunc(webHandler.HandleTasks)))
	mux.Handle("/api/tasks/", authService.Require(http.HandlerFunc(webHandler.HandleTasks)))
	mux.Handle("/api/sync/conflicts", authService.Require(http.HandlerFunc(webHandler.HandleSyncConflicts)))
	mux.Handle("/api/sync/conflicts/", authService.Require(http.HandlerFunc(webHandler.HandleSyncConflicts)))

	// CalDAV routes
	mux.Handle("/caldav/tasks", authService.Require(http.HandlerFunc(caldavHandler.HandleTasks)))
	mux.Handle("/caldav/tasks/", authService.Require(http.HandlerFunc(caldavHandler.HandleTask)))

	// Backend configuration routes
	mux.Handle("/backends", authService.Require(http.HandlerFunc(backendHandler.HandleBackends)))
	mux.Handle("/backends/", authService.Require(http.HandlerFunc(backendHandler.HandleBackend)))
	mux.Handle("/backends/validate", authService.Require(http.HandlerFunc(backendHandler.HandleValidateConnection)))

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

		redirectAddr, err := configuredListenAddress(httpRedirectPort)
		if err != nil {
			log.Fatalf("Invalid HTTP redirect listen configuration: %v", err)
		}
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
	log.Printf("Starting Momentum %s on %s (TLS)", version, addr)
	log.Printf("Listening on https://%s", net.JoinHostPort(listenHost, port))
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
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServeTLS(tlsCert, tlsKey) }()
	if stop == nil {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		stop = signals
	}
	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	case <-stop:
		log.Println("Shutdown signal received; stopping Momentum gracefully")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Graceful shutdown failed: %v", err)
		}
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// configuredListenAddress returns a validated TCP listen address. Loopback is
// the safe default because packaged development certificates authenticate only
// localhost/loopback. Operators may explicitly opt into a hostname or IP via
// LISTEN_ADDR when a trusted certificate and network controls are in place.
func configuredListenAddress(port string) (string, error) {
	host := strings.TrimSpace(getEnv("LISTEN_ADDR", "127.0.0.1"))
	if host == "" || strings.ContainsAny(host, "\r\n/\\") {
		return "", fmt.Errorf("LISTEN_ADDR must be a non-empty hostname or IP address")
	}
	portNumber, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", fmt.Errorf("PORT must be an integer between 1 and 65535")
	}
	return net.JoinHostPort(host, strconv.Itoa(portNumber)), nil
}
