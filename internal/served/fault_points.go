package served

// The trial's fault points (P9, P12). A harness arms one through
// POST /v1/fault, and the next matching write takes it. Nothing arms a fault
// unless a harness asks.
const (
	faultKillBeforeRename = "kill-before-rename"
	faultKillAfterRename  = "kill-after-rename"
	faultDurableFail      = "durable-fail"
	faultDropReply        = "drop-reply"
	faultTailwindHold     = "tailwind-hold"
)
