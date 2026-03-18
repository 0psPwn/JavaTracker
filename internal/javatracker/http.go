package javatracker

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed web
var webFS embed.FS

type Server struct {
	addr        string
	service     *Service
	defaultRoot string
	static      http.Handler
	httpServer  *http.Server
	requestLog  *log.Logger
}

func NewServer(addr, defaultRoot string) (*Server, error) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return nil, err
	}
	srv := &Server{
		addr:        addr,
		service:     NewService(),
		defaultRoot: defaultRoot,
		static:      http.FileServer(http.FS(sub)),
		requestLog:  log.New(os.Stdout, "[JavaTracker] ", log.LstdFlags),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", srv.handleStatus)
	mux.HandleFunc("/api/index", srv.handleIndex)
	mux.HandleFunc("/api/upload", srv.handleUpload)
	mux.HandleFunc("/api/snippet", srv.handleSnippet)
	mux.HandleFunc("/api/search", srv.handleSearch)
	mux.HandleFunc("/api/graph", srv.handleGraph)
	mux.HandleFunc("/api/node", srv.handleNode)
	mux.Handle("/", srv.handleSPA())
	srv.httpServer = &http.Server{
		Addr:              addr,
		Handler:           srv.loggingMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv, nil
}

func (s *Server) Start() error {
	if s.defaultRoot != "" {
		if err := s.service.Build(s.defaultRoot); err != nil {
			s.requestLog.Printf("initial index failed: %v", err)
		}
	}
	s.requestLog.Printf("listening on %s", s.addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.service.Status())
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, s.service.Status())
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var payload struct {
		Root string `json:"root"`
	}
	if err := decodeJSON(r.Body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if strings.TrimSpace(payload.Root) == "" {
		payload.Root = s.defaultRoot
	}
	if payload.Root == "" {
		writeError(w, http.StatusBadRequest, "root is required")
		return
	}

	root, err := filepath.Abs(payload.Root)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid root path")
		return
	}

	if err := s.service.Build(root); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.service.Status())
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	results, err := s.service.Search(query, limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": results,
		"total": len(results),
	})
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "files is required")
		return
	}

	uploadResult, err := SaveUploadedJavaProject("/data/workspace/JavaTracker", files)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.service.Build(uploadResult.Root); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"upload": uploadResult,
		"status": s.service.Status(),
	})
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("node")
	if nodeID == "" {
		writeError(w, http.StatusBadRequest, "node is required")
		return
	}
	depth, _ := strconv.Atoi(r.URL.Query().Get("depth"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	direction := Direction(r.URL.Query().Get("direction"))
	includeBody := parseBoolDefault(r.URL.Query().Get("include_body"), true)

	graph, err := s.service.Graph(nodeID, QueryOptions{
		Direction:   direction,
		Depth:       depth,
		Limit:       limit,
		IncludeBody: includeBody,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, graph)
}

func (s *Server) handleSnippet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var payload SnippetPayload
	if err := decodeJSON(r.Body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	uploadResult, err := SaveJavaSnippet("/data/workspace/JavaTracker", payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.service.Build(uploadResult.Root); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"upload": uploadResult,
		"status": s.service.Status(),
	})
}

func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("id")
	if nodeID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	details, err := s.service.Details(nodeID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, details)
}

func (s *Server) handleSPA() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			s.serveStatic("index.html", w, r)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := webFS.ReadFile("web/" + path); err == nil {
			s.serveStatic(path, w, r)
			return
		}
		s.serveStatic("index.html", w, r)
	})
}

func (s *Server) serveStatic(path string, w http.ResponseWriter, r *http.Request) {
	file, err := webFS.ReadFile("web/" + path)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	http.ServeContent(w, r, path, time.Now(), bytes.NewReader(file))
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.requestLog.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func decodeJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func parseBoolDefault(value string, fallback bool) bool {
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func Run(addr, root string) error {
	server, err := NewServer(addr, root)
	if err != nil {
		return err
	}
	err = server.Start()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
