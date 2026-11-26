package printerdev

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// Epson printers answer the EPSON-CTRL service of their 1284.4 packet
// mode. Commands are the two-letter verb, the little-endian length of the
// payload and the payload: "st\x01\x00\x01" asks for the status. Modern
// printers answer the status in the binary @BDC ST2 frame; older ones in
// the text form carrying the ink levels as hex pairs behind an "IQ:" tag.

// epsonService is the socket name every Epson inkjet offers.
const epsonService = "EPSON-CTRL"

// epsonMaxAnswer bounds one control answer; the ST2 frame of a real
// printer is a few hundred bytes.
const epsonMaxAnswer = 8192

// epsonProber is the Epson vendor probe.
type epsonProber struct{}

// matches reports whether the IEEE 1284 device ID is Epson's.
func (epsonProber) matches(deviceID string) bool {
	mfg := field1284(deviceID, "MFG")
	return strings.Contains(strings.ToLower(mfg), "epson")
}

// probe reads the printer over rw: the status frame for supplies and
// error bits, then the version information for the firmware string. Each
// is best effort — a failure of one does not lose the other.
func (epsonProber) probe(ctx context.Context, rw io.ReadWriter, deviceID string) (Facts, error) {
	facts := Facts{Model: field1284(deviceID, "MDL"), PagesColor: -1, PagesMonochrome: -1, PagesBlank: -1}
	if err := ctx.Err(); err != nil {
		return Facts{}, err
	}
	session, err := startD4(rw)
	if err != nil {
		return Facts{}, err
	}
	channel, err := session.open(epsonService)
	if err != nil {
		return Facts{}, fmt.Errorf("open %s: %w", epsonService, err)
	}
	defer channel.close()

	var statusErr error
	status, err := epsonCommand(channel, "st", []byte{0x01})
	if err != nil {
		statusErr = err
	} else {
		epsonParseStatus(status, &facts)
	}
	firmware, err := epsonFirmware(channel)
	if err == nil {
		facts.Firmware = firmware
	}
	if len(facts.Supplies) == 0 && facts.Serial == "" && statusErr != nil {
		return Facts{}, fmt.Errorf("read status: %w", statusErr)
	}
	return facts, nil
}

// epsonCommand frames one control command and reads its answer.
func epsonCommand(channel *d4Channel, verb string, payload []byte) ([]byte, error) {
	if len(verb) != 2 {
		return nil, errors.New("epson: command verb must be two bytes")
	}
	if len(payload) > 0xffff {
		return nil, errors.New("epson: payload too large")
	}
	frame := make([]byte, 4, 4+len(payload))
	copy(frame, verb)
	binary.LittleEndian.PutUint16(frame[2:4], uint16(len(payload)))
	if err := channel.write(append(frame, payload...)); err != nil {
		return nil, fmt.Errorf("send %s: %w", verb, err)
	}
	answer, err := channel.read(epsonMaxAnswer)
	if err != nil {
		return nil, fmt.Errorf("read %s answer: %w", verb, err)
	}
	return answer, nil
}

// epsonFirmware reads the version information and picks the firmware
// token out of the "vi:00:XXXXXX" answer; the index byte selects among
// the printer's firmware components, 0 the main one.
func epsonFirmware(channel *d4Channel) (string, error) {
	answer, err := epsonCommand(channel, "vi", []byte{0x00})
	if err != nil {
		return "", err
	}
	text := string(answer)
	idx := strings.Index(text, "vi:00:")
	if idx < 0 || idx+len("vi:00:")+6 > len(text) {
		return "", fmt.Errorf("no firmware token in %q", text)
	}
	token := text[idx+len("vi:00:") : idx+len("vi:00:")+6]
	return token, nil
}

// epsonParseStatus fills facts from one status answer, whichever of the
// two shapes it takes.
func epsonParseStatus(answer []byte, facts *Facts) {
	if st2, ok := st2Frame(answer); ok {
		parseST2(st2, facts)
		return
	}
	// The text form: hex pairs behind the IQ: tag, one pair per ink.
	text := string(answer)
	if idx := strings.Index(text, "IQ:"); idx >= 0 {
		parseEpsonIQ(text[idx+len("IQ:"):], facts)
	}
}

// st2Frame extracts the @BDC ST2 frame of an answer: the ten-byte magic,
// the little-endian length and the TLV payload. The answer may carry a
// leading NUL or a command echo before the magic.
func st2Frame(answer []byte) ([]byte, bool) {
	const magic = "@BDC ST2\r\n"
	idx := strings.Index(string(answer), magic)
	if idx < 0 {
		return nil, false
	}
	rest := answer[idx+len(magic):]
	if len(rest) < 2 {
		return nil, false
	}
	length := int(binary.LittleEndian.Uint16(rest[:2]))
	rest = rest[2:]
	if length > len(rest) {
		return nil, false
	}
	return rest[:length], true
}

// st2 colour cartridge ids (0x0f records, first byte) and ink colours
// (second byte), from the Epson programming manuals the reverse
// implementations document. Only what the bridge can name is mapped.
var st2InkColors = map[byte]string{
	0x00: "black",
	0x01: "cyan",
	0x02: "magenta",
	0x03: "yellow",
	0x04: "light-cyan",
	0x05: "light-magenta",
	0x06: "dark-yellow",
	0x07: "gray",
	0x08: "light-black",
	0x09: "red",
	0x0a: "blue",
	0x0c: "light-gray",
	0x0d: "orange",
}

// st2CartridgeNames names the cartridge kind when the ink colour is
// unknown, for the supply description.
var st2CartridgeNames = map[byte]string{
	0x01: "Black",
	0x03: "Cyan",
	0x04: "Magenta",
	0x05: "Yellow",
	0x06: "Light Cyan",
	0x07: "Light Magenta",
	0x0a: "Light Black",
	0x0b: "Matte Black",
	0x0f: "Light Light Black",
	0x10: "Orange",
	0x11: "Green",
}

// st2ErrorReasons maps the error codes of the status frame to
// printer-state-reasons keywords, without severity suffixes.
var st2ErrorReasons = map[byte]string{
	0x02: "cover-open",
	0x04: "media-jam",
	0x05: "marker-supply-empty",
	0x06: "media-empty",
	0x10: "marker-waste-full",
	0x1a: "cover-open",
	0x22: "marker-waste-full",
	0x25: "cover-open",
	0x2b: "cover-open",
	0x36: "cover-open",
	0x37: "cover-open",
	0x4a: "marker-waste-almost-full",
}

// parseST2 walks the type-length-value elements of the frame and decodes
// the ones the bridge reports: ink levels (0x0f), waste ink percents
// (0x0d), the serial (0x1f) and the error code (0x02).
func parseST2(frame []byte, facts *Facts) {
	for len(frame) >= 2 {
		ftype, length := frame[0], int(frame[1])
		frame = frame[2:]
		if length > len(frame) {
			return // truncated element: keep what was decoded
		}
		item := frame[:length]
		frame = frame[length:]
		switch ftype {
		case 0x0f:
			parseST2Ink(item, facts)
		case 0x0d:
			parseST2Waste(item, facts)
		case 0x02:
			if len(item) > 0 {
				if reason, ok := st2ErrorReasons[item[0]]; ok {
					facts.StateReasons = append(facts.StateReasons, reason)
				}
			}
		case 0x1f:
			if serial := strings.TrimRight(string(item), "\x00"); serial != "" {
				facts.Serial = serial
			}
		case 0x36:
			parseST2PageCounts(item, facts)
		}
	}
}

// parseST2Ink decodes the ink-information element: the first byte is the
// record stride, then per ink the cartridge id, the ink colour and the
// level in percent.
func parseST2Ink(item []byte, facts *Facts) {
	if len(item) < 1 {
		return
	}
	stride := int(item[0])
	if stride < 3 {
		return
	}
	for offset := 1; offset+3 <= len(item); offset += stride {
		cartridge, inkColor, level := item[offset], item[offset+1], item[offset+2]
		if level > core.MaxSupplyPercent {
			continue // not a percentage: a placeholder record
		}
		supply := core.Supply{
			Kind:  core.SupplyInk,
			Level: core.SupplyLevel(level),
		}
		if color, ok := st2InkColors[inkColor]; ok {
			supply.Color = color
		}
		if name, ok := st2CartridgeNames[cartridge]; ok {
			supply.Name = name + " ink"
		}
		facts.Supplies = append(facts.Supplies, supply)
	}
}

// parseST2PageCounts decodes the paper-count element: five little-endian
// signed counters — plain and photo paper counters first, then the color,
// monochrome and blank sheet counts. The counters are signed because -1
// is how a model says it does not count that kind of sheet; negative
// values are kept so Facts can tell "zero sheets" from "no counter".
func parseST2PageCounts(item []byte, facts *Facts) {
	if len(item) != 20 {
		return
	}
	facts.PagesColor = int(int32(binary.LittleEndian.Uint32(item[8:12])))
	facts.PagesMonochrome = int(int32(binary.LittleEndian.Uint32(item[12:16])))
	facts.PagesBlank = int(int32(binary.LittleEndian.Uint32(item[16:20])))
}

// parseST2Waste decodes the maintenance-tank element: one byte per waste
// tank, the percent filled.
func parseST2Waste(item []byte, facts *Facts) {
	for i, level := range item {
		if level > core.MaxSupplyPercent {
			continue
		}
		name := "Waste ink"
		if len(item) > 1 {
			name = fmt.Sprintf("Waste ink %d", i+1)
		}
		facts.Supplies = append(facts.Supplies, core.Supply{
			Name:       name,
			Kind:       core.SupplyWasteInk,
			Level:      core.SupplyLevel(level),
			Receptacle: true,
		})
	}
}

// parseEpsonIQ decodes the legacy text form: hex pairs, one per ink, in
// cartridge order (black first on the consumer models this form covers).
func parseEpsonIQ(text string, facts *Facts) {
	end := strings.IndexByte(text, ';')
	if end >= 0 {
		text = text[:end]
	}
	text = strings.TrimSpace(text)
	if len(text) == 0 || len(text)%2 != 0 {
		return
	}
	// Cartridge order of the text form: black, cyan, magenta, yellow for
	// the four-ink models; extras are named by position.
	order := []string{"black", "cyan", "magenta", "yellow"}
	for i := 0; i+2 <= len(text); i += 2 {
		var level int
		if _, err := fmt.Sscanf(text[i:i+2], "%02x", &level); err != nil {
			return
		}
		if level > core.MaxSupplyPercent {
			continue
		}
		supply := core.Supply{Kind: core.SupplyInk, Level: core.SupplyLevel(level)}
		if idx := i / 2; idx < len(order) {
			supply.Color = order[idx]
		}
		facts.Supplies = append(facts.Supplies, supply)
	}
}
