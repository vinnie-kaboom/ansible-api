package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"ansible-api/internal/server"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	logger := log.With().Str("component", "main").Logger()

	srv, err := server.New()
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create server")
	}

	// Channel to listen for errors coming from the server
	serverErrors := make(chan error, 1)

	// Start the server in a goroutine
	go func() {
		logger.Info().Msg("Starting server...")
		serverErrors <- srv.Start()
	}()

	// Channel to listen for an interrupt or terminate signal from the OS
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Blocking select waiting for either a server error or a shutdown signal
	select {
	case err := <-serverErrors:
		logger.Fatal().Err(err).Msg("Server failed")

	case sig := <-shutdown:
		logger.Info().Str("signal", sig.String()).Msg("Shutdown signal received")
		if err := srv.Stop(); err != nil {
			logger.Error().Err(err).Msg("Could not stop server gracefully")
			os.Exit(1)
		}
	}
}
