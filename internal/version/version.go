// Package version carries the build-injected version string. It defaults to "dev" for a plain
// `go build`/`go run`; release builds set it via the Makefile's ldflags:
//
//	-ldflags "-X github.com/ny4rl4th0t3p/seedward-rehearsal/internal/version.Version=<v>"
package version

// Version is the seedward-rehearsal build version. It identifies the build that produced a
// rehearsal result (reported as engine_version in the result fact); since go.mod pins the engine
// (seedward-gentool/pkg/rehearse), this build version transitively identifies the engine too.
var Version = "dev"
