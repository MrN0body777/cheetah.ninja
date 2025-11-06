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

var roomsMutex sync.Mutex
var rooms = make(map[string]*Room)

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

func generateID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
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

func handleWebSocket(ws *websocket.Conn) {
	roomID := ws.Request().URL.Query().Get("room")
	if roomID == "" {
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

		if len(msg) > maxMessageLength {
			errorMsg := "System: Message exceeds the 160 character limit and was not sent."
			websocket.Message.Send(client.Conn, errorMsg)
			continue
		}

		formattedMsg := userID + ": " + msg

		room.Mutex.Lock()
		for clientToSendTo := range room.Clients {
			websocket.Message.Send(clientToSendTo.Conn, formattedMsg)
		}
		room.Mutex.Unlock()
	}
}

func main() {
	http.HandleFunc("/", servePage)
	http.Handle("/ws", websocket.Handler(handleWebSocket))
	http.ListenAndServe(":8080", nil)
}
