package connection

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
)

// USR-VCOM Baud Rate Synchronization Protocol
// Packet format (8 bytes): 55 AA 55 [baud_hi] [baud_mid] [baud_lo] [param] [checksum]
// This is a fire-and-forget protocol - no response expected

const (
	USRVCOMHeaderLen = 3
	USRVCOMPacketLen = 8
)

var USRVCOMHeader = []byte{0x55, 0xAA, 0x55}

// USRVCOMConfig represents parsed serial port configuration from USR-VCOM packet
type USRVCOMConfig struct {
	BaudRate uint32
	DataBits uint8
	Parity   uint8  // 0=None, 1=Odd, 2=Even, 3=Mark, 4=Space
	StopBits uint8  // 1 or 2
	Valid    bool   // true if packet was valid
	RawData  []byte // original packet data
}

// ParityString returns human-readable parity name
func (c *USRVCOMConfig) ParityString() string {
	switch c.Parity {
	case 0:
		return "None"
	case 1:
		return "Odd"
	case 2:
		return "Even"
	case 3:
		return "Mark"
	case 4:
		return "Space"
	default:
		return fmt.Sprintf("Unknown(%d)", c.Parity)
	}
}

// ModeString returns mode string like "8N1", "8E1", etc.
func (c *USRVCOMConfig) ModeString() string {
	parityChar := []byte{'N', 'O', 'E', 'M', 'S'}
	p := byte('?')
	if int(c.Parity) < len(parityChar) {
		p = parityChar[c.Parity]
	}
	return fmt.Sprintf("%d%c%d", c.DataBits, p, c.StopBits)
}

// String returns human-readable description
func (c *USRVCOMConfig) String() string {
	if !c.Valid {
		return "USR-VCOM: <invalid>"
	}
	return fmt.Sprintf("USR-VCOM: %d baud, %s", c.BaudRate, c.ModeString())
}

// ParseUSRVCOM parses USR-VCOM Baud Rate Sync packet from data
// Returns nil if data doesn't contain valid USR-VCOM packet
func ParseUSRVCOM(data []byte) *USRVCOMConfig {
	// Check minimum length
	if len(data) < USRVCOMPacketLen {
		return nil
	}

	// Find header 55 AA 55
	idx := -1
	for i := 0; i <= len(data)-USRVCOMPacketLen; i++ {
		if data[i] == 0x55 && data[i+1] == 0xAA && data[i+2] == 0x55 {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}

	packet := data[idx : idx+USRVCOMPacketLen]

	// Parse baud rate (24-bit big-endian)
	baudRate := uint32(packet[3])<<16 | uint32(packet[4])<<8 | uint32(packet[5])

	// Parse parameter byte
	param := packet[6]
	// Bit 1-0: Data bits (00=5, 01=6, 10=7, 11=8)
	// Bit 2:   Stop bits (0=1 bit, 1=2 bits)
	// Bit 3:   Parity enable (0=disabled, 1=enabled)
	// Bit 5-4: Parity type (00=ODD, 01=EVEN, 10=Mark, 11=Space)
	dataBits := uint8(5 + (param & 0x03))
	stopBits := uint8(1)
	if param&0x04 != 0 {
		stopBits = 2
	}
	parity := uint8(0) // None
	if param&0x08 != 0 {
		// Parity enabled
		parityType := (param >> 4) & 0x03
		switch parityType {
		case 0:
			parity = 1 // Odd
		case 1:
			parity = 2 // Even
		case 2:
			parity = 3 // Mark
		case 3:
			parity = 4 // Space
		}
	}

	// Verify checksum
	checksum := packet[7]
	calculated := (packet[3] + packet[4] + packet[5] + packet[6]) & 0xFF
	if checksum != calculated {
		log.Printf("[usrvcom] checksum mismatch: got %02X, expected %02X", checksum, calculated)
		// Still return config but mark raw data
	}

	return &USRVCOMConfig{
		BaudRate: baudRate,
		DataBits: dataBits,
		Parity:   parity,
		StopBits: stopBits,
		Valid:    true,
		RawData:  packet,
	}
}

// IsUSRVCOM checks if data starts with USR-VCOM header
func IsUSRVCOM(data []byte) bool {
	if len(data) < USRVCOMHeaderLen {
		return false
	}
	return data[0] == 0x55 && data[1] == 0xAA && data[2] == 0x55
}

// ToRFC2217Commands converts USR-VCOM config to RFC2217 commands
func (c *USRVCOMConfig) ToRFC2217Commands() []RFC2217Command {
	if !c.Valid {
		return nil
	}

	var commands []RFC2217Command

	// SET-BAUDRATE (command 1)
	baudData := make([]byte, 4)
	binary.BigEndian.PutUint32(baudData, c.BaudRate)
	commands = append(commands, RFC2217Command{
		Command: SetBaudrate,
		Data:    baudData,
	})

	// SET-DATASIZE (command 2)
	commands = append(commands, RFC2217Command{
		Command: SetDatasize,
		Data:    []byte{c.DataBits},
	})

	// SET-PARITY (command 3)
	commands = append(commands, RFC2217Command{
		Command: SetParity,
		Data:    []byte{c.Parity},
	})

	// SET-STOPSIZE (command 4)
	// RFC2217: 1=1 stop bit, 2=2 stop bits, 3=1.5 stop bits
	commands = append(commands, RFC2217Command{
		Command: SetStopsize,
		Data:    []byte{c.StopBits},
	})

	return commands
}

// BuildRFC2217Packet builds RFC2217 packet for sending to device
func (c *USRVCOMConfig) BuildRFC2217Packet() []byte {
	commands := c.ToRFC2217Commands()
	if len(commands) == 0 {
		return nil
	}

	var packet []byte
	for _, cmd := range commands {
		// IAC SB COM-PORT-OPTION <cmd> <data...> IAC SE
		packet = append(packet, IAC, SB, ComPortOption, cmd.Command)
		packet = append(packet, cmd.Data...)
		packet = append(packet, IAC, SE)
	}
	return packet
}

// SendToDevice sends RFC2217 commands to device based on USR-VCOM config
func (c *USRVCOMConfig) SendToDevice(deviceConn net.Conn) error {
	if !c.Valid || deviceConn == nil {
		return nil
	}

	packet := c.BuildRFC2217Packet()
	if len(packet) == 0 {
		return nil
	}

	log.Printf("[usrvcom] sending RFC2217 config to device: %s", hex.EncodeToString(packet))
	_, err := deviceConn.Write(packet)
	return err
}

// LogConfig logs the parsed configuration
func (c *USRVCOMConfig) LogConfig(prefix string) {
	if !c.Valid {
		return
	}
	log.Printf("[usrvcom] %s: %d baud, %d data bits, %s parity, %d stop bits (%s)",
		prefix, c.BaudRate, c.DataBits, c.ParityString(), c.StopBits, c.ModeString())
	log.Printf("[usrvcom] %s: raw packet: %s", prefix, hex.EncodeToString(c.RawData))
}
