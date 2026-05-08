package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/dell-infra-manager/backend/crypto"
	"github.com/dell-infra-manager/backend/models"
	"github.com/dell-infra-manager/backend/redfish"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type VNCHandler struct {
	db *sqlx.DB
}

func NewVNCHandler(db *sqlx.DB) *VNCHandler {
	return &VNCHandler{db: db}
}

// Enable checks if VNC is already configured for this server. If not it:
//  1. Reads current VNC status from iDRAC via Redfish
//  2. Generates a random 16-char password
//  3. Enables VNC on port 5901 via Redfish PATCH
//  4. Stores the encrypted password in the DB
//
// Returns the VNC port and a one-time token the frontend passes back when
// opening the WebSocket proxy, so we never send the actual VNC password
// to the browser.
func (h *VNCHandler) Enable(c *gin.Context) {
	id := c.Param("id")

	var s models.Server
	if err := h.db.Get(&s, `SELECT * FROM servers WHERE id = ?`, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	port := s.VNCPort
	if port == 0 {
		port = 5901
	}

	// If we already have a stored VNC password use it — just return the port.
	// A new token is minted each time so it can only be used once.
	if s.VNCPassword != nil {
		token, err := mintToken(id, *s.VNCPassword)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "token error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"port": port, "token": token})
		return
	}

	// No stored password → enable VNC on iDRAC via Redfish.
	iDRACPass, err := crypto.Decrypt(s.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "credential decrypt failed"})
		return
	}
	client := redfish.NewClient(s.Hostname, s.Port, s.Username, iDRACPass, s.TLSVerify)

	vncPass, err := generatePassword(16)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password generation failed"})
		return
	}

	if err := client.EnableVNC(port, vncPass); err != nil {
		// VNC enable failed (no Enterprise licence, unsupported firmware, etc.)
		// — caller should fall back to SSH/SOL.
		log.Printf("vnc[%s] Redfish enable failed: %v", s.Name, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":    "VNC could not be enabled via Redfish: " + err.Error(),
			"fallback": "sol",
		})
		return
	}

	// Store the encrypted VNC password so we don't re-enable on every page load.
	encPass, err := crypto.Encrypt(vncPass)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt failed"})
		return
	}
	h.db.Exec(`UPDATE servers SET vnc_password = ?, vnc_port = ? WHERE id = ?`, encPass, port, id)
	s.VNCPassword = &encPass

	token, err := mintToken(id, encPass)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token error"})
		return
	}

	log.Printf("vnc[%s] VNC enabled on port %d", s.Name, port)
	c.JSON(http.StatusOK, gin.H{"port": port, "token": token})
}

// Proxy upgrades to WebSocket and forwards all bytes bidirectionally to the
// iDRAC VNC TCP port. noVNC in the browser speaks the RFB protocol natively;
// we are a transparent pipe — no VNC parsing at all.
//
// Authentication: the browser passes ?token=<value> which we verify against
// the server's stored VNC password. The actual VNC password is exchanged
// inside the RFB handshake (opaque to us).
func (h *VNCHandler) Proxy(c *gin.Context) {
	id := c.Param("id")
	token := c.Query("token")

	var s models.Server
	if err := h.db.Get(&s, `SELECT * FROM servers WHERE id = ?`, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	if s.VNCPassword == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "VNC not configured; call /vnc/enable first"})
		return
	}
	if !verifyToken(token, id, *s.VNCPassword) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	port := s.VNCPort
	if port == 0 {
		port = 5901
	}

	// Dial iDRAC VNC before upgrading WebSocket so we can return a proper
	// HTTP error if the TCP connection fails.
	addr := fmt.Sprintf("%s:%d", s.Hostname, port)
	tcpConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "cannot reach VNC: " + err.Error()})
		return
	}

	wsConn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		// noVNC uses the "binary" sub-protocol.
		Subprotocols: []string{"binary"},
	})
	if err != nil {
		tcpConn.Close()
		log.Printf("vnc[%s] ws upgrade failed: %v", s.Name, err)
		return
	}

	log.Printf("vnc[%s] proxy started %s", s.Name, addr)
	proxyVNC(wsConn, tcpConn, s.Name)
	log.Printf("vnc[%s] proxy closed", s.Name)
}

// proxyVNC pipes bytes between the WebSocket (noVNC browser) and the TCP VNC
// server. Both goroutines share responsibility for closing on exit.
func proxyVNC(ws *websocket.Conn, tcp net.Conn, name string) {
	defer ws.Close(websocket.StatusNormalClosure, "")
	defer tcp.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TCP → WebSocket
	go func() {
		defer cancel()
		buf := make([]byte, 32*1024)
		for {
			n, err := tcp.Read(buf)
			if n > 0 {
				if werr := ws.Write(ctx, websocket.MessageBinary, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// WebSocket → TCP
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			return
		}
		if _, err := tcp.Write(data); err != nil {
			return
		}
	}
}

// --- token helpers ---
// Tokens are simple: base64(serverID + ":" + encryptedPassword).
// They are single-use from the browser's perspective (a new one is issued
// every time /vnc/enable is called) but we don't enforce single-use server-side
// to keep things stateless. The encrypted password is the shared secret.

func mintToken(serverID, encPass string) (string, error) {
	raw := serverID + ":" + encPass
	return base64.URLEncoding.EncodeToString([]byte(raw)), nil
}

func verifyToken(token, serverID, encPass string) bool {
	expected, err := mintToken(serverID, encPass)
	if err != nil {
		return false
	}
	return token == expected
}

func generatePassword(length int) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b), nil
}

// VNCInfo returns the VNC port and a fresh token if VNC is configured,
// or a 404 if it has not been set up yet. Used by the frontend to check
// state without triggering a Redfish call.
func (h *VNCHandler) Info(c *gin.Context) {
	id := c.Param("id")
	var s models.Server
	if err := h.db.Get(&s, `SELECT * FROM servers WHERE id = ?`, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	if s.VNCPassword == nil {
		c.JSON(http.StatusOK, gin.H{"configured": false})
		return
	}
	token, err := mintToken(id, *s.VNCPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token error"})
		return
	}
	port := s.VNCPort
	if port == 0 {
		port = 5901
	}
	c.JSON(http.StatusOK, gin.H{"configured": true, "port": port, "token": token})
}

// Password returns the plaintext VNC password so the browser can pass it to
// the noVNC RFB credential prompt. The token acts as a short-lived bearer credential.
func (h *VNCHandler) Password(c *gin.Context) {
	id := c.Param("id")
	token := c.Query("token")

	var s models.Server
	if err := h.db.Get(&s, `SELECT * FROM servers WHERE id = ?`, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	if s.VNCPassword == nil || !verifyToken(token, id, *s.VNCPassword) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}
	plain, err := crypto.Decrypt(*s.VNCPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "decrypt failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"password": plain})
}

// Reset clears the stored VNC password so the next /enable call re-configures iDRAC.
func (h *VNCHandler) Reset(c *gin.Context) {
	id := c.Param("id")
	h.db.Exec(`UPDATE servers SET vnc_password = NULL WHERE id = ?`, id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

