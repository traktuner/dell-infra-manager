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

const defaultVNCPort = 5901

// vncTokenSecret derives a server-binding secret from the encrypted VNC password.
// Tokens are short and stateless: base64(serverID + ":" + encryptedPassword).
// The encrypted password rotates whenever we (re)configure VNC, which invalidates
// older tokens automatically.
func mintToken(serverID, encPass string) string {
	return base64.URLEncoding.EncodeToString([]byte(serverID + ":" + encPass))
}

func verifyToken(token, serverID, encPass string) bool {
	return token == mintToken(serverID, encPass)
}

func generatePassword(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	rand.Read(b) //nolint:errcheck — crypto/rand never returns an error in practice
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b)
}

// Enable returns a usable VNC port + auth token. Always PATCHes iDRAC so the
// SSL=Disabled enforcement is idempotent (without it, iDRAC's default
// "Auto Negotiate" makes noVNC reject the stream as "unexpected data message").
//
// Password handling: reuse the stored password if we have one, generate a new
// one only on first call. This avoids surprising password rotations for users
// who connect with other VNC clients in parallel.
//
// Port: whatever iDRAC currently has, or 5901 by default. Users who have
// manually configured a different port don't get overridden.
func (h *VNCHandler) Enable(c *gin.Context) {
	id := c.Param("id")

	var s models.Server
	if err := h.db.Get(&s, `SELECT * FROM servers WHERE id = ?`, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	idracPass, err := crypto.Decrypt(s.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "credential decrypt failed"})
		return
	}
	client := redfish.NewClient(s.Hostname, s.Port, s.Username, idracPass, s.TLSVerify)

	status, err := client.GetVNCStatus()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":    "Redfish: " + err.Error(),
			"fallback": "sol",
		})
		return
	}

	port := status.Port
	if port == 0 {
		port = defaultVNCPort
	}

	// Reuse stored password if it's spec-compliant; otherwise mint a fresh one.
	// iDRAC9 VNC passwords must be 1–8 ASCII characters — anything longer
	// makes the PATCH fail with 400 Bad Request. Older builds of this app
	// generated 16-char passwords; this length check forces a rotation.
	var vncPass string
	if s.VNCPassword != nil {
		vncPass, _ = crypto.Decrypt(*s.VNCPassword)
	}
	if len(vncPass) == 0 || len(vncPass) > 8 {
		vncPass = generatePassword(8)
	}

	if err := client.ConfigureVNC(port, vncPass); err != nil {
		log.Printf("vnc[%s] configure failed: %v", s.Name, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":    err.Error(),
			"fallback": "sol",
		})
		return
	}

	encPass, err := crypto.Encrypt(vncPass)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt failed"})
		return
	}
	h.db.Exec(`UPDATE servers SET vnc_password = ?, vnc_port = ? WHERE id = ?`, encPass, port, id)

	log.Printf("vnc[%s] configured on port %d", s.Name, port)
	c.JSON(http.StatusOK, gin.H{"port": port, "token": mintToken(id, encPass)})
}

// Reset clears stored VNC credentials so the next /enable forces a fresh
// password rotation against iDRAC. Use this if the password gets out of sync.
func (h *VNCHandler) Reset(c *gin.Context) {
	h.db.Exec(`UPDATE servers SET vnc_password = NULL WHERE id = ?`, c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Password returns the plaintext VNC password for the noVNC RFB credential prompt.
// Auth is via the token from /enable, which is bound to the encrypted password
// (so rotation invalidates old tokens).
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

// Proxy upgrades to WebSocket and pipes bytes between the browser (noVNC) and
// the iDRAC VNC TCP port. The RFB handshake is end-to-end opaque to us.
func (h *VNCHandler) Proxy(c *gin.Context) {
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

	port := s.VNCPort
	if port == 0 {
		port = defaultVNCPort
	}
	addr := fmt.Sprintf("%s:%d", s.Hostname, port)

	tcpConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "VNC TCP: " + err.Error()})
		return
	}

	wsConn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		Subprotocols:       []string{"binary"},
	})
	if err != nil {
		tcpConn.Close()
		log.Printf("vnc[%s] ws upgrade failed: %v", s.Name, err)
		return
	}

	log.Printf("vnc[%s] proxy → %s", s.Name, addr)
	pipeWStoTCP(wsConn, tcpConn)
	log.Printf("vnc[%s] proxy closed", s.Name)
}

// pipeWStoTCP forwards bytes both ways between a WebSocket and a TCP connection.
// Returns when either side closes.
func pipeWStoTCP(ws *websocket.Conn, tcp net.Conn) {
	defer ws.Close(websocket.StatusNormalClosure, "")
	defer tcp.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TCP → WS
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

	// WS → TCP
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
