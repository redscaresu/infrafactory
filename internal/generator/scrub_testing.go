package generator

import "testing"

// ResetScrubbedAccountsForTest clears the account registry now and again
// when t ends, so a test that registers an id cannot leak it into the
// next. For tests only; a test that registers must not run in parallel.
func ResetScrubbedAccountsForTest(t testing.TB) {
	t.Helper()
	resetScrubbedAccounts()
	t.Cleanup(resetScrubbedAccounts)
}

func resetScrubbedAccounts() {
	accountsMu.Lock()
	defer accountsMu.Unlock()
	knownAccountIDs = nil
	knownAccountPattern.Store(nil)
}
