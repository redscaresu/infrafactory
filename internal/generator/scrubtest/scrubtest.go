// Package scrubtest isolates tests that register scrubbed accounts. Only
// _test.go files import it, so the shipped binary links neither it nor
// the testing package.
package scrubtest

import (
	"testing"

	"github.com/redscaresu/infrafactory/internal/generator"
)

// Register registers ids for the rest of t and puts the registry back
// when t ends. With no ids it only snapshots, for a test whose code under
// test registers (buildRuntime). A test using it must not run in parallel.
func Register(t testing.TB, ids ...string) {
	t.Helper()
	t.Cleanup(generator.RegisterScrubbedAccounts(ids...))
}
