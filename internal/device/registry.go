package device

import (
	"net"
	"sync"
	"time"
)

// Device represents a connected IoT device
type Device struct {
	ID            string
	Conn          net.Conn
	RegisteredAt  time.Time
	InSession     bool
	SessionID     string
	StopKeepalive chan struct{} // Signal to stop keepalive goroutine

	mu sync.Mutex
}

// SetSession marks device as in session
func (d *Device) SetSession(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.InSession = true
	d.SessionID = sessionID
}

// ClearSession marks device as not in session
func (d *Device) ClearSession() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.InSession = false
	d.SessionID = ""
}

// IsInSession returns true if device is in active session
func (d *Device) IsInSession() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.InSession
}

// Registry manages connected devices
type Registry struct {
	devices sync.Map // map[string]*Device
}

// NewRegistry creates a new device registry
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a device to the registry
func (r *Registry) Register(device *Device) {
	r.devices.Store(device.ID, device)
}

// Unregister removes a device from the registry
func (r *Registry) Unregister(deviceID string) {
	r.devices.Delete(deviceID)
}

// Get returns a device by ID
func (r *Registry) Get(deviceID string) (*Device, bool) {
	val, ok := r.devices.Load(deviceID)
	if !ok {
		return nil, false
	}
	return val.(*Device), true
}

// List returns all registered devices
func (r *Registry) List() []*Device {
	var devices []*Device
	r.devices.Range(func(key, value any) bool {
		devices = append(devices, value.(*Device))
		return true
	})
	return devices
}

// Count returns number of registered devices
func (r *Registry) Count() int {
	count := 0
	r.devices.Range(func(key, value any) bool {
		count++
		return true
	})
	return count
}

// DeviceInfo is used for API responses
type DeviceInfo struct {
	ID           string    `json:"id"`
	RegisteredAt time.Time `json:"registered_at"`
	InSession    bool      `json:"in_session"`
	SessionID    string    `json:"session_id,omitempty"`
	RemoteAddr   string    `json:"remote_addr"`
}

// ListInfo returns device info for API
func (r *Registry) ListInfo() []DeviceInfo {
	var infos []DeviceInfo
	r.devices.Range(func(key, value any) bool {
		d := value.(*Device)
		d.mu.Lock()
		info := DeviceInfo{
			ID:           d.ID,
			RegisteredAt: d.RegisteredAt,
			InSession:    d.InSession,
			SessionID:    d.SessionID,
			RemoteAddr:   d.Conn.RemoteAddr().String(),
		}
		d.mu.Unlock()
		infos = append(infos, info)
		return true
	})
	return infos
}
