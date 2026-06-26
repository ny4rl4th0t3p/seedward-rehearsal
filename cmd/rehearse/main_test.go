package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ny4rl4th0t3p/seedward-gentool/pkg/rehearse"
)

func TestExitCode(t *testing.T) {
	assert.Equal(t, exitPass, exitCode(rehearse.OutcomePass))
	assert.Equal(t, exitFail, exitCode(rehearse.OutcomeFail))
	assert.Equal(t, exitError, exitCode(rehearse.OutcomeError))
}

func TestReadGentxs(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"a":1}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{"b":2}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("x"), 0o600))

	gentxs, err := readGentxs(dir)
	require.NoError(t, err)
	assert.Len(t, gentxs, 2)

	_, err = readGentxs("")
	require.Error(t, err)

	_, err = readGentxs(filepath.Join(dir, "empty"))
	require.Error(t, err, "missing dir should error")
}
