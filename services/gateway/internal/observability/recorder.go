package observability

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type logger interface {
	Println(...any)
}

type metricKey struct {
	Method string
	Route  string
	Status string
}

// Recorder emits redacted JSON request events and a bounded Prometheus surface.
type Recorder struct {
	mu         sync.Mutex
	logger     logger
	started    time.Time
	inFlight   int64
	requests   map[metricKey]uint64
	durationMS map[metricKey]uint64
	denials    map[string]uint64
}

func NewRecorder(output logger) *Recorder {
	if output == nil {
		output = log.Default()
	}
	return &Recorder{
		logger: output, started: time.Now().UTC(), requests: make(map[metricKey]uint64),
		durationMS: make(map[metricKey]uint64), denials: make(map[string]uint64),
	}
}

func (r *Recorder) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		started := time.Now()
		r.mu.Lock()
		r.inFlight++
		r.mu.Unlock()
		capture := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(capture, request)
		duration := time.Since(started)
		route := routeClass(request.URL.Path)
		key := metricKey{Method: request.Method, Route: route, Status: statusClass(capture.status)}
		r.mu.Lock()
		r.inFlight--
		r.requests[key]++
		r.durationMS[key] += uint64(duration.Milliseconds())
		if capture.status == http.StatusUnauthorized || capture.status == http.StatusForbidden || capture.status == http.StatusTooManyRequests {
			r.denials[fmt.Sprint(capture.status)]++
		}
		r.mu.Unlock()
		event, _ := json.Marshal(map[string]any{
			"duration_ms": duration.Milliseconds(), "event": "http_request", "level": level(capture.status),
			"method": request.Method, "request_id": request.Header.Get("X-Request-ID"),
			"route_class": route, "status": capture.status,
		})
		r.logger.Println(string(event))
	})
}

func (r *Recorder) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	inFlight := r.inFlight
	started := r.started
	requests := make(map[metricKey]uint64, len(r.requests))
	durations := make(map[metricKey]uint64, len(r.durationMS))
	denials := make(map[string]uint64, len(r.denials))
	for key, value := range r.requests {
		requests[key] = value
	}
	for key, value := range r.durationMS {
		durations[key] = value
	}
	for key, value := range r.denials {
		denials[key] = value
	}
	r.mu.Unlock()

	keys := make([]metricKey, 0, len(requests))
	for key := range requests {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j])
	})
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, "mindcreek_gateway_uptime_seconds %d\n", int64(time.Since(started).Seconds()))
	fmt.Fprintf(w, "mindcreek_gateway_in_flight_requests %d\n", inFlight)
	for _, key := range keys {
		labels := fmt.Sprintf("method=%q,route=%q,status_class=%q", key.Method, key.Route, key.Status)
		fmt.Fprintf(w, "mindcreek_gateway_http_requests_total{%s} %d\n", labels, requests[key])
		fmt.Fprintf(w, "mindcreek_gateway_http_duration_milliseconds_sum{%s} %d\n", labels, durations[key])
	}
	denialKeys := make([]string, 0, len(denials))
	for key := range denials {
		denialKeys = append(denialKeys, key)
	}
	sort.Strings(denialKeys)
	for _, status := range denialKeys {
		fmt.Fprintf(w, "mindcreek_gateway_security_denials_total{status=%q} %d\n", status, denials[status])
	}
}

func routeClass(path string) string {
	switch {
	case path == "/health" || path == "/version":
		return "service"
	case path == "/internal/metrics":
		return "metrics"
	case strings.HasPrefix(path, "/api/v1/mindcreek/oidc/"):
		return "oidc"
	case path == "/mcp":
		return "mcp"
	case strings.HasPrefix(path, "/api/v1/mindcreek/") || path == "/api/v1/knowledge-spaces":
		return "mindcreek_api"
	case strings.HasPrefix(path, "/api/"):
		return "upstream_api"
	default:
		return "other"
	}
}

func statusClass(status int) string { return fmt.Sprintf("%dxx", status/100) }
func level(status int) string {
	if status >= 500 {
		return "error"
	}
	if status >= 400 {
		return "warn"
	}
	return "info"
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("hijacking is unavailable")
	}
	return hijacker.Hijack()
}
func (w *statusWriter) Push(target string, options *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, options)
	}
	return http.ErrNotSupported
}
func (w *statusWriter) ReadFrom(source io.Reader) (int64, error) {
	if reader, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return reader.ReadFrom(source)
	}
	return io.Copy(w.ResponseWriter, source)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
