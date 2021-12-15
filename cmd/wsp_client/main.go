package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/root-gg/wsp/client"
)

func main() {
	ctx := context.Background()

	configFile := flag.String("config", "wsp_client.cfg", "config file path")
	flag.Parse()

	// Load configuration
	config, err := client.LoadConfiguration(*configFile)
	if err != nil {
		log.Fatalf("Unable to load configuration : %s", err)
	}

	proxy := client.NewClient(config)

	// Handle SIGINT
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for {
			<-c
			log.Println("SIGINT Detected")
			proxy.Shutdown()
			os.Exit(0)
		}
	}()

	proxy.Start(ctx)

	select {}
}
