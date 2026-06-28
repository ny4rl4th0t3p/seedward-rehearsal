package bridge

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ny4rl4th0t3p/seedward-gentool/pkg/rehearse"
	"github.com/ny4rl4th0t3p/seedward-libs/canonicaljson"
)

func sampleResult() *rehearse.Result {
	return &rehearse.Result{
		Outcome:    rehearse.OutcomePass,
		Summary:    "all good",
		Validators: 2,
		Steps: []rehearse.Step{
			{Name: "build", Status: rehearse.StepPass},
			{Name: "assert:supply_reconciles", Status: rehearse.StepPass, Detail: "7000000"},
		},
		StartedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 1, 1, 0, 0, 5, 0, time.UTC),
	}
}

func TestFactFromResult(t *testing.T) {
	bin := Binary{Name: "gaiad", Version: "v27.2.0", SHA256: "abc"}
	fact := FactFromResult("L1", "hash1", "eng1", bin, sampleResult())

	assert.Equal(t, 1, fact.SchemaVersion)
	assert.Equal(t, "L1", fact.LaunchID)
	assert.Equal(t, "hash1", fact.InputSetHash)
	assert.Nil(t, fact.AttemptID)
	assert.Equal(t, "PASS", fact.Outcome)
	assert.Nil(t, fact.FailedStep, "PASS → no failed step")
	assert.Equal(t, "all good", fact.Summary)
	require.Len(t, fact.Steps, 2)
	assert.Equal(t, "assert:supply_reconciles", fact.Steps[1].Name)
	assert.Equal(t, "gaiad", fact.Rehearsal.BinaryName)
	assert.Equal(t, "eng1", fact.Rehearsal.EngineVersion)
	assert.Equal(t, 2, fact.Rehearsal.Validators)
	assert.Equal(t, "2026-01-01T00:00:00Z", fact.StartedAt)
	assert.Equal(t, "2026-01-01T00:00:05Z", fact.FinishedAt)
}

func TestFactFromResult_FailedStep(t *testing.T) {
	res := sampleResult()
	res.Outcome = rehearse.OutcomeFail
	res.FailedStep = "build"
	fact := FactFromResult("L1", "h", "eng", Binary{}, res)
	require.NotNil(t, fact.FailedStep)
	assert.Equal(t, "build", *fact.FailedStep)
}

// TestSign_VerifiesWithSharedCanonicalJSON is the load-bearing test: it proves the fact's
// signature verifies under the exact scheme coordd uses (seedward-libs canonicaljson + Ed25519).
func TestSign_VerifiesWithSharedCanonicalJSON(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	fact := FactFromResult("L1", "hash1", "eng1", Binary{Name: "gaiad"}, sampleResult())
	require.NoError(t, fact.Sign(priv))

	assert.Equal(t, base64.StdEncoding.EncodeToString(pub), fact.ServicePubkey)
	require.NotEmpty(t, fact.Signature)

	// Verify exactly as coordd would: MarshalForSigning strips "signature", Ed25519.Verify.
	sig, err := base64.StdEncoding.DecodeString(fact.Signature)
	require.NoError(t, err)
	msg, err := canonicaljson.MarshalForSigning(&fact)
	require.NoError(t, err)
	assert.True(t, ed25519.Verify(pub, msg, sig), "signature must verify against service_pubkey")
}

func TestSign_Deterministic(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	f1 := FactFromResult("L1", "h", "e", Binary{}, sampleResult())
	f2 := FactFromResult("L1", "h", "e", Binary{}, sampleResult())
	require.NoError(t, f1.Sign(priv))
	require.NoError(t, f2.Sign(priv))
	assert.Equal(t, f1.Signature, f2.Signature, "canonical signing must be deterministic")
}

func TestPostRehearsalResults(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	fact := FactFromResult("L1", "h", "e", Binary{}, sampleResult())
	require.NoError(t, fact.Sign(priv))

	var gotMethod, gotPath, gotAuth, gotCT string
	var got ResultFact
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "ops-token", srv.Client())
	require.NoError(t, c.PostRehearsalResults(context.Background(), "L1", fact))

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/launches/L1/rehearsal-results", gotPath)
	assert.Equal(t, "Bearer ops-token", gotAuth)
	assert.Equal(t, "application/json", gotCT)
	assert.Equal(t, "PASS", got.Outcome)
	assert.NotEmpty(t, got.Signature)
}
