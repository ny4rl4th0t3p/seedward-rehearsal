package daemon

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ny4rl4th0t3p/seedward-gentool/pkg/rehearse"
	"github.com/ny4rl4th0t3p/seedward-libs/canonicaljson"

	"github.com/ny4rl4th0t3p/seedward-rehearsal/internal/bridge"
)

type fakeBridge struct {
	input   bridge.RehearsalInput
	getErr  error
	postErr error

	mu     sync.Mutex
	posted *bridge.ResultFact
}

func (f *fakeBridge) GetRehearsalInput(context.Context, string) (bridge.RehearsalInput, error) {
	return f.input, f.getErr
}

func (f *fakeBridge) PostRehearsalResults(_ context.Context, _ string, fact bridge.ResultFact) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posted = &fact
	return f.postErr
}

func (f *fakeBridge) lastPosted() *bridge.ResultFact {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.posted
}

type fakeRunner struct {
	res     *rehearse.Result
	err     error
	started chan struct{} // closed when Run is entered (for the conflict test)
	release chan struct{} // Run blocks until closed
}

func (f *fakeRunner) Run(ctx context.Context, _ rehearse.Input) (*rehearse.Result, error) {
	if f.started != nil {
		close(f.started)
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.res, f.err
}

func sampleInput() bridge.RehearsalInput {
	return bridge.RehearsalInput{
		LaunchID:     "L1",
		Chain:        bridge.Chain{Bech32Prefix: "cosmos", Denom: "uatom", TotalSupply: "1000", Binary: bridge.Binary{Name: "gaiad", Version: "v27", SHA256: "sha"}},
		InputSetHash: "hash1",
	}
}

func sampleResult() *rehearse.Result {
	return &rehearse.Result{
		Outcome:    rehearse.OutcomePass,
		Summary:    "ok",
		Validators: 2,
		Steps: []rehearse.Step{
			{Name: "build", Status: rehearse.StepPass},
			{Name: "boot", Status: rehearse.StepPass},
		},
	}
}

func newTestServer(t *testing.T, b Bridge, r Runner) (*Server, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return NewServer(b, r, priv, "/bin/chaind", "eng1", "ops-token", nil), pub
}

func post(t *testing.T, ts *httptest.Server, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/launches/L1/rehearse", http.NoBody)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	return resp
}

func readEvents(t *testing.T, resp *http.Response) []streamEvent {
	t.Helper()
	var events []streamEvent
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		var ev streamEvent
		require.NoError(t, json.Unmarshal(sc.Bytes(), &ev))
		events = append(events, ev)
	}
	require.NoError(t, sc.Err())
	return events
}

func TestRehearse_HappyPath(t *testing.T) {
	br := &fakeBridge{input: sampleInput()}
	s, pub := newTestServer(t, br, &fakeRunner{res: sampleResult()})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp := post(t, ts, "ops-token")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	steps, facts := 0, 0
	var fact *bridge.ResultFact
	for _, e := range readEvents(t, resp) {
		switch e.Kind {
		case "step":
			steps++
		case "fact":
			facts++
			fact = e.Fact
		case "error":
			t.Fatalf("unexpected error event: %s", e.Error)
		}
	}
	assert.Equal(t, 2, steps)
	assert.Equal(t, 1, facts)
	require.NotNil(t, fact)
	assert.Equal(t, "PASS", fact.Outcome)

	posted := br.lastPosted()
	require.NotNil(t, posted, "fact must be posted to coordd")
	assert.Equal(t, "hash1", posted.InputSetHash)
	assert.Equal(t, "eng1", posted.Rehearsal.EngineVersion)
	assert.Equal(t, "gaiad", posted.Rehearsal.BinaryName)

	// the posted fact's signature must verify with the service pubkey
	sig, err := base64.StdEncoding.DecodeString(posted.Signature)
	require.NoError(t, err)
	msg, err := canonicaljson.MarshalForSigning(posted)
	require.NoError(t, err)
	assert.True(t, ed25519.Verify(pub, msg, sig))
}

func TestRehearse_Unauthorized(t *testing.T) {
	s, _ := newTestServer(t, &fakeBridge{input: sampleInput()}, &fakeRunner{res: sampleResult()})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	for _, token := range []string{"", "wrong-token"} {
		resp := post(t, ts, token)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		resp.Body.Close()
	}
}

func TestRehearse_Conflict(t *testing.T) {
	rn := &fakeRunner{res: sampleResult(), started: make(chan struct{}), release: make(chan struct{})}
	s, _ := newTestServer(t, &fakeBridge{input: sampleInput()}, rn)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	done := make(chan struct{})
	go func() {
		// Inline (no require/t.FailNow off the test goroutine): this run only needs to hold
		// the lock; correctness is asserted on the second request below.
		defer close(done)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/launches/L1/rehearse", http.NoBody)
		if err != nil {
			return
		}
		req.Header.Set("Authorization", "Bearer ops-token")
		if resp, err := ts.Client().Do(req); err == nil {
			resp.Body.Close()
		}
	}()

	<-rn.started // first run holds the lock
	resp := post(t, ts, "ops-token")
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()

	close(rn.release)
	<-done
}

func TestRehearse_InputError(t *testing.T) {
	br := &fakeBridge{getErr: errors.New("coordd 503")}
	s, _ := newTestServer(t, br, &fakeRunner{res: sampleResult()})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp := post(t, ts, "ops-token")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode) // streaming already started

	events := readEvents(t, resp)
	require.Len(t, events, 1)
	assert.Equal(t, "error", events[0].Kind)
	assert.Contains(t, events[0].Error, "fetch input")
	assert.Nil(t, br.lastPosted())
}

func TestHealthz(t *testing.T) {
	s, _ := newTestServer(t, &fakeBridge{}, &fakeRunner{})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/healthz", http.NoBody)
	require.NoError(t, err)
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
