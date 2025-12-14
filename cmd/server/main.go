package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/chrisbelyea/momentum/internal/caldav"
	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/web"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// Get configuration from environment
	dbPath := getEnv("DB_PATH", "momentum.db")
	port := getEnv("PORT", "8080")

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

	// Initialize CalDAV handler
	caldavHandler := caldav.NewHandler(taskRepo)

	// Initialize Web handler
	webHandler := web.NewHandler(taskRepo)

	// Setup routes
	mux := http.NewServeMux()
	
	// Web UI routes
	mux.HandleFunc("/", webHandler.HandleIndex)
	mux.HandleFunc("/api/tasks/", webHandler.HandleUpdateStatus)
	
	// CalDAV API routes
	mux.HandleFunc("/caldav/tasks", caldavHandler.HandleTasks)
	mux.HandleFunc("/caldav/tasks/", caldavHandler.HandleTask)

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
