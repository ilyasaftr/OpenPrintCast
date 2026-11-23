package net

import (
	"sync"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// printerIdentity is the host and DNS-SD instance naming the advertiser
// publishes for the IPP handler to mirror in its attributes.
type printerIdentity struct {
	identityMu   sync.RWMutex
	identityHost string
	instanceName func(core.PrinterInfo) string
}

func (s *printerIdentity) setIdentityHost(host string) {
	s.identityMu.Lock()
	s.identityHost = host
	s.identityMu.Unlock()
}

func (s *printerIdentity) setInstanceName(name func(core.PrinterInfo) string) {
	s.identityMu.Lock()
	s.instanceName = name
	s.identityMu.Unlock()
}

// advertisedInstanceName is the DNS-SD instance name published for printer,
// or "" when none has been assigned yet.
func (s *printerIdentity) advertisedInstanceName(printer core.PrinterInfo) string {
	s.identityMu.RLock()
	name := s.instanceName
	s.identityMu.RUnlock()
	if name == nil {
		return ""
	}
	return name(printer)
}

func (s *printerIdentity) printerIdentityHost(fallback string) string {
	s.identityMu.RLock()
	defer s.identityMu.RUnlock()
	if s.identityHost != "" {
		return s.identityHost
	}
	return fallback
}
