package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/root-gg/utils"
	"github.com/root-gg/wsp/client"
)

func main() {
	configFile := flag.String("config", "wsp_client.cfg", "config file path")
	flag.Parse()

	// Load configuration
	config, err := client.LoadConfiguration(*configFile)
	if err != nil {
		log.Fatalf("Unable to load configuration : %s", err)
	}
	utils.Dump(config)

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

	proxy.Start()

	select {}
}
