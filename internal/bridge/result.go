package bridge

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/ny4rl4th0t3p/seedward-gentool/pkg/rehearse"
	"github.com/ny4rl4th0t3p/seedward-libs/canonicaljson"
)

// ResultStep is one step verdict in the result fact (§4).
type ResultStep struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// RehearsalMeta is the §4 rehearsal block: what actually ran.
type RehearsalMeta struct {
	EngineVersion  string `json:"engine_version"`
	BinaryName     string `json:"binary_name"`
	BinaryVersion  string `json:"binary_version"`
	BinarySHA256   string `json:"binary_sha256"`
	Validators     int    `json:"validators"`
	BlocksAdvanced int    `json:"blocks_advanced"`
}

// ResultFact is the §4 write-back payload: the signed rehearsal verdict.
type ResultFact struct {
	SchemaVersion int     `json:"schema_version"`
	LaunchID      string  `json:"launch_id"`
	InputSetHash  string  `json:"input_set_hash"`
	AttemptID     *string `json:"attempt_id"` // null for the v1 manual flow

	Outcome    string       `json:"outcome"`
	FailedStep *string      `json:"failed_step"` // null on PASS
	Summary    string       `json:"summary"`
	Steps      []ResultStep `json:"steps"`

	Rehearsal RehearsalMeta `json:"rehearsal"`

	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`

	ServicePubkey string `json:"service_pubkey"`
	Signature     string `json:"signature"`
}

// FactFromResult assembles an unsigned ResultFact from an engine Result and the run's metadata.
// bin is the binary reference from the rehearsal input; engineVersion identifies the engine.
func FactFromResult(launchID, inputSetHash, engineVersion string, bin Binary, res *rehearse.Result) ResultFact {
	fact := ResultFact{
		SchemaVersion: 1,
		LaunchID:      launchID,
		InputSetHash:  inputSetHash,
		Outcome:       string(res.Outcome),
		Summary:       res.Summary,
		Rehearsal: RehearsalMeta{
			EngineVersion: engineVersion,
			BinaryName:    bin.Name,
			BinaryVersion: bin.Version,
			BinarySHA256:  bin.SHA256,
			Validators:    res.Validators,
			// BlocksAdvanced: the engine gates on height ≥ 1 but does not yet surface the
			// height reached; left 0 until rehearse.Result carries it.
		},
		StartedAt:  res.StartedAt.UTC().Format(time.RFC3339),
		FinishedAt: res.FinishedAt.UTC().Format(time.RFC3339),
	}
	if res.FailedStep != "" {
		fs := res.FailedStep
		fact.FailedStep = &fs
	}
	for _, s := range res.Steps {
		fact.Steps = append(fact.Steps, ResultStep{Name: s.Name, Status: string(s.Status), Detail: s.Detail})
	}
	return fact
}

// Sign sets ServicePubkey and Signature on the fact, mirroring auditlog.Append:
// canonical JSON with signature stripped, Ed25519-signed, base64-encoded. canonicaljson is the
// shared implementation coordd verifies against, so the bytes match exactly.
func (f *ResultFact) Sign(priv ed25519.PrivateKey) error {
	f.Signature = ""
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return fmt.Errorf("service key is not an ed25519 public key")
	}
	f.ServicePubkey = base64.StdEncoding.EncodeToString(pub)

	msg, err := canonicaljson.MarshalForSigning(f)
	if err != nil {
		return fmt.Errorf("canonicalize fact: %w", err)
	}
	f.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg))
	return nil
}
