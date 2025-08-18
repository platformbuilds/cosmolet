package frr

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
)

type VTYSH struct {
	cfg Config
}

func NewVTYSH(cfg Config) *VTYSH {
	return &VTYSH{cfg: cfg}
}

func (v *VTYSH) run(cmds []string) error {
	args := []string{"-c", "configure terminal"}
	for _, c := range cmds {
		args = append(args, "-c", c)
	}
	bin := v.cfg.VTYSHPath
	if bin == "" {
		bin = "/usr/bin/vtysh"
	}
	cmd := exec.Command(bin, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vtysh error: %v stdout=%q stderr=%q", err, out.String(), errb.String())
	}
	return nil
}

func (v *VTYSH) AnnounceVIP(ip net.IP, prefixLen int) error {
	fam := ipFamily(ip)
	cmds := []string{}

	if v.cfg.EnsureStatic {
		if fam == "ipv4" {
			cmds = append(cmds, fmt.Sprintf("ip route %s/%d Null0", ip.String(), prefixLen))
		} else {
			cmds = append(cmds, fmt.Sprintf("ipv6 route %s/%d Null0", ip.String(), prefixLen))
		}
	}

	if fam == "ipv4" {
		cmds = append(
			cmds,
			fmt.Sprintf("router bgp %d", v.cfg.ASN),
			" address-family ipv4 unicast",
			fmt.Sprintf("  network %s/%d", ip.String(), prefixLen),
			" exit-address-family",
		)
	} else {
		cmds = append(
			cmds,
			fmt.Sprintf("router bgp %d", v.cfg.ASN),
			" address-family ipv6 unicast",
			fmt.Sprintf("  network %s/%d", ip.String(), prefixLen),
			" exit-address-family",
		)
	}
	return v.run(cmds)
}

func (v *VTYSH) WithdrawVIP(ip net.IP, prefixLen int) error {
	fam := ipFamily(ip)
	cmds := []string{}

	if fam == "ipv4" {
		cmds = append(
			cmds,
			fmt.Sprintf("router bgp %d", v.cfg.ASN),
			" address-family ipv4 unicast",
			fmt.Sprintf("  no network %s/%d", ip.String(), prefixLen),
			" exit-address-family",
		)
		if v.cfg.EnsureStatic {
			cmds = append(cmds, fmt.Sprintf("no ip route %s/%d Null0", ip.String(), prefixLen))
		}
	} else {
		cmds = append(
			cmds,
			fmt.Sprintf("router bgp %d", v.cfg.ASN),
			" address-family ipv6 unicast",
			fmt.Sprintf("  no network %s/%d", ip.String(), prefixLen),
			" exit-address-family",
		)
		if v.cfg.EnsureStatic {
			cmds = append(cmds, fmt.Sprintf("no ipv6 route %s/%d Null0", ip.String(), prefixLen))
		}
	}
	return v.run(cmds)
}

func (v *VTYSH) ListAnnounced() (map[string]bool, error) {
	return map[string]bool{}, nil
}
