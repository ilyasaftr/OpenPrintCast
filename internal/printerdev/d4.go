package printerdev

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// IEEE 1284.4 ("D4") is the packet layer Epson printers speak once they
// leave print-data mode: channels to named services, with credit flow
// control in both directions. This implementation follows the wire
// behavior documented by three independent reverse-engineered
// implementations (epson-usb, reink, abrasive/epson-reversing), which
// agree byte for byte.

// d4EnterSequence switches the printer from interpreting the stream as
// print data into packet mode. The three NULs guard against a partial
// command left in the parser; the EJL line does the switch.
var d4EnterSequence = []byte("\x00\x00\x00\x1b\x01@EJL 1284.4\n@EJL\n@EJL\n")

// Packet control bits (IEEE 1284.4): end-of-message marks the last packet
// of one logical transfer; out-of-band is unused but parsed through.
const (
	d4ControlOOB = 1
	d4ControlEOM = 2
)

// Socket-zero commands, the channel-0 control conversation every session
// starts with. Responses carry the command byte with the high bit set, a
// zero status byte, then the result.
const (
	d4cmdInit         = 0
	d4cmdOpenChannel  = 1
	d4cmdCloseChannel = 2
	d4cmdCredit       = 3
	d4cmdCreditReq    = 4
	d4cmdExit         = 8
	d4cmdGetSocketID  = 9
)

// d4HeaderLen is the packet header: primary socket, secondary socket,
// length including the header, credit, control.
const d4HeaderLen = 6

// d4MaxPayload bounds one packet's payload, far above the small frames of
// a control conversation; anything larger is a framing error.
const d4MaxPayload = 65536 - d4HeaderLen

// d4Session is one packet-mode conversation over the stream. Packets of
// the control conversation and of open channels interleave on the wire,
// so reads demultiplex them into per-socket queues.
type d4Session struct {
	rw      io.ReadWriter
	pending []byte // bytes read past the last packet boundary
	queues  map[uint8][]d4Packet
}

// d4Packet is one packet: the sockets it travels between, its payload,
// the credit it carries and its control bits.
type d4Packet struct {
	psid, ssid uint8
	payload    []byte
	credit     uint8
	eom        bool
}

// startD4 enters packet mode and runs the Init handshake. The printer
// answers the mode switch with a fixed prelude (eight bytes) which is
// discarded.
func startD4(rw io.ReadWriter) (*d4Session, error) {
	if _, err := rw.Write(d4EnterSequence); err != nil {
		return nil, fmt.Errorf("enter 1284.4 mode: %w", err)
	}
	s := &d4Session{rw: rw, queues: make(map[uint8][]d4Packet)}
	if _, err := io.ReadFull(s, make([]byte, 8)); err != nil {
		return nil, fmt.Errorf("read 1284.4 prelude: %w", err)
	}
	resp, err := s.command(d4cmdInit, []byte{0x10})
	if err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}
	if len(resp) != 1 || resp[0] != 0x10 {
		return nil, fmt.Errorf("init: unexpected answer %x", resp)
	}
	return s, nil
}

// Read satisfies io.Reader for the session's buffered reads.
func (s *d4Session) Read(p []byte) (int, error) {
	if len(s.pending) > 0 {
		n := copy(p, s.pending)
		s.pending = s.pending[n:]
		return n, nil
	}
	return s.rw.Read(p)
}

// fill reads exactly len bytes into the pending buffer.
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

// take consumes n bytes of the pending buffer.
func (s *d4Session) take(n int) []byte {
	out := append([]byte(nil), s.pending[:n]...)
	s.pending = s.pending[n:]
	return out
}

// writePacket emits one packet.
func (s *d4Session) writePacket(psid, ssid uint8, credit uint8, eom bool, payload []byte) error {
	if len(payload) > d4MaxPayload {
		return errors.New("d4: payload too large")
	}
	control := uint8(0)
	if eom {
		control |= d4ControlEOM
	}
	header := []byte{psid, ssid, 0, 0, credit, control}
	binary.BigEndian.PutUint16(header[2:4], uint16(len(payload)+d4HeaderLen))
	if _, err := s.rw.Write(append(header, payload...)); err != nil {
		return fmt.Errorf("write packet: %w", err)
	}
	return nil
}

// readPacket reads the next packet for socket, queueing packets of other
// sockets until they are wanted.
func (s *d4Session) readPacket(socket uint8) (d4Packet, error) {
	for {
		if queue := s.queues[socket]; len(queue) > 0 {
			packet := queue[0]
			s.queues[socket] = queue[1:]
			return packet, nil
		}
		packet, err := s.readPacketRaw()
		if err != nil {
			return d4Packet{}, err
		}
		s.queues[packet.psid] = append(s.queues[packet.psid], packet)
	}
}

// readPacketRaw reads the next packet off the wire, whichever socket it
// belongs to.
func (s *d4Session) readPacketRaw() (d4Packet, error) {
	if err := s.fill(d4HeaderLen); err != nil {
		return d4Packet{}, fmt.Errorf("read packet header: %w", err)
	}
	header := s.take(d4HeaderLen)
	length := int(binary.BigEndian.Uint16(header[2:4]))
	if length < d4HeaderLen || length > 65535 {
		return d4Packet{}, fmt.Errorf("d4: bad packet length %d", length)
	}
	if err := s.fill(length - d4HeaderLen); err != nil {
		return d4Packet{}, fmt.Errorf("read packet payload: %w", err)
	}
	return d4Packet{
		psid:    header[0],
		ssid:    header[1],
		payload: s.take(length - d4HeaderLen),
		credit:  header[4],
		eom:     header[5]&d4ControlEOM != 0,
	}, nil
}

// command runs one socket-zero control command and returns the result
// bytes of its response.
func (s *d4Session) command(cmd byte, payload []byte) ([]byte, error) {
	if err := s.writePacket(0, 0, 0, true, append([]byte{cmd}, payload...)); err != nil {
		return nil, err
	}
	resp, err := s.readPacket(0)
	if err != nil {
		return nil, err
	}
	if len(resp.payload) < 2 {
		return nil, errors.New("d4: short command response")
	}
	if resp.payload[0] != cmd|0x80 {
		return nil, fmt.Errorf("d4: command %#x answered by %#x", cmd, resp.payload[0])
	}
	if resp.payload[1] != 0 {
		return nil, fmt.Errorf("d4: command %#x failed with status %#x", cmd, resp.payload[1])
	}
	return resp.payload[2:], nil
}

// socketID resolves a service name ("EPSON-CTRL") to its socket number.
func (s *d4Session) socketID(name string) (uint8, error) {
	resp, err := s.command(d4cmdGetSocketID, []byte(name))
	if err != nil {
		return 0, fmt.Errorf("get socket id of %s: %w", name, err)
	}
	if len(resp) < 1 || resp[0] == 0 {
		return 0, fmt.Errorf("service %s not offered", name)
	}
	return resp[0], nil
}

// openChannel opens the channel to socket and returns its MTU and the
// initial credit.
func (s *d4Session) openChannel(socket uint8) (mtu, credit int, err error) {
	req := make([]byte, 10)
	req[0], req[1] = socket, socket
	for i := 2; i < 10; i += 2 {
		binary.BigEndian.PutUint16(req[i:i+2], 0xffff)
	}
	resp, err := s.command(d4cmdOpenChannel, req)
	if err != nil {
		return 0, 0, fmt.Errorf("open channel %d: %w", socket, err)
	}
	if len(resp) < 8 {
		return 0, 0, errors.New("d4: short open-channel response")
	}
	mtu = int(binary.BigEndian.Uint16(resp[2:4]))
	credit = int(binary.BigEndian.Uint16(resp[6:8]))
	if mtu < d4HeaderLen+1 {
		return 0, 0, fmt.Errorf("d4: implausible channel mtu %d", mtu)
	}
	return mtu, credit, nil
}

// closeChannel closes the channel; the session then exits packet mode.
func (s *d4Session) closeChannel(socket uint8) {
	// Best effort: the printer is about to leave packet mode anyway, and
	// the caller is often already on an error path.
	_ = s.writePacket(0, 0, 0, true, []byte{d4cmdCloseChannel, socket, socket})
	_, _ = s.readPacket(0)
}

// exit leaves packet mode.
func (s *d4Session) exit() {
	_, _ = s.command(d4cmdExit, nil)
}

// credit grants the printer n more packets' worth of send credit on the
// channel.
func (s *d4Session) credit(socket uint8, n int) error {
	req := []byte{socket, socket, 0, 0}
	binary.BigEndian.PutUint16(req[2:4], uint16(n))
	_, err := s.command(d4cmdCredit, req)
	return err
}

// channel is one open service channel with its flow-control state.
type d4Channel struct {
	session *d4Session
	socket  uint8
	mtu     int
	credit  int // send credit: packets we may still write
}

// open resolves the service and opens its channel.
func (s *d4Session) open(service string) (*d4Channel, error) {
	socket, err := s.socketID(service)
	if err != nil {
		return nil, err
	}
	mtu, credit, err := s.openChannel(socket)
	if err != nil {
		return nil, err
	}
	return &d4Channel{session: s, socket: socket, mtu: mtu, credit: credit}, nil
}

// close ends the channel and packet mode.
func (c *d4Channel) close() {
	c.session.closeChannel(c.socket)
	c.session.exit()
}

// write sends data on the channel, chunked to the MTU with end-of-message
// on the last packet. Writes larger than the initial credit allow are
// refused rather than deadlocked: the control conversation of these
// printers never exceeds it.
func (c *d4Channel) write(data []byte) error {
	for len(data) > 0 {
		if c.credit <= 0 {
			return errors.New("d4: out of send credit")
		}
		chunk := len(data)
		if maxPayload := c.mtu - d4HeaderLen; chunk > maxPayload {
			chunk = maxPayload
		}
		eom := chunk == len(data)
		// The credit byte grants the printer one answer packet, as the
		// reference implementations write it; lenient firmware expects it
		// beside the explicit grants of the control conversation.
		if err := c.session.writePacket(c.socket, c.socket, 1, eom, data[:chunk]); err != nil {
			return err
		}
		c.credit--
		data = data[chunk:]
	}
	return nil
}

// read collects one logical message from the channel: packets until their
// end-of-message bit, granting one credit per packet wanted. maxBytes
// bounds the message; the printer's answers are a few hundred bytes.
func (c *d4Channel) read(maxBytes int) ([]byte, error) {
	var out []byte
	for {
		if err := c.session.credit(c.socket, 1); err != nil {
			return nil, fmt.Errorf("grant credit: %w", err)
		}
		packet, err := c.session.readPacket(c.socket)
		if err != nil {
			return nil, err
		}
		out = append(out, packet.payload...)
		if len(out) > maxBytes {
			return nil, errors.New("d4: answer exceeds bound")
		}
		if packet.eom {
			return out, nil
		}
	}
}
