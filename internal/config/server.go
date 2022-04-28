package config

import (
	"fmt"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/rs/cors"
	log "github.com/sirupsen/logrus"
	"net/http"
)

type Route struct {
	Path    string
	Method  string
	Handler http.HandlerFunc
}

// Server defines the server struct
type Server struct {
	router *mux.Router
}

type ServerConfigOption func(server *Server)

//NewServer creates a new server
func NewServer(options ...ServerConfigOption) *Server {
	s := &Server{
		router: mux.NewRouter().StrictSlash(true),
	}

	for _, opt := range options {
		opt(s)
	}

	return s
}

func (s *Server) WithRoutes(basePath string, routes ...Route) *Server {
	sub := s.router.PathPrefix(basePath).Subrouter()
	for _, route := range routes {
		sub.HandleFunc(route.Path, route.Handler).Methods(route.Method)
		log.WithFields(map[string]interface{}{
			"method": route.Method,
			"path":   fmt.Sprintf("%s%s", basePath, route.Path),
		}).Infof("registered path")
	}
	return s
}

//Start the server on the defined port
func (s *Server) Start(addr string, port int) {
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedHeaders:   []string{"Access-Control-Allow-Origin", "Content-Type", "Origin", "Accept-Encoding", "Accept-Language", "Authorization"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "OPTIONS", "DELETE"},
		AllowCredentials: true,
	})
	handler := c.Handler(s.router)
	panic(
		http.ListenAndServe(
			fmt.Sprintf("%s:%v", addr, port),
			handlers.RecoveryHandler()(handler)),
	)
}
