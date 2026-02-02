package connection

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

// AT command constants
const (
	CmdReg     = "AT+REG"     // Device registration: AT+REG=<token>
	CmdConnect = "AT+CONNECT" // Client connection: AT+CONNECT=<token>
	CmdDT      = "ATDT"       // Dial tone (optional after registration), may have phone number
	CmdDP      = "ATDP"       // Dial pulse (optional after registration), may have phone number

	RespOK    = "OK\r\n"
	RespError = "ERROR\r\n"
)

// ATCommand represents a parsed AT command
type ATCommand struct {
	Cmd        string         // Command name (AT+REG, AT+CONNECT)
	Param      string         // Parameter value (token)
	Skipped    []byte         // Bytes received before AT command (may contain RFC2217 data)
	USRVCOMCfg *USRVCOMConfig // USR-VCOM configuration if received before AT command
}

// ReadATCommandWithPresets reads AT command, handling USR-VCOM and RFC2217 data before it
// USR-VCOM packets (55 AA 55) are accepted silently (no response)
// RFC2217 data is collected and returned in Skipped field
func ReadATCommandWithPresets(reader *bufio.Reader, conn net.Conn, timeout time.Duration) (*ATCommand, error) {
	var usrvcomCfg *USRVCOMConfig
	var allSkipped []byte
	startTime := time.Now()

	for {
		// Check timeout
		if timeout > 0 && time.Since(startTime) > timeout {
			return nil, fmt.Errorf("timeout waiting for AT command")
		}

		// Set read deadline for this iteration
		if timeout > 0 {
			remaining := timeout - time.Since(startTime)
			if remaining > 0 {
				conn.SetReadDeadline(time.Now().Add(remaining))
			}
		}

		line, skipped, err := readLineWithSkipped(reader)
		if err != nil {
			// If we have USR-VCOM config and got timeout, keep waiting
			if usrvcomCfg != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Printf("[protocol] timeout after USR-VCOM, continuing...")
					continue
				}
			}
			return nil, err
		}

		// Process skipped bytes (data before AT command on this line)
		if len(skipped) > 0 {
			// Check for USR-VCOM Baud Rate Sync packet (55 AA 55)
			if IsUSRVCOM(skipped) {
				cfg := ParseUSRVCOM(skipped)
				if cfg != nil && cfg.Valid {
					cfg.LogConfig("received")
					usrvcomCfg = cfg
					// No response per PUSR specification - just accept silently
					log.Printf("[protocol] USR-VCOM accepted, waiting for AT command...")
				} else {
					log.Printf("[protocol] USR-VCOM parse failed: %s", hex.EncodeToString(skipped))
				}
			} else if isRFC2217Data(skipped) {
				// RFC2217 data - collect it
				log.Printf("[protocol] RFC2217 data before AT: %s", hex.EncodeToString(skipped))
				allSkipped = append(allSkipped, skipped...)
			} else {
				// Unknown binary data
				log.Printf("[protocol] skipped %d bytes: %s", len(skipped), hex.EncodeToString(skipped))
				allSkipped = append(allSkipped, skipped...)
			}
		}

		cmdLine := strings.TrimSpace(string(line))

		// Empty line - continue waiting (USR-VCOM packet without AT on same line)
		if cmdLine == "" {
			continue
		}

		log.Printf("[protocol] received: %q", cmdLine)

		// Parse AT command
		if cmd := parseATCommand(cmdLine); cmd != nil {
			cmd.Skipped = allSkipped
			cmd.USRVCOMCfg = usrvcomCfg
			return cmd, nil
		}

		// Not an AT command
		// If we already have USR-VCOM config, ignore unknown data and keep waiting
		if usrvcomCfg != nil {
			log.Printf("[protocol] ignoring non-AT data after USR-VCOM: %q", cmdLine)
			continue
		}

		// No USR-VCOM received - this is an error
		return nil, fmt.Errorf("unknown command: %q", cmdLine)
	}
}

// ReadATCommand reads AT command (legacy, for backward compatibility)
func ReadATCommand(reader *bufio.Reader, conn net.Conn) (*ATCommand, error) {
	return ReadATCommandWithPresets(reader, conn, 0)
}

// readLineWithSkipped reads bytes until CR/LF, separating skipped bytes from AT command
func readLineWithSkipped(reader *bufio.Reader) (line []byte, skipped []byte, err error) {
	inATCommand := false

	for {
		b, err := reader.ReadByte()
		if err != nil {
			return line, skipped, err
		}

		// Stop on CR or LF
		if b == '\r' || b == '\n' {
			// Skip following LF if present (for \r\n)
			if b == '\r' {
				if next, err := reader.Peek(1); err == nil && len(next) > 0 && next[0] == '\n' {
					reader.ReadByte()
				}
			}
			return line, skipped, nil
		}

		// Look for 'A' to start AT command
		if !inATCommand && b == 'A' {
			if next, err := reader.Peek(1); err == nil && len(next) > 0 && next[0] == 'T' {
				inATCommand = true
				line = append(line, b)
				continue
			}
		}

		if inATCommand {
			line = append(line, b)
		} else {
			skipped = append(skipped, b)
		}
	}
}

// parseATCommand parses AT command string and returns ATCommand or nil
func parseATCommand(cmdLine string) *ATCommand {
	if strings.HasPrefix(cmdLine, CmdReg+"=") {
		return &ATCommand{Cmd: CmdReg, Param: strings.TrimPrefix(cmdLine, CmdReg+"=")}
	}
	if strings.HasPrefix(cmdLine, CmdConnect+"=") {
		return &ATCommand{Cmd: CmdConnect, Param: strings.TrimPrefix(cmdLine, CmdConnect+"=")}
	}
	if strings.HasPrefix(cmdLine, CmdDT) {
		return &ATCommand{Cmd: CmdDT, Param: strings.TrimPrefix(cmdLine, CmdDT)}
	}
	if strings.HasPrefix(cmdLine, CmdDP) {
		return &ATCommand{Cmd: CmdDP, Param: strings.TrimPrefix(cmdLine, CmdDP)}
	}
	return nil
}

// isRFC2217Data checks if data looks like RFC2217 (starts with IAC)
func isRFC2217Data(data []byte) bool {
	return len(data) >= 3 && data[0] == 0xFF && (data[1] == 0xFA || data[1] == 0xFB || data[1] == 0xFC || data[1] == 0xFD)
}

// WriteOK sends OK response
func WriteOK(conn net.Conn) error {
	_, err := conn.Write([]byte(RespOK))
	return err
}

// WriteError sends ERROR response
func WriteError(conn net.Conn) error {
	_, err := conn.Write([]byte(RespError))
	return err
}
