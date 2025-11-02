package main

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/net/websocket"
)

// --- Backend Data & Logic ---

var roomsMutex sync.Mutex
var rooms = make(map[string]*Room)

type Room struct {
	Clients map[*Client]bool
	Mutex   sync.Mutex
}

type Client struct {
	Conn   *websocket.Conn
	UserID string
}

// PageData is the structure for data injected into our HTML templates.
type PageData struct {
	RoomID        string
	ResponsiveCSS template.CSS
}

// CSS blocks are now constants in the backend.
const cssSmall = `
@media (max-width: 389px) {
    #header, #messages, #inputArea { padding: 8px; }
    #messages, #msgInput, #charCount, #sendBtn, #shareBtn { font-size: 14px; }
    #inputArea { gap: 8px; }
    #send-controls { gap: 6px; }
    #msgInput { padding: 8px 12px; }
    #sendBtn, #shareBtn { padding: 8px 16px; }
    .msg { margin-bottom: 4px; }
    button { padding:10px 16px; font-size:14px; }
}
`

const cssMedium = `
@media (min-width: 390px) and (max-width: 429px) {
    #header, #messages, #inputArea { padding: 12px; }
    #messages, #msgInput, #charCount, #sendBtn, #shareBtn { font-size: 16px; }
    #inputArea { gap: 12px; }
    #send-controls { gap: 8px; }
    #msgInput { padding: 12px 16px; }
    #sendBtn, #shareBtn { padding: 12px 20px; }
    .msg { margin-bottom: 6px; }
    button { padding:12px 20px; font-size:16px; }
}
`

const cssLarge = `
@media (min-width: 430px) {
    #header, #messages, #inputArea { padding: 14px; }
    #messages, #msgInput, #charCount, #sendBtn, #shareBtn { font-size: 17px; }
    #inputArea { gap: 14px; }
    #send-controls { gap: 10px; }
    #msgInput { padding: 14px 18px; }
    #sendBtn, #shareBtn { padding: 14px 24px; }
    .msg { margin-bottom: 8px; }
    button { padding:14px 24px; font-size:18px; }
}
`

// --- Backend Functions ---

func generateShortID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func detectDeviceType(userAgent string) string {
	ua := strings.ToLower(userAgent)
	if strings.Contains(ua, "iphone se") {
		return "small"
	}
	if strings.Contains(ua, "pro max") || strings.Contains(ua, "plus") {
		return "large"
	}
	if strings.Contains(ua, "iphone") {
		return "medium"
	}
	return "large"
}

func getResponsiveCSS(userAgent string) template.CSS {
	switch detectDeviceType(userAgent) {
	case "small":
		return template.CSS(cssSmall)
	case "medium":
		return template.CSS(cssMedium)
	default:
		return template.CSS(cssLarge)
	}
}

// --- HTTP Handlers ---

func renderTemplate(w http.ResponseWriter, tmplName string, data PageData) {
	tmpl, err := template.ParseFiles("templates/" + tmplName + ".html")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		ResponsiveCSS: getResponsiveCSS(r.Header.Get("User-Agent")),
	}
	renderTemplate(w, "index", data)
}

func serveChat(w http.ResponseWriter, r *http.Request) {
	roomID := strings.TrimPrefix(r.URL.Path, "/")
	if roomID == "" {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	data := PageData{
		RoomID:        roomID,
		ResponsiveCSS: getResponsiveCSS(r.Header.Get("User-Agent")),
	}
	renderTemplate(w, "chat", data)
}

// --- WebSocket Logic ---

func handleWebSocket(ws *websocket.Conn) {
	roomID := ws.Request().URL.Query().Get("room")
	if roomID == "" {
		ws.Close()
		return
	}
	userID := generateShortID()

	roomsMutex.Lock()
	room, exists := rooms[roomID]
	if !exists {
		room = &Room{Clients: make(map[*Client]bool)}
		rooms[roomID] = room
	}
	client := &Client{Conn: ws, UserID: userID}
	room.Clients[client] = true
	roomsMutex.Unlock()

	defer func() {
		room.Mutex.Lock()
		delete(room.Clients, client)
		if len(room.Clients) == 0 {
			roomsMutex.Lock()
			delete(rooms, roomID)
			roomsMutex.Unlock()
		}
		room.Mutex.Unlock()
		ws.Close()
	}()

	for {
		var msg string
		err := websocket.Message.Receive(ws, &msg)
		if err != nil {
			break
		}
		formattedMsg := userID + ": " + msg

		room.Mutex.Lock()
		// --- FIX IS HERE ---
		// The message is now sent to ALL clients, including the sender.
		for client := range room.Clients {
			websocket.Message.Send(client.Conn, formattedMsg)
		}
		room.Mutex.Unlock()
	}
}

// --- Main Function ---

func main() {
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/chat/", serveChat)
	http.Handle("/ws", websocket.Handler(handleWebSocket))

	http.ListenAndServe(":8080", nil)
}
