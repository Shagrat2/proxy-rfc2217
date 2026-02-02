package connection

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
)

// RFC2217 Telnet constants
const (
	IAC = 0xFF // Interpret As Command
	SB  = 0xFA // Subnegotiation Begin
	SE  = 0xF0 // Subnegotiation End

	ComPortOption = 0x2C // COM-PORT-OPTION (44)

	// Client to server commands (requests)
	SetBaudrate = 0x01
	SetDatasize = 0x02
	SetParity   = 0x03
	SetStopsize = 0x04
	SetControl  = 0x05

	// Server response offset
	ServerResponseOffset = 100
)

// RFC2217Command represents a parsed RFC2217 subnegotiation
type RFC2217Command struct {
	Command byte   // Command code (1-5 for client requests)
	Data    []byte // Command data
}

// RFC2217Buffer holds RFC2217 commands received before AT command
type RFC2217Buffer struct {
	Commands []RFC2217Command
	RawData  []byte // Original raw data to forward
}

// ParseRFC2217Commands parses RFC2217 subnegotiations from raw bytes
// Returns parsed commands and any remaining non-RFC2217 data
func ParseRFC2217Commands(data []byte) *RFC2217Buffer {
	buf := &RFC2217Buffer{
		RawData: data,
	}

	i := 0
	for i < len(data)-2 {
		// Look for IAC SB COM-PORT-OPTION
		if data[i] == IAC && data[i+1] == SB && i+2 < len(data) && data[i+2] == ComPortOption {
			// Find IAC SE
			start := i + 3 // After IAC SB COM-PORT-OPTION
			end := -1
			for j := start; j < len(data)-1; j++ {
				if data[j] == IAC && data[j+1] == SE {
					end = j
					break
				}
			}
			if end > start {
				cmdData := data[start:end]
				if len(cmdData) >= 1 {
					cmd := RFC2217Command{
						Command: cmdData[0],
						Data:    cmdData[1:],
					}
					buf.Commands = append(buf.Commands, cmd)
				}
				i = end + 2 // Skip past IAC SE
				continue
			}
		}
		i++
	}

	return buf
}

// BuildResponse builds RFC2217 server response for a command
// Server responses use command code + 100
func (c *RFC2217Command) BuildResponse() []byte {
	resp := []byte{IAC, SB, ComPortOption, c.Command + ServerResponseOffset}
	resp = append(resp, c.Data...)
	resp = append(resp, IAC, SE)
	return resp
}

// IsQuery returns true if this is a query command (value=0 means "request current value")
func (c *RFC2217Command) IsQuery() bool {
	switch c.Command {
	case SetBaudrate:
		if len(c.Data) >= 4 {
			baud := binary.BigEndian.Uint32(c.Data[:4])
			return baud == 0
		}
	case SetDatasize, SetParity, SetStopsize, SetControl:
		if len(c.Data) >= 1 {
			return c.Data[0] == 0
		}
	}
	return false
}

// String returns human-readable description of the command
func (c *RFC2217Command) String() string {
	switch c.Command {
	case SetBaudrate:
		if len(c.Data) >= 4 {
			baud := binary.BigEndian.Uint32(c.Data[:4])
			return fmt.Sprintf("SET-BAUDRATE: %d", baud)
		}
		return "SET-BAUDRATE: <invalid>"
	case SetDatasize:
		if len(c.Data) >= 1 {
			return fmt.Sprintf("SET-DATASIZE: %d bits", c.Data[0])
		}
		return "SET-DATASIZE: <invalid>"
	case SetParity:
		if len(c.Data) >= 1 {
			parity := []string{"NONE", "ODD", "EVEN", "MARK", "SPACE"}
			p := int(c.Data[0])
			if p < len(parity) {
				return fmt.Sprintf("SET-PARITY: %s", parity[p])
			}
			return fmt.Sprintf("SET-PARITY: %d", p)
		}
		return "SET-PARITY: <invalid>"
	case SetStopsize:
		if len(c.Data) >= 1 {
			stop := []string{"1", "2", "1.5"}
			s := int(c.Data[0])
			if s > 0 && s <= len(stop) {
				return fmt.Sprintf("SET-STOPSIZE: %s", stop[s-1])
			}
			return fmt.Sprintf("SET-STOPSIZE: %d", s)
		}
		return "SET-STOPSIZE: <invalid>"
	case SetControl:
		if len(c.Data) >= 1 {
			return fmt.Sprintf("SET-CONTROL: %d", c.Data[0])
		}
		return "SET-CONTROL: <invalid>"
	default:
		return fmt.Sprintf("UNKNOWN-%d: %s", c.Command, hex.EncodeToString(c.Data))
	}
}

// SendRFC2217Responses sends RFC2217 acknowledgments to client
func SendRFC2217Responses(conn net.Conn, buf *RFC2217Buffer) error {
	if buf == nil || len(buf.Commands) == 0 {
		return nil
	}

	for _, cmd := range buf.Commands {
		log.Printf("[rfc2217] client request: %s", cmd.String())
		resp := cmd.BuildResponse()
		if _, err := conn.Write(resp); err != nil {
			return fmt.Errorf("write RFC2217 response: %w", err)
		}
		log.Printf("[rfc2217] sent response: %s", hex.EncodeToString(resp))
	}
	return nil
}

// ForwardRFC2217ToDevice sends buffered RFC2217 data to device
func ForwardRFC2217ToDevice(deviceConn net.Conn, buf *RFC2217Buffer) error {
	if buf == nil || len(buf.RawData) == 0 {
		return nil
	}

	log.Printf("[rfc2217] forwarding %d bytes to device: %s",
		len(buf.RawData), hex.EncodeToString(buf.RawData))

	_, err := deviceConn.Write(buf.RawData)
	return err
}
