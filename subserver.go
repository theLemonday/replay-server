package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// instance is a live sub-server bound on two listeners.
type instance struct {
	mu     sync.RWMutex
	snap   SubServer        // kept in sync with routes map
	routes map[string]Route // key: routeID(method, path)

	ipv4 *http.Server
	ipv6 *http.Server
}

// startInstance binds both stacks and starts serving.
func startInstance(sub SubServer) (*instance, error) {
	inst := &instance{
		snap:   sub,
		routes: make(map[string]Route, len(sub.Routes)),
	}
	for _, r := range sub.Routes {
		inst.routes[r.ID] = r
	}

	mux := inst.buildMux()
	inst.ipv4 = &http.Server{Handler: mux}
	inst.ipv6 = &http.Server{Handler: mux}

	addr4 := fmt.Sprintf("0.0.0.0:%d", sub.Port)
	addr6 := fmt.Sprintf("[::]:%d", sub.Port)

	ln4, err := net.Listen("tcp4", addr4)
	if err != nil {
		return nil, fmt.Errorf("listen tcp4 %s: %w", addr4, err)
	}
	ln6, err := net.Listen("tcp6", addr6)
	if err != nil {
		ln4.Close()
		return nil, fmt.Errorf("listen tcp6 %s: %w", addr6, err)
	}

	go func() {
		if err := inst.ipv4.Serve(ln4); err != nil && err != http.ErrServerClosed {
			slog.Error("subserver ipv4 error", "port", sub.Port, "err", err)
		}
	}()
	go func() {
		if err := inst.ipv6.Serve(ln6); err != nil && err != http.ErrServerClosed {
			slog.Error("subserver ipv6 error", "port", sub.Port, "err", err)
		}
	}()

	slog.Info("subserver started", "port", sub.Port)
	return inst, nil
}

func (inst *instance) shutdown(ctx context.Context) {
	if err := inst.ipv4.Shutdown(ctx); err != nil {
		slog.Warn("subserver ipv4 shutdown", "port", inst.snap.Port, "err", err)
	}
	if err := inst.ipv6.Shutdown(ctx); err != nil {
		slog.Warn("subserver ipv6 shutdown", "port", inst.snap.Port, "err", err)
	}
}

// addRoute registers a route. Returns false if the route already exists.
func (inst *instance) addRoute(r Route) bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if _, exists := inst.routes[r.ID]; exists {
		return false
	}
	inst.routes[r.ID] = r
	inst.syncSnap()
	inst.swapMux()
	return true
}

// removeRoute deletes a route by ID. Returns false if not found.
func (inst *instance) removeRoute(id string) bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if _, ok := inst.routes[id]; !ok {
		return false
	}
	delete(inst.routes, id)
	inst.syncSnap()
	inst.swapMux()
	return true
}

// snapshot returns a safe copy of the sub-server state.
func (inst *instance) snapshot() SubServer {
	inst.mu.RLock()
	defer inst.mu.RUnlock()
	s := inst.snap
	s.Routes = make([]Route, len(inst.snap.Routes))
	copy(s.Routes, inst.snap.Routes)
	return s
}

// syncSnap rebuilds inst.snap.Routes from the routes map.
// Must be called with inst.mu held (write).
func (inst *instance) syncSnap() {
	routes := make([]Route, 0, len(inst.routes))
	for _, r := range inst.routes {
		routes = append(routes, r)
	}
	inst.snap.Routes = routes
}

// swapMux builds a fresh Chi router and hot-swaps it onto both servers.
// In-flight requests finish on the old handler; new connections see the new one.
// Must be called with inst.mu held (write).
func (inst *instance) swapMux() {
	mux := inst.buildMux()
	inst.ipv4.Handler = mux
	inst.ipv6.Handler = mux
}

// buildMux constructs the Chi router from current inst.routes.
// Safe to call without the lock (used during startInstance before goroutines
// start, and from swapMux which holds the lock).
func (inst *instance) buildMux() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	for _, route := range inst.routes {
		r.MethodFunc(route.Method, route.Path, func(w http.ResponseWriter, req *http.Request) {
			sc := route.Response.StatusCode
			if sc == 0 {
				sc = http.StatusOK
			}
			for k, v := range route.Response.Headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(sc)
			if route.Response.Body != "" {
				_, _ = w.Write([]byte(route.Response.Body))
			}
		})
	}

	// Catch-all: unregistered paths → 200 OK, empty body.
	r.HandleFunc("/*", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return r
}
