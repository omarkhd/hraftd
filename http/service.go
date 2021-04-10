// Package httpd provides the HTTP server for accessing the distributed key-value store.
// It also provides the endpoint for other nodes to join an existing cluster.
package httpd

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/otoolep/hraftd/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	httpRequestsSummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name:       "http_requests",
		Help:       "HTTP requests to the hraftd service",
		Objectives: metrics.Quantiles,
	}, []string{"endpoint", "method"})
	httpErrorsCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_request_errors",
		Help: "Failed HTTP requests to the hraftd service",
	}, []string{"endpoint", "method", "status"})
)

func init() {
	prometheus.MustRegister(httpRequestsSummary)
	prometheus.MustRegister(httpErrorsCounter)
}

// Store is the interface Raft-backed key-value stores must implement.
type Store interface {
	// Get returns the value for the given key.
	Get(key string) (string, error)

	// Set sets the value for the given key, via distributed consensus.
	Set(key, value string) error

	// Delete removes the given key, via distributed consensus.
	Delete(key string) error

	// Join joins the node, identitifed by nodeID and reachable at addr, to the cluster.
	Join(nodeID string, addr string) error

	// Status returns the store raft status.
	Status() string
}

// Service provides HTTP service.
type Service struct {
	addr string
	ln   net.Listener

	store Store
}

// New returns an uninitialized HTTP service.
func New(addr string, store Store) *Service {
	return &Service{
		addr:  addr,
		store: store,
	}
}

// Start starts the service.
func (s *Service) Start() error {
	server := http.Server{
		Handler: s,
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln

	http.Handle("/", s)

	go func() {
		err := server.Serve(s.ln)
		if err != nil {
			log.Fatalf("HTTP serve: %s", err)
		}
	}()

	return nil
}

// Close closes the service.
func (s *Service) Close() {
	s.ln.Close()
	return
}

// ServeHTTP allows Service to serve HTTP requests.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/key") {
		s.handleKeyRequest(w, r)
	} else if r.URL.Path == "/join" {
		s.handleJoin(w, r)
	} else if r.URL.Path == "/status" {
		s.handleStatus(w, r)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *Service) handleStatus(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, s.store.Status())
}

func (s *Service) handleJoin(w http.ResponseWriter, r *http.Request) {
	m := map[string]string{}
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(m) != 2 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	remoteAddr, ok := m["addr"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	nodeID, ok := m["id"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.store.Join(nodeID, remoteAddr); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *Service) handleKeyRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now().UnixNano()
	labels := map[string]string{
		"endpoint": "/key",
		"method":   r.Method,
	}
	defer func() {
		httpRequestsSummary.With(labels).Observe(
			float64(time.Now().UnixNano() - start),
		)
	}()
	getKey := func() string {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 3 {
			return ""
		}
		return parts[2]
	}
	switch r.Method {
	case "GET":
		k := getKey()
		if k == "" {
			labels["status"] = fmt.Sprint(http.StatusBadRequest)
			httpErrorsCounter.With(labels).Inc()
			w.WriteHeader(http.StatusBadRequest)
		}
		v, err := s.store.Get(k)
		if err != nil {
			labels["status"] = fmt.Sprint(http.StatusInternalServerError)
			httpErrorsCounter.With(labels).Inc()
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		b, err := json.Marshal(map[string]string{k: v})
		if err != nil {
			labels["status"] = fmt.Sprint(http.StatusInternalServerError)
			httpErrorsCounter.With(labels).Inc()
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		io.WriteString(w, string(b))

	case "POST":
		// Read the value from the POST body.
		m := map[string]string{}
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			labels["status"] = fmt.Sprint(http.StatusBadRequest)
			httpErrorsCounter.With(labels).Inc()
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for k, v := range m {
			if err := s.store.Set(k, v); err != nil {
				labels["status"] = fmt.Sprint(http.StatusInternalServerError)
				httpErrorsCounter.With(labels).Inc()
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

	case "DELETE":
		k := getKey()
		if k == "" {
			labels["status"] = fmt.Sprint(http.StatusBadRequest)
			httpErrorsCounter.With(labels).Inc()
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := s.store.Delete(k); err != nil {
			labels["status"] = fmt.Sprint(http.StatusInternalServerError)
			httpErrorsCounter.With(labels).Inc()
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		s.store.Delete(k)

	default:
		labels["status"] = fmt.Sprint(http.StatusMethodNotAllowed)
		httpErrorsCounter.With(labels).Inc()
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
	return
}

// Addr returns the address on which the Service is listening
func (s *Service) Addr() net.Addr {
	return s.ln.Addr()
}
