package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/config"
	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/device"
	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/session"
)

const authCookieName = "rfc2217_auth"

// Handlers contains HTTP API handlers
type Handlers struct {
	cfg      *config.Config
	registry *device.Registry
	sessions *session.Manager
}

// NewHandlers creates new API handlers
func NewHandlers(cfg *config.Config, registry *device.Registry, sessions *session.Manager) *Handlers {
	return &Handlers{
		cfg:      cfg,
		registry: registry,
		sessions: sessions,
	}
}

// isLoggedIn checks if user is authenticated via cookie only
// Note: Basic Auth is NOT checked here to allow proper logout
// (browsers cache Basic Auth credentials and resend them automatically)
// Use isAuthorized for API endpoints that need to support Basic Auth
func (h *Handlers) isLoggedIn(r *http.Request) bool {
	// Check cookie only
	cookie, err := r.Cookie(authCookieName)
	if err == nil && cookie.Value != "" {
		// Validate cookie (simple base64 encoded credentials)
		decoded, err := base64.StdEncoding.DecodeString(cookie.Value)
		if err == nil {
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 && parts[0] == h.cfg.WebUser && parts[1] == h.cfg.WebPass {
				return true
			}
		}
	}

	return false
}

// isAuthorized checks if user is authenticated via cookie OR Basic Auth
// Use this for API endpoints that need to support programmatic access
func (h *Handlers) isAuthorized(r *http.Request) bool {
	// Check cookie first
	if h.isLoggedIn(r) {
		return true
	}

	// Fall back to Basic Auth for API access
	return checkAuth(r, h.cfg.WebUser, h.cfg.WebPass)
}

// Login handles POST /login
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Show login form
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(loginFormHTML()))
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.ParseForm()
	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == h.cfg.WebUser && password == h.cfg.WebPass {
		// Set auth cookie
		cookieValue := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		http.SetCookie(w, &http.Cookie{
			Name:     authCookieName,
			Value:    cookieValue,
			Path:     "/",
			MaxAge:   86400 * 7, // 7 days
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	} else {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(loginFormHTML() + `<p style="color: #f66; text-align: center;">Invalid credentials</p>`))
	}
}

// Logout handles GET /logout
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Healthz handles liveness probe
func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Readyz handles readiness probe
func (h *Handlers) Readyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

// DevicesResponse is the response for GET /api/v1/devices
type DevicesResponse struct {
	Count   int                 `json:"count"`
	Devices []device.DeviceInfo `json:"devices"`
}

// ListDevices handles GET /api/v1/devices
func (h *Handlers) ListDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices := h.registry.ListInfo()
	if devices == nil {
		devices = []device.DeviceInfo{}
	}

	// Hide IP addresses if not authorized (supports Basic Auth for API)
	if !h.isAuthorized(r) {
		for i := range devices {
			devices[i].RemoteAddr = maskIP(devices[i].RemoteAddr)
		}
	}

	resp := DevicesResponse{
		Count:   len(devices),
		Devices: devices,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// SessionsResponse is the response for GET /api/v1/sessions
type SessionsResponse struct {
	Count    int                   `json:"count"`
	Sessions []session.SessionInfo `json:"sessions"`
}

// ListSessions handles GET /api/v1/sessions
func (h *Handlers) ListSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessions := h.sessions.ListInfo()
	if sessions == nil {
		sessions = []session.SessionInfo{}
	}

	// Hide IP addresses if not authorized (supports Basic Auth for API)
	if !h.isAuthorized(r) {
		for i := range sessions {
			sessions[i].ClientAddr = maskIP(sessions[i].ClientAddr)
			sessions[i].DeviceAddr = maskIP(sessions[i].DeviceAddr)
		}
	}

	resp := SessionsResponse{
		Count:    len(sessions),
		Sessions: sessions,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// TerminateSession handles DELETE /api/v1/sessions/{id}
func (h *Handlers) TerminateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Require authentication for terminate (supports Basic Auth for API)
	if !h.isAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract session ID from path: /api/v1/sessions/{id}
	sessionID := r.URL.Path[len("/api/v1/sessions/"):]
	if sessionID == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}

	if h.sessions.Terminate(sessionID) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "terminated"})
	} else {
		http.Error(w, "session not found", http.StatusNotFound)
	}
}

// StatsResponse is the response for GET /api/v1/stats
type StatsResponse struct {
	DevicesConnected int `json:"devices_connected"`
	SessionsActive   int `json:"sessions_active"`
}

// Stats handles GET /api/v1/stats
func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := StatsResponse{
		DevicesConnected: h.registry.Count(),
		SessionsActive:   h.sessions.Count(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Dashboard handles GET / - web dashboard
func (h *Handlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	isLoggedIn := h.isLoggedIn(r)

	devices := h.registry.ListInfo()
	if devices == nil {
		devices = []device.DeviceInfo{}
	}

	sessions := h.sessions.ListInfo()
	if sessions == nil {
		sessions = []session.SessionInfo{}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML(len(devices), len(sessions), devices, sessions, isLoggedIn)))
}

// maskIP masks IP address for privacy (shows only first octet)
func maskIP(addr string) string {
	if addr == "" {
		return ""
	}
	// Handle IPv4: "192.168.1.100:12345" -> "192.x.x.x:xxxxx"
	if idx := strings.Index(addr, "."); idx > 0 {
		firstOctet := addr[:idx]
		return firstOctet + ".x.x.x:xxxxx"
	}
	// Handle IPv6 or other formats
	return "x.x.x.x:xxxxx"
}

func loginFormHTML() string {
	return `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Login - RFC-2217 Proxy</title>
    <style>
        * { box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, monospace; margin: 0; padding: 20px; background: #1a1a2e; color: #eee; display: flex; justify-content: center; align-items: center; min-height: 100vh; }
        .login-box { background: #16213e; padding: 40px; border-radius: 12px; width: 100%; max-width: 400px; }
        h1 { color: #0f0; margin: 0 0 30px 0; text-align: center; font-size: 1.5em; }
        label { display: block; color: #888; margin-bottom: 5px; font-size: 0.9em; }
        input[type="text"], input[type="password"] { width: 100%; padding: 12px; margin-bottom: 20px; background: #0f3460; border: 1px solid #333; border-radius: 6px; color: #fff; font-size: 1em; }
        input[type="text"]:focus, input[type="password"]:focus { outline: none; border-color: #0cf; }
        button { width: 100%; padding: 12px; background: #0a3; color: #fff; border: none; border-radius: 6px; font-size: 1em; cursor: pointer; }
        button:hover { background: #0b4; }
        .back { text-align: center; margin-top: 20px; }
        .back a { color: #0cf; text-decoration: none; }
    </style>
</head>
<body>
    <div class="login-box">
        <h1>RFC-2217 Proxy Login</h1>
        <form method="POST" action="/login">
            <label>Username</label>
            <input type="text" name="username" required autofocus>
            <label>Password</label>
            <input type="password" name="password" required>
            <button type="submit">Login</button>
        </form>
        <div class="back"><a href="/">← Back to Dashboard</a></div>
    </div>
</body>
</html>`
}

func dashboardHTML(devCount, sessCount int, devices []device.DeviceInfo, sessions []session.SessionInfo, isLoggedIn bool) string {
	loginBtn := `<a href="/login" class="btn-login">Login</a>`
	if isLoggedIn {
		loginBtn = `<span class="logged-in">● Logged in</span> <a href="/logout" class="btn-logout">Logout</a>`
	}

	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>RFC-2217 Proxy</title>
    <meta http-equiv="refresh" content="5">
    <style>
        * { box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, monospace; margin: 0; padding: 20px; background: #1a1a2e; color: #eee; }
        .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
        h1 { color: #0f0; margin: 0; }
        h2 { color: #0cf; margin-top: 30px; border-bottom: 1px solid #333; padding-bottom: 5px; }
        .stats { display: flex; gap: 20px; margin: 20px 0; }
        .stat { background: #16213e; padding: 15px 25px; border-radius: 8px; border-left: 4px solid #0f0; }
        .stat-value { font-size: 2em; font-weight: bold; color: #0f0; }
        .stat-label { color: #888; font-size: 0.9em; }
        table { width: 100%; border-collapse: collapse; margin-top: 10px; background: #16213e; border-radius: 8px; overflow: hidden; }
        th, td { padding: 12px 15px; text-align: left; border-bottom: 1px solid #333; }
        th { background: #0f3460; color: #0cf; font-weight: normal; text-transform: uppercase; font-size: 0.85em; }
        tr:hover { background: #1a1a3e; }
        .badge { padding: 3px 8px; border-radius: 4px; font-size: 0.85em; }
        .badge-green { background: #0a3; color: #fff; }
        .badge-yellow { background: #a80; color: #fff; }
        .empty { color: #666; font-style: italic; padding: 20px; text-align: center; }
        .refresh { color: #666; font-size: 0.8em; }
        .btn-terminate { background: #a00; color: #fff; border: none; padding: 5px 10px; border-radius: 4px; cursor: pointer; font-size: 0.85em; }
        .btn-terminate:hover { background: #c00; }
        .btn-login { background: #0a3; color: #fff; padding: 8px 16px; border-radius: 6px; text-decoration: none; font-size: 0.9em; }
        .btn-login:hover { background: #0b4; }
        .btn-logout { color: #f66; text-decoration: none; font-size: 0.9em; margin-left: 10px; }
        .logged-in { color: #0f0; font-size: 0.9em; }
        .masked { color: #666; font-style: italic; }
        .auth-section { display: flex; align-items: center; gap: 10px; }
    </style>
</head>
<body>
    <div class="header">
        <div>
            <h1>RFC-2217 NAT Proxy</h1>
            <p class="refresh">Auto-refresh: 5s</p>
        </div>
        <div class="auth-section">
            ` + loginBtn + `
        </div>
    </div>

    <div class="stats">
        <div class="stat">
            <div class="stat-value">` + itoa(devCount) + `</div>
            <div class="stat-label">Devices Connected</div>
        </div>
        <div class="stat">
            <div class="stat-value">` + itoa(sessCount) + `</div>
            <div class="stat-label">Active Sessions</div>
        </div>
    </div>

    <h2>Devices</h2>
    <table>
        <tr>
            <th>ID</th>
            <th>Remote Address</th>
            <th>Registered</th>
            <th>Status</th>
        </tr>`

	if len(devices) == 0 {
		html += `<tr><td colspan="4" class="empty">No devices connected</td></tr>`
	} else {
		for _, d := range devices {
			status := `<span class="badge badge-green">idle</span>`
			if d.InSession {
				status = `<span class="badge badge-yellow">in session</span>`
			}
			remoteAddr := d.RemoteAddr
			if !isLoggedIn {
				remoteAddr = `<span class="masked">` + maskIP(d.RemoteAddr) + `</span>`
			}
			html += `<tr>
                <td>` + d.ID + `</td>
                <td>` + remoteAddr + `</td>
                <td>` + d.RegisteredAt.Format("2006-01-02 15:04:05") + `</td>
                <td>` + status + `</td>
            </tr>`
		}
	}

	html += `</table>

    <h2>Sessions</h2>
    <table>
        <tr>
            <th>Session ID</th>
            <th>Device</th>
            <th>Client</th>
            <th>Started</th>
            <th>Duration</th>
            <th>Traffic</th>`

	if isLoggedIn {
		html += `<th>Actions</th>`
	}

	html += `</tr>`

	if len(sessions) == 0 {
		cols := "6"
		if isLoggedIn {
			cols = "7"
		}
		html += `<tr><td colspan="` + cols + `" class="empty">No active sessions</td></tr>`
	} else {
		for _, s := range sessions {
			clientAddr := s.ClientAddr
			if !isLoggedIn {
				clientAddr = `<span class="masked">` + maskIP(s.ClientAddr) + `</span>`
			}
			html += `<tr>
                <td>` + s.ID + `</td>
                <td>` + s.DeviceID + `</td>
                <td>` + clientAddr + `</td>
                <td>` + s.StartedAt.Format("15:04:05") + `</td>
                <td>` + formatDuration(s.DurationSecs) + `</td>
                <td>↓` + formatBytes(s.BytesIn) + ` ↑` + formatBytes(s.BytesOut) + `</td>`

			if isLoggedIn {
				html += `<td><button class="btn-terminate" onclick="terminateSession('` + s.ID + `')">Terminate</button></td>`
			}
			html += `</tr>`
		}
	}

	html += `</table>`

	if isLoggedIn {
		html += `
    <script>
    function terminateSession(id) {
        if (!confirm('Terminate session ' + id + '?')) return;
        fetch('/api/v1/sessions/' + id, {method: 'DELETE'})
            .then(r => { if (r.ok) location.reload(); else alert('Failed to terminate session'); })
            .catch(e => alert('Error: ' + e));
    }
    </script>`
	}

	html += `
</body>
</html>`

	return html
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func formatDuration(secs float64) string {
	if secs < 60 {
		return strconv.Itoa(int(secs)) + "s"
	}
	if secs < 3600 {
		return strconv.Itoa(int(secs/60)) + "m " + strconv.Itoa(int(secs)%60) + "s"
	}
	return strconv.Itoa(int(secs/3600)) + "h " + strconv.Itoa(int(secs/60)%60) + "m"
}

func formatBytes(b int64) string {
	if b < 1024 {
		return strconv.FormatInt(b, 10) + "B"
	}
	if b < 1024*1024 {
		return strconv.FormatInt(b/1024, 10) + "KB"
	}
	return strconv.FormatInt(b/(1024*1024), 10) + "MB"
}
