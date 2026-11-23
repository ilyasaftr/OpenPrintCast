package net

type instanceConflict struct {
	id         string
	generation uint64
	host       bool
}

func (a *Advertiser) markConflict(notice instanceConflict) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if notice.generation != a.generation || !a.registered[notice.id] {
		return false
	}
	if notice.host {
		a.renameHost()
	} else {
		a.names[notice.id]++
	}
	a.sig = ""
	return true
}

// renameHost picks the next conflict-free host label. Callers hold mu.
func (a *Advertiser) renameHost() {
	a.hostAttempts++
	a.host = conflictHost(a.identityHost, a.ip+nulSeparator+a.network, a.hostAttempts)
	componentLogger(componentAdvertise).Info("resolving host-name conflict", "host", a.host+".local")
}
