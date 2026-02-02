package connection

import (
	"testing"
)

func TestParseUSRVCOM(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantBaud uint32
		wantData uint8
		wantPar  uint8
		wantStop uint8
		wantMode string
		wantNil  bool
	}{
		{
			name:     "2400 8N1",
			data:     []byte{0x55, 0xAA, 0x55, 0x00, 0x09, 0x60, 0x03, 0x6C},
			wantBaud: 2400,
			wantData: 8,
			wantPar:  0, // None
			wantStop: 1,
			wantMode: "8N1",
		},
		{
			name:     "9600 8N1",
			data:     []byte{0x55, 0xAA, 0x55, 0x00, 0x25, 0x80, 0x03, 0xA8},
			wantBaud: 9600,
			wantData: 8,
			wantPar:  0, // None
			wantStop: 1,
			wantMode: "8N1",
		},
		{
			name:     "9600 8E1",
			data:     []byte{0x55, 0xAA, 0x55, 0x00, 0x25, 0x80, 0x1B, 0xC0},
			wantBaud: 9600,
			wantData: 8,
			wantPar:  2, // Even
			wantStop: 1,
			wantMode: "8E1",
		},
		{
			name:     "300 8E1",
			data:     []byte{0x55, 0xAA, 0x55, 0x00, 0x01, 0x2C, 0x1B, 0x48},
			wantBaud: 300,
			wantData: 8,
			wantPar:  2, // Even
			wantStop: 1,
			wantMode: "8E1",
		},
		{
			name:     "300 8N1",
			data:     []byte{0x55, 0xAA, 0x55, 0x00, 0x01, 0x2C, 0x03, 0x30},
			wantBaud: 300,
			wantData: 8,
			wantPar:  0, // None
			wantStop: 1,
			wantMode: "8N1",
		},
		{
			name:     "115200 8N1",
			data:     []byte{0x55, 0xAA, 0x55, 0x01, 0xC2, 0x00, 0x03, 0xC6},
			wantBaud: 115200,
			wantData: 8,
			wantPar:  0, // None
			wantStop: 1,
			wantMode: "8N1",
		},
		{
			name:    "too short",
			data:    []byte{0x55, 0xAA, 0x55, 0x00},
			wantNil: true,
		},
		{
			name:    "wrong header",
			data:    []byte{0x55, 0xAA, 0x00, 0x00, 0x25, 0x80, 0x03, 0xA8},
			wantNil: true,
		},
		{
			name:     "with prefix garbage",
			data:     []byte{0x00, 0x00, 0x55, 0xAA, 0x55, 0x00, 0x25, 0x80, 0x03, 0xA8},
			wantBaud: 9600,
			wantData: 8,
			wantPar:  0,
			wantStop: 1,
			wantMode: "8N1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ParseUSRVCOM(tt.data)

			if tt.wantNil {
				if cfg != nil {
					t.Errorf("expected nil, got %+v", cfg)
				}
				return
			}

			if cfg == nil {
				t.Fatal("expected config, got nil")
			}

			if !cfg.Valid {
				t.Error("expected Valid=true")
			}

			if cfg.BaudRate != tt.wantBaud {
				t.Errorf("BaudRate = %d, want %d", cfg.BaudRate, tt.wantBaud)
			}

			if cfg.DataBits != tt.wantData {
				t.Errorf("DataBits = %d, want %d", cfg.DataBits, tt.wantData)
			}

			if cfg.Parity != tt.wantPar {
				t.Errorf("Parity = %d, want %d", cfg.Parity, tt.wantPar)
			}

			if cfg.StopBits != tt.wantStop {
				t.Errorf("StopBits = %d, want %d", cfg.StopBits, tt.wantStop)
			}

			if cfg.ModeString() != tt.wantMode {
				t.Errorf("ModeString() = %q, want %q", cfg.ModeString(), tt.wantMode)
			}
		})
	}
}

func TestIsUSRVCOM(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"valid header", []byte{0x55, 0xAA, 0x55, 0x00, 0x00, 0x00, 0x00, 0x00}, true},
		{"old header 55 AA", []byte{0x55, 0xAA, 0x00}, false},
		{"too short", []byte{0x55, 0xAA}, false},
		{"wrong", []byte{0xFF, 0xFA, 0x2C}, false},
		{"empty", []byte{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUSRVCOM(tt.data); got != tt.want {
				t.Errorf("IsUSRVCOM() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUSRVCOMConfig_ToRFC2217Commands(t *testing.T) {
	cfg := &USRVCOMConfig{
		BaudRate: 9600,
		DataBits: 8,
		Parity:   0, // None
		StopBits: 1,
		Valid:    true,
	}

	commands := cfg.ToRFC2217Commands()

	if len(commands) != 4 {
		t.Fatalf("expected 4 commands, got %d", len(commands))
	}

	// Check baud rate command
	if commands[0].Command != SetBaudrate {
		t.Errorf("command[0] = %d, want SetBaudrate(%d)", commands[0].Command, SetBaudrate)
	}

	// Check data size command
	if commands[1].Command != SetDatasize || commands[1].Data[0] != 8 {
		t.Errorf("command[1] = %d/%d, want SetDatasize/8", commands[1].Command, commands[1].Data[0])
	}

	// Check parity command
	if commands[2].Command != SetParity || commands[2].Data[0] != 0 {
		t.Errorf("command[2] = %d/%d, want SetParity/0", commands[2].Command, commands[2].Data[0])
	}

	// Check stop size command
	if commands[3].Command != SetStopsize || commands[3].Data[0] != 1 {
		t.Errorf("command[3] = %d/%d, want SetStopsize/1", commands[3].Command, commands[3].Data[0])
	}
}

func TestUSRVCOMConfig_BuildRFC2217Packet(t *testing.T) {
	cfg := &USRVCOMConfig{
		BaudRate: 9600,
		DataBits: 8,
		Parity:   0,
		StopBits: 1,
		Valid:    true,
	}

	packet := cfg.BuildRFC2217Packet()

	if len(packet) == 0 {
		t.Fatal("expected non-empty packet")
	}

	// Should start with IAC SB COM-PORT-OPTION
	if packet[0] != IAC || packet[1] != SB || packet[2] != ComPortOption {
		t.Errorf("packet doesn't start with IAC SB COM-PORT-OPTION: %x", packet[:3])
	}
}
