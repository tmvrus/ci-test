package server

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var compressibleContentTypes = []string{
	"text/html",
	"text/css",
	"text/plain",
	"text/javascript",
	"application/javascript",
	"application/x-javascript",
	"application/json",
	"application/atom+xml",
	"application/rss+xml",
	"image/svg+xml",
}

type Server struct {
	server *http.Server
}

func New(address string, handler *chi.Mux) *Server {
	return &Server{
		server: &http.Server{
			Handler: handler,
			Addr:    address,
		},
	}
}

func NewRouter() *chi.Mux {

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(9, compressibleContentTypes...))

	return r
}

func (s *Server) Run(ctx context.Context) (err error) {

	errChan := make(chan error, 1)

	group := &sync.WaitGroup{}
	group.Add(1)
	go func() {
		defer group.Done()
		if err := s.server.ListenAndServe(); err != nil {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		group.Add(1)
		go func() {
			defer group.Done()
			gracefullCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

			if err := s.server.Shutdown(gracefullCtx); err != nil {
				log.Printf("Server Shutdown Failed:%+v", err)
			}
			cancel()

		}()
	case err = <-errChan:
	}

	group.Wait()
	return err
}
