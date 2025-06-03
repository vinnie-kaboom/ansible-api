package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"ansible-api/internal/api"
	"ansible-api/internal/config"
)

func main() {
	cfg := config.DefaultConfig

	// Ensure template directory exists
	if err := os.MkdirAll(cfg.TemplateDir, 0755); err != nil {
		log.Fatal(err)
	}

	// Load templates
	templates, err := template.ParseGlob(filepath.Join(cfg.TemplateDir, "*.html"))
	if err != nil {
		log.Fatal(err)
	}

	// Initialize handler
	handler := api.NewHandler(&cfg, templates)

	// Setup routes
	http.HandleFunc("/", handler.IndexHandler)
	http.HandleFunc("/api/logs/", handler.LogsHandler)
	http.HandleFunc("/api/stats", handler.StatsHandler)

	// Serve static files
	fs := http.FileServer(http.Dir(cfg.TemplateDir))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Port)
	fmt.Printf("Starting dashboard on port %d\n", cfg.Port)
	log.Fatal(http.ListenAndServe(addr, nil))
}