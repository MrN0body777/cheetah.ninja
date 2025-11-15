package main

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/securecookie"
	"golang.org/x/net/websocket"
)

var hashKey = []byte("super-secret-hash-key-32-bytes-long")
var blockKey = []byte("32-byte-long-encryption-key-1234")
var scookie = securecookie.New(hashKey, blockKey)

var roomsMutex sync.Mutex
var rooms = make(map[string]*Room)

var indexTemplate = template.Must(template.ParseFiles("templates/index.html"))
var chatTemplate = template.Must(template.ParseFiles("templates/chat.html"))

type Room struct {
	Clients map[*Client]bool
	Mutex   sync.Mutex
}

type Client struct {
	Conn        *websocket.Conn
	UserID      string
	DisplayName string
}

type ChatPageData struct {
	RoomID        string
	MetaTags      template.HTML
	ResponsiveCSS template.CSS
	WSUrl         string
}

const maxMessageLength = 160

var adjectives = []string{
	"silent", "shadow", "stealth", "phantom", "dusk", "night", "sable", "obsidian",
	"invisible", "twilight", "void", "iron", "frost", "swift", "quick", "wind", "echo",
	"crimson", "cobalt", "storm", "wild", "tempest", "gale", "blade", "dark", "hazy",
	"wraith", "cryptic", "veiled", "covert", "rapid", "sudden", "blur", "flash",
	"kinetic", "agile", "nimble", "sonic", "hyper",
}
var animals = []string{
	"cat", "tiger", "panther", "cheetah", "leopard", "lynx", "jaguar", "wolf", "fox",
	"cobra", "viper", "mongoose", "falcon", "hawk", "eagle", "owl", "shark", "stalker",
	"hunter", "ninja", "ghost", "wraith", "shade", "specter", "prowler", "shadow",
	"blade", "strike", "dash", "pounce", "claw", "fist", "bolt",
}

func generateID() string {
	b := make([]byte, 16)
	if _, err := crand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func generateDisplayName() string {
	adj := adjectives[rand.Intn(len(adjectives))]
	animal := animals[rand.Intn(len(animals))]
	num := rand.Intn(99) + 1
	return fmt.Sprintf("%s_%s%d", adj, animal, num)
}

func servePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" && r.Method == http.MethodPost {
		roomID := generateID()
		http.Redirect(w, r, "/"+roomID, http.StatusSeeOther)
		return
	}

	if r.URL.Path == "/" && r.Method == http.MethodGet {
		err := indexTemplate.Execute(w, nil)
		if err != nil {
			log.Printf("Error executing index template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	roomID := strings.TrimPrefix(r.URL.Path, "/")
	if roomID == "" {
		http.NotFound(w, r)
		return
	}

	metaTags := `<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0, shrink-to-fit=no, viewport-fit=cover">
<title>Chat Room ` + template.HTMLEscapeString(roomID) + `</title>`

	responsiveCSS := `html { 
    height:var(--app-height,100vh); 
    -webkit-text-size-adjust:100%; 
    -ms-text-size-adjust:100%; 
    text-size-adjust:100%; 
    overflow:hidden; 
}
body { 
    height:100%; 
    padding-top:env(safe-area-inset-top); 
    padding-bottom:env(safe-area-inset-bottom); 
}
#messages { 
    padding-top: calc(84px + 12px + env(safe-area-inset-top,0)); 
}`

	wsProtocol := "ws"
	if r.TLS != nil {
		wsProtocol = "wss"
	}
	wsUrl := fmt.Sprintf("%s://%s/ws?room=%s", wsProtocol, r.Host, roomID)

	data := ChatPageData{
		RoomID:        roomID,
		MetaTags:      template.HTML(metaTags),
		ResponsiveCSS: template.CSS(responsiveCSS),
		WSUrl:         wsUrl,
	}

	err := chatTemplate.Execute(w, data)
	if err != nil {
		log.Printf("Error executing chat template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func wsAuthWrapper(h websocket.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var displayName string
		cookie, err := r.Cookie("chatDisplayName")

		if err == nil {
			if err = scookie.Decode("chatDisplayName", cookie.Value, &displayName); err != nil {
				log.Printf("Error decoding cookie, possibly tampered: %v", err)
				displayName = ""
			}
		}

		if displayName == "" {
			displayName = generateDisplayName()
			encoded, err := scookie.Encode("chatDisplayName", displayName)
			if err != nil {
				log.Printf("Error encoding cookie: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     "chatDisplayName",
				Value:    encoded,
				Path:     "/",
				MaxAge:   86400 * 30,
				HttpOnly: true,
				Secure:   r.TLS != nil,
				SameSite: http.SameSiteLaxMode,
			})
		}

		q := r.URL.Query()
		q.Set("name", displayName)
		r.URL.RawQuery = q.Encode()

		h.ServeHTTP(w, r)
	}
}

func handleWebSocket(ws *websocket.Conn) {
	roomID := ws.Request().URL.Query().Get("room")
	userID := generateID()
	displayName := ws.Request().URL.Query().Get("name")

	if displayName == "" {
		log.Printf("Error: WebSocket connection without a display name.")
		ws.Close()
		return
	}

	roomsMutex.Lock()
	room, exists := rooms[roomID]
	if !exists {
		room = &Room{Clients: make(map[*Client]bool)}
		rooms[roomID] = room
	}
	client := &Client{
		Conn:        ws,
		UserID:      userID,
		DisplayName: displayName,
	}
	room.Clients[client] = true
	roomsMutex.Unlock()

	defer func() {
		room.Mutex.Lock()
		delete(room.Clients, client)
		isEmpty := len(room.Clients) == 0
		room.Mutex.Unlock()

		if isEmpty {
			roomsMutex.Lock()
			delete(rooms, roomID)
			roomsMutex.Unlock()
		}
		ws.Close()
	}()

	pingTicker := time.NewTicker(60 * time.Second)
	defer pingTicker.Stop()
	go func() {
		for range pingTicker.C {
			if err := websocket.Message.Send(ws, ""); err != nil {
				ws.Close()
				return
			}
		}
	}()

	for {
		var msg string
		err := websocket.Message.Receive(ws, &msg)
		if err != nil {
			break
		}

		msg = strings.TrimSpace(msg)
		if len(msg) == 0 {
			continue
		}

		if msg == "CHECKMARK_CLICKED" {
			continue
		}

		if len(msg) > maxMessageLength {
			errorMsg := "System: Message exceeds 160 character limit and was not sent."
			websocket.Message.Send(client.Conn, errorMsg)
			continue
		}

		formattedMsg := client.DisplayName + ": " + msg

		room.Mutex.Lock()
		for clientConn := range room.Clients {
			if err := websocket.Message.Send(clientConn.Conn, formattedMsg); err != nil {
				log.Printf("Error sending message to client %s: %v", clientConn.UserID, err)
			}
		}
		room.Mutex.Unlock()
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())
	mux := http.NewServeMux()
	mux.HandleFunc("/", servePage)
	mux.Handle("/ws", wsAuthWrapper(websocket.Handler(handleWebSocket)))

	fmt.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
