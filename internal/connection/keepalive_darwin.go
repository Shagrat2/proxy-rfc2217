//go:build darwin

package connection

import (
	"net"
	"syscall"
	"time"
)

const (
	TCP_KEEPIDLE  = 0x10 // TCP_KEEPALIVE on macOS
	TCP_KEEPINTVL = 0x101
	TCP_KEEPCNT   = 0x102
)

func setKeepaliveOptions(conn *net.TCPConn, idle, interval time.Duration, count int) error {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return err
	}

	var sysErr error
	err = rawConn.Control(func(fd uintptr) {
		// Set TCP_KEEPALIVE (idle time) on macOS
		sysErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, TCP_KEEPIDLE, int(idle.Seconds()))
		if sysErr != nil {
			return
		}

		// TCP_KEEPINTVL and TCP_KEEPCNT may not be available on older macOS
		// Ignore errors for these
		syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, TCP_KEEPINTVL, int(interval.Seconds()))
		syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, TCP_KEEPCNT, count)
	})

	if err != nil {
		return err
	}
	return sysErr
}
