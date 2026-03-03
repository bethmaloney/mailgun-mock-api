package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gorilla "github.com/gorilla/websocket"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// startHub creates a new Hub, starts its Run loop in a goroutine, and returns
// the hub. The goroutine is tied to the test lifetime.
func startHub(t *testing.T) *Hub {
	t.Helper()
	hub := NewHub()
	go hub.Run()
	return hub
}

// startTestServer creates an httptest.Server wired to the hub's HandleWebSocket
// handler and returns the server along with the WebSocket URL to connect to.
func startTestServer(t *testing.T, hub *Hub) (*httptest.Server, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	t.Cleanup(server.Close)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/mock/ws"
	return server, wsURL
}

// dialWS connects to the WebSocket URL and returns the connection. It registers
// a cleanup function to close the connection when the test ends.
func dialWS(t *testing.T, url string) *gorilla.Conn {
	t.Helper()
	conn, _, err := gorilla.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to dial WebSocket at %s: %v", url, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// readJSON reads a single JSON message from the WebSocket connection with a
// timeout and decodes it into the provided target.
func readJSON(t *testing.T, conn *gorilla.Conn, target interface{}, timeout time.Duration) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(timeout))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read WebSocket message: %v", err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("failed to unmarshal WebSocket message %q: %v", string(raw), err)
	}
}

// ===========================================================================
// 1. Hub functionality tests
// ===========================================================================

// ---------------------------------------------------------------------------
// 1a. NewHub creates a properly initialised hub
// ---------------------------------------------------------------------------

func TestNewHub(t *testing.T) {
	hub := NewHub()

	t.Run("Broadcast channel is non-nil", func(t *testing.T) {
		if hub.Broadcast == nil {
			t.Error("expected Broadcast channel to be non-nil")
		}
	})

	t.Run("Register channel is non-nil", func(t *testing.T) {
		if hub.Register == nil {
			t.Error("expected Register channel to be non-nil")
		}
	})

	t.Run("Unregister channel is non-nil", func(t *testing.T) {
		if hub.Unregister == nil {
			t.Error("expected Unregister channel to be non-nil")
		}
	})

	t.Run("clients map is non-nil", func(t *testing.T) {
		if hub.clients == nil {
			t.Error("expected clients map to be non-nil")
		}
	})

	t.Run("clients map starts empty", func(t *testing.T) {
		if len(hub.clients) != 0 {
			t.Errorf("expected clients map to be empty, got %d entries", len(hub.clients))
		}
	})
}

// ---------------------------------------------------------------------------
// 1b. Register adds a client to the hub
// ---------------------------------------------------------------------------

func TestHub_RegisterClient(t *testing.T) {
	hub := startHub(t)

	client := &Client{
		Hub:  hub,
		Send: make(chan []byte, 256),
	}

	hub.Register <- client

	// Give the hub's Run loop time to process the registration.
	time.Sleep(50 * time.Millisecond)

	t.Run("client is tracked after registration", func(t *testing.T) {
		// We cannot directly inspect hub.clients from outside, but the
		// broadcast test below will confirm the client receives messages.
		// For the unit-level check we send a broadcast and verify the client
		// gets it, proving registration worked.
		msg := BroadcastMessage{
			Type: "test.ping",
			Data: map[string]string{"status": "ok"},
		}
		hub.Broadcast <- msg

		select {
		case raw := <-client.Send:
			var received BroadcastMessage
			if err := json.Unmarshal(raw, &received); err != nil {
				t.Fatalf("failed to unmarshal broadcast: %v", err)
			}
			if received.Type != "test.ping" {
				t.Errorf("expected type %q, got %q", "test.ping", received.Type)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("timed out waiting for broadcast on registered client")
		}
	})
}

// ---------------------------------------------------------------------------
// 1c. Unregister removes a client from the hub
// ---------------------------------------------------------------------------

func TestHub_UnregisterClient(t *testing.T) {
	hub := startHub(t)

	client := &Client{
		Hub:  hub,
		Send: make(chan []byte, 256),
	}

	// Register then unregister.
	hub.Register <- client
	time.Sleep(50 * time.Millisecond)

	hub.Unregister <- client
	time.Sleep(50 * time.Millisecond)

	t.Run("client no longer receives broadcasts after unregister", func(t *testing.T) {
		msg := BroadcastMessage{
			Type: "test.after_unregister",
			Data: nil,
		}
		hub.Broadcast <- msg

		// Allow the hub to process the broadcast.
		time.Sleep(100 * time.Millisecond)

		select {
		case data := <-client.Send:
			t.Errorf("expected no message after unregister, but got: %s", string(data))
		default:
			// Good -- nothing received.
		}
	})
}

// ---------------------------------------------------------------------------
// 1d. Broadcast sends a message to a single connected client
// ---------------------------------------------------------------------------

func TestHub_BroadcastSingleClient(t *testing.T) {
	hub := startHub(t)

	client := &Client{
		Hub:  hub,
		Send: make(chan []byte, 256),
	}

	hub.Register <- client
	time.Sleep(50 * time.Millisecond)

	msg := BroadcastMessage{
		Type: "message.new",
		Data: map[string]string{"id": "abc123"},
	}
	hub.Broadcast <- msg

	select {
	case raw := <-client.Send:
		var received BroadcastMessage
		if err := json.Unmarshal(raw, &received); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		t.Run("type matches", func(t *testing.T) {
			if received.Type != "message.new" {
				t.Errorf("expected type %q, got %q", "message.new", received.Type)
			}
		})

		t.Run("data is present", func(t *testing.T) {
			if received.Data == nil {
				t.Error("expected data to be non-nil")
			}
		})
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for broadcast")
	}
}

// ---------------------------------------------------------------------------
// 1e. Broadcast sends a message to multiple connected clients simultaneously
// ---------------------------------------------------------------------------

func TestHub_BroadcastMultipleClients(t *testing.T) {
	hub := startHub(t)

	const numClients = 5
	clients := make([]*Client, numClients)
	for i := 0; i < numClients; i++ {
		clients[i] = &Client{
			Hub:  hub,
			Send: make(chan []byte, 256),
		}
		hub.Register <- clients[i]
	}
	time.Sleep(50 * time.Millisecond)

	msg := BroadcastMessage{
		Type: "event.new",
		Data: map[string]string{"event": "delivered"},
	}
	hub.Broadcast <- msg

	// Collect messages from all clients concurrently.
	var wg sync.WaitGroup
	errors := make([]string, numClients)

	for i, c := range clients {
		wg.Add(1)
		go func(idx int, cl *Client) {
			defer wg.Done()
			select {
			case raw := <-cl.Send:
				var received BroadcastMessage
				if err := json.Unmarshal(raw, &received); err != nil {
					errors[idx] = "unmarshal error: " + err.Error()
					return
				}
				if received.Type != "event.new" {
					errors[idx] = "wrong type: " + received.Type
				}
			case <-time.After(500 * time.Millisecond):
				errors[idx] = "timed out"
			}
		}(i, c)
	}

	wg.Wait()

	for i, errMsg := range errors {
		if errMsg != "" {
			t.Errorf("client[%d]: %s", i, errMsg)
		}
	}
}

// ===========================================================================
// 2. WebSocket message type tests
// ===========================================================================

// ---------------------------------------------------------------------------
// 2a. message.new
// ---------------------------------------------------------------------------

func TestBroadcastMessage_MessageNew(t *testing.T) {
	hub := startHub(t)

	client := &Client{
		Hub:  hub,
		Send: make(chan []byte, 256),
	}
	hub.Register <- client
	time.Sleep(50 * time.Millisecond)

	msg := BroadcastMessage{
		Type: "message.new",
		Data: map[string]interface{}{
			"id":      "msg-001",
			"from":    "sender@example.com",
			"to":      "recipient@example.com",
			"subject": "Hello World",
		},
	}
	hub.Broadcast <- msg

	select {
	case raw := <-client.Send:
		var received BroadcastMessage
		if err := json.Unmarshal(raw, &received); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		t.Run("type is message.new", func(t *testing.T) {
			if received.Type != "message.new" {
				t.Errorf("expected type %q, got %q", "message.new", received.Type)
			}
		})

		t.Run("data contains message fields", func(t *testing.T) {
			dataMap, ok := received.Data.(map[string]interface{})
			if !ok {
				t.Fatalf("expected data to be a map, got %T", received.Data)
			}
			if dataMap["id"] != "msg-001" {
				t.Errorf("expected id %q, got %v", "msg-001", dataMap["id"])
			}
			if dataMap["from"] != "sender@example.com" {
				t.Errorf("expected from %q, got %v", "sender@example.com", dataMap["from"])
			}
			if dataMap["subject"] != "Hello World" {
				t.Errorf("expected subject %q, got %v", "Hello World", dataMap["subject"])
			}
		})

		t.Run("serialised JSON has correct structure", func(t *testing.T) {
			var rawMap map[string]json.RawMessage
			if err := json.Unmarshal(raw, &rawMap); err != nil {
				t.Fatalf("failed to unmarshal raw JSON: %v", err)
			}
			if _, ok := rawMap["type"]; !ok {
				t.Error("JSON missing 'type' key")
			}
			if _, ok := rawMap["data"]; !ok {
				t.Error("JSON missing 'data' key")
			}
		})
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for message.new broadcast")
	}
}

// ---------------------------------------------------------------------------
// 2b. event.new
// ---------------------------------------------------------------------------

func TestBroadcastMessage_EventNew(t *testing.T) {
	hub := startHub(t)

	client := &Client{
		Hub:  hub,
		Send: make(chan []byte, 256),
	}
	hub.Register <- client
	time.Sleep(50 * time.Millisecond)

	msg := BroadcastMessage{
		Type: "event.new",
		Data: map[string]interface{}{
			"event_type": "delivered",
			"recipient":  "user@example.com",
			"timestamp":  1700000000.0,
		},
	}
	hub.Broadcast <- msg

	select {
	case raw := <-client.Send:
		var received BroadcastMessage
		if err := json.Unmarshal(raw, &received); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		t.Run("type is event.new", func(t *testing.T) {
			if received.Type != "event.new" {
				t.Errorf("expected type %q, got %q", "event.new", received.Type)
			}
		})

		t.Run("data contains event fields", func(t *testing.T) {
			dataMap, ok := received.Data.(map[string]interface{})
			if !ok {
				t.Fatalf("expected data to be a map, got %T", received.Data)
			}
			if dataMap["event_type"] != "delivered" {
				t.Errorf("expected event_type %q, got %v", "delivered", dataMap["event_type"])
			}
			if dataMap["recipient"] != "user@example.com" {
				t.Errorf("expected recipient %q, got %v", "user@example.com", dataMap["recipient"])
			}
		})
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event.new broadcast")
	}
}

// ---------------------------------------------------------------------------
// 2c. webhook.delivery
// ---------------------------------------------------------------------------

func TestBroadcastMessage_WebhookDelivery(t *testing.T) {
	hub := startHub(t)

	client := &Client{
		Hub:  hub,
		Send: make(chan []byte, 256),
	}
	hub.Register <- client
	time.Sleep(50 * time.Millisecond)

	msg := BroadcastMessage{
		Type: "webhook.delivery",
		Data: map[string]interface{}{
			"url":         "http://localhost:3000/hooks",
			"event_type":  "delivered",
			"status_code": 200,
			"domain":      "example.com",
		},
	}
	hub.Broadcast <- msg

	select {
	case raw := <-client.Send:
		var received BroadcastMessage
		if err := json.Unmarshal(raw, &received); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		t.Run("type is webhook.delivery", func(t *testing.T) {
			if received.Type != "webhook.delivery" {
				t.Errorf("expected type %q, got %q", "webhook.delivery", received.Type)
			}
		})

		t.Run("data contains webhook delivery fields", func(t *testing.T) {
			dataMap, ok := received.Data.(map[string]interface{})
			if !ok {
				t.Fatalf("expected data to be a map, got %T", received.Data)
			}
			if dataMap["url"] != "http://localhost:3000/hooks" {
				t.Errorf("expected url %q, got %v", "http://localhost:3000/hooks", dataMap["url"])
			}
			// JSON numbers are float64 by default.
			if sc, ok := dataMap["status_code"].(float64); !ok || sc != 200 {
				t.Errorf("expected status_code 200, got %v", dataMap["status_code"])
			}
		})
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for webhook.delivery broadcast")
	}
}

// ---------------------------------------------------------------------------
// 2d. config.updated
// ---------------------------------------------------------------------------

func TestBroadcastMessage_ConfigUpdated(t *testing.T) {
	hub := startHub(t)

	client := &Client{
		Hub:  hub,
		Send: make(chan []byte, 256),
	}
	hub.Register <- client
	time.Sleep(50 * time.Millisecond)

	msg := BroadcastMessage{
		Type: "config.updated",
		Data: map[string]interface{}{
			"section": "event_generation",
			"changes": map[string]interface{}{
				"auto_deliver":    false,
				"delivery_delay_ms": 500,
			},
		},
	}
	hub.Broadcast <- msg

	select {
	case raw := <-client.Send:
		var received BroadcastMessage
		if err := json.Unmarshal(raw, &received); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		t.Run("type is config.updated", func(t *testing.T) {
			if received.Type != "config.updated" {
				t.Errorf("expected type %q, got %q", "config.updated", received.Type)
			}
		})

		t.Run("data contains config update fields", func(t *testing.T) {
			dataMap, ok := received.Data.(map[string]interface{})
			if !ok {
				t.Fatalf("expected data to be a map, got %T", received.Data)
			}
			if dataMap["section"] != "event_generation" {
				t.Errorf("expected section %q, got %v", "event_generation", dataMap["section"])
			}
			changes, ok := dataMap["changes"].(map[string]interface{})
			if !ok {
				t.Fatalf("expected changes to be a map, got %T", dataMap["changes"])
			}
			if changes["auto_deliver"] != false {
				t.Errorf("expected auto_deliver=false, got %v", changes["auto_deliver"])
			}
		})
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for config.updated broadcast")
	}
}

// ---------------------------------------------------------------------------
// 2e. data.reset
// ---------------------------------------------------------------------------

func TestBroadcastMessage_DataReset(t *testing.T) {
	hub := startHub(t)

	client := &Client{
		Hub:  hub,
		Send: make(chan []byte, 256),
	}
	hub.Register <- client
	time.Sleep(50 * time.Millisecond)

	msg := BroadcastMessage{
		Type: "data.reset",
		Data: map[string]interface{}{
			"scope": "all",
		},
	}
	hub.Broadcast <- msg

	select {
	case raw := <-client.Send:
		var received BroadcastMessage
		if err := json.Unmarshal(raw, &received); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		t.Run("type is data.reset", func(t *testing.T) {
			if received.Type != "data.reset" {
				t.Errorf("expected type %q, got %q", "data.reset", received.Type)
			}
		})

		t.Run("data contains reset scope", func(t *testing.T) {
			dataMap, ok := received.Data.(map[string]interface{})
			if !ok {
				t.Fatalf("expected data to be a map, got %T", received.Data)
			}
			if dataMap["scope"] != "all" {
				t.Errorf("expected scope %q, got %v", "all", dataMap["scope"])
			}
		})
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for data.reset broadcast")
	}
}

// ---------------------------------------------------------------------------
// 2f. data.reset with domain-scoped reset
// ---------------------------------------------------------------------------

func TestBroadcastMessage_DataResetDomain(t *testing.T) {
	hub := startHub(t)

	client := &Client{
		Hub:  hub,
		Send: make(chan []byte, 256),
	}
	hub.Register <- client
	time.Sleep(50 * time.Millisecond)

	msg := BroadcastMessage{
		Type: "data.reset",
		Data: map[string]interface{}{
			"scope":  "domain",
			"domain": "example.com",
		},
	}
	hub.Broadcast <- msg

	select {
	case raw := <-client.Send:
		var received BroadcastMessage
		if err := json.Unmarshal(raw, &received); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		t.Run("type is data.reset", func(t *testing.T) {
			if received.Type != "data.reset" {
				t.Errorf("expected type %q, got %q", "data.reset", received.Type)
			}
		})

		t.Run("data contains domain-scoped reset", func(t *testing.T) {
			dataMap, ok := received.Data.(map[string]interface{})
			if !ok {
				t.Fatalf("expected data to be a map, got %T", received.Data)
			}
			if dataMap["scope"] != "domain" {
				t.Errorf("expected scope %q, got %v", "domain", dataMap["scope"])
			}
			if dataMap["domain"] != "example.com" {
				t.Errorf("expected domain %q, got %v", "example.com", dataMap["domain"])
			}
		})
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for data.reset domain broadcast")
	}
}

// ---------------------------------------------------------------------------
// 2g. BroadcastMessage JSON serialisation round-trip
// ---------------------------------------------------------------------------

func TestBroadcastMessage_JSONRoundTrip(t *testing.T) {
	testCases := []struct {
		name string
		msg  BroadcastMessage
	}{
		{
			name: "message.new",
			msg: BroadcastMessage{
				Type: "message.new",
				Data: map[string]interface{}{"id": "m1"},
			},
		},
		{
			name: "event.new",
			msg: BroadcastMessage{
				Type: "event.new",
				Data: map[string]interface{}{"event_type": "opened"},
			},
		},
		{
			name: "webhook.delivery",
			msg: BroadcastMessage{
				Type: "webhook.delivery",
				Data: map[string]interface{}{"status_code": float64(200)},
			},
		},
		{
			name: "config.updated",
			msg: BroadcastMessage{
				Type: "config.updated",
				Data: map[string]interface{}{"section": "storage"},
			},
		},
		{
			name: "data.reset",
			msg: BroadcastMessage{
				Type: "data.reset",
				Data: map[string]interface{}{"scope": "all"},
			},
		},
		{
			name: "nil data",
			msg: BroadcastMessage{
				Type: "data.reset",
				Data: nil,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.msg)
			if err != nil {
				t.Fatalf("failed to marshal BroadcastMessage: %v", err)
			}

			var decoded BroadcastMessage
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("failed to unmarshal BroadcastMessage: %v", err)
			}

			if decoded.Type != tc.msg.Type {
				t.Errorf("expected type %q after round-trip, got %q", tc.msg.Type, decoded.Type)
			}

			// Verify the JSON has exactly "type" and "data" keys.
			var rawMap map[string]json.RawMessage
			if err := json.Unmarshal(data, &rawMap); err != nil {
				t.Fatalf("failed to unmarshal to raw map: %v", err)
			}
			if _, ok := rawMap["type"]; !ok {
				t.Error("serialised JSON missing 'type' key")
			}
			if _, ok := rawMap["data"]; !ok {
				t.Error("serialised JSON missing 'data' key")
			}
			if len(rawMap) != 2 {
				t.Errorf("expected exactly 2 keys in serialised JSON, got %d", len(rawMap))
			}
		})
	}
}

// ===========================================================================
// 3. HTTP upgrade handler integration tests
// ===========================================================================

// ---------------------------------------------------------------------------
// 3a. WebSocket connection can be established
// ---------------------------------------------------------------------------

func TestHandleWebSocket_Connection(t *testing.T) {
	hub := startHub(t)
	_, wsURL := startTestServer(t, hub)

	conn, resp, err := gorilla.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	t.Run("connection succeeds", func(t *testing.T) {
		if conn == nil {
			t.Fatal("expected non-nil WebSocket connection")
		}
	})

	t.Run("upgrade response status is 101", func(t *testing.T) {
		if resp.StatusCode != http.StatusSwitchingProtocols {
			t.Errorf("expected status 101, got %d", resp.StatusCode)
		}
	})
}

// ---------------------------------------------------------------------------
// 3b. Client receives broadcast after connecting
// ---------------------------------------------------------------------------

func TestHandleWebSocket_ReceiveBroadcast(t *testing.T) {
	hub := startHub(t)
	_, wsURL := startTestServer(t, hub)

	conn := dialWS(t, wsURL)

	// Allow time for registration in the hub.
	time.Sleep(100 * time.Millisecond)

	// Broadcast a message.
	msg := BroadcastMessage{
		Type: "message.new",
		Data: map[string]interface{}{
			"id":      "ws-msg-001",
			"subject": "Test via WebSocket",
		},
	}
	hub.Broadcast <- msg

	// Read from the WebSocket.
	var received BroadcastMessage
	readJSON(t, conn, &received, 2*time.Second)

	t.Run("received message type matches", func(t *testing.T) {
		if received.Type != "message.new" {
			t.Errorf("expected type %q, got %q", "message.new", received.Type)
		}
	})

	t.Run("received message data is present", func(t *testing.T) {
		if received.Data == nil {
			t.Error("expected data to be non-nil")
		}
		dataMap, ok := received.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("expected data to be a map, got %T", received.Data)
		}
		if dataMap["id"] != "ws-msg-001" {
			t.Errorf("expected id %q, got %v", "ws-msg-001", dataMap["id"])
		}
	})
}

// ---------------------------------------------------------------------------
// 3c. Multiple WebSocket clients each receive broadcasts
// ---------------------------------------------------------------------------

func TestHandleWebSocket_MultipleBroadcast(t *testing.T) {
	hub := startHub(t)
	_, wsURL := startTestServer(t, hub)

	const numConns = 3
	conns := make([]*gorilla.Conn, numConns)
	for i := 0; i < numConns; i++ {
		conns[i] = dialWS(t, wsURL)
	}

	// Allow time for all registrations.
	time.Sleep(150 * time.Millisecond)

	msg := BroadcastMessage{
		Type: "event.new",
		Data: map[string]interface{}{"event_type": "accepted"},
	}
	hub.Broadcast <- msg

	var wg sync.WaitGroup
	errors := make([]string, numConns)

	for i, c := range conns {
		wg.Add(1)
		go func(idx int, conn *gorilla.Conn) {
			defer wg.Done()
			var received BroadcastMessage
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, raw, err := conn.ReadMessage()
			if err != nil {
				errors[idx] = "read error: " + err.Error()
				return
			}
			if err := json.Unmarshal(raw, &received); err != nil {
				errors[idx] = "unmarshal error: " + err.Error()
				return
			}
			if received.Type != "event.new" {
				errors[idx] = "wrong type: " + received.Type
			}
		}(i, c)
	}

	wg.Wait()

	for i, errMsg := range errors {
		if errMsg != "" {
			t.Errorf("conn[%d]: %s", i, errMsg)
		}
	}
}

// ---------------------------------------------------------------------------
// 3d. Client disconnect is cleaned up
// ---------------------------------------------------------------------------

func TestHandleWebSocket_DisconnectCleanup(t *testing.T) {
	hub := startHub(t)
	_, wsURL := startTestServer(t, hub)

	// Connect and then immediately close.
	conn := dialWS(t, wsURL)
	time.Sleep(100 * time.Millisecond)

	// Close the connection gracefully.
	err := conn.WriteMessage(
		gorilla.CloseMessage,
		gorilla.FormatCloseMessage(gorilla.CloseNormalClosure, "bye"),
	)
	if err != nil {
		t.Logf("write close message error (may be expected): %v", err)
	}
	conn.Close()

	// Allow time for the hub to process the disconnect.
	time.Sleep(200 * time.Millisecond)

	// Connect a second client to verify the hub is still functional after
	// the first client disconnected.
	conn2 := dialWS(t, wsURL)
	time.Sleep(100 * time.Millisecond)

	msg := BroadcastMessage{
		Type: "message.new",
		Data: map[string]interface{}{"id": "after-disconnect"},
	}
	hub.Broadcast <- msg

	var received BroadcastMessage
	readJSON(t, conn2, &received, 2*time.Second)

	t.Run("hub continues working after client disconnect", func(t *testing.T) {
		if received.Type != "message.new" {
			t.Errorf("expected type %q, got %q", "message.new", received.Type)
		}
	})
}

// ---------------------------------------------------------------------------
// 3e. Broadcast different message types over real WebSocket
// ---------------------------------------------------------------------------

func TestHandleWebSocket_AllMessageTypes(t *testing.T) {
	hub := startHub(t)
	_, wsURL := startTestServer(t, hub)

	conn := dialWS(t, wsURL)
	time.Sleep(100 * time.Millisecond)

	messageTypes := []string{
		"message.new",
		"event.new",
		"webhook.delivery",
		"config.updated",
		"data.reset",
	}

	for _, msgType := range messageTypes {
		t.Run(msgType, func(t *testing.T) {
			msg := BroadcastMessage{
				Type: msgType,
				Data: map[string]interface{}{"test": true},
			}
			hub.Broadcast <- msg

			var received BroadcastMessage
			readJSON(t, conn, &received, 2*time.Second)

			if received.Type != msgType {
				t.Errorf("expected type %q, got %q", msgType, received.Type)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3f. Non-WebSocket request to the endpoint
// ---------------------------------------------------------------------------

func TestHandleWebSocket_NonUpgradeRequest(t *testing.T) {
	hub := startHub(t)

	// Make a plain HTTP GET request (no WebSocket upgrade).
	req := httptest.NewRequest(http.MethodGet, "/mock/ws", nil)
	rec := httptest.NewRecorder()
	hub.HandleWebSocket(rec, req)

	t.Run("returns error for non-upgrade request", func(t *testing.T) {
		// The gorilla upgrader typically returns 400 Bad Request when the
		// required WebSocket headers are missing.
		if rec.Code == http.StatusSwitchingProtocols {
			t.Error("expected non-101 status for a plain HTTP request, got 101")
		}
	})
}

// ---------------------------------------------------------------------------
// 3g. Broadcast with nil data is handled correctly
// ---------------------------------------------------------------------------

func TestHandleWebSocket_BroadcastNilData(t *testing.T) {
	hub := startHub(t)
	_, wsURL := startTestServer(t, hub)

	conn := dialWS(t, wsURL)
	time.Sleep(100 * time.Millisecond)

	msg := BroadcastMessage{
		Type: "data.reset",
		Data: nil,
	}
	hub.Broadcast <- msg

	var received BroadcastMessage
	readJSON(t, conn, &received, 2*time.Second)

	t.Run("type is correct", func(t *testing.T) {
		if received.Type != "data.reset" {
			t.Errorf("expected type %q, got %q", "data.reset", received.Type)
		}
	})

	t.Run("data is nil", func(t *testing.T) {
		if received.Data != nil {
			t.Errorf("expected data to be nil, got %v", received.Data)
		}
	})
}

// ---------------------------------------------------------------------------
// 3h. Rapid sequential broadcasts are all received
// ---------------------------------------------------------------------------

func TestHandleWebSocket_RapidBroadcasts(t *testing.T) {
	hub := startHub(t)
	_, wsURL := startTestServer(t, hub)

	conn := dialWS(t, wsURL)
	time.Sleep(100 * time.Millisecond)

	const count = 10
	for i := 0; i < count; i++ {
		msg := BroadcastMessage{
			Type: "message.new",
			Data: map[string]interface{}{"seq": float64(i)},
		}
		hub.Broadcast <- msg
	}

	received := 0
	for i := 0; i < count; i++ {
		var msg BroadcastMessage
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read message %d: %v", i, err)
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("failed to unmarshal message %d: %v", i, err)
		}
		if msg.Type != "message.new" {
			t.Errorf("message %d: expected type %q, got %q", i, "message.new", msg.Type)
		}
		received++
	}

	t.Run("all messages received", func(t *testing.T) {
		if received != count {
			t.Errorf("expected %d messages, received %d", count, received)
		}
	})
}
