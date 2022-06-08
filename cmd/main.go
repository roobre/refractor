package main

import (
	log "github.com/sirupsen/logrus"
	"roob.re/shatter/providers/archlinux"
	"roob.re/shatter/server"
)

func main() {
	log.SetLevel(log.InfoLevel)

	provider := &archlinux.Provider{
		Countries: map[string]bool{
			"ES": true,
			"FR": true,
			"PT": true,
		},
		MaxScore: 10,
	}

	server := server.New(provider)
	server.Run(":8080")
}
