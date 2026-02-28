package connection

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/config"
	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/device"
	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/session"
)

// --- Test helpers ---

type testEnv struct {
	cfg      *config.Config
	registry *device.Registry
	sessions *session.Manager
	handler  *Handler
}

func newTestEnv() *testEnv {
	return newTestEnvWithAuth("")
}

func newTestEnvWithAuth(authToken string) *testEnv {
	cfg := &config.Config{
		AuthToken:          authToken,
		InitTimeout:        5 * time.Second,
		PostConnectTimeout: 3 * time.Second,
		IdleTimeout:        1 * time.Second, // short for tests
	}
	reg := device.NewRegistry()
	sess := session.NewManager(false, 1*time.Second)
	return &testEnv{
		cfg:      cfg,
		registry: reg,
		sessions: sess,
		handler:  NewHandler(cfg, reg, sess),
	}
}

// registerDevice adds a device directly to registry.
// Returns the "device test side" — the end the test reads/writes.
// Both sides auto-closed on test cleanup.
func (e *testEnv) registerDevice(t *testing.T, deviceID string) net.Conn {
	t.Helper()
	devClient, devServer := createTCPPair(t)
	dev := &device.Device{
		ID:            deviceID,
		Conn:          devServer,
		RegisteredAt:  time.Now(),
		StopKeepalive: make(chan struct{}),
	}
	e.registry.Register(dev)
	t.Cleanup(func() {
		devClient.Close()
		devServer.Close()
	})
	return devClient
}

// createTCPPair creates a pair of connected TCP connections via localhost.
func createTCPPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var serverConn net.Conn
	var acceptErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverConn, acceptErr = ln.Accept()
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		ln.Close()
		t.Fatalf("dial: %v", err)
	}

	wg.Wait()
	ln.Close()
	if acceptErr != nil {
		clientConn.Close()
		t.Fatalf("accept: %v", acceptErr)
	}
	return clientConn, serverConn
}

// runHandler starts Handle() in a goroutine, returns done channel.
func runHandler(ctx context.Context, h *Handler, conn net.Conn) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.Handle(ctx, conn)
	}()
	return done
}

// waitDone waits for handler to finish with timeout to prevent hangs.
func waitDone(t *testing.T, done <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("handler did not return in time")
	}
}

// readResponse reads data from conn with timeout.
func readResponse(t *testing.T, conn net.Conn, timeout time.Duration) string {
	t.Helper()
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(timeout))
	n, err := conn.Read(buf)
	conn.SetReadDeadline(time.Time{})
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return ""
		}
		t.Fatalf("read response: %v", err)
	}
	return string(buf[:n])
}

// readUntilContains reads from conn until response contains expected string or timeout.
// Skips over intermediate data (e.g. Telnet NOP keepalives).
func readUntilContains(t *testing.T, conn net.Conn, expected string, timeout time.Duration) string {
	t.Helper()
	var all []byte
	buf := make([]byte, 4096)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(deadline)
		n, err := conn.Read(buf)
		if n > 0 {
			all = append(all, buf[:n]...)
			if strings.Contains(string(all), expected) {
				conn.SetReadDeadline(time.Time{})
				return string(all)
			}
		}
		if err != nil {
			break
		}
	}
	conn.SetReadDeadline(time.Time{})
	return string(all)
}

// sendCmd sends an AT command line (appends \r\n).
func sendCmd(t *testing.T, conn net.Conn, cmd string) {
	t.Helper()
	_, err := conn.Write([]byte(cmd + "\r\n"))
	if err != nil {
		t.Fatalf("send %q: %v", cmd, err)
	}
}

// expectExact sends a command and checks the exact response.
func expectExact(t *testing.T, conn net.Conn, cmd, expected string) {
	t.Helper()
	sendCmd(t, conn, cmd)
	resp := readResponse(t, conn, 2*time.Second)
	if resp != expected {
		t.Errorf("cmd %q: expected %q, got %q", cmd, expected, resp)
	}
}

// === Device ESP tests ===

func TestDeviceRegistration(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runHandler(ctx, env.handler, server)

	sendCmd(t, client, "AT+REG=device123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "OK\r\n" {
		t.Fatalf("expected OK, got %q", resp)
	}

	if _, ok := env.registry.Get("device123"); !ok {
		t.Fatal("device123 not registered")
	}

	cancel()
	waitDone(t, done, 5*time.Second)
}

func TestDeviceRegistrationWithAuth(t *testing.T) {
	env := newTestEnvWithAuth("secret")
	client, server := createTCPPair(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runHandler(ctx, env.handler, server)

	sendCmd(t, client, "AT+REG=secret+device123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "OK\r\n" {
		t.Fatalf("expected OK, got %q", resp)
	}

	if _, ok := env.registry.Get("device123"); !ok {
		t.Fatal("device123 not registered")
	}

	cancel()
	waitDone(t, done, 5*time.Second)
}

func TestDeviceRegistrationInvalidAuth(t *testing.T) {
	env := newTestEnvWithAuth("secret")
	client, server := createTCPPair(t)
	defer client.Close()

	done := runHandler(context.Background(), env.handler, server)

	sendCmd(t, client, "AT+REG=wrong+device123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "ERROR\r\n" {
		t.Fatalf("expected ERROR, got %q", resp)
	}

	if _, ok := env.registry.Get("device123"); ok {
		t.Fatal("device should NOT be registered")
	}

	waitDone(t, done, 5*time.Second)
}

func TestDeviceRegistrationNoAuthFormat(t *testing.T) {
	env := newTestEnvWithAuth("secret")
	client, server := createTCPPair(t)
	defer client.Close()

	done := runHandler(context.Background(), env.handler, server)

	sendCmd(t, client, "AT+REG=device123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "ERROR\r\n" {
		t.Fatalf("expected ERROR, got %q", resp)
	}

	waitDone(t, done, 5*time.Second)
}

func TestDeviceRegistrationEmptyToken(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	done := runHandler(context.Background(), env.handler, server)

	sendCmd(t, client, "AT+REG=")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "ERROR\r\n" {
		t.Fatalf("expected ERROR, got %q", resp)
	}

	waitDone(t, done, 5*time.Second)
}

func TestDeviceWithATDT(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runHandler(ctx, env.handler, server)

	sendCmd(t, client, "AT+REG=device123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "OK\r\n" {
		t.Fatalf("expected OK for REG, got %q", resp)
	}

	sendCmd(t, client, "ATDT")
	resp = readResponse(t, client, 2*time.Second)
	if resp != "OK\r\n" {
		t.Fatalf("expected OK for ATDT, got %q", resp)
	}

	if _, ok := env.registry.Get("device123"); !ok {
		t.Fatal("device123 should still be registered")
	}

	cancel()
	waitDone(t, done, 5*time.Second)
}

func TestDeviceWithATDTParam(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runHandler(ctx, env.handler, server)

	sendCmd(t, client, "AT+REG=device123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "OK\r\n" {
		t.Fatalf("expected OK for REG, got %q", resp)
	}

	sendCmd(t, client, "ATDT12345")
	resp = readResponse(t, client, 2*time.Second)
	if resp != "OK\r\n" {
		t.Fatalf("expected OK for ATDT12345, got %q", resp)
	}

	if env.sessions.Count() != 0 {
		t.Fatal("no session should be created for device ATDT")
	}

	cancel()
	waitDone(t, done, 5*time.Second)
}

// === Client RFC-2217 tests ===

func TestClientConnect(t *testing.T) {
	env := newTestEnv()
	devConn := env.registerDevice(t, "device123")

	client, server := createTCPPair(t)

	done := runHandler(context.Background(), env.handler, server)

	sendCmd(t, client, "AT+CONNECT=device123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "OK\r\n" {
		t.Fatalf("expected OK, got %q", resp)
	}

	dev, ok := env.registry.Get("device123")
	if !ok {
		t.Fatal("device123 not found")
	}
	if !dev.IsInSession() {
		t.Fatal("device should be in session")
	}

	// Close both sides to end bridge cleanly
	devConn.Close()
	client.Close()
	waitDone(t, done, 5*time.Second)
}

func TestClientConnectDeviceNotFound(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	done := runHandler(context.Background(), env.handler, server)

	sendCmd(t, client, "AT+CONNECT=unknown")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "ERROR\r\n" {
		t.Fatalf("expected ERROR, got %q", resp)
	}

	waitDone(t, done, 5*time.Second)
}

func TestClientConnectDeviceBusy(t *testing.T) {
	env := newTestEnv()
	env.registerDevice(t, "device123")
	dev, _ := env.registry.Get("device123")
	dev.SetSession("fake-session")

	client, server := createTCPPair(t)
	defer client.Close()

	done := runHandler(context.Background(), env.handler, server)

	sendCmd(t, client, "AT+CONNECT=device123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "ERROR\r\n" {
		t.Fatalf("expected ERROR for busy device, got %q", resp)
	}

	waitDone(t, done, 5*time.Second)
}

func TestClientConnectWithAuth(t *testing.T) {
	env := newTestEnvWithAuth("secret")
	devConn := env.registerDevice(t, "device123")

	client, server := createTCPPair(t)

	done := runHandler(context.Background(), env.handler, server)

	sendCmd(t, client, "AT+CONNECT=secret+device123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "OK\r\n" {
		t.Fatalf("expected OK, got %q", resp)
	}

	devConn.Close()
	client.Close()
	waitDone(t, done, 5*time.Second)
}

func TestClientConnectInvalidAuth(t *testing.T) {
	env := newTestEnvWithAuth("secret")
	env.registerDevice(t, "device123")

	client, server := createTCPPair(t)
	defer client.Close()

	done := runHandler(context.Background(), env.handler, server)

	sendCmd(t, client, "AT+CONNECT=wrong+device123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "ERROR\r\n" {
		t.Fatalf("expected ERROR for invalid auth, got %q", resp)
	}

	waitDone(t, done, 5*time.Second)
}

func TestClientConnectEmptyToken(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	done := runHandler(context.Background(), env.handler, server)

	sendCmd(t, client, "AT+CONNECT=")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "ERROR\r\n" {
		t.Fatalf("expected ERROR for empty token, got %q", resp)
	}

	waitDone(t, done, 5*time.Second)
}

// === GSM-CSD Modem tests ===

func TestModemActivation(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runHandler(ctx, env.handler, server)

	sendCmd(t, client, "ATZ")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "\r\nOK\r\n" {
		t.Fatalf("expected modem OK, got %q", resp)
	}

	cancel()
	waitDone(t, done, 5*time.Second)
}

func TestModemCommands(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runHandler(ctx, env.handler, server)

	for _, cmd := range []string{"ATZ", "ATE0", "ATE1", "ATH", "AT"} {
		sendCmd(t, client, cmd)
		resp := readResponse(t, client, 2*time.Second)
		if resp != "\r\nOK\r\n" {
			t.Errorf("cmd %q: expected \\r\\nOK\\r\\n, got %q", cmd, resp)
		}
	}

	cancel()
	waitDone(t, done, 5*time.Second)
}

func TestModemInfoCommands(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runHandler(ctx, env.handler, server)

	tests := []struct {
		cmd      string
		contains string
	}{
		{"ATI", "RFC2217-PROXY"},
		{"AT+CGMI", "RFC2217-PROXY"},
		{"AT+CPIN?", "+CPIN: READY"},
		{"AT+CSQ", "+CSQ: 31,0"},
	}

	for _, tt := range tests {
		sendCmd(t, client, tt.cmd)
		resp := readResponse(t, client, 2*time.Second)
		if !strings.Contains(resp, tt.contains) {
			t.Errorf("cmd %q: expected response containing %q, got %q", tt.cmd, tt.contains, resp)
		}
		if !strings.Contains(resp, "OK") {
			t.Errorf("cmd %q: response should contain OK, got %q", tt.cmd, resp)
		}
	}

	cancel()
	waitDone(t, done, 5*time.Second)
}

func TestModemATV0SwitchesToNumeric(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runHandler(ctx, env.handler, server)

	// ATV0 responds in current mode (verbose), then switches to numeric
	expectExact(t, client, "ATV0", "\r\nOK\r\n")

	// Next command should get numeric response
	expectExact(t, client, "AT", "0\r")

	cancel()
	waitDone(t, done, 5*time.Second)
}

func TestModemDialConnect(t *testing.T) {
	env := newTestEnv()
	devConn := env.registerDevice(t, "device123")

	client, server := createTCPPair(t)

	done := runHandler(context.Background(), env.handler, server)

	expectExact(t, client, "ATZ", "\r\nOK\r\n")

	sendCmd(t, client, "ATDTdevice123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "\r\nCONNECT 9600\r\n" {
		t.Fatalf("expected CONNECT 9600, got %q", resp)
	}

	if env.sessions.Count() != 1 {
		t.Fatalf("expected 1 session, got %d", env.sessions.Count())
	}

	devConn.Close()
	client.Close()
	waitDone(t, done, 5*time.Second)
}

func TestModemDialConnectNumeric(t *testing.T) {
	env := newTestEnv()
	devConn := env.registerDevice(t, "device123")

	client, server := createTCPPair(t)

	done := runHandler(context.Background(), env.handler, server)

	// Switch to numeric mode (ATV0 responds verbose, then switches)
	expectExact(t, client, "ATV0", "\r\nOK\r\n")

	sendCmd(t, client, "ATDTdevice123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "1\r" {
		t.Fatalf("expected numeric CONNECT (1\\r), got %q", resp)
	}

	devConn.Close()
	client.Close()
	waitDone(t, done, 5*time.Second)
}

func TestModemDialDeviceNotFound(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	done := runHandler(context.Background(), env.handler, server)

	expectExact(t, client, "ATZ", "\r\nOK\r\n")

	sendCmd(t, client, "ATDTunknown")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "\r\nNO CARRIER\r\n" {
		t.Fatalf("expected NO CARRIER, got %q", resp)
	}

	waitDone(t, done, 5*time.Second)
}

func TestModemDialDeviceBusy(t *testing.T) {
	env := newTestEnv()
	env.registerDevice(t, "device123")
	dev, _ := env.registry.Get("device123")
	dev.SetSession("fake-session")

	client, server := createTCPPair(t)
	defer client.Close()

	done := runHandler(context.Background(), env.handler, server)

	expectExact(t, client, "ATZ", "\r\nOK\r\n")

	sendCmd(t, client, "ATDTdevice123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "\r\nNO CARRIER\r\n" {
		t.Fatalf("expected NO CARRIER, got %q", resp)
	}

	waitDone(t, done, 5*time.Second)
}

func TestModemDialWithAuth(t *testing.T) {
	env := newTestEnvWithAuth("secret")
	devConn := env.registerDevice(t, "device123")

	client, server := createTCPPair(t)

	done := runHandler(context.Background(), env.handler, server)

	expectExact(t, client, "ATZ", "\r\nOK\r\n")

	sendCmd(t, client, "ATDTsecret+device123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "\r\nCONNECT 9600\r\n" {
		t.Fatalf("expected CONNECT 9600, got %q", resp)
	}

	devConn.Close()
	client.Close()
	waitDone(t, done, 5*time.Second)
}

func TestModemDialInvalidAuth(t *testing.T) {
	env := newTestEnvWithAuth("secret")
	env.registerDevice(t, "device123")

	client, server := createTCPPair(t)
	defer client.Close()

	done := runHandler(context.Background(), env.handler, server)

	expectExact(t, client, "ATZ", "\r\nOK\r\n")

	// Invalid auth — modem format: NO CARRIER, NOT ERROR
	sendCmd(t, client, "ATDTwrong+device123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "\r\nNO CARRIER\r\n" {
		t.Fatalf("expected NO CARRIER (modem error), got %q", resp)
	}

	waitDone(t, done, 5*time.Second)
}

func TestModemDialInvalidAuthNumeric(t *testing.T) {
	env := newTestEnvWithAuth("secret")
	env.registerDevice(t, "device123")

	client, server := createTCPPair(t)
	defer client.Close()

	done := runHandler(context.Background(), env.handler, server)

	expectExact(t, client, "ATV0", "\r\nOK\r\n")

	sendCmd(t, client, "ATDTwrong+device123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "3\r" {
		t.Fatalf("expected numeric NO CARRIER (3\\r), got %q", resp)
	}

	waitDone(t, done, 5*time.Second)
}

func TestModemFullSequence(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	done := runHandler(context.Background(), env.handler, server)

	// Typical GSM-CSD initialization
	expectExact(t, client, "ATZ", "\r\nOK\r\n")
	expectExact(t, client, "ATE0", "\r\nOK\r\n")
	expectExact(t, client, "ATV0", "\r\nOK\r\n") // responds verbose, then switches

	// Now in numeric mode — dial unknown device → numeric NO CARRIER
	sendCmd(t, client, "ATDT12345")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "3\r" {
		t.Fatalf("expected numeric NO CARRIER (3\\r), got %q", resp)
	}

	waitDone(t, done, 5*time.Second)
}

// === Backward compatibility: ATDT without modem mode ===

func TestATDTWithParamNoModem(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runHandler(ctx, env.handler, server)

	// ATDT with param but NO prior modem commands → plain OK, NOT dial
	sendCmd(t, client, "ATDT12345")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "OK\r\n" {
		t.Fatalf("expected plain OK (no modem), got %q", resp)
	}

	if env.sessions.Count() != 0 {
		t.Fatal("should NOT create session for ATDT without modem mode")
	}

	cancel()
	waitDone(t, done, 5*time.Second)
}

func TestATDTNoParamNoModem(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runHandler(ctx, env.handler, server)

	sendCmd(t, client, "ATDT")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "OK\r\n" {
		t.Fatalf("expected plain OK, got %q", resp)
	}

	cancel()
	waitDone(t, done, 5*time.Second)
}

func TestATDPWithParamNoModem(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runHandler(ctx, env.handler, server)

	sendCmd(t, client, "ATDP12345")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "OK\r\n" {
		t.Fatalf("expected plain OK (no modem), got %q", resp)
	}

	if env.sessions.Count() != 0 {
		t.Fatal("should NOT create session for ATDP without modem mode")
	}

	cancel()
	waitDone(t, done, 5*time.Second)
}

func TestATDTNoParamInModemMode(t *testing.T) {
	env := newTestEnv()
	client, server := createTCPPair(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runHandler(ctx, env.handler, server)

	expectExact(t, client, "ATZ", "\r\nOK\r\n")

	// ATDT without param in modem mode — modem OK, no dial
	sendCmd(t, client, "ATDT")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "\r\nOK\r\n" {
		t.Fatalf("expected modem OK, got %q", resp)
	}

	if env.sessions.Count() != 0 {
		t.Fatal("should NOT create session for ATDT without param")
	}

	cancel()
	waitDone(t, done, 5*time.Second)
}

// === ATD without T/P suffix ===

func TestATDDialInModemMode(t *testing.T) {
	env := newTestEnv()
	devConn := env.registerDevice(t, "device123")

	client, server := createTCPPair(t)

	done := runHandler(context.Background(), env.handler, server)

	expectExact(t, client, "ATZ", "\r\nOK\r\n")

	sendCmd(t, client, "ATDdevice123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "\r\nCONNECT 9600\r\n" {
		t.Fatalf("ATD should work as dial, expected CONNECT, got %q", resp)
	}

	devConn.Close()
	client.Close()
	waitDone(t, done, 5*time.Second)
}

// === USR-VCOM tests ===

func TestUSRVCOMBeforeConnect(t *testing.T) {
	env := newTestEnv()
	devConn := env.registerDevice(t, "device123")

	client, server := createTCPPair(t)

	done := runHandler(context.Background(), env.handler, server)

	// USR-VCOM 9600 8N1 + AT+CONNECT on same line
	usrvcom := []byte{0x55, 0xAA, 0x55, 0x00, 0x25, 0x80, 0x03, 0xA8}
	atCmd := []byte("AT+CONNECT=device123\r\n")
	client.Write(append(usrvcom, atCmd...))

	resp := readResponse(t, client, 2*time.Second)
	if resp != "OK\r\n" {
		t.Fatalf("expected OK, got %q", resp)
	}

	// Device should receive RFC2217 data (converted from USR-VCOM)
	devBuf := make([]byte, 4096)
	devConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := devConn.Read(devBuf)
	if err != nil {
		t.Fatalf("device read: %v", err)
	}
	if n == 0 {
		t.Fatal("device should receive RFC2217 data")
	}
	if devBuf[0] != 0xFF || devBuf[1] != 0xFA || devBuf[2] != 0x2C {
		t.Fatalf("expected RFC2217 data (FF FA 2C...), got %x", devBuf[:n])
	}

	devConn.Close()
	client.Close()
	waitDone(t, done, 5*time.Second)
}

// === RFC2217 before AT+CONNECT ===

func TestRFC2217BeforeConnect(t *testing.T) {
	env := newTestEnv()
	devConn := env.registerDevice(t, "device123")

	client, server := createTCPPair(t)

	done := runHandler(context.Background(), env.handler, server)

	// RFC2217 SET-BAUDRATE 9600 + AT+CONNECT
	rfc2217 := []byte{
		0xFF, 0xFA, 0x2C, 0x01, 0x00, 0x00, 0x25, 0x80, 0xFF, 0xF0,
	}
	atCmd := []byte("AT+CONNECT=device123\r\n")
	client.Write(append(rfc2217, atCmd...))

	resp := readResponse(t, client, 2*time.Second)
	if resp != "OK\r\n" {
		t.Fatalf("expected OK, got %q", resp)
	}

	// Device should receive the RFC2217 data
	devBuf := make([]byte, 4096)
	devConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := devConn.Read(devBuf)
	if err != nil {
		t.Fatalf("device read: %v", err)
	}
	if n == 0 {
		t.Fatal("device should receive RFC2217 data")
	}

	devConn.Close()
	client.Close()
	waitDone(t, done, 5*time.Second)
}

// === Data bridge tests ===

func TestDataBridge(t *testing.T) {
	env := newTestEnv()
	devConn := env.registerDevice(t, "device123")

	client, server := createTCPPair(t)

	done := runHandler(context.Background(), env.handler, server)

	sendCmd(t, client, "AT+CONNECT=device123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "OK\r\n" {
		t.Fatalf("expected OK, got %q", resp)
	}

	// Client → Device
	client.Write([]byte("hello"))
	devBuf := make([]byte, 100)
	devConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := devConn.Read(devBuf)
	if err != nil {
		t.Fatalf("device read: %v", err)
	}
	if string(devBuf[:n]) != "hello" {
		t.Fatalf("device expected 'hello', got %q", string(devBuf[:n]))
	}

	// Device → Client
	devConn.Write([]byte("world"))
	clientBuf := make([]byte, 100)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err = client.Read(clientBuf)
	client.SetReadDeadline(time.Time{})
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	if string(clientBuf[:n]) != "world" {
		t.Fatalf("client expected 'world', got %q", string(clientBuf[:n]))
	}

	devConn.Close()
	client.Close()
	waitDone(t, done, 5*time.Second)
}

func TestModemDataBridge(t *testing.T) {
	env := newTestEnv()
	devConn := env.registerDevice(t, "device123")

	client, server := createTCPPair(t)

	done := runHandler(context.Background(), env.handler, server)

	expectExact(t, client, "ATZ", "\r\nOK\r\n")

	sendCmd(t, client, "ATDTdevice123")
	resp := readResponse(t, client, 2*time.Second)
	if resp != "\r\nCONNECT 9600\r\n" {
		t.Fatalf("expected CONNECT, got %q", resp)
	}

	// Bidirectional data through modem bridge
	client.Write([]byte("data1"))
	devBuf := make([]byte, 100)
	devConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ := devConn.Read(devBuf)
	if string(devBuf[:n]) != "data1" {
		t.Fatalf("device expected 'data1', got %q", string(devBuf[:n]))
	}

	devConn.Write([]byte("data2"))
	clientBuf := make([]byte, 100)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ = client.Read(clientBuf)
	client.SetReadDeadline(time.Time{})
	if string(clientBuf[:n]) != "data2" {
		t.Fatalf("client expected 'data2', got %q", string(clientBuf[:n]))
	}

	// Close both sides to end bridge
	devConn.Close()
	client.Close()
	waitDone(t, done, 5*time.Second)
}

// === Unknown command ===

func TestUnknownCommandNoModem(t *testing.T) {
	// Non-AT data goes to skipped bytes, handler waits for AT command until InitTimeout
	env := newTestEnv()
	env.cfg.InitTimeout = 2 * time.Second // short timeout for test
	client, server := createTCPPair(t)
	defer client.Close()

	done := runHandler(context.Background(), env.handler, server)

	sendCmd(t, client, "GARBAGE")
	// Handler will timeout after InitTimeout (2s) and send ERROR
	resp := readResponse(t, client, 4*time.Second)
	if resp != "ERROR\r\n" {
		t.Fatalf("expected ERROR after timeout, got %q", resp)
	}

	waitDone(t, done, 5*time.Second)
}
