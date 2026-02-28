package connection

import (
	"log"
	"net"
	"strings"
)

// ModemState tracks GSM modem emulation state for a connection
type ModemState struct {
	Verbose bool // true: text responses (OK/CONNECT/ERROR), false: numeric (0/1/4)
	Echo    bool
}

// NewModemState creates default modem state
func NewModemState() *ModemState {
	return &ModemState{
		Verbose: true,
		Echo:    true,
	}
}

// WriteModemOK sends OK response respecting verbose mode
func (m *ModemState) WriteModemOK(conn net.Conn) error {
	if m.Verbose {
		_, err := conn.Write([]byte("\r\nOK\r\n"))
		return err
	}
	_, err := conn.Write([]byte("0\r"))
	return err
}

// WriteModemError sends ERROR response respecting verbose mode
func (m *ModemState) WriteModemError(conn net.Conn) error {
	if m.Verbose {
		_, err := conn.Write([]byte("\r\nERROR\r\n"))
		return err
	}
	_, err := conn.Write([]byte("4\r"))
	return err
}

// WriteModemConnect sends CONNECT response respecting verbose mode
func (m *ModemState) WriteModemConnect(conn net.Conn) error {
	if m.Verbose {
		_, err := conn.Write([]byte("\r\nCONNECT 9600\r\n"))
		return err
	}
	_, err := conn.Write([]byte("1\r"))
	return err
}

// WriteModemNoCarrier sends NO CARRIER response respecting verbose mode
func (m *ModemState) WriteModemNoCarrier(conn net.Conn) error {
	if m.Verbose {
		_, err := conn.Write([]byte("\r\nNO CARRIER\r\n"))
		return err
	}
	_, err := conn.Write([]byte("3\r"))
	return err
}

// HandleCommand processes a generic modem AT command.
// Returns true if the command was handled.
func (m *ModemState) HandleCommand(conn net.Conn, cmdLine string) bool {
	upper := strings.ToUpper(strings.TrimSpace(cmdLine))

	switch {
	case upper == "AT":
		m.WriteModemOK(conn)
		return true

	case upper == "ATZ" || upper == "ATZ0":
		m.Verbose = true
		m.Echo = true
		m.WriteModemOK(conn)
		return true

	case upper == "ATE0":
		m.Echo = false
		m.WriteModemOK(conn)
		return true

	case upper == "ATE1":
		m.Echo = true
		m.WriteModemOK(conn)
		return true

	case upper == "ATV0":
		// Respond in current mode, then switch
		m.WriteModemOK(conn)
		m.Verbose = false
		return true

	case upper == "ATV1":
		m.WriteModemOK(conn)
		m.Verbose = true
		return true

	case upper == "ATH" || upper == "ATH0":
		m.WriteModemOK(conn)
		return true

	case strings.HasPrefix(upper, "ATI"):
		m.writeInfoResponse(conn, "RFC2217-PROXY")
		return true

	case strings.HasPrefix(upper, "AT+CGMI"):
		m.writeInfoResponse(conn, "RFC2217-PROXY")
		return true

	case strings.HasPrefix(upper, "AT+CPIN"):
		m.writeInfoResponse(conn, "+CPIN: READY")
		return true

	case strings.HasPrefix(upper, "AT+CSQ"):
		m.writeInfoResponse(conn, "+CSQ: 31,0")
		return true

	case strings.HasPrefix(upper, "ATS"),
		strings.HasPrefix(upper, "AT&"),
		strings.HasPrefix(upper, "AT\\"):
		m.WriteModemOK(conn)
		return true
	}

	// Generic AT+ commands
	if strings.HasPrefix(upper, "AT+") {
		log.Printf("[modem] unhandled AT+ command: %s, responding OK", cmdLine)
		m.WriteModemOK(conn)
		return true
	}

	// Any other AT command
	if strings.HasPrefix(upper, "AT") {
		log.Printf("[modem] unhandled command: %s, responding OK", cmdLine)
		m.WriteModemOK(conn)
		return true
	}

	return false
}

// writeInfoResponse sends an info line followed by OK
func (m *ModemState) writeInfoResponse(conn net.Conn, info string) {
	if m.Verbose {
		conn.Write([]byte("\r\n" + info + "\r\n\r\nOK\r\n"))
	} else {
		conn.Write([]byte(info + "\r0\r"))
	}
}
