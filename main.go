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

	"golang.org/x/net/websocket"
)

var roomsMutex sync.Mutex
var rooms = make(map[string]*Room)

var indexTemplate = template.Must(template.ParseFiles("templates/index.html"))
var chatTemplate = template.Must(template.ParseFiles("templates/chat.html"))

type Room struct {
	Clients map[*Client]bool
	Mutex   sync.Mutex
}

type Client struct {
	Conn            *websocket.Conn
	UserID          string
	DisplayName     string
	lastMessageTime time.Time
	mutedUntil      time.Time
}

type ChatPageData struct {
	RoomID        string
	MetaTags      template.HTML
	ResponsiveCSS template.CSS
}

const maxMessageLength = 160
const minMessageInterval = 750 * time.Millisecond
const muteDuration = 10 * time.Second

var funMessages = []string{
	"System: Whoa there, speedy! The hamster powering the server needs a break.",
	"System: A wild penguin has appeared and stolen your message.",
	"System: Error: Boop not found. Please try again later.",
	"System: You have been placed in a digital timeout. Think about what you've done.",
	"System: The squirrels are on fire again. Please wait for them to be extinguished.",
	"System: You're typing so fast the keyboard is getting dizzy.",
	"System: My circuits are overheating! Please slow down.",
	"System: A pack of capybaras has formed a protective circle around the server. Please wait.",
	"System: Your message was intercepted by a flock of geese. They were not impressed.",
	"System: The server's cat is walking on the keyboard again. Please stand by.",
	"System: A sloth is delivering your message. It will arrive... eventually.",
	"System: The bit bucket is full. Please try again after we empty it.",
	"System: A packet gremlin has misplaced your data. We're negotiating with it now.",
	"System: Your message has been queued behind 47 cat videos. Please wait.",
	"System: The color blue is currently offline. Please try a different color.",
	"System: Message rejected for containing too much Tuesday.",
	"System: The quantum state of your message is both sent and not sent. Please observe.",
	"System: Alert! The vibes are off. Please recalibrate and try again.",
	"System: You have exceeded the legal limit for awesome. Please slow down.",
	"System: A 404 error occurred. Your message was not found.",
	"System: It's not a bug, it's a feature. You've found the rate-limiting feature.",
	"System: Have you tried turning it off and on again? Please wait 10 seconds to do so.",
	"System: A cheetah-ninja has intercepted your message for being too slow. Irony.",
	"System: DNS propagation for your message is taking longer than expected. Please stand by.",
	"System: Null Pointer Exception at line 'send message'. Please reboot your enthusiasm.",
}

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
<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, shrink-to-fit=no, viewport-fit=cover">
<meta name="description" content="Simple chat application">
<meta name="theme-color" content="#ffffff">
<link rel="icon" href="data:;base64,=">
<title>Chat Room ` + template.HTMLEscapeString(roomID) + `</title>`

	responsiveCSS := `html { 
    margin:0; 
    padding:0; 
    width:100%; 
    -webkit-text-size-adjust:100%; 
    -ms-text-size-adjust:100%; 
    text-size-adjust:100%; 
    height:var(--app-height,100vh); 
    overflow:hidden; 
}
body { 
    margin:0; 
    padding:0; 
    width:100%; 
    font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; 
    height:100%; 
    display:flex; 
    flex-direction:column; 
    padding-top:env(safe-area-inset-top); 
    padding-bottom:env(safe-area-inset-bottom); 
}
#messages { 
    padding-top: calc(84px + 12px + env(safe-area-inset-top,0)); 
}`

	data := ChatPageData{
		RoomID:        roomID,
		MetaTags:      template.HTML(metaTags),
		ResponsiveCSS: template.CSS(responsiveCSS),
	}

	err := chatTemplate.Execute(w, data)
	if err != nil {
		log.Printf("Error executing chat template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func handleWebSocket(ws *websocket.Conn) {
	roomID := ws.Request().URL.Query().Get("room")

	userID := generateID()
	roomsMutex.Lock()
	room, exists := rooms[roomID]
	if !exists {
		room = &Room{Clients: make(map[*Client]bool)}
		rooms[roomID] = room
	}
	client := &Client{
		Conn:        ws,
		UserID:      userID,
		DisplayName: generateDisplayName(),
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

		now := time.Now()
		if now.Before(client.mutedUntil) {
			continue
		}

		if len(msg) > maxMessageLength {
			errorMsg := "System: Message exceeds 160 character limit and was not sent."
			websocket.Message.Send(client.Conn, errorMsg)
			continue
		}

		if !client.lastMessageTime.IsZero() && now.Sub(client.lastMessageTime) < minMessageInterval {
			client.mutedUntil = now.Add(muteDuration)
			randMessage := funMessages[rand.Intn(len(funMessages))]
			websocket.Message.Send(client.Conn, randMessage)
			continue
		}

		client.lastMessageTime = now
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
	mux.Handle("/ws", websocket.Handler(handleWebSocket))

	fmt.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
