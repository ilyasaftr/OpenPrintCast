package winspool

import (
	"context"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// The tests below run the production walker against a minimal SNMP agent on
// the loopback interface: they prove the gosnmp wiring (versions, community,
// GETBULK and GETNEXT walks, fallback, cancellation) without a network.

const agentCommunity = "printer-ro"

// fakeAgent answers GETNEXT and GETBULK requests from a sorted MIB. A v1
// agent ignores SNMPv2c messages, as old agents do; every agent ignores
// requests with another community.
type fakeAgent struct {
	conn *net.UDPConn
	mib  []gosnmp.SnmpPDU
	v1   bool
	mu   sync.Mutex
	seen map[gosnmp.SnmpVersion]int
}

// startFakeAgent serves mib, which must be sorted by OID.
func startFakeAgent(t *testing.T, mib []gosnmp.SnmpPDU, v1Only bool) *fakeAgent {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Skipf("no loopback UDP: %v", err)
	}
	agent := &fakeAgent{conn: conn, mib: mib, v1: v1Only, seen: map[gosnmp.SnmpVersion]int{}}
	go agent.serve()
	t.Cleanup(func() { _ = conn.Close() })
	return agent
}

func (a *fakeAgent) port() uint16 { return uint16(a.conn.LocalAddr().(*net.UDPAddr).Port) }

func (a *fakeAgent) requestsSeen(version gosnmp.SnmpVersion) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.seen[version]
}

func (a *fakeAgent) serve() {
	buf := make([]byte, 65535)
	for {
		n, from, err := a.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		decoder := &gosnmp.GoSNMP{Version: gosnmp.Version2c, Community: agentCommunity}
		request, err := decoder.SnmpDecodePacket(buf[:n])
		if err != nil || request.Community != agentCommunity || (a.v1 && request.Version != gosnmp.Version1) {
			continue
		}
		a.mu.Lock()
		a.seen[request.Version]++
		a.mu.Unlock()
		if reply, err := a.reply(request); err == nil {
			_, _ = a.conn.WriteToUDP(reply, from)
		}
	}
}

func (a *fakeAgent) reply(request *gosnmp.SnmpPacket) ([]byte, error) {
	count := 1
	if request.PDUType == gosnmp.GetBulkRequest {
		count = int(request.MaxRepetitions)
	}
	var out []gosnmp.SnmpPDU
	after := request.Variables[0].Name
	for _, entry := range a.mib {
		if compareOIDs(entry.Name, after) > 0 && len(out) < count {
			out = append(out, entry)
		}
	}
	if len(out) == 0 {
		out = []gosnmp.SnmpPDU{{Name: after, Type: gosnmp.EndOfMibView}}
	}
	encoder := &gosnmp.GoSNMP{Version: request.Version, Community: request.Community}
	encoder.SetRequestID(request.RequestID - 1)
	return encoder.SnmpEncodePacket(gosnmp.GetResponse, out, 0, 0)
}

// compareOIDs orders dotted OIDs numerically.
func compareOIDs(a, b string) int {
	as, bs := strings.Split(strings.TrimPrefix(a, "."), "."), strings.Split(strings.TrimPrefix(b, "."), ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, _ := strconv.Atoi(as[i])
		y, _ := strconv.Atoi(bs[i])
		if x != y {
			return x - y
		}
	}
	return len(as) - len(bs)
}

// agentMIB turns the fake walker's tables into the agent's MIB, followed by
// one value outside both tables, as every real MIB has.
func agentMIB(f *fakeWalker) []gosnmp.SnmpPDU {
	var mib []gosnmp.SnmpPDU
	for _, table := range f.tables {
		for _, v := range table {
			pdu := gosnmp.SnmpPDU{Name: "." + v.OID, Type: gosnmp.Integer, Value: v.Integer}
			if v.IsText {
				pdu.Type, pdu.Value = gosnmp.OctetString, []byte(v.Text)
			}
			mib = append(mib, pdu)
		}
	}
	mib = append(mib, gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.43.18.1.1.2.1.1", Type: gosnmp.Integer, Value: 3})
	slices.SortFunc(mib, func(a, b gosnmp.SnmpPDU) int { return compareOIDs(a.Name, b.Name) })
	return mib
}

func loopbackWalker(agent *fakeAgent, community string) *gosnmpWalker {
	return &gosnmpWalker{
		host: "127.0.0.1", port: agent.port(), community: community, timeout: 100 * time.Millisecond,
	}
}

func readFrom(t *testing.T, w *gosnmpWalker) ([]core.Supply, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return readSupplies(ctx, w, 1)
}

func TestGosnmpWalkerReadsWithV2cBulk(t *testing.T) {
	agent := startFakeAgent(t, agentMIB(cmykPrinter()), false)
	walker := loopbackWalker(agent, agentCommunity)
	got, err := readFrom(t, walker)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || got[0].Color != "black" || got[0].Level != 56 || !got[4].Receptacle || got[4].Level != 67 {
		t.Fatalf("got %+v", got)
	}
	if agent.requestsSeen(gosnmp.Version1) != 0 || agent.requestsSeen(gosnmp.Version2c) == 0 {
		t.Fatalf("v1=%d v2c=%d: the walk should use v2c only", agent.requestsSeen(gosnmp.Version1), agent.requestsSeen(gosnmp.Version2c))
	}
}

func TestGosnmpWalkerFallsBackToV1AndRemembersIt(t *testing.T) {
	agent := startFakeAgent(t, agentMIB(cmykPrinter()), true)
	walker := loopbackWalker(agent, agentCommunity)
	got, err := readFrom(t, walker)
	if err != nil || len(got) != 5 {
		t.Fatalf("%d supplies, %v", len(got), err)
	}
	if walker.version != gosnmp.Version1 {
		t.Fatalf("walker did not remember SNMPv1: %v", walker.version)
	}
	if v2c := agent.requestsSeen(gosnmp.Version2c); v2c != 0 {
		t.Fatalf("the v1-only agent saw %d v2c requests", v2c)
	}
}

func TestGosnmpWalkerWithWrongCommunityReadsNothing(t *testing.T) {
	agent := startFakeAgent(t, agentMIB(cmykPrinter()), false)
	got, err := readFrom(t, loopbackWalker(agent, "public"))
	if err == nil || got != nil {
		t.Fatalf("got %+v, %v: a printer that does not answer must give an error and no levels", got, err)
	}
	if strings.Contains(err.Error(), "public") {
		t.Fatalf("the error carries the community string: %v", err)
	}
	if agent.requestsSeen(gosnmp.Version1)+agent.requestsSeen(gosnmp.Version2c) != 0 {
		t.Fatal("the agent accepted a request with the wrong community")
	}
}

func TestGosnmpWalkerStopsWhenCancelled(t *testing.T) {
	agent := startFakeAgent(t, agentMIB(cmykPrinter()), false)
	walker := loopbackWalker(agent, "wrong")
	walker.timeout = 2 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	started := time.Now()
	if _, err := walker.Walk(ctx, oidMarkerSuppliesEntry); err == nil {
		t.Fatal("cancelled walk succeeded")
	}
	if time.Since(started) > time.Second {
		t.Fatalf("cancellation took %v", time.Since(started))
	}
}

func TestToSNMPVar(t *testing.T) {
	for name, tc := range map[string]struct {
		pdu  gosnmp.SnmpPDU
		want snmpVar
	}{
		"integer":  {gosnmp.SnmpPDU{Name: ".1.2.3", Value: 5}, snmpVar{OID: "1.2.3", Integer: 5}},
		"gauge":    {gosnmp.SnmpPDU{Name: ".1.2.3", Value: uint(7)}, snmpVar{OID: "1.2.3", Integer: 7}},
		"octets":   {gosnmp.SnmpPDU{Name: ".1.2.3", Value: []byte("cyan")}, snmpVar{OID: "1.2.3", Text: "cyan", IsText: true}},
		"no value": {gosnmp.SnmpPDU{Name: ".1.2.3"}, snmpVar{OID: "1.2.3", IsText: true}},
	} {
		if got := toSNMPVar(tc.pdu); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", name, got, tc.want)
		}
	}
}
