package zeroconf

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestHostnameOption(t *testing.T) {
	options := applyServerOpts(Hostname("print-host"))
	if options.hostname != "print-host" || options.ttl != defaultTTL {
		t.Fatalf("options = %+v", options)
	}
}

func TestHostAddressLookup(t *testing.T) {
	s := &Server{ttl: 120}
	s.service = newServiceEntry("Printer", "_ipp._tcp,_universal", "local.")
	s.service.HostName = "print-host.local."
	s.service.AddrIPv4 = []net.IP{net.ParseIP("192.0.2.10")}
	s.service.AddrIPv6 = []net.IP{net.ParseIP("2001:db8::10")}
	for _, tc := range []struct {
		qtype uint16
		want  int
	}{{dns.TypeA, 1}, {dns.TypeAAAA, 1}, {dns.TypeANY, 2}, {dns.TypeTXT, 0}} {
		resp := &dns.Msg{}
		if err := s.handleQuestion(dns.Question{Name: "PRINT-HOST.local.", Qtype: tc.qtype, Qclass: dns.ClassINET}, resp, &dns.Msg{}, 0); err != nil {
			t.Fatal(err)
		}
		if len(resp.Answer) != tc.want {
			t.Fatalf("type %d: got %d answers, want %d", tc.qtype, len(resp.Answer), tc.want)
		}
	}
}

func TestInstanceConflictDetection(t *testing.T) {
	s := &Server{ttl: 120}
	s.service = newServiceEntry("Printer", "_ipp._tcp,_universal", "local.")
	s.service.HostName = "one.local."
	s.service.Port = 631
	s.service.Text = []string{"rp=ipp/print/one", "UUID=one"}
	srv := func(host string, port uint16) *dns.SRV {
		return &dns.SRV{
			Hdr:  dns.RR_Header{Name: s.service.ServiceInstanceName(), Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: 120},
			Port: port, Target: host,
		}
	}
	txt := func(values ...string) *dns.TXT {
		return &dns.TXT{
			Hdr: dns.RR_Header{Name: s.service.ServiceInstanceName(), Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 120},
			Txt: values,
		}
	}
	response := &dns.Msg{MsgHdr: dns.MsgHdr{Response: true}, Answer: []dns.RR{srv("ONE.local.", 631), txt("UUID=one", "rp=ipp/print/one")}}
	if s.hasInstanceConflict(response) {
		t.Fatal("identical SRV and reordered TXT must not conflict")
	}
	response.Answer = []dns.RR{srv("two.local.", 631)}
	if !s.hasInstanceConflict(response) {
		t.Fatal("different SRV target must conflict")
	}
	response.Answer = []dns.RR{txt("UUID=two", "rp=ipp/print/two")}
	if !s.hasInstanceConflict(response) {
		t.Fatal("different TXT must conflict")
	}
	probe := &dns.Msg{Ns: []dns.RR{srv("two.local.", 631)}}
	if !s.hasInstanceConflict(probe) {
		t.Fatal("competing tentative probe must conflict")
	}
	s.announced.Store(true)
	if s.hasInstanceConflict(probe) {
		t.Fatal("established service must defend against a competing probe")
	}
	called := 0
	s.onConflict = func() { called++ }
	packet, err := response.Pack()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.parsePacket(packet, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.parsePacket(packet, 0, nil); err != nil {
		t.Fatal(err)
	}
	if called != 1 || !s.conflicted.Load() {
		t.Fatalf("conflict callback called %d times", called)
	}
}

func TestHostConflictDetection(t *testing.T) {
	s := &Server{ttl: 120}
	s.service = newServiceEntry("Printer", "_ipp._tcp,_universal", "local.")
	s.service.HostName = "one.local."
	s.service.AddrIPv4 = []net.IP{net.ParseIP("192.0.2.10")}
	record := func(ip string) *dns.A {
		return &dns.A{
			Hdr: dns.RR_Header{Name: "one.local.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 120},
			A:   net.ParseIP(ip),
		}
	}
	response := &dns.Msg{MsgHdr: dns.MsgHdr{Response: true}, Answer: []dns.RR{record("192.0.2.10")}}
	if s.hasHostConflict(response) {
		t.Fatal("same host address must not conflict")
	}
	response.Answer = []dns.RR{record("192.0.2.11")}
	if !s.hasHostConflict(response) {
		t.Fatal("different host address must conflict")
	}
	called := 0
	s.onHostConflict = func() { called++ }
	packet, err := response.Pack()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.parsePacket(packet, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.parsePacket(packet, 0, nil); err != nil {
		t.Fatal(err)
	}
	if called != 1 || !s.conflicted.Load() {
		t.Fatalf("host conflict callback called %d times", called)
	}
}

func TestSimultaneousProbesHaveOneWinner(t *testing.T) {
	makeServer := func(host, txt string) *Server {
		s := &Server{ttl: 120}
		s.service = newServiceEntry("Printer", "_ipp._tcp,_universal", "local.")
		s.service.HostName = host
		s.service.Port = 631
		s.service.Text = []string{txt}
		return s
	}
	a, b := makeServer("one.local.", "UUID=one"), makeServer("two.local.", "UUID=two")
	proposal := func(s *Server) *dns.Msg {
		response := &dns.Msg{}
		s.composeLookupAnswers(response, s.ttl, 0, false)
		var records []dns.RR
		for _, rr := range response.Answer {
			if rr.Header().Rrtype == dns.TypeSRV || rr.Header().Rrtype == dns.TypeTXT {
				records = append(records, rr)
			}
		}
		return &dns.Msg{Ns: records}
	}
	aLoses := a.hasInstanceConflict(proposal(b))
	bLoses := b.hasInstanceConflict(proposal(a))
	if aLoses == bLoses {
		t.Fatalf("simultaneous probe results: a loses=%v b loses=%v", aLoses, bLoses)
	}
}

func TestSubtypeQueryAnswersWithBaseInstance(t *testing.T) {
	s := &Server{ttl: 120}
	s.service = newServiceEntry("Printer", "_ipp._tcp,_universal", "local.")
	s.service.Port = 631

	const sub = "_universal._sub._ipp._tcp.local."
	resp := &dns.Msg{}
	if err := s.handleQuestion(dns.Question{Name: sub, Qtype: dns.TypePTR, Qclass: dns.ClassINET}, resp, &dns.Msg{}, 0); err != nil {
		t.Fatal(err)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("want 1 answer, got %d", len(resp.Answer))
	}
	ptr, ok := resp.Answer[0].(*dns.PTR)
	if !ok || ptr.Hdr.Name != sub || ptr.Ptr != "Printer._ipp._tcp.local." {
		t.Fatalf("unexpected answer: %v", resp.Answer[0])
	}
	if len(resp.Extra) < 2 {
		t.Fatalf("SRV/TXT additionals missing: %v", resp.Extra)
	}
}

func TestSubtypeKnownAnswerMatchesRecordName(t *testing.T) {
	s := &Server{ttl: 120}
	s.service = newServiceEntry("Printer", "_ipp._tcp,_universal", "local.")
	s.service.Port = 631
	const subtype = "_universal._sub._ipp._tcp.local."
	for _, tc := range []struct {
		name, owner, target string
		class               uint16
		ttl                 uint32
		wantAnswer          bool
	}{
		{"different owner", "_ipp._tcp.local.", "Printer._ipp._tcp.local.", dns.ClassINET, 120, true},
		{"matching record", "_UNIVERSAL._SUB._IPP._TCP.LOCAL.", "PRINTER._IPP._TCP.LOCAL.", dns.ClassINET, 60, false},
		{"different class", subtype, "Printer._ipp._tcp.local.", dns.ClassCHAOS, 120, true},
		{"expired answer", subtype, "Printer._ipp._tcp.local.", dns.ClassINET, 59, true},
		{"different target", subtype, "Other._ipp._tcp.local.", dns.ClassINET, 120, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := &dns.Msg{Answer: []dns.RR{&dns.PTR{
				Hdr: dns.RR_Header{Name: tc.owner, Rrtype: dns.TypePTR, Class: tc.class, Ttl: tc.ttl},
				Ptr: tc.target,
			}}}
			resp := &dns.Msg{}
			if err := s.handleQuestion(dns.Question{Name: subtype, Qtype: dns.TypePTR, Qclass: dns.ClassINET}, resp, query, 0); err != nil {
				t.Fatal(err)
			}
			if got := len(resp.Answer) != 0; got != tc.wantAnswer {
				t.Fatalf("answered = %t, want %t", got, tc.wantAnswer)
			}
		})
	}
}

func TestInstanceLookupMatchesWireName(t *testing.T) {
	for _, instance := range []string{
		`HP\ Air\ \(PC\)`,
		`Printer\ 2\.1\.`,
		`Printer\ \226\152\131`,
	} {
		t.Run(instance, func(t *testing.T) {
			s := &Server{ttl: 120}
			s.service = newServiceEntry(instance, "_ipp._tcp,_universal", "local.")
			s.service.HostName = "pc.local."
			s.service.Port = 631
			query := &dns.Msg{}
			query.SetQuestion(s.service.ServiceInstanceName(), dns.TypeSRV)
			wire, err := query.Pack()
			if err != nil {
				t.Fatal(err)
			}
			var decoded dns.Msg
			if err := decoded.Unpack(wire); err != nil {
				t.Fatal(err)
			}
			resp := &dns.Msg{}
			if err := s.handleQuestion(decoded.Question[0], resp, &decoded, 0); err != nil {
				t.Fatal(err)
			}
			if len(resp.Answer) == 0 {
				t.Fatalf("wire query %q did not match instance %q", decoded.Question[0].Name, s.service.ServiceInstanceName())
			}
		})
	}
}
