package session

import (
	"encoding/hex"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Telnet NOP command for keepalive
var telnetNOP = []byte{0xFF, 0xF1}

// Bridge creates a bidirectional data bridge between client and device
type Bridge struct {
	session          *Session
	lastClientActive int64 // Unix timestamp of last client activity
	lastDeviceActive int64 // Unix timestamp of last device activity
}

// NewBridge creates a new bridge for a session
func NewBridge(session *Session) *Bridge {
	now := time.Now().Unix()
	return &Bridge{
		session:          session,
		lastClientActive: now,
		lastDeviceActive: now,
	}
}

// Run starts the bidirectional data transfer
// Blocks until one side closes or an error occurs
func (b *Bridge) Run() {
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Device
	go func() {
		defer wg.Done()
		n := b.copyWithActivity(b.session.DeviceConn, b.session.ClientConn, &b.session.BytesIn, &b.lastClientActive, "client->device")
		log.Printf("[bridge] %s: client->device total: %d bytes", b.session.ID, n)
	}()

	// Device -> Client
	go func() {
		defer wg.Done()
		n := b.copyWithActivity(b.session.ClientConn, b.session.DeviceConn, &b.session.BytesOut, &b.lastDeviceActive, "device->client")
		log.Printf("[bridge] %s: device->client total: %d bytes", b.session.ID, n)
	}()

	// Wait for either direction to close
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Start keepalive goroutine
	stopKeepalive := make(chan struct{})
	go b.keepalive(stopKeepalive)

	select {
	case <-done:
	case <-b.session.done:
	}

	// Stop keepalive
	close(stopKeepalive)

	// Close both connections to ensure both goroutines exit
	b.session.ClientConn.Close()
	b.session.DeviceConn.Close()

	wg.Wait()
	log.Printf("[bridge] %s: closed (in=%d, out=%d)",
		b.session.ID,
		atomic.LoadInt64(&b.session.BytesIn),
		atomic.LoadInt64(&b.session.BytesOut))
}

// copyWithActivity transfers data from src to dst, counting bytes and updating activity timestamp
func (b *Bridge) copyWithActivity(dst, src net.Conn, counter *int64, lastActive *int64, direction string) int64 {
	buf := make([]byte, 4096)
	var total int64

	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			// Update activity timestamp
			atomic.StoreInt64(lastActive, time.Now().Unix())

			if b.session.Debug {
				log.Printf("[bridge] %s %s: %d bytes\n%s",
					b.session.ID, direction, n, hex.Dump(buf[:n]))
			}
			written, writeErr := dst.Write(buf[:n])
			if written > 0 {
				atomic.AddInt64(counter, int64(written))
				total += int64(written)
			}
			if writeErr != nil {
				return total
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				log.Printf("[bridge] %s: read error: %v", b.session.ID, readErr)
			}
			return total
		}
	}
}

// keepalive sends Telnet NOP to both connections if idle
func (b *Bridge) keepalive(stop chan struct{}) {
	if b.session.IdleTimeout <= 0 {
		return
	}

	ticker := time.NewTicker(b.session.IdleTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			now := time.Now().Unix()
			idleSecs := int64(b.session.IdleTimeout.Seconds())

			// Check client connection
			clientIdle := now - atomic.LoadInt64(&b.lastClientActive)
			if clientIdle >= idleSecs {
				b.session.ClientConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				_, err := b.session.ClientConn.Write(telnetNOP)
				b.session.ClientConn.SetWriteDeadline(time.Time{})
				if err != nil {
					log.Printf("[bridge] %s: client keepalive failed: %v", b.session.ID, err)
					b.session.ClientConn.Close()
					return
				}
				if b.session.Debug {
					log.Printf("[bridge] %s: sent NOP to client (idle %ds)", b.session.ID, clientIdle)
				}
			}

			// Check device connection
			deviceIdle := now - atomic.LoadInt64(&b.lastDeviceActive)
			if deviceIdle >= idleSecs {
				b.session.DeviceConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				_, err := b.session.DeviceConn.Write(telnetNOP)
				b.session.DeviceConn.SetWriteDeadline(time.Time{})
				if err != nil {
					log.Printf("[bridge] %s: device keepalive failed: %v", b.session.ID, err)
					b.session.DeviceConn.Close()
					return
				}
				if b.session.Debug {
					log.Printf("[bridge] %s: sent NOP to device (idle %ds)", b.session.ID, deviceIdle)
				}
			}
		}
	}
}
