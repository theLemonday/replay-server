package main

import "time"

// SubServer is the persistent model for a registered sub-server.
// Port doubles as the unique identifier.
type SubServer struct {
	Port      int       `json:"port"`
	Name      string    `json:"name,omitempty"`
	Status    string    `json:"status"` // "running" | "stopped"
	Routes    []Route   `json:"routes"`
	CreatedAt time.Time `json:"created_at"`
}

type Route struct {
	// ID is a stable key for the route, generated as "<METHOD> <path>".
	// Using the natural key directly means no UUID dependency and makes
	// deduplication trivial.
	ID       string        `json:"id"`
	Method   string        `json:"method"`
	Path     string        `json:"path"`
	Response RouteResponse `json:"response"`
}

type RouteResponse struct {
	StatusCode int               `json:"status_code,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
}

// ── API request bodies ────────────────────────────────────────────────────────

type RegisterServerRequest struct {
	Port int    `json:"port"`
	Name string `json:"name,omitempty"`
}

type RegisterRouteRequest struct {
	Method   string        `json:"method"`
	Path     string        `json:"path"`
	Response RouteResponse `json:"response"`
}

// ── API error envelope ────────────────────────────────────────────────────────

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// routeID builds the stable route identifier.
func routeID(method, path string) string {
	return method + " " + path
}
