package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Client calls coordd's rehearsal bridge endpoints. Every call carries an
// op credential as a bearer token; coordd never calls back, so this is the only
// direction.
type Client struct {
	baseURL  string
	opsToken string
	hc       *http.Client
}

// NewClient returns a bridge client for the given coordd base URL and op token. A nil hc uses
// http.DefaultClient.
func NewClient(baseURL, opsToken string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), opsToken: opsToken, hc: hc}
}

// GetRehearsalInput fetches the approved input set for a launch.
func (c *Client) GetRehearsalInput(ctx context.Context, launchID string) (RehearsalInput, error) {
	url := fmt.Sprintf("%s/launches/%s/rehearsal-input", c.baseURL, launchID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return RehearsalInput{}, err
	}
	c.authorize(req)

	resp, err := c.hc.Do(req)
	if err != nil {
		return RehearsalInput{}, fmt.Errorf("get rehearsal-input: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return RehearsalInput{}, fmt.Errorf("get rehearsal-input: coordd returned %s", resp.Status)
	}

	var in RehearsalInput
	if err := json.NewDecoder(resp.Body).Decode(&in); err != nil {
		return RehearsalInput{}, fmt.Errorf("decode rehearsal-input: %w", err)
	}
	return in, nil
}

func (c *Client) authorize(req *http.Request) {
	if c.opsToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.opsToken)
	}
}
