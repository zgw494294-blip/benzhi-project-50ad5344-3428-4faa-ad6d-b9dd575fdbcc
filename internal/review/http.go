package review

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Handler exposes the review workflow as a JSON HTTP API.
type Handler struct {
	store *Store
}

// NewHandler creates an HTTP handler backed by store.
func NewHandler(store *Store) http.Handler {
	return &Handler{store: store}
}

type createRequest struct {
	SKU    string   `json:"sku"`
	Checks []string `json:"checks"`
}

type verdictRequest struct {
	Passed *bool   `json:"passed"`
	Note   *string `json:"note"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Route against the escaped path so that a check name containing "/" (for
	// example "nutrition/allergens") survives as a single path segment when it
	// is correctly percent-encoded as "nutrition%2Fallergens". Decoded values
	// for user-supplied segments are unescaped below before being used.
	escapedPath := r.URL.EscapedPath()
	if escapedPath == "/reviews" {
		if r.Method != http.MethodPost {
			writeMethodError(w, http.MethodPost)
			return
		}
		h.create(w, r)
		return
	}

	const prefix = "/reviews/"
	if !strings.HasPrefix(escapedPath, prefix) {
		writeError(w, http.StatusNotFound, ErrNotFound)
		return
	}
	parts := strings.Split(strings.TrimPrefix(escapedPath, prefix), "/")
	if len(parts) == 1 && parts[0] != "" {
		if r.Method != http.MethodGet {
			writeMethodError(w, http.MethodGet)
			return
		}
		id, err := url.PathUnescape(parts[0])
		if err != nil {
			writeError(w, http.StatusNotFound, ErrNotFound)
			return
		}
		h.get(w, id)
		return
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "finalize" {
		if r.Method != http.MethodPost {
			writeMethodError(w, http.MethodPost)
			return
		}
		id, err := url.PathUnescape(parts[0])
		if err != nil {
			writeError(w, http.StatusNotFound, ErrNotFound)
			return
		}
		h.finalize(w, r, id)
		return
	}
	if len(parts) == 3 && parts[0] != "" && parts[1] == "checks" && parts[2] != "" {
		if r.Method != http.MethodPost {
			writeMethodError(w, http.MethodPost)
			return
		}
		id, err := url.PathUnescape(parts[0])
		if err != nil {
			writeError(w, http.StatusNotFound, ErrNotFound)
			return
		}
		checkName, err := url.PathUnescape(parts[2])
		if err != nil {
			writeError(w, http.StatusNotFound, ErrNotFound)
			return
		}
		h.verdict(w, r, id, checkName)
		return
	}
	writeError(w, http.StatusNotFound, ErrNotFound)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var request createRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	record, err := h.store.Create(r.Context(), request.SKU, request.Checks)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (h *Handler) verdict(w http.ResponseWriter, r *http.Request, id, checkName string) {
	var request verdictRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if request.Passed == nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("%w: passed is required", ErrInvalidInput))
		return
	}
	record, err := h.store.RecordVerdict(r.Context(), id, checkName, *request.Passed, request.Note)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) finalize(w http.ResponseWriter, r *http.Request, id string) {
	record, err := h.store.Finalize(context.Background(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) get(w http.ResponseWriter, id string) {
	record, err := h.store.Get(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("%w: request body must be a JSON object", ErrInvalidInput)
	}
	strict := json.NewDecoder(bytes.NewReader(trimmed))
	strict.DisallowUnknownFields()
	if err := strict.Decode(destination); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%w: request body must contain one JSON object", ErrInvalidInput)
		}
		return fmt.Errorf("decode request: %w", err)
	}
	return nil
}

func writeStoreError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, context.Canceled):
		status = 499
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	case errors.Is(err, ErrInvalidInput):
		status = http.StatusBadRequest
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrCompleted), errors.Is(err, ErrVerdictExists), errors.Is(err, ErrIncomplete):
		status = http.StatusConflict
	}
	writeError(w, status, err)
}

func writeMethodError(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorResponse{Error: err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
