package main

import (
	"context"
	"errors"
	"github.com/OpenFactorioServerManager/factorio-server-manager/telemetry"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/api"
	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
)

func main() {
	// get the all configs based on the flags
	config := bootstrap.NewConfig(os.Args[1:])

	// setup the required files for the mods
	factorio.ModStartUp()

	// Initialize Factorio Server struct
	err := factorio.NewFactorioServer()
	if err != nil {
		log.Printf("Error occurred during Server initialization: %v\n", err)
		return
	}

	// Initialize authentication system
	api.SetupAuth()

	ctx, cancelTelemetry := context.WithCancel(context.Background())
	defer cancelTelemetry()
	options := telemetry.LoadOptions(config.PrometheusURL, config.TelemetryLabels)
	api.UPS = telemetry.NewService(options)
	log.Printf("UPS: backends=%v sampling=%s history=%s legacy_prometheus=%t", options.Backends, options.Interval, options.Window, options.LegacyPrometheus)
	if !options.LegacyPrometheus {
		go api.UPS.Run(ctx, func(ctx context.Context) (telemetry.Sample, error) {
			if !factorio.GetFactorioServer().GetRunning() {
				return telemetry.Sample{}, errors.New("game stopped")
			}
			session := factorio.GameSession()
			sample, err := telemetry.ReadRCON(ctx, net.JoinHostPort("127.0.0.1", strconv.Itoa(config.FactorioRconPort)), config.FactorioRconPass)
			sample.Session = session
			if session != factorio.GameSession() {
				return telemetry.Sample{}, errors.New("game restarted")
			}
			return sample, err
		})
	}

	// Initialize HTTP router -- also initializes websocket
	router := api.NewRouter()

	log.Printf("Starting server on: %s:%s", config.ServerIP, config.ServerPort)
	web := &http.Server{Addr: config.ServerIP + ":" + config.ServerPort, Handler: router}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, os.Interrupt)
	go func() {
		<-signals
		_ = web.Close()
	}()
	err = web.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Printf("HTTP server failed: %v", err)
	}
	game := factorio.GetFactorioServer()
	if game.GetRunning() {
		if err := game.Stop(); err != nil {
			log.Printf("Error stopping game: %v", err)
		}
		deadline := time.Now().Add(110 * time.Second)
		for game.GetRunning() && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
		}
		if game.GetRunning() {
			log.Println("Timed out waiting for the game to save")
			os.Exit(1)
		}
	}

}
