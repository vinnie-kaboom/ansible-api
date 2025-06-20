package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"ansible-api/internal/server"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Warn().Err(err).Msg("Error loading .env file")
	}

	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	logger := log.With().Str("component", "main").Logger()

	srv, err := server.New()
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create server")
	}

	serverErrors := make(chan error, 1)

	go func() {
		logger.Info().Msg("Starting server...")
		serverErrors <- srv.Start()
	}()

	server.StartDriftDetection(srv)

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Fatal().Err(err).Msg("Server failed")

	case sig := <-shutdown:
		logger.Info().Str("signal", sig.String()).Msg("Shutdown signal received")
		logger.Info().Str("signal", sig.String()).Msg("Shutting down server...")
		if err := srv.Stop(); err != nil {
			logger.Error().Err(err).Msg("Could not stop server gracefully")
			os.Exit(1)
		}
	}
}
