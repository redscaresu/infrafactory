package cli

// rehearsalModuleProblems holds a module block to the module source rule
// (ADR-0042 Decision 2). Closed: every module block is refused.
func rehearsalModuleProblems(at string) []string {
	return []string{at + ": refused: the gate admits no module block"}
}
