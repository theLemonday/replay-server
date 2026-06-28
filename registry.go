package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// registry is the control plane: it owns all instances and handles the
// management API requests.
type registry struct {
	mu        sync.RWMutex
	instances map[int]*instance // key: port
}

func newRegistry() *registry {
	return &registry{instances: make(map[int]*instance)}
}

// restore re-creates instances from a persisted snapshot. Called once at
// startup before the management API accepts connections.
func (reg *registry) restore(servers []SubServer) {
	for _, s := range servers {
		s.Status = "running"
		inst, err := startInstance(s)
		if err != nil {
			slog.Warn("could not restore subserver", "port", s.Port, "err", err)
			continue
		}
		reg.instances[s.Port] = inst
	}
}

// snapshot returns all sub-server models for persistence.
func (reg *registry) snapshot() []SubServer {
	reg.mu.RLock()
	defer reg.mu.RUnlock()

	out := make([]SubServer, 0, len(reg.instances))
	for _, inst := range reg.instances {
		out = append(out, inst.snapshot())
	}
	return out
}

// shutdownAll gracefully stops every sub-server in parallel.
func (reg *registry) shutdownAll(ctx context.Context) {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	var wg sync.WaitGroup
	for _, inst := range reg.instances {
		wg.Add(1)
		go func(inst *instance) {
			defer wg.Done()
			inst.shutdown(ctx)
		}(inst)
	}
	wg.Wait()
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────

func (reg *registry) listServers(w http.ResponseWriter, r *http.Request) {
	reg.mu.RLock()
	defer reg.mu.RUnlock()

	out := make([]SubServer, 0, len(reg.instances))
	for _, inst := range reg.instances {
		out = append(out, inst.snapshot())
	}
	jsonOK(w, http.StatusOK, out)
}

func (reg *registry) registerServer(w http.ResponseWriter, r *http.Request) {
	var req RegisterServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiErr(w, http.StatusUnprocessableEntity, "invalid_body", err.Error())
		return
	}
	if req.Port < 1024 || req.Port > 65535 {
		apiErr(w, http.StatusUnprocessableEntity, "invalid_port",
			fmt.Sprintf("port %d out of range [1024, 65535]", req.Port))
		return
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()

	if _, exists := reg.instances[req.Port]; exists {
		apiErr(w, http.StatusConflict, "port_in_use",
			fmt.Sprintf("port %d is already registered", req.Port))
		return
	}

	sub := SubServer{
		Port:      req.Port,
		Name:      req.Name,
		Status:    "running",
		Routes:    []Route{},
		CreatedAt: time.Now().UTC(),
	}
	inst, err := startInstance(sub)
	if err != nil {
		slog.Error("failed to start subserver", "port", req.Port, "err", err)
		apiErr(w, http.StatusConflict, "bind_failed", err.Error())
		return
	}

	reg.instances[req.Port] = inst
	jsonOK(w, http.StatusCreated, inst.snapshot())
}

func (reg *registry) getServer(w http.ResponseWriter, r *http.Request) {
	inst, ok := reg.instanceFromPath(w, r)
	if !ok {
		return
	}
	jsonOK(w, http.StatusOK, inst.snapshot())
}

func (reg *registry) deleteServer(w http.ResponseWriter, r *http.Request) {
	port, ok := portFromPath(w, r)
	if !ok {
		return
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()

	inst, exists := reg.instances[port]
	if !exists {
		apiErr(w, http.StatusNotFound, "not_found", "sub-server not found")
		return
	}

	inst.shutdown(r.Context())
	delete(reg.instances, port)
	w.WriteHeader(http.StatusNoContent)
}

func (reg *registry) listRoutes(w http.ResponseWriter, r *http.Request) {
	inst, ok := reg.instanceFromPath(w, r)
	if !ok {
		return
	}
	jsonOK(w, http.StatusOK, inst.snapshot().Routes)
}

func (reg *registry) registerRoute(w http.ResponseWriter, r *http.Request) {
	inst, ok := reg.instanceFromPath(w, r)
	if !ok {
		return
	}

	var req RegisterRouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiErr(w, http.StatusUnprocessableEntity, "invalid_body", err.Error())
		return
	}
	if req.Method == "" || req.Path == "" {
		apiErr(w, http.StatusUnprocessableEntity, "missing_fields", "method and path are required")
		return
	}
	if req.Response.StatusCode == 0 {
		req.Response.StatusCode = http.StatusOK
	}

	route := Route{
		ID:       routeID(req.Method, req.Path),
		Method:   req.Method,
		Path:     req.Path,
		Response: req.Response,
	}

	if !inst.addRoute(route) {
		apiErr(w, http.StatusConflict, "route_exists",
			fmt.Sprintf("%s %s is already registered", req.Method, req.Path))
		return
	}
	jsonOK(w, http.StatusCreated, route)
}

func (reg *registry) deleteRoute(w http.ResponseWriter, r *http.Request) {
	inst, ok := reg.instanceFromPath(w, r)
	if !ok {
		return
	}

	rid := chi.URLParam(r, "routeId")
	if !inst.removeRoute(rid) {
		apiErr(w, http.StatusNotFound, "not_found", "route not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (reg *registry) instanceFromPath(w http.ResponseWriter, r *http.Request) (*instance, bool) {
	port, ok := portFromPath(w, r)
	if !ok {
		return nil, false
	}
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	inst, exists := reg.instances[port]
	if !exists {
		apiErr(w, http.StatusNotFound, "not_found", fmt.Sprintf("no sub-server on port %d", port))
		return nil, false
	}
	return inst, true
}

func portFromPath(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := chi.URLParam(r, "port")
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		apiErr(w, http.StatusUnprocessableEntity, "invalid_port", "port must be an integer in [1, 65535]")
		return 0, false
	}
	return port, true
}

func jsonOK(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func apiErr(w http.ResponseWriter, status int, code, message string) {
	jsonOK(w, status, APIError{Code: code, Message: message})
}
