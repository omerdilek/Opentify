//go:build !android && !ios

package discord

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hugolgst/rich-go/client"
)

const clientID = "1433179062808875028" // Discord'dan aldığınız Application ID'yi buraya yazın

type Client struct {
	mu                 sync.Mutex
	connected          bool
	lastTrack          string
	startTime          time.Time
	lastConnectAttempt time.Time
}

var (
	instance *Client
	once     sync.Once
)

// Get returns the singleton Discord client instance
func Get() *Client {
	once.Do(func() {
		instance = &Client{}
	})
	return instance
}

// Connect initializes Discord RPC connection
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// Try to login (let the library decide the best IPC socket). We keep errors, but do not spam.
	err := client.Login(clientID)
	if err != nil {
		return fmt.Errorf("discord bağlantısı başarısız: %w", err)
	}

	c.connected = true
	return nil
}

// UpdatePresence updates the Discord Rich Presence with track info
func (c *Client) UpdatePresence(trackPath string, artist string, title string, isPaused bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		// Opportunistic reconnect with a small cooldown to avoid spamming
		if time.Since(c.lastConnectAttempt) > 2*time.Second && ipcAvailable() {
			c.lastConnectAttempt = time.Now()
			if err := client.Login(clientID); err == nil {
				c.connected = true
			} else {
				return nil
			}
		} else {
			return nil
		}
	}

	// If track changed, reset start time
	if c.lastTrack != trackPath {
		c.startTime = time.Now()
		c.lastTrack = trackPath
	}

	// Build display strings
	details := title
	if details == "" {
		// Fallback to filename without extension
		base := filepath.Base(trackPath)
		details = strings.TrimSuffix(base, filepath.Ext(base))
	}

	state := artist
	if state == "" {
		state = "Opentify"
	}

	activity := client.Activity{
		Details:    details,
		State:      state,
		LargeImage: "opentify_logo", // Discord'da yüklediğiniz asset key'i
		LargeText:  "Opentify",
		Timestamps: &client.Timestamps{
			Start: &c.startTime,
		},
	}

	if isPaused {
		activity.SmallImage = "pause"
		activity.SmallText = "Duraklatıldı"
	} else {
		activity.SmallImage = "play"
		activity.SmallText = "Çalıyor"
	}

	if err := client.SetActivity(activity); err != nil {
		// If the pipe is broken (Discord restarted/closed), try to reconnect once and retry.
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "broken pipe") || strings.Contains(errStr, "use of closed network connection") || strings.Contains(errStr, "connection reset") || strings.Contains(errStr, "eof") {
			client.Logout()
			c.connected = false
			// one shot reconnect
			if time.Since(c.lastConnectAttempt) > 2*time.Second && ipcAvailable() {
				c.lastConnectAttempt = time.Now()
				if e2 := client.Login(clientID); e2 == nil {
					c.connected = true
					_ = client.SetActivity(activity)
					return nil
				}
			}
			return nil
		}
		// Non-pipe errors: return once (let caller log if needed)
		return err
	}
	return nil
}

// ClearPresence clears the Rich Presence
func (c *Client) ClearPresence() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}
	c.lastTrack = ""
	if !c.connected {
		return nil
	}
	if err := client.SetActivity(client.Activity{}); err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "broken pipe") || strings.Contains(errStr, "use of closed network connection") || strings.Contains(errStr, "connection reset") || strings.Contains(errStr, "eof") {
			client.Logout()
			c.connected = false
			return nil
		}
		return err
	}
	return nil
}

// Disconnect closes the Discord RPC connection
func (c *Client) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		client.Logout()
		c.connected = false
	}
}

// ipcAvailable checks for the presence of a Discord IPC socket on this OS.
func ipcAvailable() bool {
	switch runtime.GOOS {
	case "linux":
		base := fmt.Sprintf("/run/user/%d", os.Getuid())
		matches, _ := filepath.Glob(filepath.Join(base, "discord-ipc-*"))
		for _, m := range matches {
			if c, err := net.DialTimeout("unix", m, 200*time.Millisecond); err == nil {
				_ = c.Close()
				return true
			}
		}
		return false
	case "darwin":
		matches, _ := filepath.Glob("/tmp/discord-ipc-*")
		for _, m := range matches {
			if c, err := net.DialTimeout("unix", m, 200*time.Millisecond); err == nil {
				_ = c.Close()
				return true
			}
		}
		return false
	default:
		// Best-effort: for unsupported OS checks, allow trying to connect.
		return true
	}
}
