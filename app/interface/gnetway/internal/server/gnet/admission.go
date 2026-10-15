package gnet

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/config"
	"golang.org/x/time/rate"
)

const admissionStateIdleTTL = 10 * time.Minute

type admissionIPState struct {
	connections       int
	lastSeen          time.Time
	connectionLimiter *rate.Limiter
	handshakeLimiter  *rate.Limiter
}

// connectionAdmission owns process-local limits. It deliberately does not
// pretend to be a distributed account quota; that decision belongs to the
// session service shared by all gateway replicas.
type connectionAdmission struct {
	maxConnections      int
	maxConnectionsPerIP int
	maxAuthConnections  int
	connectionRate      rate.Limit
	connectionBurst     int
	handshakeRate       rate.Limit
	handshakeBurst      int
	handshakeSlots      chan struct{}
	active              atomic.Int64
	mu                  sync.Mutex
	byIP                map[string]*admissionIPState
	byAuthKey           map[int64]int
}

func newConnectionAdmission(c config.GnetwayConfig) *connectionAdmission {
	c.Normalize()
	return &connectionAdmission{
		maxConnections:      c.MaxConnections,
		maxConnectionsPerIP: c.MaxConnectionsPerIP,
		maxAuthConnections:  c.MaxConnectionsPerAuthKey,
		connectionRate:      rate.Limit(c.ConnectionRatePerIP),
		connectionBurst:     c.ConnectionBurstPerIP,
		handshakeRate:       rate.Limit(c.HandshakeRatePerIP),
		handshakeBurst:      c.HandshakeBurstPerIP,
		handshakeSlots:      make(chan struct{}, c.MaxHandshakesInFlight),
		byIP:                make(map[string]*admissionIPState),
		byAuthKey:           make(map[int64]int),
	}
}

func (a *connectionAdmission) ipStateLocked(ip string, now time.Time) *admissionIPState {
	state := a.byIP[ip]
	if state == nil {
		state = &admissionIPState{lastSeen: now}
		if a.connectionRate > 0 {
			state.connectionLimiter = rate.NewLimiter(a.connectionRate, maxInt(a.connectionBurst, 1))
		}
		if a.handshakeRate > 0 {
			state.handshakeLimiter = rate.NewLimiter(a.handshakeRate, maxInt(a.handshakeBurst, 1))
		}
		a.byIP[ip] = state
	}
	state.lastSeen = now
	return state
}

func (a *connectionAdmission) acquireConnection(ip string) (bool, string) {
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.maxConnections > 0 && a.active.Load() >= int64(a.maxConnections) {
		return false, "global_limit"
	}
	state := a.ipStateLocked(ip, now)
	if a.maxConnectionsPerIP > 0 && state.connections >= a.maxConnectionsPerIP {
		return false, "ip_limit"
	}
	if state.connectionLimiter != nil && !state.connectionLimiter.AllowN(now, 1) {
		return false, "ip_rate"
	}
	state.connections++
	a.active.Add(1)
	return true, ""
}

func (a *connectionAdmission) releaseConnection(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if state := a.byIP[ip]; state != nil {
		if state.connections > 0 {
			state.connections--
		}
		state.lastSeen = time.Now()
	}
	if a.active.Load() > 0 {
		a.active.Add(-1)
	}
}

func (a *connectionAdmission) acquireHandshake(ip string) bool {
	now := time.Now()
	a.mu.Lock()
	state := a.ipStateLocked(ip, now)
	allowed := state.handshakeLimiter == nil || state.handshakeLimiter.AllowN(now, 1)
	a.mu.Unlock()
	if !allowed {
		return false
	}
	select {
	case a.handshakeSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (a *connectionAdmission) releaseHandshake() {
	select {
	case <-a.handshakeSlots:
	default:
	}
}

func (a *connectionAdmission) attachAuth(authKeyID int64) bool {
	if authKeyID <= 0 || a.maxAuthConnections <= 0 {
		return true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.byAuthKey[authKeyID] >= a.maxAuthConnections {
		return false
	}
	a.byAuthKey[authKeyID]++
	return true
}

func (a *connectionAdmission) releaseAuth(authKeyID int64) {
	if authKeyID <= 0 || a.maxAuthConnections <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if count := a.byAuthKey[authKeyID]; count <= 1 {
		delete(a.byAuthKey, authKeyID)
	} else {
		a.byAuthKey[authKeyID] = count - 1
	}
}

func (a *connectionAdmission) cleanup(now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for ip, state := range a.byIP {
		if state.connections == 0 && now.Sub(state.lastSeen) > admissionStateIdleTTL {
			delete(a.byIP, ip)
		}
	}
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
