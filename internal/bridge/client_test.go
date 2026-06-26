package bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRehearsalInput(t *testing.T) {
	const body = `{
		"schema_version": 1,
		"launch_id": "L1",
		"chain": {"chain_id": "test-1", "bech32_prefix": "cosmos", "denom": "uatom", "total_supply": "7000000"},
		"gentxs": [],
		"allocations": {},
		"input_set_hash": "hash123"
	}`

	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient(srv.URL+"/", "ops-token", srv.Client())
	in, err := c.GetRehearsalInput(context.Background(), "L1")
	require.NoError(t, err)

	assert.Equal(t, "/launches/L1/rehearsal-input", gotPath)
	assert.Equal(t, "Bearer ops-token", gotAuth)
	assert.Equal(t, "test-1", in.Chain.ChainID)
	assert.Equal(t, "7000000", in.Chain.TotalSupply)
	assert.Equal(t, "hash123", in.InputSetHash)
}

func TestGetRehearsalInput_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", srv.Client())
	_, err := c.GetRehearsalInput(context.Background(), "L1")
	require.Error(t, err)
}
