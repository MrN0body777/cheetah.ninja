package main

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

var roomsMutex sync.Mutex
var rooms = make(map[string]*Room)

var templates = template.Must(template.ParseFiles("templates/index.html", "templates/chat.html"))

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

type PageData struct {
	RoomID                string
	ResponsiveCSS         template.CSS
	MetaTags              template.HTML
	NeedsChromeAndroidFix bool
}

// A list of fun messages to send to spammers.
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
	"System: Fatal error: The coffee is empty. Aborting message.",
	"System: Your message has been queued behind 47 cat videos. Please wait.",
	"System: The color blue is currently offline. Please try a different color.",
	"System: Message rejected for containing too much Tuesday.",
	"System: The quantum state of your message is both sent and not sent. Please observe.",
	"System: Alert! The vibes are off. Please recalibrate and try again.",
	"System: You have exceeded the legal limit for awesome. Please slow down.",
	"System: Your typing privileges have been temporarily revoked. See you in 15 seconds.",
	"System: A 404 error occurred. Your message was not found.",
	"System: It's not a bug, it's a feature. You've found the rate-limiting feature.",
	"System: Have you tried turning it off and on again? Please wait 15 seconds to do so.",
	"System: The ninjas at cheetah.ninja have deemed your message unworthy. Try again later.",
	"System: A cheetah-ninja has intercepted your message for being too slow. Irony.",
	"System: DNS propagation for your message is taking longer than expected. Please stand by.",
	"System: Null Pointer Exception at line 'send message'. Please reboot your enthusiasm.",
}

// Word lists for generating usernames.
var adjectives = []string{
	"swift", "silent", "shadow", "stealth", "quick", "phantom", "dusk", "night",
	"golden", "wind", "sable", "solar", "crimson", "obsidian", "azure", "echo",
	"rapid", "leopard", "sand", "tempest", "invisible", "blade", "gale", "twilight",
	"zenith", "void", "cobalt", "emerald", "iron", "frost", "storm", "wild",
}

var animals = []string{
	"pounce", "claw", "dash", "ghost", "fist", "stalker", "runner", "strike",
	"jaws", "fade", "viper", "dancer", "whisper", "flare", "ninja", "hunter",
	"cat", "tiger", "panther", "cheetah", "leopard", "lynx", "jaguar", "mongoose",
	"cobra", "falcon", "eagle", "wolf", "fox", "shark", "hawk", "owl",
}

const cssFirefoxAndroid = `
body { font-size:14px; }
#msgInput { font-size:14px; padding:10px 14px; }
#sendBtn, #shareBtn { font-size:14px; padding:10px 16px; }
`

const cssChromeAndroid = `
body { font-size:16px; }
#msgInput { font-size:16px; padding:12px 16px; }
#sendBtn, #shareBtn { font-size:16px; padding:12px 20px; }
`

const cssDefault = `
body { font-size:16px; }
#msgInput { font-size:16px; padding:12px 16px; }
#sendBtn, #shareBtn { font-size:16px; padding:12px 20px; }
`

const maxMessageLength = 160
const idLength = 32

// Rate limiting constants
const minMessageInterval = 750 * time.Millisecond // Minimum time between messages.
const muteDuration = 10 * time.Second             // How long to mute a spammer.

func generateID() string {
	bytes := make([]byte, 16)
	if _, err := crand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}

// generateDisplayName creates a random username in the format "adjective_animal##".
func generateDisplayName() string {
	adj := adjectives[rand.Intn(len(adjectives))]
	animal := animals[rand.Intn(len(animals))]
	num := rand.Intn(99) + 1 // Random number from 1 to 99
	return fmt.Sprintf("%s_%s%d", adj, animal, num)
}

func isValidID(id string) bool {
	if len(id) != idLength {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func detectDeviceType(userAgent string) string {
	ua := strings.ToLower(userAgent)
	if strings.Contains(ua, "firefox") && strings.Contains(ua, "android") && strings.Contains(ua, "mobile") {
		return "firefox-android"
	}
	if strings.Contains(ua, "chrome") && strings.Contains(ua, "android") && strings.Contains(ua, "mobile") {
		return "chrome-android"
	}
	return "default"
}

func getResponsiveCSS(userAgent string) template.CSS {
	switch detectDeviceType(userAgent) {
	case "firefox-android":
		return template.CSS(cssFirefoxAndroid)
	case "chrome-android":
		return template.CSS(cssChromeAndroid)
	default:
		return template.CSS(cssDefault)
	}
}

func needsChromeAndroidFocusFix(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	return strings.Contains(ua, "chrome") && strings.Contains(ua, "android") && strings.Contains(ua, "mobile")
}

func generateMetaTags(userAgent string) template.HTML {
	ua := strings.ToLower(userAgent)

	var themeColorTag string
	if !(strings.Contains(ua, "firefox") && strings.Contains(ua, "android")) {
		themeColorTag = `<meta name="theme-color" content="#ffffff">`
	}

	var firefoxPatch string
	if strings.Contains(ua, "firefox") {
		firefoxPatch = `
        <style>
            #messages { 
                border-top: 50px solid transparent !important; 
                padding-top: 40px !important;
            }
        </style>
        `
	}

	var iOSSafeAreaPatch string
	if strings.Contains(ua, "safari") || strings.Contains(ua, "mobile") {
		iOSSafeAreaPatch = `
        <style>
            @supports (padding: max(0px)) {
                #header { top: env(safe-area-inset-top) !important; }
                #messages { padding-top: calc(90px + env(safe-area-inset-top)) !important; }
            }
        </style>
        `
	}

	metaTags := `
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no, viewport-fit=cover">
    <meta name="description" content="Simple chat application">
    <link rel="icon" href="data:;base64,=">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
    <meta name="apple-mobile-web-app-title" content="Chat App">
    <meta name="mobile-web-app-capable" content="yes">
    ` + themeColorTag + `
    ` + firefoxPatch + iOSSafeAreaPatch
	return template.HTML(metaTags)
}

func renderTemplate(w http.ResponseWriter, tmplName string, data PageData) {
	err := templates.ExecuteTemplate(w, tmplName+".html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func servePage(w http.ResponseWriter, r *http.Request) {
	userAgent := r.Header.Get("User-Agent")
	metaTags := generateMetaTags(userAgent)

	if r.URL.Path == "/" {
		if r.Method == "POST" {
			roomID := generateID()
			http.Redirect(w, r, "/"+roomID, http.StatusSeeOther)
			return
		}
		data := PageData{
			ResponsiveCSS:         getResponsiveCSS(userAgent),
			MetaTags:              metaTags,
			NeedsChromeAndroidFix: needsChromeAndroidFocusFix(userAgent),
		}
		renderTemplate(w, "index", data)
		return
	}

	roomID := strings.TrimPrefix(r.URL.Path, "/")
	if !isValidID(roomID) {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	data := PageData{
		RoomID:                roomID,
		ResponsiveCSS:         getResponsiveCSS(userAgent),
		MetaTags:              metaTags,
		NeedsChromeAndroidFix: needsChromeAndroidFocusFix(userAgent),
	}
	renderTemplate(w, "chat", data)
}

func handleWebSocket(ws *websocket.Conn) {
	roomID := ws.Request().URL.Query().Get("room")
	if !isValidID(roomID) {
		ws.Close()
		return
	}
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

		msg = strings.TrimSpace(msg)
		if len(msg) == 0 || len(msg) > maxMessageLength {
			if len(msg) > maxMessageLength {
				errorMsg := "System: Message exceeds the 160 character limit and was not sent."
				websocket.Message.Send(client.Conn, errorMsg)
			}
			continue
		}

		now := time.Now()

		if now.Before(client.mutedUntil) {
			continue
		}

		if !client.lastMessageTime.IsZero() && now.Sub(client.lastMessageTime) < minMessageInterval {
			client.mutedUntil = now.Add(muteDuration)
			funMsg := funMessages[rand.Intn(len(funMessages))]
			websocket.Message.Send(client.Conn, funMsg)
			continue
		}

		client.lastMessageTime = now

		formattedMsg := client.DisplayName + ": " + msg

		room.Mutex.Lock()
		for clientToSendTo := range room.Clients {
			websocket.Message.Send(clientToSendTo.Conn, formattedMsg)
		}
		room.Mutex.Unlock()
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())

	http.HandleFunc("/", servePage)
	http.Handle("/ws", websocket.Handler(handleWebSocket))
	http.ListenAndServe(":8080", nil)
}
