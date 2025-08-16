package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type WebSocketHandler struct {
	upgrader websocket.Upgrader
	clients  map[*websocket.Conn]bool
	broadcast chan []byte
}

type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func NewWebSocketHandler() *WebSocketHandler {
	return &WebSocketHandler{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// In production, implement proper origin checking
				return true
			},
		},
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan []byte),
	}
}

func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Register client
	h.clients[conn] = true
	log.Printf("WebSocket client connected. Total clients: %d", len(h.clients))

	// Handle messages from client
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			delete(h.clients, conn)
			break
		}
		// For now, we only broadcast server-to-client messages
		// Client messages can be handled here if needed
	}

	log.Printf("WebSocket client disconnected. Total clients: %d", len(h.clients))
}

func (h *WebSocketHandler) BroadcastInvoiceUpdate(invoiceID uint64, status string, data interface{}) {
	message := WSMessage{
		Type: "invoice_update",
		Data: map[string]interface{}{
			"invoice_id": invoiceID,
			"status":     status,
			"data":       data,
		},
	}

	h.broadcastMessage(message)
}

func (h *WebSocketHandler) BroadcastPaymentReceived(invoiceID uint64, payment interface{}) {
	message := WSMessage{
		Type: "payment_received",
		Data: map[string]interface{}{
			"invoice_id": invoiceID,
			"payment":    payment,
		},
	}

	h.broadcastMessage(message)
}

func (h *WebSocketHandler) broadcastMessage(message WSMessage) {
	// Convert message to JSON and broadcast to all clients
	// Implementation would serialize the message and send to all connected clients
	for client := range h.clients {
		err := client.WriteJSON(message)
		if err != nil {
			log.Printf("WebSocket write error: %v", err)
			client.Close()
			delete(h.clients, client)
		}
	}
}
