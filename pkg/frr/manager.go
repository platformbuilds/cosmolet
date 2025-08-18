
package frr

import (
	"fmt"
	"net"
)

type Manager interface {
	AnnounceVIP(ip net.IP, prefixLen int) error
	WithdrawVIP(ip net.IP, prefixLen int) error
	ListAnnounced() (map[string]bool, error)
}

type Config struct {
	ASN          int
	VTYSHPath    string
	EnsureStatic bool
}

func ipFamily(ip net.IP) string {
	if ip.To4() != nil { return "ipv4" }
	return "ipv6"
}

func Key(ip net.IP, prefixLen int) string {
	return fmt.Sprintf("%s/%d", ip.String(), prefixLen)
}
