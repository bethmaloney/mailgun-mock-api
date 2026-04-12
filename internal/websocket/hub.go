package websocket

import (
	"encoding/json"
	"log"
	"net/http"

	gorilla "github.com/gorilla/websocket"
)

// BroadcastMessage is the message format sent to WebSocket clients.
type BroadcastMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// Client represents a WebSocket connection.
type Client struct {
	Hub  *Hub
	Conn *gorilla.Conn
	Send chan []byte
	done chan struct{} // closed when the client is unregistered
}

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	Broadcast  chan BroadcastMessage
	Register   chan *Client
	Unregister chan *Client
	clients    map[*Client]bool
}

// broadcastBufferSize bounds the number of pending broadcasts. HTTP handlers
// publish via Publish(), which drops on full rather than blocking, so this
// size only needs to absorb normal bursts — not unlimited traffic.
const broadcastBufferSize = 256

// NewHub creates a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		Broadcast:  make(chan BroadcastMessage, broadcastBufferSize),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

// Publish sends a broadcast message without blocking the caller.
// If the broadcast queue is full (e.g. Hub.Run is stalled or overwhelmed),
// the message is dropped and logged. HTTP handlers MUST use Publish rather
// than sending to Broadcast directly, so that WebSocket plumbing can never
// wedge an API request.
func (h *Hub) Publish(msg BroadcastMessage) {
	select {
	case h.Broadcast <- msg:
	default:
		log.Printf("websocket: broadcast queue full, dropping %s", msg.Type)
	}
}

// Run starts the hub's main loop. It should be launched in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.clients[client] = true

		case client := <-h.Unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if client.done != nil {
					close(client.done)
				}
			}

		case msg := <-h.Broadcast:
			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("websocket: failed to marshal broadcast message: %v", err)
				continue
			}
			for client := range h.clients {
				select {
				case client.Send <- data:
				default:
					// Client's send buffer is full; disconnect it.
					delete(h.clients, client)
					if client.done != nil {
						close(client.done)
					}
				}
			}
		}
	}
}

var upgrader = gorilla.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// HandleWebSocket handles WebSocket upgrade requests.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket: upgrade error: %v", err)
		return
	}

	client := &Client{
		Hub:  h,
		Conn: conn,
		Send: make(chan []byte, 256),
		done: make(chan struct{}),
	}

	h.Register <- client

	go client.writePump()
	go client.readPump()
}

// writePump pumps messages from the hub to the WebSocket connection.
func (c *Client) writePump() {
	defer c.Conn.Close()
	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				// Send channel was closed unexpectedly.
				c.Conn.WriteMessage(gorilla.CloseMessage, []byte{})
				return
			}
			w, err := c.Conn.NextWriter(gorilla.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)
			if err := w.Close(); err != nil {
				return
			}
		case <-c.done:
			c.Conn.WriteMessage(gorilla.CloseMessage, []byte{})
			return
		}
	}
}

// readPump reads messages from the WebSocket connection and discards them.
// It detects disconnection and cleans up the client.
func (c *Client) readPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()
	c.Conn.SetReadLimit(512)
	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}
	}
}
