package main

import (
	"flag"
	log "github.com/sirupsen/logrus"
	"os"
	"roob.re/shatter/server"
)

func main() {
	configPath := flag.String("config", "shatter.yaml", "Path to shatter.yaml file")
	address := flag.String("address", ":8080", "Address to listen on")
	logLvl := flag.String("log-level", "info", "Verbosity level. Accepts levels understood by logrus")
	flag.Parse()

	level, err := log.ParseLevel(*logLvl)
	if err != nil {
		log.Fatalf("Could not parse log level %q", *logLvl)
	}

	log.SetLevel(level)

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
