package gnet

import (
	"testing"
	"time"

	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/config"
)

func TestConnectionAdmissionEnforcesGlobalAndPerIPLimits(t *testing.T) {
	a := newConnectionAdmission(config.GnetwayConfig{MaxConnections: 2, MaxConnectionsPerIP: 1, MaxHandshakesInFlight: 1})
	if ok, reason := a.acquireConnection("192.0.2.1"); !ok || reason != "" {
		t.Fatalf("first connection = %v, %q", ok, reason)
	}
	if ok, reason := a.acquireConnection("192.0.2.1"); ok || reason != "ip_limit" {
		t.Fatalf("per-ip limit = %v, %q", ok, reason)
	}
	if ok, reason := a.acquireConnection("192.0.2.2"); !ok || reason != "" {
		t.Fatalf("second connection = %v, %q", ok, reason)
	}
	if ok, reason := a.acquireConnection("192.0.2.3"); ok || reason != "global_limit" {
		t.Fatalf("global limit = %v, %q", ok, reason)
	}
	a.releaseConnection("192.0.2.1")
	a.releaseConnection("192.0.2.2")
	if got := a.active.Load(); got != 0 {
		t.Fatalf("active after release = %d", got)
	}
}

func TestConnectionAdmissionBoundsHandshakeSlotsAndAuthKeys(t *testing.T) {
	a := newConnectionAdmission(config.GnetwayConfig{MaxConnections: 2, MaxHandshakesInFlight: 1, MaxConnectionsPerAuthKey: 1})
	if !a.acquireHandshake("192.0.2.1") {
		t.Fatal("first handshake rejected")
	}
	if a.acquireHandshake("192.0.2.2") {
		t.Fatal("second handshake bypassed global slot")
	}
	a.releaseHandshake()
	if !a.attachAuth(42) || a.attachAuth(42) {
		t.Fatal("auth-key quota not enforced")
	}
	a.releaseAuth(42)
	if !a.attachAuth(42) {
		t.Fatal("released auth-key quota was not reusable")
	}
	a.releaseAuth(42)
}

func TestConnectionAdmissionRateLimit(t *testing.T) {
	a := newConnectionAdmission(config.GnetwayConfig{MaxConnections: 4, ConnectionRatePerIP: 1, ConnectionBurstPerIP: 1, MaxHandshakesInFlight: 1})
	if ok, _ := a.acquireConnection("192.0.2.1"); !ok {
		t.Fatal("burst connection rejected")
	}
	if ok, reason := a.acquireConnection("192.0.2.1"); ok || reason != "ip_rate" {
		t.Fatalf("rate limit = %v, %q", ok, reason)
	}
	time.Sleep(1100 * time.Millisecond)
	a.releaseConnection("192.0.2.1")
	if ok, _ := a.acquireConnection("192.0.2.1"); !ok {
		t.Fatal("connection was not admitted after rate refill")
	}
}

func TestServerCloseWithoutEngineIsSafeAndIdempotent(t *testing.T) {
	server := new(Server)
	server.Close()
	server.Close()
	if !server.draining.Load() {
		t.Fatal("close did not leave the server in draining state")
	}
}
