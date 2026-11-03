package serverctl

import (
	"net"
	"strings"
	"testing"
)

func TestPortInUse(t *testing.T) {
	// A listener we own makes the port occupied; closing it frees it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	if !portInUse(addr) {
		t.Errorf("portInUse(%s) = false while listening, want true", addr)
	}

	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if portInUse(addr) {
		t.Errorf("portInUse(%s) = true after close, want false", addr)
	}
}

func TestPortInUseErr(t *testing.T) {
	err := portInUseErr("/data/nabu.pid", "127.0.0.1:8090")
	msg := err.Error()
	// The guidance must name the port and offer a sudo command to stop it.
	for _, want := range []string{"8090", "sudo", "/data/nabu.pid"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q missing %q", msg, want)
		}
	}
}
