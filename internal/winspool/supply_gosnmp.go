package winspool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

// SNMP client settings. Reading supplies must never hold anything up: the
// caller bounds the whole read (supplyReadTimeout) and each request gets
// one retry at most.
const (
	// snmpPort is the standard SNMP agent port; the Windows port settings
	// have no field for another.
	snmpPort = 161
	// snmpRequestTimeout is how long one request waits for its answer.
	snmpRequestTimeout = 500 * time.Millisecond
	// snmpRetries is the number of re-sends after a lost request.
	snmpRetries = 1
	// snmpMaxRepetitions bounds the rows one GETBULK answer carries, so the
	// answer fits one UDP datagram on printers with a small buffer.
	snmpMaxRepetitions = 10
)

// gosnmpWalker walks one printer's MIB read-only with gosnmp. It speaks
// SNMPv2c first and falls back to SNMPv1 when the printer does not answer:
// v2c walks with GETBULK, a few round trips instead of one per value, and
// every printer made in the last twenty years answers it; the rare v1-only
// agent drops v2c messages, which shows as a timeout. The version that
// worked is kept for the walker's later walks. Only GET, GETNEXT and
// GETBULK are ever sent, so the community string needs read access only.
type gosnmpWalker struct {
	host      string
	port      uint16
	timeout   time.Duration // per request
	community string
	version   gosnmp.SnmpVersion // zero until a walk has succeeded
}

// newGosnmpWalker builds a walker for the printer's own address.
func newGosnmpWalker(target snmpTarget) *gosnmpWalker {
	return &gosnmpWalker{host: target.Host, port: snmpPort, timeout: snmpRequestTimeout, community: target.Community}
}

// Walk implements snmpWalker.
func (w *gosnmpWalker) Walk(ctx context.Context, rootOID string) ([]snmpVar, error) {
	if w.version != 0 {
		return w.walk(ctx, w.version, rootOID)
	}
	vars, err := w.walk(ctx, gosnmp.Version2c, rootOID)
	if err == nil {
		w.version = gosnmp.Version2c
		return vars, nil
	}
	if ctx.Err() != nil {
		return nil, err
	}
	vars, v1Err := w.walk(ctx, gosnmp.Version1, rootOID)
	if v1Err != nil {
		return nil, fmt.Errorf("SNMPv2c: %v; SNMPv1: %w", err, v1Err)
	}
	w.version = gosnmp.Version1
	return vars, nil
}

// errTooManyVariables stops a walk of a printer that never ends it.
var errTooManyVariables = errors.New("printer returned too many values")

// walk reads every value below rootOID with one SNMP version, and returns
// as soon as ctx is done. gosnmp itself only looks at ctx between requests,
// so a cancelled walk finishes its request in the background (at most one
// timeout, one retry) without sending another.
func (w *gosnmpWalker) walk(ctx context.Context, version gosnmp.SnmpVersion, rootOID string) ([]snmpVar, error) {
	type outcome struct {
		vars []snmpVar
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		vars, err := w.walkBlocking(ctx, version, rootOID)
		done <- outcome{vars, err}
	}()
	select {
	case result := <-done:
		return result.vars, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// walkBlocking is walk without the cancellation wrapper. A failed walk
// gives no values: half a table would show wrong levels.
func (w *gosnmpWalker) walkBlocking(ctx context.Context, version gosnmp.SnmpVersion, rootOID string) ([]snmpVar, error) {
	client := &gosnmp.GoSNMP{
		Target:         w.host,
		Port:           w.port,
		Transport:      "udp",
		Community:      w.community,
		Version:        version,
		Timeout:        w.timeout,
		Retries:        snmpRetries,
		MaxRepetitions: snmpMaxRepetitions,
		Context:        ctx,
	}
	if err := client.Connect(); err != nil {
		return nil, err
	}
	defer client.Close()
	var vars []snmpVar
	collect := func(pdu gosnmp.SnmpPDU) error {
		if len(vars) >= maxSNMPVariables {
			return errTooManyVariables
		}
		vars = append(vars, toSNMPVar(pdu))
		return nil
	}
	walk := client.BulkWalk
	if version == gosnmp.Version1 {
		walk = client.Walk
	}
	if err := walk(rootOID, collect); err != nil {
		return nil, err
	}
	return vars, nil
}

// toSNMPVar converts a gosnmp value; types the Printer MIB does not use
// come out as empty text, which the parsers ignore.
func toSNMPVar(pdu gosnmp.SnmpPDU) snmpVar {
	v := snmpVar{OID: strings.TrimPrefix(pdu.Name, ".")}
	switch value := pdu.Value.(type) {
	case int:
		v.Integer = value
	case uint:
		v.Integer = int(value)
	case uint32:
		v.Integer = int(value)
	case int64:
		v.Integer = int(value)
	case uint64:
		v.Integer = int(value)
	case []byte:
		v.Text, v.IsText = string(value), true
	case string:
		v.Text, v.IsText = value, true
	default:
		v.IsText = true
	}
	return v
}
