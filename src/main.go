package main

import (
	"context"
	"errors"
	"github.com/GSH-LAN/Unwindia_common/src/go/config"
	"github.com/GSH-LAN/Unwindia_tmt2/src/environment"
	"github.com/GSH-LAN/Unwindia_tmt2/src/server"
	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-pulsar/pkg/pulsar"
	pulsarClient "github.com/apache/pulsar-client-go/pulsar"
	"github.com/gammazero/workerpool"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	_ "time/tzdata"
)

func main() {
	// TODO: make common
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	mainContext, cancel := context.WithCancel(context.Background())
	err := godotenv.Load()
	if err != nil && !strings.Contains(err.Error(), "no such file") {
		log.Fatal().Err(err).Msg("Error loading .env file")
	}

	env := environment.Get()

	var configClient config.ConfigClient
	if env.ConfigFileName != "" {
		configClient, err = config.NewConfigFile(mainContext, env.ConfigFileName, env.ConfigTemplatesDir)
	} else {
		configClient, err = config.NewConfigClient()
	}

	if err != nil {
		cancel()
		log.Fatal().Err(err).Msg("Error initializing config")
	}

	wp := workerpool.New(env.WorkerCount)

	conn, err := pulsarClient.NewClient(pulsarClient.ClientOptions{
		URL:            env.PulsarURL,
		Authentication: env.PulsarAuth,
	})
	if err != nil {
		panic(errors.New("cannot connect to pulsar"))
	}

	matchPublisher, err := pulsar.NewPublisherWithPulsarClient(
		conn,
		watermill.NewStdLoggerWithOut(log.Logger, zerolog.GlobalLevel() <= zerolog.DebugLevel, zerolog.GlobalLevel() == zerolog.TraceLevel),
	)

	srv, err := server.NewServer(mainContext, env, configClient, matchPublisher, wp)
	if err != nil {
		cancel()
		log.Fatal().Err(err).Msg("Error creating server")
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGTERM)
	go func() {
		<-c
		cancel()
		if err := srv.Stop(); err != nil {
			log.Error().Err(err).Msg("Error stopping server")
		}
	}()

	err = srv.Start()
	if err != nil {
		log.Fatal().Err(err).Msg("Error starting server")
	}
}
