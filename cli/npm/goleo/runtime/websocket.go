package runtime

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

var hub = &Hub{
	register:   make(chan *WSClient),
	unregister: make(chan *WSClient),
	clients:    make(map[*WSClient]bool),
}

type Hub struct {
	register   chan *WSClient
	unregister chan *WSClient
	clients    map[*WSClient]bool
	mu         sync.RWMutex
}

func (h *Hub) GetAll() []*WSClient {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var result []*WSClient
	for c := range h.clients {
		result = append(result, c)
	}
	return result
}

func init() {
	go hub.run()
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
		}
	}
}

type WSClient struct {
	conn *websocket.Conn
	send chan []byte
}

func (c *WSClient) readPump(bridge *Bridge) {
	defer func() {
		hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(32768)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("websocket error: %v", err)
			}
			break
		}

		var envelope struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data,omitempty"`
		}

		if err := json.Unmarshal(message, &envelope); err != nil {
			log.Printf("invalid websocket message: %v", err)
			continue
		}

		switch envelope.Type {
		case "invoke":
			var req InvokeRequest
			if err := json.Unmarshal(envelope.Data, &req); err != nil {
				log.Printf("invalid invoke request: %v", err)
				continue
			}
			resp := bridge.HandleRequest(req)
			respData, _ := json.Marshal(map[string]any{
				"type": "invokeResult",
				"data": resp,
			})
			select {
			case c.send <- respData:
			default:
			}

		case "event":
			var msg EventMessage
			if err := json.Unmarshal(envelope.Data, &msg); err != nil {
				log.Printf("invalid event message: %v", err)
				continue
			}
			bridge.DispatchEvent(msg.Event, msg.Data)

		case "ping":
			respData, _ := json.Marshal(map[string]string{"type": "pong"})
			select {
			case c.send <- respData:
			default:
			}
		}
	}
}

func (c *WSClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
