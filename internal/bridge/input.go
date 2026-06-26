// Package bridge is the coordd-connected daemon's client for the chaincoord ↔ rehearsal bridge:
// it pulls the approved rehearsal input and posts back the signed result fact.
// It is used only by cmd/rehearsald — the standalone cmd/rehearse never touches coordd.
package bridge

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/ny4rl4th0t3p/seedward-gentool/pkg/genesis"
	"github.com/ny4rl4th0t3p/seedward-gentool/pkg/rehearse"
)

// RehearsalInput is the read payload: the complete approved build input for one launch.
type RehearsalInput struct {
	SchemaVersion int                   `json:"schema_version"`
	LaunchID      string                `json:"launch_id"`
	GeneratedAt   string                `json:"generated_at"`
	Chain         Chain                 `json:"chain"`
	Gentxs        []Gentx               `json:"gentxs"`
	Allocations   map[string]Allocation `json:"allocations"`
	InputSetHash  string                `json:"input_set_hash"`
}

// Chain mirrors the chain record (from launch.ChainRecord).
type Chain struct {
	ChainID                 string  `json:"chain_id"`
	Bech32Prefix            string  `json:"bech32_prefix"`
	Denom                   string  `json:"denom"`
	TotalSupply             string  `json:"total_supply"` // bigint string, base denom (D5)
	MinSelfDelegation       string  `json:"min_self_delegation"`
	MaxCommissionRate       string  `json:"max_commission_rate"`
	MaxCommissionChangeRate string  `json:"max_commission_change_rate"`
	MinValidatorCount       int     `json:"min_validator_count"`
	GenesisTime             *string `json:"genesis_time"` // RFC3339 or null
	Binary                  Binary  `json:"binary"`
}

// Binary is the chain binary reference (the path is service-local, not in the payload).
type Binary struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	SHA256     string `json:"sha256"`
	RepoURL    string `json:"repo_url"`
	RepoCommit string `json:"repo_commit"`
}

// Gentx is one approved join request's gentx and its extracted fields.
type Gentx struct {
	OperatorAddress string          `json:"operator_address"`
	ConsensusPubkey string          `json:"consensus_pubkey"`
	Moniker         string          `json:"moniker"`
	SelfDelegation  string          `json:"self_delegation"`
	Gentx           json.RawMessage `json:"gentx"` // joinrequest.GentxJSON, verbatim
}

// Allocation is one committee-approved allocation file.
type Allocation struct {
	SHA256             string `json:"sha256"`
	ApprovedByProposal string `json:"approved_by_proposal"`
	ContentB64         string `json:"content_b64"`
}

// allocationTypes maps the payload's allocation keys to the engine's AllocationType.
var allocationTypes = map[string]rehearse.AllocationType{
	"accounts": rehearse.AllocationAccounts,
	"claims":   rehearse.AllocationClaims,
	"grants":   rehearse.AllocationGrants,
	"authz":    rehearse.AllocationAuthz,
	"feegrant": rehearse.AllocationFeegrant,
}

// ToInput maps the bridge payload onto the engine's Input. binaryPath is the service-local,
// pre-provisioned chaind binary (the engine verifies it against chain.binary.sha256). Only the
// fields the contract carries are set; the rest fall back to the engine/binary defaults, which
// the assertion suite SKIPs.
func (ri RehearsalInput) ToInput(binaryPath string) (rehearse.Input, error) {
	supply, err := strconv.ParseInt(ri.Chain.TotalSupply, 10, 64)
	if err != nil {
		return rehearse.Input{}, fmt.Errorf("parse total_supply %q: %w", ri.Chain.TotalSupply, err)
	}

	cfg := genesis.ChainConfig{
		ChainID:       ri.Chain.ChainID,
		AddressPrefix: ri.Chain.Bech32Prefix,
		BondDenom:     ri.Chain.Denom,
		TotalSupply:   supply,
	}
	if ri.Chain.GenesisTime != nil && *ri.Chain.GenesisTime != "" {
		t, err := time.Parse(time.RFC3339, *ri.Chain.GenesisTime)
		if err != nil {
			return rehearse.Input{}, fmt.Errorf("parse genesis_time %q: %w", *ri.Chain.GenesisTime, err)
		}
		cfg.GenesisTime = t.Unix()
	}

	gentxs := make([][]byte, 0, len(ri.Gentxs))
	for _, g := range ri.Gentxs {
		gentxs = append(gentxs, []byte(g.Gentx))
	}

	alloc := make(map[rehearse.AllocationType][]byte, len(ri.Allocations))
	for key, typ := range allocationTypes {
		a, ok := ri.Allocations[key]
		if !ok {
			continue
		}
		content, err := base64.StdEncoding.DecodeString(a.ContentB64)
		if err != nil {
			return rehearse.Input{}, fmt.Errorf("decode %s allocation: %w", key, err)
		}
		alloc[typ] = content
	}

	return rehearse.Input{
		Config:       cfg,
		Allocations:  alloc,
		Gentxs:       gentxs,
		BinaryPath:   binaryPath,
		BinarySHA256: ri.Chain.Binary.SHA256,
	}, nil
}
