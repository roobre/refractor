package main

import (
	log "github.com/sirupsen/logrus"
	"roob.re/shatter/provider/providers/archlinux"
	"roob.re/shatter/server"
)

func main() {
	log.SetLevel(log.InfoLevel)

	provider, err := archlinux.New(&archlinux.Config{
		Countries: map[string]bool{
			"ES": true,
			"FR": true,
			"PT": true,
		},
		MaxScore: 10,
	})
	if err != nil {
		log.Fatal(err)
	}

	server := server.New(provider)
	server.Run(":8080")
}
