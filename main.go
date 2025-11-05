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
	RoomID                string
	ResponsiveCSS         template.CSS
	MetaTags              template.HTML
	NeedsChromeAndroidFix bool
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

const cssDefault = `
body { font-size:16px; }
#msgInput { font-size:16px; padding:12px 16px; }
#sendBtn, #shareBtn { font-size:16px; padding:12px 20px; }
`

func generateUUID() string {
	return uuid.New().String()
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

// --- CHANGE: New backend-only fix for Firefox using border-top ---
func generateMetaTags(userAgent string) template.HTML {
	ua := strings.ToLower(userAgent)

	// Fix for Firefox moving URL bar to the bottom.
	var themeColorTag string
	if !(strings.Contains(ua, "firefox") && strings.Contains(ua, "android")) {
		themeColorTag = `<meta name="theme-color" content="#ffffff">`
	}

	// Patch for Firefox's layout issue ONLY.
	var firefoxPatch string
	if strings.Contains(ua, "firefox") {
		firefoxPatch = `
        <style>
            /* Use a transparent border-top to create space, which is more reliable than padding */
            #messages { 
                border-top: 50px solid transparent !important; 
                padding-top: 40px !important; /* Reduce the original padding to account for the new border */
            }
        </style>
        `
	}

	// Patch for iOS safe area for all browsers that support it.
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
    
    <!-- Apple iOS Meta Tags -->
    <meta name="apple-mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
    <meta name="apple-mobile-web-app-title" content="Chat App">
    
    <!-- Google Chrome Meta Tags -->
    <meta name="mobile-web-app-capable" content="yes">
    ` + themeColorTag + `
    ` + firefoxPatch + iOSSafeAreaPatch
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
	userAgent := r.Header.Get("User-Agent")
	metaTags := generateMetaTags(userAgent)

	if r.URL.Path == "/" {
		if r.Method == "POST" {
			roomID := generateUUID()
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
	if roomID == "" {
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

// --- WebSocket Logic (Unchanged) ---

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
	http.ListenAndServe(":8080", nil)
}
