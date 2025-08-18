
package frr

import (
	"fmt"
	"net"
)

// Manager is an interface for programming FRR on the *local* node.
type Manager interface {
	AnnounceVIP(ip net.IP, prefixLen int) error
	WithdrawVIP(ip net.IP, prefixLen int) error
	ListAnnounced() (map[string]bool, error) // ip/cidr string -> true
}

// Config contains minimal knobs for FRR programming.
type Config struct {
	ASN          int
	VTYSHPath    string // default: /usr/bin/vtysh
	EnsureStatic bool   // ensure static Null0 for the VIP before 'network'
}

func ipFamily(ip net.IP) string {
	if ip.To4() != nil { return "ipv4" }
	return "ipv6"
}

// Key renders canonical CIDR string.
func Key(ip net.IP, prefixLen int) string {
	return fmt.Sprintf("%s/%d", ip.String(), prefixLen)
}
