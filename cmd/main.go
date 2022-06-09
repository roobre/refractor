package main

import (
	"flag"
	log "github.com/sirupsen/logrus"
	"os"
	"roob.re/shatter/server"
)

func main() {
	log.SetLevel(log.InfoLevel)

	configPath := flag.String("config", "shatter.yaml", "Path to shatter.yaml file")
	address := flag.String("address", ":8080", "Address to listen on")
	flag.Parse()

	config, err := os.Open(*configPath)
	if err != nil {
		log.Fatalf("Could not open %s", *configPath)
	}

	s, err := server.New(config)
	if err != nil {
		log.Fatalf("Could not create server: %v", err)
	}

	err = s.Run(*address)
	if err != nil {
		log.Errorf("Server exited with error: %v", err)
	}
}
