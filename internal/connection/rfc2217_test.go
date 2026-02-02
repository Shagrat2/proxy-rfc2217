package connection

import (
	"bytes"
	"testing"
)

func TestParseRFC2217Commands(t *testing.T) {
	// Test data from the log:
	// fffa2c0100000960fff0fffa2c0208fff0fffa2c0301fff0fffa2c0401fff0fffa2c0501fff0
	data, _ := hexDecode("fffa2c0100000960fff0fffa2c0208fff0fffa2c0301fff0fffa2c0401fff0fffa2c0501fff0")

	buf := ParseRFC2217Commands(data)

	if len(buf.Commands) != 5 {
		t.Fatalf("expected 5 commands, got %d", len(buf.Commands))
	}

	// Check SET-BAUDRATE: 2400 (0x960)
	if buf.Commands[0].Command != SetBaudrate {
		t.Errorf("cmd[0]: expected SetBaudrate, got %d", buf.Commands[0].Command)
	}
	if !bytes.Equal(buf.Commands[0].Data, []byte{0x00, 0x00, 0x09, 0x60}) {
		t.Errorf("cmd[0]: unexpected data: %x", buf.Commands[0].Data)
	}

	// Check SET-DATASIZE: 8
	if buf.Commands[1].Command != SetDatasize {
		t.Errorf("cmd[1]: expected SetDatasize, got %d", buf.Commands[1].Command)
	}
	if !bytes.Equal(buf.Commands[1].Data, []byte{0x08}) {
		t.Errorf("cmd[1]: unexpected data: %x", buf.Commands[1].Data)
	}

	// Check SET-PARITY: 1 (NONE)
	if buf.Commands[2].Command != SetParity {
		t.Errorf("cmd[2]: expected SetParity, got %d", buf.Commands[2].Command)
	}
	if !bytes.Equal(buf.Commands[2].Data, []byte{0x01}) {
		t.Errorf("cmd[2]: unexpected data: %x", buf.Commands[2].Data)
	}

	// Check SET-STOPSIZE: 1
	if buf.Commands[3].Command != SetStopsize {
		t.Errorf("cmd[3]: expected SetStopsize, got %d", buf.Commands[3].Command)
	}
	if !bytes.Equal(buf.Commands[3].Data, []byte{0x01}) {
		t.Errorf("cmd[3]: unexpected data: %x", buf.Commands[3].Data)
	}

	// Check SET-CONTROL: 1
	if buf.Commands[4].Command != SetControl {
		t.Errorf("cmd[4]: expected SetControl, got %d", buf.Commands[4].Command)
	}
	if !bytes.Equal(buf.Commands[4].Data, []byte{0x01}) {
		t.Errorf("cmd[4]: unexpected data: %x", buf.Commands[4].Data)
	}

	// Verify RawData is preserved
	if !bytes.Equal(buf.RawData, data) {
		t.Errorf("RawData mismatch")
	}
}

func TestBuildResponse(t *testing.T) {
	cmd := RFC2217Command{
		Command: SetBaudrate,
		Data:    []byte{0x00, 0x00, 0x09, 0x60},
	}

	resp := cmd.BuildResponse()
	// Expected: IAC SB COM-PORT-OPTION (SetBaudrate+100) data IAC SE
	// ff fa 2c 65 00 00 09 60 ff f0
	expected, _ := hexDecode("fffa2c6500000960fff0")

	if !bytes.Equal(resp, expected) {
		t.Errorf("expected %x, got %x", expected, resp)
	}
}

func TestCommandString(t *testing.T) {
	tests := []struct {
		cmd      RFC2217Command
		expected string
	}{
		{RFC2217Command{Command: SetBaudrate, Data: []byte{0x00, 0x00, 0x09, 0x60}}, "SET-BAUDRATE: 2400"},
		{RFC2217Command{Command: SetDatasize, Data: []byte{0x08}}, "SET-DATASIZE: 8 bits"},
		{RFC2217Command{Command: SetParity, Data: []byte{0x00}}, "SET-PARITY: NONE"},
		{RFC2217Command{Command: SetParity, Data: []byte{0x01}}, "SET-PARITY: ODD"},
		{RFC2217Command{Command: SetParity, Data: []byte{0x02}}, "SET-PARITY: EVEN"},
		{RFC2217Command{Command: SetStopsize, Data: []byte{0x01}}, "SET-STOPSIZE: 1"},
		{RFC2217Command{Command: SetStopsize, Data: []byte{0x02}}, "SET-STOPSIZE: 2"},
		{RFC2217Command{Command: SetControl, Data: []byte{0x01}}, "SET-CONTROL: 1"},
	}

	for _, tt := range tests {
		got := tt.cmd.String()
		if got != tt.expected {
			t.Errorf("String() = %q, want %q", got, tt.expected)
		}
	}
}

func hexDecode(s string) ([]byte, error) {
	result := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		var b byte
		for j := 0; j < 2; j++ {
			c := s[i+j]
			var v byte
			switch {
			case c >= '0' && c <= '9':
				v = c - '0'
			case c >= 'a' && c <= 'f':
				v = c - 'a' + 10
			case c >= 'A' && c <= 'F':
				v = c - 'A' + 10
			}
			b = b<<4 | v
		}
		result[i/2] = b
	}
	return result, nil
}

func TestIsQuery(t *testing.T) {
	tests := []struct {
		cmd      RFC2217Command
		expected bool
		name     string
	}{
		// Query commands (value=0)
		{RFC2217Command{Command: SetBaudrate, Data: []byte{0x00, 0x00, 0x00, 0x00}}, true, "baudrate query"},
		{RFC2217Command{Command: SetDatasize, Data: []byte{0x00}}, true, "datasize query"},
		{RFC2217Command{Command: SetParity, Data: []byte{0x00}}, true, "parity query"},
		{RFC2217Command{Command: SetStopsize, Data: []byte{0x00}}, true, "stopsize query"},
		{RFC2217Command{Command: SetControl, Data: []byte{0x00}}, true, "control query"},

		// Setting commands (value>0)
		{RFC2217Command{Command: SetBaudrate, Data: []byte{0x00, 0x00, 0x09, 0x60}}, false, "baudrate=2400"},
		{RFC2217Command{Command: SetDatasize, Data: []byte{0x08}}, false, "datasize=8"},
		{RFC2217Command{Command: SetParity, Data: []byte{0x02}}, false, "parity=EVEN"},
		{RFC2217Command{Command: SetStopsize, Data: []byte{0x01}}, false, "stopsize=1"},
		{RFC2217Command{Command: SetControl, Data: []byte{0x01}}, false, "control=1"},
	}

	for _, tt := range tests {
		got := tt.cmd.IsQuery()
		if got != tt.expected {
			t.Errorf("%s: IsQuery() = %v, want %v", tt.name, got, tt.expected)
		}
	}
}

func TestParseRFC2217Commands_RealData(t *testing.T) {
	// Real data from log: fffa2c0100000960fff0fffa2c0208fff0fffa2c0303fff0fffa2c0401fff0fffa2c0501fff0
	// SET-BAUDRATE: 2400, SET-DATASIZE: 8, SET-PARITY: 3 (EVEN), SET-STOPSIZE: 1, SET-CONTROL: 1
	data, _ := hexDecode("fffa2c0100000960fff0fffa2c0208fff0fffa2c0303fff0fffa2c0401fff0fffa2c0501fff0")

	t.Logf("Input data (%d bytes)", len(data))

	buf := ParseRFC2217Commands(data)

	t.Logf("Parsed %d commands", len(buf.Commands))
	for i, cmd := range buf.Commands {
		t.Logf("  Command %d: cmd=%d data=%x -> %s", i, cmd.Command, cmd.Data, cmd.String())
	}

	if len(buf.Commands) != 5 {
		t.Errorf("expected 5 commands, got %d", len(buf.Commands))
	}

	// Check first command: SET-BAUDRATE 2400
	if len(buf.Commands) > 0 {
		if buf.Commands[0].Command != SetBaudrate {
			t.Errorf("command[0] = %d, want SetBaudrate", buf.Commands[0].Command)
		}
	}
}
