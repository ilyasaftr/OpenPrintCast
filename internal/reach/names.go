package reach

// String names a state for logs and diagnostics.
func (s State) String() string {
	switch s {
	case StateOK:
		return "ok"
	case StateBlocked:
		return "blocked"
	case StateNotListening:
		return "not listening"
	}
	return "unknown"
}

// String names a cause for logs and diagnostics.
func (c Cause) String() string {
	switch c {
	case CauseNone:
		return "none"
	case CausePublicNetwork:
		return "public network"
	case CauseNoRule:
		return "no firewall rule"
	case CauseBlockRule:
		return "block rule"
	case CauseOtherProgramOnPort:
		return "port used by another program"
	case CauseBindFailed:
		return "could not listen"
	}
	return "unknown"
}
