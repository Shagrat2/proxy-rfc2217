//go:build !linux && !darwin

package connection

import (
	"net"
	"time"
)

func setKeepaliveOptions(conn *net.TCPConn, idle, interval time.Duration, count int) error {
	// Platform-specific keepalive options not available
	// SetKeepAlive and SetKeepAlivePeriod are already set in the main function
	return nil
}
