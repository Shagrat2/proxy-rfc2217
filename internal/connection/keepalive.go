package connection

import (
	"net"
	"time"
)

// SetTCPKeepalive configures aggressive TCP keepalive on a connection
// idle: time before first probe
// interval: time between probes
// count: number of probes before connection is considered dead
func SetTCPKeepalive(conn net.Conn, idle, interval time.Duration, count int) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil // Not a TCP connection, skip
	}

	// Enable keepalive
	if err := tcpConn.SetKeepAlive(true); err != nil {
		return err
	}

	// Set keepalive period (this sets TCP_KEEPINTVL on most systems)
	if err := tcpConn.SetKeepAlivePeriod(interval); err != nil {
		return err
	}

	// Set platform-specific options (TCP_KEEPIDLE, TCP_KEEPCNT)
	return setKeepaliveOptions(tcpConn, idle, interval, count)
}
