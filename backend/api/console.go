package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/dell-infra-manager/backend/crypto"
	"github.com/dell-infra-manager/backend/models"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	gossh "golang.org/x/crypto/ssh"
)

type ConsoleHandler struct {
	db *sqlx.DB
}

func NewConsoleHandler(db *sqlx.DB) *ConsoleHandler {
	return &ConsoleHandler{db: db}
}

// Connect upgrades the HTTP request to a WebSocket and proxies a Serial-over-LAN
// (SOL) session to the iDRAC via SSH.
//
// iDRAC9 exposes SOL through its SSH interface: after authenticating you run
// `console com2` which connects stdin/stdout directly to the host serial port.
// We bridge that SSH session to the browser over WebSocket so xterm.js can
// render a full interactive terminal without any client-side SSH library.
//
// Wire format (binary WebSocket frames):
//   browser → backend : raw bytes to write to the SSH stdin
//   backend → browser : raw bytes read from the SSH stdout/stderr
//
// Resize events are sent as a JSON text frame:
//   {"type":"resize","cols":220,"rows":50}
func (h *ConsoleHandler) Connect(c *gin.Context) {
	id := c.Param("id")

	var s models.Server
	if err := h.db.Get(&s, `SELECT * FROM servers WHERE id = ?`, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	password, err := crypto.Decrypt(s.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "credential decrypt failed"})
		return
	}

	// Upgrade to WebSocket first — this lets us stream errors back to the
	// browser as text if the SSH connection itself fails.
	wsConn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // same origin handled by SvelteKit proxy in dev
	})
	if err != nil {
		log.Printf("console[%s] ws upgrade failed: %v", s.Name, err)
		return
	}
	defer wsConn.Close(websocket.StatusNormalClosure, "session ended")

	ctx := wsConn.CloseRead(context.Background())

	// Connect to iDRAC SSH.
	sshCfg := &gossh.ClientConfig{
		User:            s.Username,
		Auth:            []gossh.AuthMethod{gossh.Password(password)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec — self-signed iDRAC certs
		Timeout:         15 * time.Second,
	}
	// iDRAC SSH is always on port 22 regardless of the configured HTTPS port.
	sshClient, err := gossh.Dial("tcp", s.Hostname+":22", sshCfg)
	if err != nil {
		_ = wsConn.Write(ctx, websocket.MessageText,
			[]byte("\r\n\x1b[31mSSH connection failed: "+err.Error()+"\x1b[0m\r\n"))
		return
	}
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		_ = wsConn.Write(ctx, websocket.MessageText,
			[]byte("\r\n\x1b[31mSSH session failed: "+err.Error()+"\x1b[0m\r\n"))
		return
	}
	defer session.Close()

	// Request a pseudo-terminal so iDRAC's SOL respects terminal dimensions.
	if err := session.RequestPty("xterm-256color", 24, 80, gossh.TerminalModes{
		gossh.ECHO:          1,
		gossh.TTY_OP_ISPEED: 115200,
		gossh.TTY_OP_OSPEED: 115200,
	}); err != nil {
		_ = wsConn.Write(ctx, websocket.MessageText,
			[]byte("\r\n\x1b[31mPTY request failed: "+err.Error()+"\x1b[0m\r\n"))
		return
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return
	}
	// Pipe stderr to the same WebSocket writer by using a separate goroutine below.
	stderr, err := session.StderrPipe()
	if err != nil {
		return
	}

	// Start a shell (this enters iDRAC SMASH CLP).
	// The user can then type `console com2` to enter SOL, or interact with
	// the iDRAC CLI directly.
	if err := session.Shell(); err != nil {
		_ = wsConn.Write(ctx, websocket.MessageText,
			[]byte("\r\n\x1b[31mShell start failed: "+err.Error()+"\x1b[0m\r\n"))
		return
	}

	log.Printf("console[%s] session established", s.Name)

	// pipeToWS reads from an SSH pipe and forwards all bytes as binary WS frames.
	pipeToWS := func(r interface{ Read([]byte) (int, error) }) {
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				if werr := wsConn.Write(ctx, websocket.MessageBinary, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}

	go pipeToWS(stdout)
	go pipeToWS(stderr)

	// WebSocket → SSH stdin + resize handling.
	for {
		msgType, data, err := wsConn.Read(ctx)
		if err != nil {
			break
		}
		switch msgType {
		case websocket.MessageBinary:
			if _, err := stdin.Write(data); err != nil {
				goto done
			}
		case websocket.MessageText:
			// Resize event: {"type":"resize","cols":220,"rows":50}
			var ev struct {
				Type string `json:"type"`
				Cols uint32 `json:"cols"`
				Rows uint32 `json:"rows"`
			}
			if jsonErr := json.Unmarshal(data, &ev); jsonErr == nil && ev.Type == "resize" {
				_ = session.WindowChange(int(ev.Rows), int(ev.Cols))
			}
		}
	}
done:
	log.Printf("console[%s] session closed", s.Name)
}
