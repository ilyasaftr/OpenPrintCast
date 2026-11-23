//go:build !windows

package winspool

// platformSupplyTarget has no Windows port settings to read off Windows,
// so no printer has an SNMP source and Linux CI never touches the network.
func platformSupplyTarget(printer string) (snmpTarget, error) {
	return snmpTarget{}, errNoSupplySource
}
