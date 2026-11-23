//go:build windows

package winspool

import (
	"errors"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// tcpipPortsKey is the registry key of the Standard TCP/IP Port monitor
// (tcpmon); each port it manages has a subkey named like the port. The
// values below are the ones the monitor's own configuration dialog writes;
// Microsoft documents them nowhere, they are known from the port settings
// dialog and from deployment scripts.
const tcpipPortsKey = `SYSTEM\CurrentControlSet\Control\Print\Monitors\Standard TCP/IP Port\Ports`

// Value names of a Standard TCP/IP port's key.
const (
	valueHostName      = "HostName"       // REG_SZ, "" when the port was made with an address
	valueIPAddress     = "IPAddress"      // REG_SZ, "" when the port was made with a name
	valueSNMPEnabled   = "SNMP Enabled"   // REG_DWORD, 0 or 1
	valueSNMPCommunity = "SNMP Community" // REG_SZ
	valueSNMPIndex     = "SNMP Index"     // REG_DWORD, the SNMP device index
)

// platformSupplyTarget finds where to read printer's supplies from the
// port the Windows queue prints to. Only the Standard TCP/IP port's own
// settings are used; every other kind of port (WSD, USB, IPP, files) has
// no key there and gives errNoSupplySource.
func platformSupplyTarget(printer string) (snmpTarget, error) {
	port, ok := printerPortName(printer)
	if !ok {
		return snmpTarget{}, errNoSupplySource
	}
	settings, err := readTCPIPPortSettings(port)
	if err != nil {
		return snmpTarget{}, err
	}
	target, ok := targetFromPortSettings(settings)
	if !ok {
		return snmpTarget{}, errNoSupplySource
	}
	return target, nil
}

// printerPortName is the spooler port printer prints to (pPortName of its
// PRINTER_INFO_2).
func printerPortName(printer string) (string, bool) {
	handle, err := openPrinter(printer)
	if err != nil {
		return "", false
	}
	defer closePrinter(handle)
	info, ok := fetchPrinterInfo2(handle)
	if !ok || info.port() == nil {
		return "", false
	}
	name := utf16PtrToString(unsafe.Pointer(info.port()))
	runtime.KeepAlive(info)
	return name, name != ""
}

// readTCPIPPortSettings reads the SNMP-related values of the port's key,
// read-only. A port without a key is not a Standard TCP/IP port.
func readTCPIPPortSettings(port string) (tcpipPortSettings, error) {
	if strings.ContainsRune(port, '\\') {
		return tcpipPortSettings{}, errNoSupplySource
	}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, tcpipPortsKey+`\`+port, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return tcpipPortSettings{}, errNoSupplySource
	}
	if err != nil {
		return tcpipPortSettings{}, err
	}
	defer key.Close()
	enabled, _ := integerValue(key, valueSNMPEnabled)
	index, _ := integerValue(key, valueSNMPIndex)
	return tcpipPortSettings{
		PortName:      port,
		HostName:      stringValue(key, valueHostName),
		IPAddress:     stringValue(key, valueIPAddress),
		SNMPEnabled:   enabled != 0,
		SNMPCommunity: stringValue(key, valueSNMPCommunity),
		SNMPIndex:     int(index),
	}, nil
}

// stringValue reads a REG_SZ value; a missing or differently typed value
// reads as "".
func stringValue(key registry.Key, name string) string {
	value, _, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	return value
}

// integerValue reads a REG_DWORD (or QWORD) value; a missing or
// differently typed value reads as 0.
func integerValue(key registry.Key, name string) (uint64, bool) {
	value, _, err := key.GetIntegerValue(name)
	return value, err == nil
}
