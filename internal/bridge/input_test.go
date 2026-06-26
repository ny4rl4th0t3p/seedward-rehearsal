package bridge

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ny4rl4th0t3p/seedward-gentool/pkg/rehearse"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestToInput_MapsPayload(t *testing.T) {
	gt := "2026-01-01T00:00:00Z"
	ri := RehearsalInput{
		SchemaVersion: 1,
		LaunchID:      "L1",
		Chain: Chain{
			ChainID:      "test-1",
			Bech32Prefix: "cosmos",
			Denom:        "uatom",
			TotalSupply:  "7000000",
			GenesisTime:  &gt,
			Binary:       Binary{SHA256: "abc123"},
		},
		Gentxs: []Gentx{
			{OperatorAddress: "cosmosvaloper1aaa", Gentx: json.RawMessage(`{"a":1}`)},
			{OperatorAddress: "cosmosvaloper1bbb", Gentx: json.RawMessage(`{"b":2}`)},
		},
		Allocations: map[string]Allocation{
			"accounts": {ContentB64: b64("addr,100\n")},
			"claims":   {ContentB64: b64("c,200,val\n")},
		},
		InputSetHash: "hash123",
	}

	in, err := ri.ToInput("/bin/chaind")
	require.NoError(t, err)

	assert.Equal(t, "test-1", in.Config.ChainID)
	assert.Equal(t, "cosmos", in.Config.AddressPrefix)
	assert.Equal(t, "uatom", in.Config.BondDenom)
	assert.Equal(t, int64(7000000), in.Config.TotalSupply)
	want, _ := time.Parse(time.RFC3339, gt)
	assert.Equal(t, want.Unix(), in.Config.GenesisTime)

	assert.Equal(t, "/bin/chaind", in.BinaryPath)
	assert.Equal(t, "abc123", in.BinarySHA256)

	require.Len(t, in.Gentxs, 2)
	assert.JSONEq(t, `{"a":1}`, string(in.Gentxs[0]))
	assert.JSONEq(t, `{"b":2}`, string(in.Gentxs[1]))

	assert.Equal(t, "addr,100\n", string(in.Allocations[rehearse.AllocationAccounts]))
	assert.Equal(t, "c,200,val\n", string(in.Allocations[rehearse.AllocationClaims]))
	_, hasGrants := in.Allocations[rehearse.AllocationGrants]
	assert.False(t, hasGrants, "absent allocation key must not appear")
}

func TestToInput_NoGenesisTime(t *testing.T) {
	ri := RehearsalInput{Chain: Chain{Bech32Prefix: "cosmos", Denom: "uatom", TotalSupply: "1"}}
	in, err := ri.ToInput("/bin/chaind")
	require.NoError(t, err)
	assert.Zero(t, in.Config.GenesisTime)
}

func TestToInput_Errors(t *testing.T) {
	t.Run("bad total_supply", func(t *testing.T) {
		_, err := RehearsalInput{Chain: Chain{TotalSupply: "notanint"}}.ToInput("/bin/chaind")
		require.Error(t, err)
	})
	t.Run("bad genesis_time", func(t *testing.T) {
		bad := "not-a-time"
		_, err := RehearsalInput{Chain: Chain{TotalSupply: "1", GenesisTime: &bad}}.ToInput("/bin/chaind")
		require.Error(t, err)
	})
	t.Run("bad base64 allocation", func(t *testing.T) {
		ri := RehearsalInput{
			Chain:       Chain{TotalSupply: "1"},
			Allocations: map[string]Allocation{"accounts": {ContentB64: "!!!not-base64!!!"}},
		}
		_, err := ri.ToInput("/bin/chaind")
		require.Error(t, err)
	})
}
