package classroom

import (
	"net"
	"reflect"
	"strings"
	"testing"

	"github.com/mrmxf/nabu/internal/content"
)

func TestPickLAN(t *testing.T) {
	var ips []net.IP
	for _, s := range []string{"127.0.0.1", "192.168.1.20", "10.0.0.5", "169.254.3.4",
		"100.101.102.103", "203.0.113.9", "192.168.1.20", "fe80::1", "0.0.0.0"} {
		ips = append(ips, net.ParseIP(s))
	}
	got := pickLAN(ips)
	want := []string{"10.0.0.5", "192.168.1.20", "203.0.113.9"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pickLAN = %v, want %v (private first, sorted, no loopback/link-local/tailnet/dupes)", got, want)
	}
}

func TestSkipInterface(t *testing.T) {
	for name, skip := range map[string]bool{"docker0": true, "veth12ab": true, "br-1f2e": true,
		"podman0": true, "tailscale0": true, "eth0": false, "wlan0": false, "en0": false} {
		if skipInterface(name) != skip {
			t.Errorf("skipInterface(%q) = %v, want %v", name, !skip, skip)
		}
	}
}

func TestOfflineGaps(t *testing.T) {
	courses := []content.Course{{Slug: "c", Lessons: []content.Lesson{{Slug: "l", Slides: []content.Slide{
		{Slug: "b", AgentJobs: []content.AgentJob{{ID: "j", Agent: "search"}}},
		{Slug: "a", SimHTML: "<p>sample</p>", AgentJobs: []content.AgentJob{{ID: "ok", Agent: "sort", SimOutputFallback: true}}},
		{Slug: "c", AgentJobs: []content.AgentJob{{ID: "empty", Agent: "present", SimOutputFallback: true}}},
		{Slug: "d"},
	}}}}}
	gaps := OfflineGaps(courses)
	if len(gaps) != 2 {
		t.Fatalf("gaps = %+v, want 2", gaps)
	}
	if gaps[0].Where != "c/l/b#j" || gaps[1].Where != "c/l/c#empty" {
		t.Errorf("gaps not sorted by Where: %+v", gaps)
	}
	if !strings.Contains(gaps[1].Why, "no sim_output") {
		t.Errorf("fallback with no sim_output: why = %q", gaps[1].Why)
	}
}

func TestQR(t *testing.T) {
	out, err := QR("http://192.168.1.20:8090")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	width := len([]rune(lines[0]))
	// a version 2-3 code is 25-29 modules, plus a 2-module quiet zone each side
	if width < 29 || width > 33 {
		t.Errorf("QR width %d modules", width)
	}
	if len(lines) != (width+1)/2 {
		t.Errorf("QR has %d rows for %d modules: want two modules per row", len(lines), width)
	}
	for i, l := range lines {
		if len([]rune(l)) != width {
			t.Fatalf("row %d is %d wide, want %d", i, len([]rune(l)), width)
		}
	}
	if !strings.HasPrefix(lines[0], "████") {
		t.Error("QR has no light quiet zone")
	}
}
