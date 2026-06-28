package daemon

import (
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/ny4rl4th0t3p/seedward-gentool/pkg/rehearse"

	"github.com/ny4rl4th0t3p/seedward-rehearsal/internal/bridge"
)

// Bridge is the coordd bridge surface the daemon needs (satisfied by *bridge.Client).
type Bridge interface {
	GetRehearsalInput(ctx context.Context, launchID string) (bridge.RehearsalInput, error)
	PostRehearsalResults(ctx context.Context, launchID string, fact bridge.ResultFact) error
}

// Runner runs a rehearsal (satisfied by *rehearse.Engine).
type Runner interface {
	Run(ctx context.Context, in rehearse.Input) (*rehearse.Result, error)
}

// Server is the rehearsal daemon. It exposes an ops-authed trigger endpoint that pulls a
// launch's input, runs the engine, signs the result fact and posts it back, streaming progress
// as NDJSON. Runs are serialized — one ephemeral chain per instance (D-h).
type Server struct {
	bridge        Bridge
	runner        Runner
	signer        ed25519.PrivateKey
	binaryPath    string
	engineVersion string
	opsToken      string
	log           *slog.Logger
	mu            sync.Mutex // serializes runs
}

// NewServer builds a daemon server. A nil log uses slog.Default().
func NewServer(b Bridge, r Runner, signer ed25519.PrivateKey, binaryPath, engineVersion, opsToken string, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		bridge:        b,
		runner:        r,
		signer:        signer,
		binaryPath:    binaryPath,
		engineVersion: engineVersion,
		opsToken:      opsToken,
		log:           log,
	}
}

// Handler returns the daemon's HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /launches/{id}/rehearse", s.withAuth(s.handleRehearse))
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	return mux
}

func (*Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// streamEvent is one NDJSON line in the trigger response: a step as it is reported, the final
// signed fact, or an error.
type streamEvent struct {
	Kind  string             `json:"kind"` // "step" | "fact" | "error"
	Step  *bridge.ResultStep `json:"step,omitempty"`
	Fact  *bridge.ResultFact `json:"fact,omitempty"`
	Error string             `json:"error,omitempty"`
}

func (s *Server) handleRehearse(w http.ResponseWriter, r *http.Request) {
	launchID := r.PathValue("id")
	if launchID == "" {
		http.Error(w, "missing launch id", http.StatusBadRequest)
		return
	}
	// Serialize runs: a busy instance rejects rather than booting a second chain.
	if !s.mu.TryLock() {
		http.Error(w, "a rehearsal is already running", http.StatusConflict)
		return
	}
	defer s.mu.Unlock()

	w.Header().Set("Content-Type", "application/x-ndjson")
	enc := json.NewEncoder(w)
	emit := func(ev streamEvent) {
		if err := enc.Encode(ev); err != nil {
			s.log.Error("stream encode failed", "err", err)
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	s.rehearse(r.Context(), launchID, emit)
}

// rehearse runs the full bridge flow, emitting NDJSON events. Errors after the first emit are
// in-band (the HTTP status is already 200): a single "error" event ends the stream.
func (s *Server) rehearse(ctx context.Context, launchID string, emit func(streamEvent)) {
	in, err := s.bridge.GetRehearsalInput(ctx, launchID)
	if err != nil {
		emit(streamEvent{Kind: "error", Error: "fetch input: " + err.Error()})
		return
	}
	engineInput, err := in.ToInput(s.binaryPath)
	if err != nil {
		emit(streamEvent{Kind: "error", Error: "map input: " + err.Error()})
		return
	}

	res, err := s.runner.Run(ctx, engineInput)
	if err != nil {
		emit(streamEvent{Kind: "error", Error: "engine: " + err.Error()})
		return
	}
	for i := range res.Steps {
		emit(streamEvent{Kind: "step", Step: &bridge.ResultStep{
			Name:   res.Steps[i].Name,
			Status: string(res.Steps[i].Status),
			Detail: res.Steps[i].Detail,
		}})
	}

	fact := bridge.FactFromResult(launchID, in.InputSetHash, s.engineVersion, in.Chain.Binary, res)
	if err := fact.Sign(s.signer); err != nil {
		emit(streamEvent{Kind: "error", Error: "sign fact: " + err.Error()})
		return
	}
	emit(streamEvent{Kind: "fact", Fact: &fact})

	if err := s.bridge.PostRehearsalResults(ctx, launchID, fact); err != nil {
		emit(streamEvent{Kind: "error", Error: "post to coordd: " + err.Error()})
	}
}

// withAuth enforces the ops credential as a bearer token, compared in constant time.
func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.opsToken)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
