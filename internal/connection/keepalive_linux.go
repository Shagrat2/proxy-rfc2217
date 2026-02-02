//go:build linux

package connection

import (
	"net"
	"syscall"
	"time"
)

const (
	TCP_KEEPIDLE     = 4   // Time before first probe (seconds)
	TCP_KEEPINTVL    = 5   // Interval between probes (seconds)
	TCP_KEEPCNT      = 6   // Number of probes
	TCP_USER_TIMEOUT = 0x12 // Max time for data to remain unacked (milliseconds)
)

func setKeepaliveOptions(conn *net.TCPConn, idle, interval time.Duration, count int) error {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return err
	}

	var sysErr error
	err = rawConn.Control(func(fd uintptr) {
		// Set TCP_KEEPIDLE - time before first probe
		sysErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, TCP_KEEPIDLE, int(idle.Seconds()))
		if sysErr != nil {
			return
		}

		// Set TCP_KEEPINTVL - interval between probes
		sysErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, TCP_KEEPINTVL, int(interval.Seconds()))
		if sysErr != nil {
			return
		}

		// Set TCP_KEEPCNT - number of probes
		sysErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, TCP_KEEPCNT, count)
		if sysErr != nil {
			return
		}

		// Set TCP_USER_TIMEOUT - close connection if data not acked within timeout
		// Value is in milliseconds. Use idle + (interval * count) as reasonable default
		userTimeout := int(idle.Milliseconds()) + int(interval.Milliseconds())*count
		sysErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, TCP_USER_TIMEOUT, userTimeout)
	})

	if err != nil {
		return err
	}
	return sysErr
}
