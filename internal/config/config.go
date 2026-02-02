package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port               string
	APIPort            string
	AuthToken          string
	WebUser            string
	WebPass            string
	KeepAlive          time.Duration
	InitTimeout        time.Duration
	PostConnectTimeout time.Duration
	IdleTimeout        time.Duration // Timeout for NOP keepalive
	Debug              bool
	DebugHTTP          bool
	ProxyProtocol      bool
}

func Load() *Config {
	return &Config{
		Port:        getEnv("PORT", "2217"),
		APIPort:     getEnv("API_PORT", "8080"),
		AuthToken:   getEnv("AUTH_TOKEN", ""),
		WebUser:     getEnv("WEB_USER", "admin"),
		WebPass:     getEnv("WEB_PASS", "admin"),
		KeepAlive:   getDurationEnv("KEEPALIVE", 30*time.Second),
		InitTimeout:        getDurationEnv("INIT_TIMEOUT", 5*time.Second),
		PostConnectTimeout: getDurationEnv("POST_CONNECT_TIMEOUT", 60*time.Second),
		IdleTimeout:        getDurationEnv("IDLE_TIMEOUT", 30*time.Second),
		Debug:              getBoolEnv("DEBUG", false),
		DebugHTTP:     getBoolEnv("DEBUG_HTTP", false),
		ProxyProtocol: getBoolEnv("PROXY_PROTOCOL", false),
	}
}

func getBoolEnv(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		return val == "1" || val == "true" || val == "yes"
	}
	return defaultVal
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if secs, err := strconv.Atoi(val); err == nil {
			return time.Duration(secs) * time.Second
		}
	}
	return defaultVal
}
