package loader.policy_test

# v1 syntax without `import rego.v1`: it parses under v1 only, so the v0-compat
# loader fails on it. The deny rules would surface if its package were queried.

deny contains "test package queried"

deny_state contains "test package queried"

test_policy_denies if {
	data.loader.policy.deny
}
