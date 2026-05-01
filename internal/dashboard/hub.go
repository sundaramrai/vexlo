package dashboard

import (
	"context"
	"encoding/json"
	"sync"
)

type Event struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	Payload   any    `json:"payload"`
}

type Client struct {
	SessionID string
	Send      chan []byte
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan Event
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		clients:    map[*Client]bool{},
		broadcast:  make(chan Event, 512),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
	}
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			delete(h.clients, client)
			close(client.Send)
			h.mu.Unlock()
		case evt := <-h.broadcast:
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			h.mu.RLock()
			for client := range h.clients {
				if evt.SessionID != "" && client.SessionID != evt.SessionID {
					continue
				}
				select {
				case client.Send <- data:
				default:
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) Register(c *Client) {
	h.register <- c
}

func (h *Hub) Unregister(c *Client) {
	h.unregister <- c
}

func (h *Hub) Broadcast(evt Event) {
	h.broadcast <- evt
}
