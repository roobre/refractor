package server

import (
	log "github.com/sirupsen/logrus"
	"net/http"
	"roob.re/shatter/pool"
	"roob.re/shatter/providers"
)

type Server struct {
	pool     *pool.Pool
	provider providers.Provider
}

func New(provider providers.Provider) *Server {
	s := &Server{}
	s.pool = pool.New(8)
	s.provider = provider

	return s
}

func (s *Server) Run(address string) {
	go s.pool.Run()
	go s.pool.Feed(s.provider)

	log.Infof("Listening on %s", address)
	err := http.ListenAndServe(address, s.pool)
	if err != nil {
		log.Error("Server stopped: %v", err)
	}
}
