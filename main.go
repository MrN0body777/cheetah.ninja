package main

import (
	"log"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

type Room struct {
	Clients map[*websocket.Conn]string
	Mutex   sync.Mutex
}

var rooms = make(map[string]*Room)

func main() {
	http.HandleFunc("/", serveFile)
	http.HandleFunc("/ws", handleWebSocket)
	log.Println("Server starting on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func serveFile(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.ServeFile(w, r, "static/index.html")
	} else {
		// Always serve chat page for any UUID path
		http.ServeFile(w, r, "static/chat.html")
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("room")
	userID := uuid.New().String()
	conn, _ := upgrader.Upgrade(w, r, nil)

	roomsMutex.Lock()
	room, exists := rooms[roomID]
	if !exists {
		// Recreate room if someone joins with an old link
		room = &Room{Clients: make(map[*websocket.Conn]string)}
		rooms[roomID] = room
	}
	roomsMutex.Unlock()

	room.Mutex.Lock()
	room.Clients[conn] = userID
	room.Mutex.Unlock()

	defer func() {
		room.Mutex.Lock()
		delete(room.Clients, conn)
		if len(room.Clients) == 0 {
			roomsMutex.Lock()
			delete(rooms, roomID)
			roomsMutex.Unlock()
		}
		room.Mutex.Unlock()
		conn.Close()
	}()

	for {
		_, message, _ := conn.ReadMessage()
		room.Mutex.Lock()
		for client := range room.Clients {
			if client != conn {
				client.WriteMessage(websocket.TextMessage, message)
			}
		}
		room.Mutex.Unlock()
	}
}

var roomsMutex sync.Mutex
