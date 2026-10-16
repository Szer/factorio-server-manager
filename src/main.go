package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
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
