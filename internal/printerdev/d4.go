package printerdev

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

// IEEE 1284.4 ("D4") is the packet layer Epson printers speak once they
// leave print-data mode. This implementation is a port of the wire behavior
// of the epson-usb library, the implementation verified byte for byte
// against real hardware of the L3250 generation the L3210 belongs to: the
// same handshake order, the same packet bytes, the same credit dance. Where
// it differs from the older references (abrasive/epson-reversing), the
// hardware-verified behavior wins.

// d4EnterSequence switches the printer from interpreting the stream as
// print data into packet mode. The three NULs guard against a partial
// command left in the parser; the EJL line does the switch.
var d4EnterSequence = []byte("\x00\x00\x00\x1b\x01@EJL 1284.4\n@EJL\n@EJL\n")

// epsonCtrlSocket is the well-known socket of the EPSON-CTRL service. The
// hardware-verified implementation uses it directly rather than resolving
// it with GetSocketID.
const epsonCtrlSocket = 0x02

// Revisions of the D4 protocol. The host offers 0x20 first; the printer of
// the L3250 family answers with its own, 0x10, and the host retries with
// it. Later structures depend on the negotiated revision.
const (
	d4InitialRevision = 0x20
	d4Revision10      = 0x10
)

// d4HeaderLen is the packet header: primary socket, secondary socket,
// length including the header, credit, control.
const d4HeaderLen = 6

// The pauses and retry counts of the verified implementation. The printer
// needs a moment after the mode switch before it speaks packets.
const (
	d4EnterPause  = 200 * time.Millisecond
	d4DrainPause  = 300 * time.Millisecond
	d4ReplySweeps = 14
)

// d4Session is one packet-mode conversation over the stream. Bytes are read
// ahead into pending, so packets are always parsed aligned; packets of one
// socket arriving while another is wanted are queued per socket.
type d4Session struct {
	rw      io.ReadWriter
	pending []byte
	queues  map[uint8][][]byte
	rev     byte
}

// writePacket emits one packet.
func (s *d4Session) writePacket(psid, ssid uint8, credit uint8, payload []byte) error {
	header := []byte{psid, ssid, 0, 0, credit, 0}
	binary.BigEndian.PutUint16(header[2:4], uint16(len(payload)+d4HeaderLen))
	if _, err := s.rw.Write(append(header, payload...)); err != nil {
		return fmt.Errorf("write packet: %w", err)
	}
	return nil
}

// fill reads at least n bytes into the pending buffer.
func (s *d4Session) fill(n int) error {
	for len(s.pending) < n {
		buf := make([]byte, n-len(s.pending)+128)
		read, err := s.rw.Read(buf)
		if read > 0 {
			s.pending = append(s.pending, buf[:read]...)
		}
		if err != nil {
			return err
		}
		if read == 0 {
			return io.ErrNoProgress
		}
	}
	return nil
}

// packet is one parsed packet.
type packet struct {
	psid    uint8
	payload []byte
}

// readPacket reads the next packet for socket, queueing packets of other
// sockets until they are wanted. A packet whose declared length is smaller
// than the header is malformed and consumed, resynchronizing the stream.
func (s *d4Session) readPacket(socket uint8) (packet, error) {
	for {
		if queue := s.queues[socket]; len(queue) > 0 {
			payload := queue[0]
			s.queues[socket] = queue[1:]
			return packet{psid: socket, payload: payload}, nil
		}
		p, err := s.readPacketRaw()
		if err != nil {
			return packet{}, err
		}
		if p.psid == socket {
			return p, nil
		}
		s.queues[p.psid] = append(s.queues[p.psid], p.payload)
	}
}

// readPacketRaw reads the next packet off the wire, whichever socket it
// belongs to.
func (s *d4Session) readPacketRaw() (packet, error) {
	if err := s.fill(d4HeaderLen); err != nil {
		return packet{}, fmt.Errorf("read packet header: %w", err)
	}
	header := append([]byte(nil), s.pending[:d4HeaderLen]...)
	s.pending = s.pending[d4HeaderLen:]
	length := int(binary.BigEndian.Uint16(header[2:4]))
	if length < d4HeaderLen {
		return packet{psid: header[0]}, nil
	}
	if err := s.fill(length - d4HeaderLen); err != nil {
		return packet{}, fmt.Errorf("read packet payload: %w", err)
	}
	payload := append([]byte(nil), s.pending[:length-d4HeaderLen]...)
	s.pending = s.pending[length-d4HeaderLen:]
	return packet{psid: header[0], payload: payload}, nil
}

// drain throws away whatever the printer sent before the session started:
// periodic status bytes left in the pipe by other software would
// desynchronize the first packet read. A device that can shorten its read
// timeout is asked to; the drain costs one such timeout when the pipe is
// empty, which is the common case.
func drain(rw io.ReadWriter) {
	shorter, ok := rw.(interface{ setReadTimeout(time.Duration) })
	if !ok {
		return
	}
	shorter.setReadTimeout(d4DrainPause)
	defer shorter.setReadTimeout(ioTimeout)
	buf := make([]byte, 1024)
	for {
		if _, err := rw.Read(buf); err != nil {
			return
		}
	}
}

// control runs one socket-zero command: the command byte and its payload,
// with the credit byte the verified implementation grants on control
// packets. The reply payload is returned without the command echo and
// status bytes.
func (s *d4Session) control(payload []byte) ([]byte, error) {
	if err := s.writePacket(0, 0, 1, payload); err != nil {
		return nil, err
	}
	resp, err := s.readPacket(0)
	if err != nil {
		return nil, err
	}
	return resp.payload, nil
}

// startD4 enters packet mode and runs the handshake, exactly as the
// hardware-verified implementation does: drain, the mode switch, a pause,
// one packet-aligned reply (tolerated if absent), then the Init revision
// negotiation offering 0x20 first and retrying with the printer's own.
func startD4(rw io.ReadWriter) (*d4Session, error) {
	drain(rw)
	if _, err := rw.Write(d4EnterSequence); err != nil {
		return nil, fmt.Errorf("enter 1284.4 mode: %w", err)
	}
	time.Sleep(d4EnterPause)
	s := &d4Session{rw: rw, queues: make(map[uint8][][]byte)}
	// The enter reply is one packet, whatever its length; it is read for
	// alignment, not for its content, and a printer that sends none is not
	// a failure here — the Init below is the real probe.
	_ = s.readEnterReply()
	rev := byte(d4InitialRevision)
	ok := false
	for range 3 {
		resp, err := s.control([]byte{0x00, rev})
		if err != nil {
			return nil, fmt.Errorf("init: %w", err)
		}
		if len(resp) >= 3 && resp[0] == 0x80 {
			if resp[1] == 0x00 {
				ok = true
				break
			}
			if resp[2] != 0 && resp[2] != rev {
				rev = resp[2]
				continue
			}
		}
		break
	}
	if !ok {
		return nil, fmt.Errorf("init: the printer did not answer D4 (tried revision %#x)", rev)
	}
	s.rev = rev
	if err := s.openChannel(); err != nil {
		return nil, err
	}
	return s, nil
}

// readEnterReply consumes one packet without requiring it.
func (s *d4Session) readEnterReply() error {
	_, err := s.readPacketRaw()
	return err
}

// openChannel opens the EPSON-CTRL channel with the byte layout of the
// negotiated revision: revision 0x10 carries one field more. The reply is
// validated by its echo and status bytes alone; the mtu and credit fields
// are the printer's business, since every request takes fresh credit.
func (s *d4Session) openChannel() error {
	var payload []byte
	if s.rev == d4Revision10 {
		payload = []byte{0x01, 0x02, epsonCtrlSocket, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00}
	} else {
		payload = []byte{0x01, 0x02, epsonCtrlSocket, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}
	}
	resp, err := s.control(payload)
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	if len(resp) < 2 || resp[0] != 0x81 || resp[1] != 0x00 {
		return fmt.Errorf("open channel refused: %x", resp)
	}
	return nil
}

// request sends one EPSON-CTRL frame on the control channel and returns the
// first answer packet's payload. The credit dance comes first, exactly as
// the hardware-verified implementation does it: take send credit from the
// printer with CreditRequest, grant it eight reply credits, then send the
// frame carrying eight credits. A printer that stays quiet answers with
// nothing within the sweeps.
func (s *d4Session) request(frame []byte) ([]byte, error) {
	if err := s.creditRequest(); err != nil {
		return nil, fmt.Errorf("credit request: %w", err)
	}
	grant := []byte{0x03, 0x02, epsonCtrlSocket, 0x00, 0x08}
	if _, err := s.control(grant); err != nil {
		return nil, fmt.Errorf("credit grant: %w", err)
	}
	if err := s.writePacket(epsonCtrlSocket, epsonCtrlSocket, 8, frame); err != nil {
		return nil, err
	}
	var lastErr error
	for range d4ReplySweeps {
		p, err := s.readPacket(epsonCtrlSocket)
		if err != nil {
			lastErr = err
			continue
		}
		if len(p.payload) > 0 {
			return p.payload, nil
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("no reply to the command")
}

// creditRequest takes send credit for the channel from the printer. The
// printer answers with a credit reply, or with nothing — some firmware
// grants silently — which the verified implementation tolerates after a
// few sweeps.
func (s *d4Session) creditRequest() error {
	var payload []byte
	if s.rev == d4Revision10 {
		payload = []byte{0x04, 0x02, epsonCtrlSocket, 0x00, 0x80, 0xff, 0xff}
	} else {
		payload = []byte{0x04, 0x02, epsonCtrlSocket, 0x00, 0x08}
	}
	if err := s.writePacket(0, 0, 1, payload); err != nil {
		return err
	}
	for range 4 {
		p, err := s.readPacket(0)
		if err != nil {
			return nil // no answer within the sweeps: the printer grants silently
		}
		if len(p.payload) > 0 && p.payload[0] == 0x84 {
			return nil
		}
	}
	return nil
}
