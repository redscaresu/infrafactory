package cli

const sandboxRealDeploySkippedMessage = "(real deployment skipped for cost reasons for now)"

func sandboxDeferredDetail() string {
	return "sandbox/live deploy layer is intentionally deferred due cost/credentials policy " + sandboxRealDeploySkippedMessage
}

func sandboxBlockedStageDetail() string {
	return "layer enabled but implementation is deferred " + sandboxRealDeploySkippedMessage
}

