package printerdev

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// fakeD4Printer is a scripted IEEE 1284.4 printer: it consumes the bytes
// the session writes and answers with correctly framed packets, the way
// the wire captures of the reverse implementations show a real one does.
type fakeD4Printer struct {
	readBuf  bytes.Buffer
	writeBuf bytes.Buffer
	// st2 is the TLV payload of the status answer; vi the version answer.
	st2, vi   []byte
	packetLen int // split channel answers into packets of at most this many bytes
	socket    uint8
	mtu       uint16
}

func newFakeD4Printer() *fakeD4Printer {
	return &fakeD4Printer{socket: 7, mtu: 512, packetLen: 1 << 15}
}

// queuePacket frames one outgoing packet into the read buffer.
func (f *fakeD4Printer) queuePacket(psid, ssid uint8, credit uint8, eom bool, payload []byte) {
	header := []byte{psid, ssid, 0, 0, credit, 0}
	if eom {
		header[5] = d4ControlEOM
	}
	binary.BigEndian.PutUint16(header[2:4], uint16(len(payload)+d4HeaderLen))
	f.readBuf.Write(header)
	f.readBuf.Write(payload)
}

// commandResponse queues the socket-zero response to a control command.
func (f *fakeD4Printer) commandResponse(cmd byte, result []byte) {
	f.queuePacket(0, 0, 1, true, append([]byte{cmd | 0x80, 0}, result...))
}

func (f *fakeD4Printer) Write(p []byte) (int, error) {
	f.writeBuf.Write(p)
	for {
		b := f.writeBuf.Bytes()
		if bytes.Contains(b, []byte("@EJL 1284.4")) {
			// The mode switch is answered by an eight-byte prelude.
			f.readBuf.Write(make([]byte, 8))
			f.writeBuf.Reset()
			return len(p), nil
		}
		if len(b) < d4HeaderLen {
			return len(p), nil
		}
		length := int(binary.BigEndian.Uint16(b[2:4]))
		if len(b) < length {
			return len(p), nil // partial packet: wait for more
		}
		packet := append([]byte(nil), b[:length]...)
		f.writeBuf.Next(length)
		f.handlePacket(packet)
		if len(f.writeBuf.Bytes()) == 0 {
			return len(p), nil
		}
	}
}

func (f *fakeD4Printer) handlePacket(packet []byte) {
	psid := packet[0]
	payload := packet[d4HeaderLen:]
	if psid == 0 {
		if len(payload) == 0 {
			return
		}
		switch payload[0] {
		case d4cmdInit:
			f.commandResponse(d4cmdInit, []byte{0x10})
		case d4cmdGetSocketID:
			f.commandResponse(d4cmdGetSocketID, []byte{f.socket})
		case d4cmdOpenChannel:
			resp := []byte{f.socket, f.socket, 0, 0, 0xff, 0xff, 0, 8}
			binary.BigEndian.PutUint16(resp[2:4], f.mtu)
			f.commandResponse(d4cmdOpenChannel, resp)
		case d4cmdCredit:
			f.commandResponse(d4cmdCredit, nil)
		case d4cmdCloseChannel:
			f.commandResponse(d4cmdCloseChannel, nil)
		case d4cmdExit:
			f.commandResponse(d4cmdExit, nil)
		}
		return
	}
	// A channel packet: the control command it carries, answered in
	// packets of at most packetLen bytes with eom on the last.
	verb := string(payload[:2])
	var answer []byte
	switch verb {
	case "st":
		answer = append([]byte("@BDC ST2\r\n"), 0, 0)
		answer = append(answer, f.st2...)
		binary.LittleEndian.PutUint16(answer[10:12], uint16(len(f.st2)))
	case "vi":
		answer = f.vi
	default:
		return
	}
	for len(answer) > 0 {
		chunk := len(answer)
		if chunk > f.packetLen {
			chunk = f.packetLen
		}
		eom := chunk == len(answer)
		f.queuePacket(f.socket, f.socket, 0, eom, answer[:chunk])
		answer = answer[chunk:]
	}
}

func (f *fakeD4Printer) Read(p []byte) (int, error) {
	if f.readBuf.Len() == 0 {
		return 0, nil
	}
	return f.readBuf.Read(p)
}

// st2Element appends one TLV element.
func st2Element(ftype byte, item ...byte) []byte {
	return append([]byte{ftype, byte(len(item))}, item...)
}

// sampleST2 is the TLV payload of a four-ink EcoTank: ink records with a
// stride of 4, one waste tank, the serial and no error.
func sampleST2() []byte {
	var frame []byte
	// Ink information: stride 4; cartridge, colour, level, padding.
	frame = append(frame, st2Element(0x0f,
		4,
		0x01, 0x00, 75, 0, // black cartridge, black ink, 75%
		0x03, 0x01, 50, 0, // cyan
		0x04, 0x02, 25, 0, // magenta
		0x05, 0x03, 8, 0, // yellow, low
	)...)
	frame = append(frame, st2Element(0x0d, 35)...)   // waste ink 35% filled
	frame = append(frame, st2Element(0x02, 0x00)...) // no error
	frame = append(frame, st2Element(0x1f, []byte("X1A2B3C4D5")...)...)
	counts := make([]byte, 20)
	binary.LittleEndian.PutUint32(counts[0:4], 0xFFFFFFFF) // plain counter unsupported
	binary.LittleEndian.PutUint32(counts[4:8], 0xFFFFFFFF) // photo counter unsupported
	binary.LittleEndian.PutUint32(counts[8:12], 11483)     // color sheets
	binary.LittleEndian.PutUint32(counts[12:16], 2313)     // monochrome sheets
	binary.LittleEndian.PutUint32(counts[16:20], 160)      // blank sheets
	frame = append(frame, st2Element(0x36, counts...)...)
	return frame
}

func TestEpsonProbeReadsSuppliesAndFacts(t *testing.T) {
	printer := newFakeD4Printer()
	printer.st2 = sampleST2()
	printer.vi = []byte("vi:00:RF11I5;")
	facts, err := epsonProber{}.probe(context.Background(), printer, "MFG:EPSON;MDL:L3210 Series;")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Supplies) != 5 {
		t.Fatalf("want 5 supplies (4 inks + waste), got %d: %+v", len(facts.Supplies), facts.Supplies)
	}
	wantLevels := []struct {
		color string
		level core.SupplyLevel
	}{
		{"black", 75}, {"cyan", 50}, {"magenta", 25}, {"yellow", 8},
	}
	for i, want := range wantLevels {
		supply := facts.Supplies[i]
		if supply.Color != want.color || supply.Level != want.level || supply.Kind != core.SupplyInk {
			t.Errorf("supply %d: want %s %d%%, got %+v", i, want.color, want.level, supply)
		}
	}
	waste := facts.Supplies[4]
	if waste.Kind != core.SupplyWasteInk || !waste.Receptacle || waste.Level != 35 {
		t.Errorf("waste: want waste-ink receptacle 35%%, got %+v", waste)
	}
	if facts.Serial != "X1A2B3C4D5" {
		t.Errorf("serial: got %q", facts.Serial)
	}
	if facts.Firmware != "RF11I5" {
		t.Errorf("firmware: got %q", facts.Firmware)
	}
	if len(facts.StateReasons) != 0 {
		t.Errorf("no error in the frame, want no reasons, got %v", facts.StateReasons)
	}
	if facts.PagesColor != 11483 || facts.PagesMonochrome != 2313 || facts.PagesBlank != 160 {
		t.Errorf("page counters: got %d/%d/%d", facts.PagesColor, facts.PagesMonochrome, facts.PagesBlank)
	}
	if total, ok := facts.TotalPages(); !ok || total != 11483+2313+160 {
		t.Errorf("total pages: got %d, %v", total, ok)
	}
}

func TestEpsonProbeReadsErrorReasons(t *testing.T) {
	printer := newFakeD4Printer()
	var frame []byte
	frame = append(frame, st2Element(0x0f, 4, 0x01, 0x00, 75, 0)...)
	frame = append(frame, st2Element(0x02, 0x04)...) // paper jam
	printer.st2 = frame
	printer.vi = []byte("vi:00:RF11I5;")
	facts, err := epsonProber{}.probe(context.Background(), printer, "MFG:EPSON;MDL:L3210 Series;")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.StateReasons) != 1 || facts.StateReasons[0] != "media-jam" {
		t.Errorf("want media-jam, got %v", facts.StateReasons)
	}
}

func TestEpsonProbeReassemblesSplitAnswers(t *testing.T) {
	printer := newFakeD4Printer()
	printer.st2 = sampleST2()
	printer.vi = []byte("vi:00:RF11I5;")
	printer.packetLen = 7 // force many small packets
	facts, err := epsonProber{}.probe(context.Background(), printer, "MFG:EPSON;MDL:L3210 Series;")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Supplies) != 5 {
		t.Fatalf("want 5 supplies across split packets, got %d", len(facts.Supplies))
	}
}

func TestParseEpsonIQ(t *testing.T) {
	facts := &Facts{}
	parseEpsonIQ("4B32080F;", facts)
	if len(facts.Supplies) != 4 {
		t.Fatalf("want 4 inks, got %d", len(facts.Supplies))
	}
	if facts.Supplies[0].Color != "black" || facts.Supplies[0].Level != 0x4b {
		t.Errorf("black: got %+v", facts.Supplies[0])
	}
	if facts.Supplies[3].Color != "yellow" || facts.Supplies[3].Level != 0x0f {
		t.Errorf("yellow: got %+v", facts.Supplies[3])
	}
}

func TestST2FrameExtraction(t *testing.T) {
	inner := st2Element(0x1f, []byte("SN")...)
	full := append([]byte("@BDC ST2\r\n"), 0, 0)
	full = append(full, inner...)
	binary.LittleEndian.PutUint16(full[10:12], uint16(len(inner)))
	// A leading NUL or echo before the magic is tolerated.
	noisy := append([]byte{0x00}, full...)
	frame, ok := st2Frame(noisy)
	if !ok {
		t.Fatal("frame not found")
	}
	facts := &Facts{}
	parseST2(frame, facts)
	if facts.Serial != "SN" {
		t.Errorf("serial: got %q", facts.Serial)
	}
}

func TestEpsonMatches(t *testing.T) {
	if !(epsonProber{}).matches("MFG:EPSON;MDL:L3210 Series;DES:EPSON L3210 Series;") {
		t.Error("epson device not recognized")
	}
	if (epsonProber{}).matches("MFG:HP;MDL:LaserJet;") {
		t.Error("hp device wrongly recognized")
	}
}

func TestNormalizeAndField(t *testing.T) {
	id := "MFG:EPSON;MDL:L3210 Series;DES:EPSON L3210 Series;SN:ABC;"
	if got := field1284(id, "MDL"); got != "L3210 Series" {
		t.Errorf("MDL: got %q", got)
	}
	if !deviceNameMatches(normalize("EPSON L3210 Series"), id) {
		t.Error("queue name should match the device model")
	}
	if deviceNameMatches(normalize("Microsoft Print to PDF"), id) {
		t.Error("unrelated queue should not match")
	}
}

func TestTotalPagesAbsentWhenUncounted(t *testing.T) {
	uncounted := Facts{PagesColor: -1, PagesMonochrome: -1, PagesBlank: -1}
	if total, ok := uncounted.TotalPages(); ok {
		t.Errorf("a printer without counters claimed %d sheets", total)
	}
	brandNew := Facts{PagesColor: 0, PagesMonochrome: -1, PagesBlank: -1}
	if total, ok := brandNew.TotalPages(); !ok || total != 0 {
		t.Errorf("zero sheets is a real count: got %d, %v", total, ok)
	}
}
