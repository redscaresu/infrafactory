package test.state

import rego.v1

deny_state contains "public database endpoint detected" if {
	input.rdb.public_endpoint == true
}
