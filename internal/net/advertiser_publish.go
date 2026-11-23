package net

import (
	"fmt"

	stdnet "net"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	"github.com/ilyasaftr/OpenPrintCast/internal/zeroconf"
)

// registration is everything zeroconf needs to publish one printer, shared
// for its IPP service.
type registration struct {
	instance string
	txt      []string
	ifaces   []stdnet.Interface
	options  []zeroconf.ServerOption
}

// Publish advertises one printer under its display name.
func (a *Advertiser) Publish(p core.PrinterInfo) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.registered[p.ID] {
		return nil
	}
	// BuildAirPrintTXT derives adminurl from its ip argument; pass the
	// LAN address here rather than re-editing the flattened keys.
	reg := registration{
		txt:      flattenTXT(core.BuildAirPrintTXT(p, PrinterUUID(p.ID, a.identityHost), a.host, a.adminHost(), a.port)),
		ifaces:   a.ifaces(),
		instance: instanceName(p.DisplayName(), p.ID, a.host, a.names[p.ID]),
		options:  a.serverOptions(p.ID),
	}
	if err := a.registerService(reg); err != nil {
		return err
	}
	a.registered[p.ID] = true
	if a.instances == nil {
		a.instances = make(map[string]string)
	}
	a.instances[p.ID] = instanceLabel(p.DisplayName(), p.ID, a.host, a.names[p.ID])
	componentLogger(componentAdvertise).Info("advertising printer", "printer", p.ID, "instance", reg.instance,
		"host", a.host+".local", "port", a.port, "services", "_ipp,_universal")
	return nil
}

// serverOptions pins the host name and routes the printer's name and host
// conflicts to the conflict queue, tagged with the current generation.
func (a *Advertiser) serverOptions(id string) []zeroconf.ServerOption {
	notice := instanceConflict{id: id, generation: a.generation}
	hostNotice := notice
	hostNotice.host = true
	return []zeroconf.ServerOption{
		zeroconf.Hostname(a.host),
		zeroconf.OnConflict(func() { a.notifyConflict(notice) }),
		zeroconf.OnHostConflict(func() { a.notifyConflict(hostNotice) }),
	}
}

// notifyConflict queues notice without blocking; a full queue drops it.
func (a *Advertiser) notifyConflict(notice instanceConflict) {
	select {
	case a.conflicts <- notice:
	default:
	}
}

// registerService starts the IPP service. Callers hold mu.
func (a *Advertiser) registerService(reg registration) error {
	srv, err := zeroconf.Register(reg.instance, browseService(), Domain, a.port, reg.txt, reg.ifaces, reg.options...)
	if err != nil {
		return fmt.Errorf("register %s: %w", browseService(), err)
	}
	a.servers = append(a.servers, srv)
	return nil
}

// Withdraw unregisters every advertised printer.
func (a *Advertiser) Withdraw() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, srv := range a.servers {
		srv.Shutdown()
	}
	a.servers = nil
	a.registered = make(map[string]bool)
	a.instances = make(map[string]string)
	a.generation++
	a.sig = ""
}

func (a *Advertiser) advertisedName(printer core.PrinterInfo) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.registered[printer.ID] {
		return ""
	}
	return a.instances[printer.ID]
}
