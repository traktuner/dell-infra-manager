package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

type WSEvent struct {
	Type     string      `json:"type"`
	ServerID string      `json:"server_id,omitempty"`
	JobID    string      `json:"job_id,omitempty"`
	Data     interface{} `json:"data"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*wsClient]struct{}
	Broadcast chan WSEvent
}

type wsClient struct {
	conn *websocket.Conn
	send chan WSEvent
}

func NewHub() *Hub {
	return &Hub{
		clients:   make(map[*wsClient]struct{}),
		Broadcast: make(chan WSEvent, 256),
	}
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-h.Broadcast:
			h.mu.RLock()
			for c := range h.clients {
				select {
				case c.send <- event:
				default:
					// slow client — skip
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) register(c *wsClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) unregister(c *wsClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (h *Hub) HandleWS(c *gin.Context) {
	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // allow all origins in homelab
	})
	if err != nil {
		log.Printf("ws accept: %v", err)
		return
	}

	client := &wsClient{conn: conn, send: make(chan WSEvent, 64)}
	h.register(client)
	defer h.unregister(client)

	ctx := c.Request.Context()
	go func() {
		// read loop — just drain incoming messages (we don't process client→server commands yet)
		for {
			_, _, err := conn.Read(ctx)
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case event := <-client.send:
			data, _ := json.Marshal(event)
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		case <-ctx.Done():
			conn.Close(websocket.StatusNormalClosure, "")
			return
		}
	}
}

// Emit is a helper to send an event to all connected clients.
func (h *Hub) Emit(eventType, serverID string, data interface{}) {
	h.Broadcast <- WSEvent{Type: eventType, ServerID: serverID, Data: data}
}

// EmitJob sends a job-specific event.
func (h *Hub) EmitJob(eventType, jobID string, data interface{}) {
	h.Broadcast <- WSEvent{Type: eventType, JobID: jobID, Data: data}
}

// HandleWSGin is used in router setup.
func HandleWSGin(hub *Hub) gin.HandlerFunc {
	return hub.HandleWS
}

// upgradeMiddleware sets the necessary header for websocket upgrade check.
func upgradeCheck(w http.ResponseWriter, r *http.Request) bool {
	return r.Header.Get("Upgrade") == "websocket"
}
