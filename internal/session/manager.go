package session

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Session represents an active client-device session
type Session struct {
	ID          string
	DeviceID    string
	ClientConn  net.Conn
	DeviceConn  net.Conn
	StartedAt   time.Time
	BytesIn     int64 // bytes from client to device
	BytesOut    int64 // bytes from device to client
	Debug       bool
	IdleTimeout time.Duration // Timeout for NOP keepalive

	done chan struct{}
}

// Manager manages active sessions
type Manager struct {
	sessions    sync.Map // map[string]*Session
	counter     uint64
	debug       bool
	idleTimeout time.Duration
	onStart     func(*Session)
	onEnd       func(*Session)
}

// NewManager creates a new session manager
func NewManager(debug bool, idleTimeout time.Duration) *Manager {
	return &Manager{debug: debug, idleTimeout: idleTimeout}
}

// SetCallbacks sets session lifecycle callbacks
func (m *Manager) SetCallbacks(onStart, onEnd func(*Session)) {
	m.onStart = onStart
	m.onEnd = onEnd
}

// Create creates a new session
func (m *Manager) Create(deviceID string, clientConn, deviceConn net.Conn) *Session {
	id := fmt.Sprintf("sess_%d_%d", time.Now().Unix(), atomic.AddUint64(&m.counter, 1))

	sess := &Session{
		ID:          id,
		DeviceID:    deviceID,
		ClientConn:  clientConn,
		DeviceConn:  deviceConn,
		StartedAt:   time.Now(),
		Debug:       m.debug,
		IdleTimeout: m.idleTimeout,
		done:        make(chan struct{}),
	}

	m.sessions.Store(id, sess)

	if m.onStart != nil {
		m.onStart(sess)
	}

	return sess
}

// End ends a session
func (m *Manager) End(sessionID string) {
	val, ok := m.sessions.LoadAndDelete(sessionID)
	if !ok {
		return
	}

	sess := val.(*Session)
	close(sess.done)

	if m.onEnd != nil {
		m.onEnd(sess)
	}
}

// Terminate forcefully terminates a session by closing connections
func (m *Manager) Terminate(sessionID string) bool {
	val, ok := m.sessions.Load(sessionID)
	if !ok {
		return false
	}

	sess := val.(*Session)
	// Closing connections will cause bridge to exit and call End()
	sess.ClientConn.Close()
	sess.DeviceConn.Close()
	return true
}

// Get returns a session by ID
func (m *Manager) Get(sessionID string) (*Session, bool) {
	val, ok := m.sessions.Load(sessionID)
	if !ok {
		return nil, false
	}
	return val.(*Session), true
}

// GetByDevice returns session for a device
func (m *Manager) GetByDevice(deviceID string) (*Session, bool) {
	var found *Session
	m.sessions.Range(func(key, value any) bool {
		sess := value.(*Session)
		if sess.DeviceID == deviceID {
			found = sess
			return false
		}
		return true
	})
	return found, found != nil
}

// List returns all active sessions
func (m *Manager) List() []*Session {
	var sessions []*Session
	m.sessions.Range(func(key, value any) bool {
		sessions = append(sessions, value.(*Session))
		return true
	})
	return sessions
}

// Count returns number of active sessions
func (m *Manager) Count() int {
	count := 0
	m.sessions.Range(func(key, value any) bool {
		count++
		return true
	})
	return count
}

// SessionInfo is used for API responses
type SessionInfo struct {
	ID           string    `json:"id"`
	DeviceID     string    `json:"device_id"`
	ClientAddr   string    `json:"client_addr"`
	DeviceAddr   string    `json:"device_addr"`
	StartedAt    time.Time `json:"started_at"`
	DurationSecs float64   `json:"duration_secs"`
	BytesIn      int64     `json:"bytes_in"`
	BytesOut     int64     `json:"bytes_out"`
}

// ListInfo returns session info for API
func (m *Manager) ListInfo() []SessionInfo {
	var infos []SessionInfo
	now := time.Now()
	m.sessions.Range(func(key, value any) bool {
		sess := value.(*Session)
		info := SessionInfo{
			ID:           sess.ID,
			DeviceID:     sess.DeviceID,
			ClientAddr:   sess.ClientConn.RemoteAddr().String(),
			DeviceAddr:   sess.DeviceConn.RemoteAddr().String(),
			StartedAt:    sess.StartedAt,
			DurationSecs: now.Sub(sess.StartedAt).Seconds(),
			BytesIn:      atomic.LoadInt64(&sess.BytesIn),
			BytesOut:     atomic.LoadInt64(&sess.BytesOut),
		}
		infos = append(infos, info)
		return true
	})
	return infos
}
