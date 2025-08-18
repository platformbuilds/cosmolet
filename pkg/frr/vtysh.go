package frr

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

// VTYSH implements Manager using FRR's vtysh binary.
type VTYSH struct {
	cfg Config
}

func NewVTYSH(cfg Config) *VTYSH { return &VTYSH{cfg: cfg} }

// run executes a sequence of vtysh -c commands inside "configure terminal".
func (v *VTYSH) run(cmds []string) error {
	args := []string{"-c", "configure terminal"}
	for _, c := range cmds {
		args = append(args, "-c", c)
	}
	bin := v.cfg.VTYSHPath
	if bin == "" {
		bin = "/usr/bin/vtysh"
	}

	// --- Security hardening for gosec G204 ---
	// 1) Binary path must be absolute and from an allowlist.
	bin = filepath.Clean(bin)
	if !filepath.IsAbs(bin) {
		return fmt.Errorf("vtysh path must be absolute: %q", bin)
	}
	switch bin {
	case "/usr/bin/vtysh", "/sbin/vtysh", "/usr/sbin/vtysh":
		// allowed
	default:
		return fmt.Errorf("vtysh path not in allowlist: %q", bin)
	}

	// 2) Validate each '-c' payload to only contain safe characters
	//    (letters, digits, spaces, / . : , - _ and typical FRR tokens including IPv6).
	safe := regexp.MustCompile(`^[a-zA-Z0-9\s\/\.\:\,\-\_]+$`)
	for i := 0; i < len(args); i++ {
		if args[i] != "-c" {
			return fmt.Errorf("unexpected vtysh arg: %q (only -c allowed)", args[i])
		}
		i++
		if i >= len(args) {
			return fmt.Errorf("missing command after -c")
		}
		if !safe.MatchString(args[i]) {
			return fmt.Errorf("unsafe characters in vtysh command: %q", args[i])
		}
	}

	// 3) Use a context to bound execution time
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// #nosec G204 -- exec.CommandContext uses a validated absolute binary path from a strict allowlist,
	// and each "-c" payload is regex-validated (no shell involved, bounded by context timeout).
	cmd := exec.CommandContext(ctx, bin, args...)

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
	// TODO: implement `show running-config` parse if needed.
	return map[string]bool{}, nil
}
