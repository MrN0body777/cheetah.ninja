package main

import (
	"html/template"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/net/websocket"
)

var roomsMutex sync.Mutex
var rooms = make(map[string]*Room)

// Load templates from the templates folder
var templates = template.Must(template.ParseFiles("templates/index.html", "templates/chat.html"))

type Room struct {
	Clients map[*Client]bool
	Mutex   sync.Mutex
}

type Client struct {
	Conn   *websocket.Conn
	UserID string
}

type PageData struct {
	RoomID          string
	ResponsiveCSS   template.CSS
	IsChromeAndroid bool
	MetaTags        template.HTML
}

// Device-specific CSS constants
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

const cssSmall = `
body { font-size:14px; }
#msgInput { font-size:14px; padding:10px 14px; }
#sendBtn, #shareBtn { font-size:14px; padding:10px 16px; }
`

const cssMedium = `
body { font-size:16px; }
#msgInput { font-size:16px; padding:12px 16px; }
#sendBtn, #shareBtn { font-size:16px; padding:12px 20px; }
`

const cssLarge = `
body { font-size:18px; }
#msgInput { font-size:18px; padding:14px 18px; }
#sendBtn, #shareBtn { font-size:18px; padding:14px 22px; }
`

func generateUUID() string {
	return uuid.New().String()
}

func detectDeviceType(userAgent string) string {
	ua := strings.ToLower(userAgent)
	// Check for Firefox on Android first
	if strings.Contains(ua, "firefox") && strings.Contains(ua, "android") && strings.Contains(ua, "mobile") {
		return "firefox-android"
	}
	// Then check for Chrome on Android
	if strings.Contains(ua, "chrome") && strings.Contains(ua, "android") && strings.Contains(ua, "mobile") {
		return "chrome-android"
	}
	// Then check for iOS devices
	if strings.Contains(ua, "iphone se") {
		return "small"
	}
	if strings.Contains(ua, "pro max") || strings.Contains(ua, "plus") {
		return "large"
	}
	if strings.Contains(ua, "iphone") {
		return "medium"
	}
	// Default to large for desktops/tablets
	return "large"
}

func getResponsiveCSS(userAgent string) template.CSS {
	switch detectDeviceType(userAgent) {
	case "firefox-android":
		return template.CSS(cssFirefoxAndroid)
	case "chrome-android":
		return template.CSS(cssChromeAndroid)
	case "small":
		return template.CSS(cssSmall)
	case "medium":
		return template.CSS(cssMedium)
	default:
		return template.CSS(cssLarge)
	}
}

func generateMetaTags() template.HTML {
	metaTags := `
    <!-- Basic Meta Tags -->
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <meta name="description" content="Simple chat application">
    <meta name="author" content="Chat App">
    
    <!-- Empty Favicon to prevent browser requests -->
    <link rel="icon" href="data:;base64,=">
    
    <!-- Apple iOS Meta Tags -->
    <meta name="apple-mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
    <meta name="apple-mobile-web-app-title" content="Chat App">
    <meta name="format-detection" content="telephone=no">
    
    <!-- Google Chrome Meta Tags -->
    <meta name="mobile-web-app-capable" content="yes">
    <meta name="theme-color" content="#ffffff">
    <meta name="application-name" content="Chat App">
    
    <!-- Microsoft Edge/IE Meta Tags -->
    <meta name="msapplication-TileColor" content="#ffffff">
    <meta name="msapplication-config" content="none">
    <meta name="msapplication-starturl" content="/">
    
    <!-- Firefox Meta Tags -->
    <meta name="theme-color" content="#ffffff">
    <meta name="browsermode" content="application">
    
    <!-- Opera Meta Tags -->
    <meta name="theme-color" content="#ffffff">
    
    <!-- Security Meta Tags -->
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="referrer" content="no-referrer">
        
    <!-- Open Graph Meta Tags -->
    <meta property="og:type" content="website">
    <meta property="og:title" content="Chat App">
    <meta property="og:description" content="Simple chat application">
    <meta property="og:site_name" content="Chat App">
    `
	return template.HTML(metaTags)
}

// --- HTTP Handlers ---

func renderTemplate(w http.ResponseWriter, tmplName string, data PageData) {
	err := templates.ExecuteTemplate(w, tmplName+".html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func servePage(w http.ResponseWriter, r *http.Request) {
	// Check for Chrome Android first to set the flag correctly
	isChromeAndroid := detectDeviceType(r.Header.Get("User-Agent")) == "chrome-android"

	// Generate meta tags
	metaTags := generateMetaTags()

	if r.URL.Path == "/" {
		if r.Method == "POST" {
			// Generate a new room ID and redirect - moved from frontend
			roomID := generateUUID()
			http.Redirect(w, r, "/"+roomID, http.StatusSeeOther)
			return
		}
		// For GET request, serve the landing page
		data := PageData{
			ResponsiveCSS:   getResponsiveCSS(r.Header.Get("User-Agent")),
			IsChromeAndroid: isChromeAndroid,
			MetaTags:        metaTags,
		}
		renderTemplate(w, "index", data)
		return
	}

	roomID := strings.TrimPrefix(r.URL.Path, "/")
	if roomID == "" {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	data := PageData{
		RoomID:          roomID,
		ResponsiveCSS:   getResponsiveCSS(r.Header.Get("User-Agent")),
		IsChromeAndroid: isChromeAndroid,
		MetaTags:        metaTags,
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
	userID := generateUUID()

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
		for client := range room.Clients {
			websocket.Message.Send(client.Conn, formattedMsg)
		}
		room.Mutex.Unlock()
	}
}

// --- Main Function ---

func main() {
	http.HandleFunc("/", servePage)
	http.Handle("/ws", websocket.Handler(handleWebSocket))

	// Serve static files if needed
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	http.ListenAndServe(":8080", nil)
}
