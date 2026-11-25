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
	// faultSettleLost makes a structure settle never reach the service, as when
	// it is slow past the deadline or restarting; rel names the first step's source.
	faultSettleLost = "settle-lost"
)
