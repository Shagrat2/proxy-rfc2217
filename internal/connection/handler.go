package connection

import (
	"bufio"
	"context"
	"log"
	"net"
	"strings"
	"time"

	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/config"
	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/device"
	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/session"
)

// Handler handles all connections (devices and clients)
type Handler struct {
	cfg      *config.Config
	registry *device.Registry
	sessions *session.Manager
}

// NewHandler creates a new connection handler
func NewHandler(cfg *config.Config, registry *device.Registry, sessions *session.Manager) *Handler {
	return &Handler{
		cfg:      cfg,
		registry: registry,
		sessions: sessions,
	}
}

// Handle processes an incoming connection
// Determines if it's a device or client based on AT command
// Supports USR-VCOM and RFC2217 data before AT command
func (h *Handler) Handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	log.Printf("[conn] new connection from %s", remoteAddr)

	reader := bufio.NewReader(conn)

	// Track USR-VCOM config across command loop
	var usrvcomCfg *USRVCOMConfig
	timeout := h.cfg.InitTimeout // Start with init timeout

	for {
		// Read AT command with support for USR-VCOM/RFC2217 presets
		// USR-VCOM packets are accepted silently (no ERROR response)
		cmd, err := ReadATCommandWithPresets(reader, conn, timeout)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Try to see what was received before timeout
				if reader.Buffered() > 0 {
					peek, _ := reader.Peek(reader.Buffered())
					log.Printf("[conn] %s: init timeout, partial data: %x", remoteAddr, peek)
				} else {
					log.Printf("[conn] %s: init timeout (no data received in %v)", remoteAddr, h.cfg.InitTimeout)
				}
			} else {
				log.Printf("[conn] %s: read command: %v", remoteAddr, err)
			}
			WriteError(conn)
			return
		}

		// Save USR-VCOM config if received
		if cmd.USRVCOMCfg != nil {
			usrvcomCfg = cmd.USRVCOMCfg
		}

		// Log received command with USR-VCOM info if present
		if cmd.USRVCOMCfg != nil {
			log.Printf("[conn] %s: received command: %s param: %q (with USR-VCOM: %d baud %s)",
				remoteAddr, cmd.Cmd, cmd.Param, cmd.USRVCOMCfg.BaudRate, cmd.USRVCOMCfg.ModeString())
		} else {
			log.Printf("[conn] %s: received command: %s param: %q", remoteAddr, cmd.Cmd, cmd.Param)
		}

		switch cmd.Cmd {
		case CmdDT, CmdDP:
			// ATDT/ATDP - respond OK and wait for next command (AT+REG or AT+CONNECT)
			if err := WriteOK(conn); err != nil {
				log.Printf("[conn] %s: write OK: %v", remoteAddr, err)
				return
			}
			// Switch to post-connect timeout (longer) for next command
			timeout = h.cfg.PostConnectTimeout
			continue
		case CmdReg:
			h.handleDevice(ctx, conn, cmd.Param, remoteAddr)
			return
		case CmdConnect:
			// Preserve USR-VCOM config for client handler
			if usrvcomCfg != nil && cmd.USRVCOMCfg == nil {
				cmd.USRVCOMCfg = usrvcomCfg
			}
			h.handleClient(ctx, conn, reader, cmd, remoteAddr)
			return
		default:
			log.Printf("[conn] %s: unexpected command: %s", remoteAddr, cmd.Cmd)
			WriteError(conn)
			return
		}
	}
}

// handleDevice handles device registration
func (h *Handler) handleDevice(ctx context.Context, conn net.Conn, token string, remoteAddr string) {
	if token == "" {
		log.Printf("[device] %s: empty token", remoteAddr)
		WriteError(conn)
		return
	}

	var deviceID string

	// Parse token: AUTH_TOKEN+DEVICE_ID or just DEVICE_ID
	if h.cfg.AuthToken != "" {
		// Expect format: AUTH_TOKEN+DEVICE_ID
		parts := strings.SplitN(token, "+", 2)
		if len(parts) != 2 {
			log.Printf("[device] %s: invalid token format (expected TOKEN+DEVICE_ID)", remoteAddr)
			WriteError(conn)
			return
		}
		if parts[0] != h.cfg.AuthToken {
			log.Printf("[device] %s: invalid auth token", remoteAddr)
			WriteError(conn)
			return
		}
		deviceID = parts[1]
	} else {
		// No auth configured, token is device ID
		deviceID = token
	}

	if deviceID == "" {
		log.Printf("[device] %s: empty DEVICE_ID", remoteAddr)
		WriteError(conn)
		return
	}

	// Check if device already registered
	if existing, ok := h.registry.Get(deviceID); ok {
		log.Printf("[device] %s: device %s already registered, closing old connection", remoteAddr, deviceID)
		// Stop old keepalive goroutine first
		if existing.StopKeepalive != nil {
			close(existing.StopKeepalive)
		}
		existing.Conn.Close()
		h.registry.Unregister(deviceID)
	}

	// Enable aggressive TCP keepalive for fast dead connection detection
	// idle=30s, interval=10s, count=3 => dead connection detected in ~60s
	if err := SetTCPKeepalive(conn, 30*time.Second, 10*time.Second, 3); err != nil {
		log.Printf("[device] %s: failed to set TCP keepalive: %v", remoteAddr, err)
	}

	// Register device
	dev := &device.Device{
		ID:            deviceID,
		Conn:          conn,
		RegisteredAt:  time.Now(),
		StopKeepalive: make(chan struct{}),
	}
	h.registry.Register(dev)
	defer h.registry.Unregister(deviceID)

	log.Printf("[device] %s: registered device %s", remoteAddr, deviceID)

	// Send OK
	if err := WriteOK(conn); err != nil {
		log.Printf("[device] %s: write OK: %v", remoteAddr, err)
		return
	}

	// Wait for optional ATDT/ATDP command
	reader := bufio.NewReader(conn)
	conn.SetReadDeadline(time.Now().Add(h.cfg.PostConnectTimeout))
	cmd, err := ReadATCommand(reader, conn)
	if err == nil && (cmd.Cmd == CmdDT || cmd.Cmd == CmdDP) {
		if cmd.Param != "" {
			log.Printf("[device] %s: received %s%s", deviceID, cmd.Cmd, cmd.Param)
		} else {
			log.Printf("[device] %s: received %s", deviceID, cmd.Cmd)
		}
		WriteOK(conn)
	}

	// Clear deadline
	conn.SetReadDeadline(time.Time{})

	// Start keepalive - send NOP periodically to detect dead connections
	connClosed := make(chan struct{})
	if h.cfg.IdleTimeout > 0 {
		go h.deviceKeepalive(conn, deviceID, dev.StopKeepalive, connClosed)
	}

	// Start reader goroutine to detect connection close immediately
	readClosed := make(chan struct{})
	go h.deviceReader(conn, deviceID, dev.StopKeepalive, readClosed)

	// Wait for context cancellation or connection close
	select {
	case <-ctx.Done():
		log.Printf("[device] %s: context cancelled", deviceID)
	case <-connClosed:
		log.Printf("[device] %s: connection closed by keepalive", deviceID)
	case <-readClosed:
		log.Printf("[device] %s: connection closed by device", deviceID)
	}
}

// deviceReader reads from device connection to detect close immediately
func (h *Handler) deviceReader(conn net.Conn, deviceID string, stop <-chan struct{}, closed chan<- struct{}) {
	buf := make([]byte, 256)
	for {
		select {
		case <-stop:
			return
		default:
			// Set read deadline to allow checking stop channel periodically
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, err := conn.Read(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// Timeout is expected, continue loop
					continue
				}
				// Real error or EOF - connection closed
				log.Printf("[device] %s: read error: %v", deviceID, err)
				close(closed)
				return
			}
			// Got data from device while waiting - just discard
			// (device shouldn't send data outside of session)
		}
	}
}

// deviceKeepalive sends Telnet NOP to device periodically
func (h *Handler) deviceKeepalive(conn net.Conn, deviceID string, stop <-chan struct{}, closed chan<- struct{}) {
	ticker := time.NewTicker(h.cfg.IdleTimeout)
	defer ticker.Stop()

	nop := []byte{0xFF, 0xF1} // Telnet NOP

	for {
		select {
		case <-stop:
			// Stopped due to device re-registration, exit silently
			return
		case <-ticker.C:
			// Set write deadline to detect dead connections faster
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := conn.Write(nop); err != nil {
				log.Printf("[device] %s: keepalive failed: %v", deviceID, err)
				conn.Close()
				close(closed)
				return
			}
			conn.SetWriteDeadline(time.Time{}) // Clear deadline
		}
	}
}

// handleClient handles client connection request
// Supports both USR-VCOM and RFC2217 presets before AT command
func (h *Handler) handleClient(_ context.Context, conn net.Conn, reader *bufio.Reader, atCmd *ATCommand, remoteAddr string) {
	token := atCmd.Param
	if token == "" {
		log.Printf("[client] %s: empty token", remoteAddr)
		WriteError(conn)
		return
	}

	var deviceID string

	// Parse token: AUTH_TOKEN+DEVICE_ID or just DEVICE_ID
	if h.cfg.AuthToken != "" {
		// Expect format: AUTH_TOKEN+DEVICE_ID
		parts := strings.SplitN(token, "+", 2)
		if len(parts) != 2 {
			log.Printf("[client] %s: invalid token format (expected TOKEN+DEVICE_ID)", remoteAddr)
			WriteError(conn)
			return
		}
		if parts[0] != h.cfg.AuthToken {
			log.Printf("[client] %s: invalid auth token", remoteAddr)
			WriteError(conn)
			return
		}
		deviceID = parts[1]
	} else {
		// No auth configured, token is device ID
		deviceID = token
	}

	if deviceID == "" {
		log.Printf("[client] %s: empty DEVICE_ID", remoteAddr)
		WriteError(conn)
		return
	}

	// Build RFC2217 buffer from presets (USR-VCOM or RFC2217 data)
	var rfc2217Buf *RFC2217Buffer

	// Priority 1: USR-VCOM config (parsed before AT command)
	if atCmd.USRVCOMCfg != nil && atCmd.USRVCOMCfg.Valid {
		log.Printf("[client] %s: USR-VCOM presets: %d baud, %s",
			remoteAddr, atCmd.USRVCOMCfg.BaudRate, atCmd.USRVCOMCfg.ModeString())
		// Convert USR-VCOM to RFC2217 commands for device
		rfc2217Buf = &RFC2217Buffer{
			Commands: atCmd.USRVCOMCfg.ToRFC2217Commands(),
			RawData:  atCmd.USRVCOMCfg.BuildRFC2217Packet(),
		}
		log.Printf("[client] %s: converted USR-VCOM to %d RFC2217 commands", remoteAddr, len(rfc2217Buf.Commands))
	}

	// Priority 2: RFC2217 data in Skipped bytes
	if rfc2217Buf == nil && len(atCmd.Skipped) > 0 {
		log.Printf("[client] %s: skipped data before AT (%d bytes): %x", remoteAddr, len(atCmd.Skipped), atCmd.Skipped)

		if IsUSRVCOM(atCmd.Skipped) {
			// Late USR-VCOM detection (shouldn't happen with new parser, but just in case)
			cfg := ParseUSRVCOM(atCmd.Skipped)
			if cfg != nil && cfg.Valid {
				cfg.LogConfig("client presets (late)")
				rfc2217Buf = &RFC2217Buffer{
					Commands: cfg.ToRFC2217Commands(),
					RawData:  cfg.BuildRFC2217Packet(),
				}
			}
		} else {
			// Parse as RFC2217
			rfc2217Buf = ParseRFC2217Commands(atCmd.Skipped)
			if rfc2217Buf != nil && len(rfc2217Buf.Commands) > 0 {
				log.Printf("[client] %s: parsed RFC2217: %d commands", remoteAddr, len(rfc2217Buf.Commands))
			}
		}
	}

	// Log and respond to RFC2217 commands if present
	if rfc2217Buf != nil && len(rfc2217Buf.Commands) > 0 {
		isQuery := true
		for _, cmd := range rfc2217Buf.Commands {
			if !cmd.IsQuery() {
				isQuery = false
				break
			}
		}

		if isQuery {
			log.Printf("[client] %s: RFC2217 queries (before AT):", remoteAddr)
		} else {
			log.Printf("[client] %s: RFC2217 port settings (before AT):", remoteAddr)
		}
		for _, cmd := range rfc2217Buf.Commands {
			log.Printf("[client] %s:   - %s", remoteAddr, cmd.String())
		}
	}

	log.Printf("[client] %s: requesting session with device %s", remoteAddr, deviceID)

	// Find the device
	dev, ok := h.registry.Get(deviceID)
	if !ok {
		log.Printf("[client] %s: device %s not found", remoteAddr, deviceID)
		WriteError(conn)
		return
	}

	// Check if device is already in a session
	if dev.IsInSession() {
		log.Printf("[client] %s: device %s is busy", remoteAddr, deviceID)
		WriteError(conn)
		return
	}

	// Create session
	sess := h.sessions.Create(deviceID, conn, dev.Conn)
	dev.SetSession(sess.ID)

	log.Printf("[client] %s: created session %s with device %s", remoteAddr, sess.ID, deviceID)

	// Clear deadline and send OK to client
	conn.SetReadDeadline(time.Time{})
	if err := WriteOK(conn); err != nil {
		log.Printf("[client] %s: write OK error: %v", remoteAddr, err)
		h.sessions.End(sess.ID)
		dev.ClearSession()
		return
	}

	// Forward RFC2217 data to device after session is established
	if rfc2217Buf != nil && len(rfc2217Buf.RawData) > 0 {
		if err := ForwardRFC2217ToDevice(dev.Conn, rfc2217Buf); err != nil {
			log.Printf("[client] %s: RFC2217 forward error: %v", remoteAddr, err)
		}
	}

	// Check if there's buffered data from client (after AT command)
	// This may contain RFC2217 commands sent after the AT+CONNECT
	if reader.Buffered() > 0 {
		buffered := make([]byte, reader.Buffered())
		reader.Read(buffered)

		// Log raw data for debugging
		log.Printf("[client] %s: buffered data (%d bytes): %x", remoteAddr, len(buffered), buffered)

		// Try to parse as RFC2217 or USR-VCOM
		var bufferedRFC2217 *RFC2217Buffer
		if IsUSRVCOM(buffered) {
			cfg := ParseUSRVCOM(buffered)
			if cfg != nil && cfg.Valid {
				cfg.LogConfig("buffered presets")
				bufferedRFC2217 = &RFC2217Buffer{
					Commands: cfg.ToRFC2217Commands(),
					RawData:  cfg.BuildRFC2217Packet(),
				}
			}
		} else if len(buffered) >= 3 && buffered[0] == 0xFF && buffered[1] == 0xFA && buffered[2] == 0x2C {
			// RFC2217 data
			bufferedRFC2217 = ParseRFC2217Commands(buffered)
		}

		if bufferedRFC2217 != nil && len(bufferedRFC2217.Commands) > 0 {
			// Check if these are queries (value=0) or actual settings
			isQuery := true
			for _, cmd := range bufferedRFC2217.Commands {
				if !cmd.IsQuery() {
					isQuery = false
					break
				}
			}

			if isQuery {
				log.Printf("[client] %s: received %d RFC2217 queries (requesting current values):", remoteAddr, len(bufferedRFC2217.Commands))
			} else {
				log.Printf("[client] %s: received %d RFC2217 port settings:", remoteAddr, len(bufferedRFC2217.Commands))
			}
			for _, cmd := range bufferedRFC2217.Commands {
				log.Printf("[client] %s:   - %s", remoteAddr, cmd.String())
			}
			// Forward RFC2217 to device
			if err := ForwardRFC2217ToDevice(dev.Conn, bufferedRFC2217); err != nil {
				log.Printf("[client] %s: RFC2217 forward error: %v", remoteAddr, err)
			}
		} else {
			// Unknown data, log hex and forward as-is
			log.Printf("[client] %s: forwarding %d buffered bytes to device: %x", remoteAddr, len(buffered), buffered)
			dev.Conn.Write(buffered)
		}
	}

	// Start the bridge - blocks until session ends
	bridge := session.NewBridge(sess)
	bridge.Run()

	// Clean up
	h.sessions.End(sess.ID)
	dev.ClearSession()

	log.Printf("[client] %s: session %s ended", remoteAddr, sess.ID)
}
