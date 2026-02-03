package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WebSocketMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type Client struct {
	conn   *websocket.Conn
	send   chan WebSocketMessage
	chatID int64
	userID string
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan WebSocketMessage
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

var hub = Hub{
	clients:    make(map[*Client]bool),
	broadcast:  make(chan WebSocketMessage, 256),
	register:   make(chan *Client),
	unregister: make(chan *Client),
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

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func init() {
	go hub.run()
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}

	client := &Client{
		conn:   conn,
		send:   make(chan WebSocketMessage, 256),
		chatID: -1,
		userID: "anonymous",
	}

	hub.register <- client

	go client.writePump()
	client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512 * 1024)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Println("WebSocket read error:", err)
			}
			break
		}

		var msg WebSocketMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "join_chat":
			if payload, ok := msg.Payload.(map[string]interface{}); ok {
				if chatID, ok := payload["chat_id"].(float64); ok {
					c.chatID = int64(chatID)
				}
			}
		case "leave_chat":
			c.chatID = -1
		case "typing":
			if c.chatID > 0 {
				hub.broadcast <- WebSocketMessage{
					Type: "user_typing",
					Payload: map[string]interface{}{
						"chat_id": c.chatID,
						"user_id": c.userID,
					},
				}
			}
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			data, _ := json.Marshal(message)
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
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

func BroadcastChatUpdate(chatID int64, updateType string, data interface{}) {
	message := WebSocketMessage{
		Type: updateType,
		Payload: map[string]interface{}{
			"chat_id": chatID,
			"data":    data,
		},
	}
	hub.broadcast <- message
}

func BroadcastMessage(chatID int64, message interface{}) {
	hub.broadcast <- WebSocketMessage{
		Type:    "new_message",
		Payload: message,
	}
}

type WSMiddleware struct {
	next http.Handler
}

func (w *WSMiddleware) ServeHTTP(wr http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/ws" {
		websocketHandler(wr, r)
		return
	}
	w.next.ServeHTTP(wr, r)
}

func WSNotify(messageType string, payload interface{}) {
	select {
	case hub.broadcast <- WebSocketMessage{Type: messageType, Payload: payload}:
	default:
	}
}

func WSIsConnected() int {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return len(hub.clients)
}
