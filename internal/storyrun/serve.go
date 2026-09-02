package storyrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
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
	opts     ServeOptions
	mu       sync.Mutex
	payload  stories.PagePayload
	clients  map[chan string]struct{}
	queue    chan runRequest
	roots    []string
	excluded []string
	lastFP   uint64
	// hosts are the Host header values the server answers for. Anything
	// else is refused so a DNS-rebinding page cannot drive the server.
	hosts map[string]bool
}

// Serve blocks until ctx is cancelled.
func Serve(ctx context.Context, opts ServeOptions) error {
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:4649"
	}
	s := &Server{opts: opts, clients: map[chan string]struct{}{}, queue: make(chan runRequest, 64), hosts: map[string]bool{}}
	if err := s.refresh(); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", opts.Addr)
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String()
	s.allowHost(ln.Addr().String())
	s.allowHost(opts.Addr)
	if _, port, err := net.SplitHostPort(ln.Addr().String()); err == nil {
		for _, h := range []string{"localhost", "127.0.0.1", "[::1]"} {
			s.allowHost(h + ":" + port)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/catalog.json", s.handleCatalog)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/run", s.handleRun)
	mux.HandleFunc("/update", s.handleUpdate)
	srv := &http.Server{Handler: s.guard(mux), ReadHeaderTimeout: 5 * time.Second}

	go s.worker(ctx)
	if opts.RunOnStart {
		s.queue <- runRequest{}
	}
	if opts.Watch {
		s.refreshRoots()
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

func (s *Server) allowHost(h string) {
	h = strings.ToLower(strings.TrimSpace(h))
	if h == "" {
		return
	}
	s.hosts[h] = true
	// A listen address like ":4649" or "0.0.0.0:4649" is reachable under
	// any interface name; only add the concrete forms we know.
	if host, port, err := net.SplitHostPort(h); err == nil && (host == "" || host == "0.0.0.0" || host == "::") {
		delete(s.hosts, h)
		s.hosts["localhost:"+port] = true
		s.hosts["127.0.0.1:"+port] = true
	}
}

// guard rejects requests that do not target one of the server's own host
// names (DNS rebinding) and mutations that do not look like they came from
// the page: JSON content type (forces a CORS preflight, which the server
// never approves) and, when an Origin header is present, a same-origin one.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := strings.ToLower(r.Host)
		if !s.hosts[host] {
			http.Error(w, "unexpected host", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(strings.ToLower(ct), "application/json") {
				http.Error(w, "content-type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				if o := strings.ToLower(strings.TrimPrefix(origin, "http://")); !s.hosts[o] {
					http.Error(w, "cross-origin request refused", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// collectOptions resolves the roots from config alone so a manifest with a
// syntax error still shows up in the catalog as a parse_error row instead of
// taking the whole page down.
func (s *Server) collectOptions() (stories.CollectOptions, error) {
	paths := s.opts.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}
	rt, err := config.LoadRuntime(paths[0], s.opts.ConfigPath, s.opts.Environment)
	if err != nil {
		return stories.CollectOptions{}, err
	}
	return stories.ResolveRoots(rt, s.opts.ArtifactRoot).CollectOptions(paths, s.opts.ConfigPath, s.opts.Environment), nil
}

// refresh rebuilds the payload from disk and broadcasts it.
func (s *Server) refresh() error {
	collect, err := s.collectOptions()
	if err != nil {
		return err
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
				// Accepting a golden must touch exactly the row that was
				// reviewed, never its variants; a rerun may fan out.
				opts.Exact = req.update
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
			if s.opts.Watch {
				// Filter never changes WatchRoots, so the worker's plan is the
				// authoritative watch set: no second discovery needed. The
				// fingerprint is reset after the run so build output under a
				// watched directory does not re-trigger it.
				s.setRoots(plan)
				s.resetFingerprint()
			}
			s.broadcast("busy", "false")
		}
	}
}

// setRoots stores the watch set from a discovered plan: manifests,
// harness.watch paths, story spec directories, plus --watch-path extras. The
// output roots (artifacts, goldens, index) are excluded from fingerprinting
// so a run's own writes never look like a source change. A nil plan (the
// manifest failed to parse) falls back to watching the manifests themselves
// so the fix re-triggers.
func (s *Server) setRoots(plan *Plan) {
	var roots, excluded []string
	if plan != nil {
		roots = append(roots, plan.WatchRoots...)
		excluded = plan.OutputRoots()
	} else if collect, err := s.collectOptions(); err == nil {
		if manifests, ferr := stories.FindManifests(collect.Paths); ferr == nil {
			roots = append(roots, manifests...)
		}
		excluded = []string{collect.ArtifactRoot, collect.SnapshotRoot, collect.StoriesRoot}
	}
	roots = append(roots, s.opts.ExtraWatch...)
	s.mu.Lock()
	s.roots = watchfs.Roots(roots...)
	s.excluded = excluded
	s.mu.Unlock()
}

// refreshRoots discovers the plan once and stores its watch set.
func (s *Server) refreshRoots() {
	plan, err := Discover(Options{Paths: s.opts.Paths, ConfigPath: s.opts.ConfigPath, Environment: s.opts.Environment})
	if err != nil {
		plan = nil
	}
	s.setRoots(plan)
	s.resetFingerprint()
}

func (s *Server) fingerprint() uint64 {
	s.mu.Lock()
	roots := append([]string(nil), s.roots...)
	excluded := append([]string(nil), s.excluded...)
	s.mu.Unlock()
	return watchfs.FingerprintExcluding(roots, excluded)
}

// resetFingerprint records the current state as the baseline the watch loop
// compares against.
func (s *Server) resetFingerprint() {
	fp := s.fingerprint()
	s.mu.Lock()
	s.lastFP = fp
	s.mu.Unlock()
}

func (s *Server) watchLoop(ctx context.Context) {
	ticker := time.NewTicker(watchfs.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fp := s.fingerprint()
			s.mu.Lock()
			changed := fp != s.lastFP
			s.lastFP = fp
			s.mu.Unlock()
			if !changed {
				continue
			}
			log.Info("stories: change detected, re-running")
			select {
			case s.queue <- runRequest{}:
			default:
			}
		}
	}
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
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		// A malformed body must not silently become "rerun everything".
		http.Error(w, "malformed JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
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
