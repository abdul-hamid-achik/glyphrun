package storyrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/log"
	"github.com/abdul-hamid-achik/glyphrun/internal/stories"
	"github.com/abdul-hamid-achik/glyphrun/internal/watchfs"
)

// ServeOptions configure the live catalog server.
type ServeOptions struct {
	Options
	// Addr is the listen address; loopback by default so a dev server never
	// exposes local screens to the network.
	Addr string
	// Watch re-runs stories on source changes and pushes the catalog.
	Watch bool
	// ExtraWatch adds paths to the watch set.
	ExtraWatch []string
	// RunOnStart runs every story before serving the first page.
	RunOnStart bool
	// Ready is called with the bound URL once the listener is open.
	Ready func(url string)
}

type runRequest struct {
	key    string
	update bool
}

// Server is the stdlib HTTP server behind `glyph stories serve`. It holds the
// latest catalog, streams updates over SSE, and serialises reruns through a
// single worker so two clicks never race the same story.
type Server struct {
	opts    ServeOptions
	mu      sync.Mutex
	payload stories.PagePayload
	clients map[chan string]struct{}
	queue   chan runRequest
}

// Serve blocks until ctx is cancelled.
func Serve(ctx context.Context, opts ServeOptions) error {
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:4649"
	}
	s := &Server{opts: opts, clients: map[chan string]struct{}{}, queue: make(chan runRequest, 64)}
	if err := s.refresh(); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", opts.Addr)
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/catalog.json", s.handleCatalog)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/run", s.handleRun)
	mux.HandleFunc("/update", s.handleUpdate)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go s.worker(ctx)
	if opts.RunOnStart {
		s.queue <- runRequest{}
	}
	if opts.Watch {
		go s.watchLoop(ctx)
	}
	if opts.Ready != nil {
		opts.Ready(url)
	}
	log.Info("stories: serving", "url", url, "watch", opts.Watch)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// refresh rebuilds the payload from disk and broadcasts it.
func (s *Server) refresh() error {
	plan, err := Discover(Options{Paths: s.opts.Paths, ConfigPath: s.opts.ConfigPath, Environment: s.opts.Environment, ArtifactRoot: s.opts.ArtifactRoot})
	if err != nil && !errors.Is(err, ErrNoStories) {
		return err
	}
	collect := stories.CollectOptions{Paths: s.opts.Paths, ConfigPath: s.opts.ConfigPath, Environment: s.opts.Environment}
	if plan != nil {
		collect.ArtifactRoot = plan.ArtifactRoot
		collect.SnapshotRoot = plan.SnapshotRoot
		collect.StoriesRoot = plan.StoriesRoot
		collect.DefaultTerminal = plan.Runtime.SpecParseOptions().DefaultTerminal
	}
	cat, err := stories.Collect(collect)
	if err != nil && !errors.Is(err, stories.ErrNoSpecs) {
		return err
	}
	payload := stories.BuildPayload(cat, true, time.Now().UTC().Format(time.RFC3339))
	data, _ := json.Marshal(payload)
	s.mu.Lock()
	s.payload = payload
	s.mu.Unlock()
	s.broadcast("catalog", string(data))
	return nil
}

func (s *Server) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-s.queue:
			s.broadcast("busy", "true")
			opts := s.opts.Options
			opts.Update = req.update
			if req.key != "" {
				opts.Only = []string{req.key}
			}
			plan, err := Discover(opts)
			if err != nil {
				log.Warn("stories: discover failed", "err", err)
			} else if _, err := Run(ctx, opts, plan); err != nil {
				log.Warn("stories: run failed", "err", err)
			}
			if err := s.refresh(); err != nil {
				log.Warn("stories: refresh failed", "err", err)
			}
			s.broadcast("busy", "false")
		}
	}
}

func (s *Server) watchLoop(ctx context.Context) {
	roots := s.watchRoots()
	last := watchfs.Fingerprint(roots)
	ticker := time.NewTicker(watchfs.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			roots = s.watchRoots()
			fp := watchfs.Fingerprint(roots)
			if fp == last {
				continue
			}
			last = fp
			log.Info("stories: change detected, re-running")
			select {
			case s.queue <- runRequest{}:
			default:
			}
		}
	}
}

func (s *Server) watchRoots() []string {
	plan, err := Discover(Options{Paths: s.opts.Paths, ConfigPath: s.opts.ConfigPath, Environment: s.opts.Environment})
	var roots []string
	if err == nil {
		roots = append(roots, plan.WatchRoots...)
	}
	roots = append(roots, s.opts.ExtraWatch...)
	return watchfs.Roots(roots...)
}

func (s *Server) broadcast(event, data string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msg := "event: " + event + "\ndata: " + strings.ReplaceAll(data, "\n", "\ndata: ") + "\n\n"
	for ch := range s.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	payload := s.payload
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(stories.RenderHTMLPayload(payload)))
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	payload := s.payload
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	ch := make(chan string, 8)
	s.mu.Lock()
	s.clients[ch] = struct{}{}
	data, _ := json.Marshal(s.payload)
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, ch)
		s.mu.Unlock()
	}()
	_, _ = fmt.Fprintf(w, "event: catalog\ndata: %s\n\n", data)
	flusher.Flush()
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			_, _ = w.Write([]byte(msg))
			flusher.Flush()
		case <-keepalive.C:
			_, _ = w.Write([]byte(": keepalive\n\n"))
			flusher.Flush()
		}
	}
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	s.enqueue(w, r, false)
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	s.enqueue(w, r, true)
}

func (s *Server) enqueue(w http.ResponseWriter, r *http.Request, update bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body)
	if update && strings.TrimSpace(body.Key) == "" {
		http.Error(w, "update requires a story key", http.StatusBadRequest)
		return
	}
	select {
	case s.queue <- runRequest{key: strings.TrimSpace(body.Key), update: update}:
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"queued":true}`))
	default:
		http.Error(w, "run queue is full", http.StatusTooManyRequests)
	}
}
