// Package classroom backs `nabu classroom`: one command that turns a laptop
// into the room's server. It seeds the course, switches classroom fast login
// on for the run, serves on every interface, prints the LAN addresses and a QR
// code for students, and warns about agent jobs that cannot run offline.
//
// Mode: classroom. See help:reference#deployment-modes.
package classroom

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"rsc.io/qr"

	"github.com/mrmxf/nabu/internal/content"
)

// Gap is an agent job that shows nothing useful when no worker is reachable.
type Gap struct {
	// Where names the job as <course>/<lesson>/<slide>#<job>.
	Where string
	// Agent is the authored agent key.
	Agent string
	// Why says what is missing.
	Why string
}

// OfflineGaps lists the agent jobs that need a worker: those without
// `sim_output_fallback: true`, and those with it whose slide has no
// `sim_output` to show. The list is sorted by Where.
func OfflineGaps(courses []content.Course) []Gap {
	var gaps []Gap
	for _, c := range courses {
		for _, l := range c.Lessons {
			for _, s := range l.Slides {
				for _, j := range s.AgentJobs {
					where := fmt.Sprintf("%s/%s/%s#%s", c.Slug, l.Slug, s.Slug, j.ID)
					switch {
					case !j.SimOutputFallback:
						gaps = append(gaps, Gap{where, j.Agent, "no sim_output_fallback: true"})
					case strings.TrimSpace(s.SimHTML) == "":
						gaps = append(gaps, Gap{where, j.Agent, "sim_output_fallback is on but the slide has no sim_output"})
					}
				}
			}
		}
	}
	sort.Slice(gaps, func(i, k int) bool { return gaps[i].Where < gaps[k].Where })
	return gaps
}

// skipInterface reports interfaces students cannot reach: container bridges and
// virtual links that every laptop with podman or docker carries.
func skipInterface(name string) bool {
	for _, p := range []string{"docker", "veth", "br-", "virbr", "podman", "cni", "flannel", "vmnet", "vboxnet", "tailscale", "utun"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// LANAddrs returns this machine's IPv4 addresses that students on the room's
// network can reach: up, not loopback, not a container bridge or VPN tunnel.
// Private (RFC 1918) addresses come first; the result is sorted.
func LANAddrs() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list network interfaces: %w", err)
	}
	var ips []net.IP
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 || skipInterface(ifc.Name) {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				ips = append(ips, n.IP)
			}
		}
	}
	return pickLAN(ips), nil
}

// pickLAN keeps the IPv4 addresses a student device can use, private ones
// first, each group sorted. Loopback, link-local and tailnet (100.64/10)
// addresses are dropped: the room cannot reach them.
func pickLAN(ips []net.IP) []string {
	var private, other []string
	seen := map[string]bool{}
	for _, ip := range ips {
		v4 := ip.To4()
		if v4 == nil || v4.IsLoopback() || v4.IsLinkLocalUnicast() || v4.IsUnspecified() || isTailnet(v4) {
			continue
		}
		s := v4.String()
		if seen[s] {
			continue
		}
		seen[s] = true
		if v4.IsPrivate() {
			private = append(private, s)
		} else {
			other = append(other, s)
		}
	}
	sort.Strings(private)
	sort.Strings(other)
	return append(private, other...)
}

// isTailnet reports an address in Tailscale's 100.64.0.0/10 range.
func isTailnet(ip net.IP) bool {
	return ip[0] == 100 && ip[1]&0xc0 == 64
}

// QR renders text as a QR code for a terminal, two modules per character row,
// with a quiet zone. Light modules are drawn as blocks, so it scans on a dark
// terminal; phone cameras also read the inverted form on a light one.
func QR(text string) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", fmt.Errorf("encode QR code: %w", err)
	}
	const quiet = 2
	n := code.Size + 2*quiet
	light := func(x, y int) bool {
		x, y = x-quiet, y-quiet
		if x < 0 || y < 0 || x >= code.Size || y >= code.Size {
			return true
		}
		return !code.Black(x, y)
	}
	var b strings.Builder
	for y := 0; y < n; y += 2 {
		for x := 0; x < n; x++ {
			top, bottom := light(x, y), y+1 >= n || light(x, y+1)
			switch {
			case top && bottom:
				b.WriteString("█")
			case top:
				b.WriteString("▀")
			case bottom:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
