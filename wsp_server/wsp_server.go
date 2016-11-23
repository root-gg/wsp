package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/root-gg/utils"

	"github.com/root-gg/wsp/server"
)

func main() {
	configFile := flag.String("config", "wsp_server.cfg", "config file path")
	flag.Parse()

	// Load configuration
	config, err := server.LoadConfiguration(*configFile)
	if err != nil {
		log.Fatalf("Unable to load configuration : %s", err)
	}
	utils.Dump(config)

	server := server.NewServer(config)

	// Handle SIGINT
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for {
			<-c
			log.Println("SIGINT Detected")
			server.Shutdown()
			os.Exit(0)
		}
	}()

	server.Start()

	select {}
}
