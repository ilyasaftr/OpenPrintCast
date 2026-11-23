package reach

import (
	"context"
	"net"
	"os"
	"time"
)

// CollectTimeout bounds one collection of the Windows facts.
const CollectTimeout = 10 * time.Second

// Collector reads the Windows side: connected networks, firewall profiles,
// rules and the owner of the port. It fails when it cannot tell.
type Collector interface {
	Collect(ctx context.Context) (Facts, error)
}

// Checker runs a check: it collects the facts, adds what the app knows and
// classifies the whole.
type Checker struct {
	Collector Collector
	// Listen reports the server's own state.
	Listen func() Listen
	// LocalIP is the LAN address devices reach this PC on ("" when none).
	LocalIP func() string
	Port    int
}

// Check never fails: whatever it could not collect becomes StateUnknown.
func (c Checker) Check(ctx context.Context) Assessment {
	in := Inputs{Port: c.Port, OwnPID: os.Getpid(), Listen: c.Listen()}
	in.ExePath, _ = os.Executable()
	in.LocalNet = localNet(c.LocalIP())
	ctx, cancel := context.WithTimeout(ctx, CollectTimeout)
	defer cancel()
	facts, err := c.Collector.Collect(ctx)
	if err != nil {
		if in.Listen.Attempted && !in.Listen.Listening {
			return Assess(in) // the server's own failure needs no Windows facts
		}
		return unknown(err.Error())
	}
	in.Facts = facts
	return Assess(in)
}

// localNet finds the subnet of the interface holding ip, nil when unknown.
func localNet(ip string) *net.IPNet {
	want := net.ParseIP(ip)
	if want == nil {
		return nil
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.Equal(want) {
			return &net.IPNet{IP: n.IP.Mask(n.Mask), Mask: n.Mask}
		}
	}
	return nil
}
