package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func newRouter(reg *registry) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(scalarHTML)
	})

	r.Get("/scalar.bundle.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Write(scalarBundle)
	})

	// Serve the spec for client introspection.
	r.Get("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		http.ServeFile(w, r, "openapi.yaml")
	})

	r.Get("/servers", reg.listServers)
	r.Post("/servers", reg.registerServer)

	r.Get("/servers/{port}", reg.getServer)
	r.Delete("/servers/{port}", reg.deleteServer)

	r.Get("/servers/{port}/routes", reg.listRoutes)
	r.Post("/servers/{port}/routes", reg.registerRoute)
	r.Delete("/servers/{port}/routes/{routeId}", reg.deleteRoute)

	return r
}
