package net

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	"github.com/ilyasaftr/OpenPrintCast/internal/zeroconf"
)

// DNS-SD constants for AirPrint advertisement.
const (
	// ServiceType is the DNS-SD type browsed by IPP clients.
	ServiceType = "_ipp._tcp"
	// AirPrintSubtype is the subtype browsed specifically by iOS AirPrint.
	AirPrintSubtype = "_universal"
	// Domain is the mDNS domain services register into.
	Domain = "local."

	// conflictQueueSize bounds queued conflict notices; extras are dropped.
	conflictQueueSize = 64
)

func browseService() string { return ServiceType + "," + AirPrintSubtype }

// Advertiser owns one zeroconf registration per advertised printer (base
// service with the AirPrint subtype) and tracks them so sharing can be
// withdrawn on pause or shutdown.
type Advertiser struct {
	mu           sync.Mutex
	port         int
	host         string
	identityHost string
	hostAttempts int
	ip           string
	network      string
	snapshot     func() (string, string)
	servers      []*zeroconf.Server
	registered   map[string]bool
	instances    map[string]string
	names        map[string]int
	generation   uint64
	conflicts    chan instanceConflict
	sig          string
}

// NewAdvertiser records the port to advertise; host and LAN IP resolve
// from the machine. No network happens until RegisterAll/Publish.
func NewAdvertiser(port int) *Advertiser {
	ip, network := networkSnapshot()
	host := HostLabel()
	return &Advertiser{
		port:         port,
		host:         host,
		identityHost: host,
		ip:           ip,
		network:      network,
		snapshot:     networkSnapshot,
		registered:   make(map[string]bool),
		instances:    make(map[string]string),
		names:        make(map[string]int),
		conflicts:    make(chan instanceConflict, conflictQueueSize),
	}
}

// Host is the mDNS host label services register under.
func (a *Advertiser) Host() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.host
}

// adminHost is the address iOS must reach: the LAN IP, or host.local
// when no route exists.
func (a *Advertiser) adminHost() string {
	if a.ip != "" {
		return a.ip
	}
	return a.host + ".local"
}

// RegisterAll advertises every printer in printers, skipping ones
// already registered so re-advertising is idempotent per printer.
func (a *Advertiser) RegisterAll(printers []core.PrinterInfo) error {
	for _, p := range printers {
		if err := a.Publish(p); err != nil {
			return err
		}
	}
	a.mu.Lock()
	a.sig = advertSignature(printers)
	a.mu.Unlock()
	return nil
}

// advertSignature fingerprints everything a printer's mDNS record is
// built from, so Resync can skip edits that change nothing advertised.
func advertSignature(printers []core.PrinterInfo) string {
	var sb strings.Builder
	for _, p := range printers {
		fmt.Fprintf(&sb, "%s\x00%s\x00%s\x00%s\x00%t\x00%t\x01", p.ID, p.DisplayName(), p.Location, p.WindowsName, p.SupportsColor(), p.SupportsDuplex())
	}
	return sb.String()
}

// Resync makes the advertised set match printers: it withdraws everything
// and re-registers, unless nothing advertised changed. It reports whether
// the network was touched.
func (a *Advertiser) Resync(printers []core.PrinterInfo) (bool, error) {
	ip, network := a.snapshot()
	a.mu.Lock()
	same := a.sig == advertSignature(printers) && a.ip == ip && a.network == network
	a.mu.Unlock()
	if same {
		return false, nil
	}
	a.Withdraw()
	a.mu.Lock()
	a.ip = ip
	a.mu.Unlock()
	if err := a.RegisterAll(printers); err != nil {
		return true, err
	}
	a.mu.Lock()
	a.network = network
	a.mu.Unlock()
	return true, nil
}
