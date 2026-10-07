// Package daemonapi exposes the local control plane over a permission-bounded
// Unix socket using a versioned HTTP/JSON protocol.
package daemonapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/mark1708/tmh-next/internal/control"
	"github.com/mark1708/tmh-next/internal/domain"
)

const Version = 1
const maxRequestBytes = 1 << 20

type Service interface {
	control.Client
	control.Watcher
}

type Server struct {
	service Service
	handler http.Handler
}

func New(service Service) (*Server, error) {
	if service == nil {
		return nil, fmt.Errorf("daemon API service is nil")
	}
	server := &Server{service: service}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", server.health)
	mux.HandleFunc("GET /v1/snapshot", server.snapshot)
	mux.HandleFunc("GET /v1/watch", server.watch)
	mux.HandleFunc("POST /v1/search", server.search)
	mux.HandleFunc("POST /v1/execute", server.execute)
	server.handler = securityHeaders(mux)
	return server, nil
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"api_version": Version, "status": "ok"})
}

func (s *Server) snapshot(w http.ResponseWriter, request *http.Request) {
	snapshot, err := s.service.Snapshot(request.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) search(w http.ResponseWriter, request *http.Request) {
	var query domain.SearchQuery
	if err := decode(request, &query); err != nil {
		writeError(w, domain.Fail(domain.CodeValidation, "%v", err))
		return
	}
	result, err := s.service.Search(request.Context(), query)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) execute(w http.ResponseWriter, request *http.Request) {
	var action domain.Action
	if err := decode(request, &action); err != nil {
		writeError(w, domain.Fail(domain.CodeValidation, "%v", err))
		return
	}
	result, err := s.service.Execute(request.Context(), action)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) watch(w http.ResponseWriter, request *http.Request) {
	after, err := strconv.ParseUint(request.URL.Query().Get("after"), 10, 64)
	if err != nil && request.URL.Query().Get("after") != "" {
		writeError(w, domain.Fail(domain.CodeValidation, "invalid after sequence"))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	events, errs := s.service.Watch(request.Context(), after)
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	encoder := json.NewEncoder(w)
	for {
		select {
		case <-request.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := encoder.Encode(event); err != nil {
				return
			}
			flusher.Flush()
			after = event.Seq
		case streamErr, ok := <-errs:
			if ok && streamErr != nil {
				_ = encoder.Encode(map[string]string{"stream_error": streamErr.Error()})
				flusher.Flush()
			}
			return
		case <-heartbeat.C:
			event := control.WatchEvent{Seq: max(after, 1), BaseRevision: 0, Revision: 0, Kind: control.EventHeartbeat}
			if err := encoder.Encode(event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func decode(request *http.Request, target any) error {
	defer request.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(request.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request must contain one JSON value")
	}
	return nil
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := domain.ErrCodeOf(err)
	switch {
	case domain.IsRevisionConflict(err):
		status, code = http.StatusConflict, domain.CodeRevisionConflict
	case code == domain.CodeNotFound:
		status = http.StatusNotFound
	case code == domain.CodeValidation || code == domain.CodeBadTarget ||
		code == domain.CodeEmptyValue || code == domain.CodeEmptySelection:
		status = http.StatusBadRequest
	case code == domain.CodeConfirmationNeeded || code == domain.CodeProtected ||
		code == domain.CodeInvalidState || code == domain.CodeNotLive ||
		code == domain.CodeStalePlan || code == domain.CodeUndoUnavailable ||
		code == domain.CodeUndoStale:
		status = http.StatusUnprocessableEntity
	case code == domain.CodeAlreadyClaimed:
		status = http.StatusConflict
	case code == domain.CodeProbeTimeout:
		status = http.StatusGatewayTimeout
	case code == domain.CodeBusy || code == domain.CodeUnreachable:
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]string{"code": string(code), "message": domain.ErrorMessage(err)})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		next.ServeHTTP(w, request)
	})
}

var _ = context.Canceled
