package main

import (
	"bufio"
	"compress/gzip"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/securecookie"
	"golang.org/x/net/websocket"
)

var scookie *securecookie.SecureCookie
var jwtSecret []byte

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

type CustomClaims struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	RoomID      string `json:"room_id"`
	jwt.RegisteredClaims
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

type brotliResponseWriter struct {
	http.ResponseWriter
	writer *brotli.Writer
}

func (w *brotliResponseWriter) Write(b []byte) (int, error) {
	return w.writer.Write(b)
}

func (w *brotliResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("hijacking not supported")
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer *gzip.Writer
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.writer.Write(b)
}

func (w *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("hijacking not supported")
}

func compressionHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acceptEncoding := r.Header.Get("Accept-Encoding")

		switch {
		case strings.Contains(acceptEncoding, "br"):
			w.Header().Set("Content-Encoding", "br")
			w.Header().Set("Vary", "Accept-Encoding")
			br := brotli.NewWriter(w)
			defer br.Close()
			next.ServeHTTP(&brotliResponseWriter{ResponseWriter: w, writer: br}, r)

		case strings.Contains(acceptEncoding, "gzip"):
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Vary", "Accept-Encoding")
			gz := gzip.NewWriter(w)
			defer gz.Close()
			next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, writer: gz}, r)

		default:
			next.ServeHTTP(w, r)
		}
	})
}

func init() {
	hashKeyStr := os.Getenv("HASH_KEY")
	blockKeyStr := os.Getenv("BLOCK_KEY")
	jwtSecretStr := os.Getenv("JWT_SECRET")

	if hashKeyStr == "" || blockKeyStr == "" || jwtSecretStr == "" {
		log.Fatal("FATAL: HASH_KEY, BLOCK_KEY, and JWT_SECRET environment variables must be set.")
	}

	scookie = securecookie.New([]byte(hashKeyStr), []byte(blockKeyStr))
	jwtSecret = []byte(jwtSecretStr)
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

func generateJWT(userID, displayName, roomID string) (string, error) {
	claims := CustomClaims{
		UserID:      userID,
		DisplayName: displayName,
		RoomID:      roomID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func servePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" && r.Method == http.MethodPost {
		roomID := generateID()
		http.Redirect(w, r, "/"+roomID, http.StatusSeeOther)
		return
	}

	if r.URL.Path == "/" && r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8") // <-- FIX: Set header for index page
		err := indexTemplate.Execute(w, nil)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	roomID := strings.TrimPrefix(r.URL.Path, "/")
	if roomID == "" {
		http.NotFound(w, r)
		return
	}

	var userID string
	userIDCookie, err := r.Cookie("room_user_id_" + roomID)
	if err == nil {
		userID = userIDCookie.Value
	}

	if userID == "" {
		userID = generateID()
		http.SetCookie(w, &http.Cookie{
			Name:     "room_user_id_" + roomID,
			Value:    userID,
			Path:     "/",
			MaxAge:   86400 * 30,
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
		})
	}

	var displayName string
	cookie, err := r.Cookie("chatDisplayName")
	if err == nil {
		_ = scookie.Decode("chatDisplayName", cookie.Value, &displayName)
	}

	if displayName == "" {
		displayName = generateDisplayName()
		encoded, err := scookie.Encode("chatDisplayName", displayName)
		if err == nil {
			http.SetCookie(w, &http.Cookie{
				Name:     "chatDisplayName",
				Value:    encoded,
				Path:     "/",
				MaxAge:   86400 * 30,
				HttpOnly: true,
				Secure:   r.TLS != nil,
				SameSite: http.SameSiteLaxMode,
			})
		} else {
			log.Printf("Error encoding display name cookie: %v", err)
		}
	}

	tokenString, err := generateJWT(userID, displayName, roomID)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    tokenString,
		Path:     "/",
		MaxAge:   86400 * 1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

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
    padding-top: calc(84px + 12px + env(safe-area-inset-top,0px)); 
}`

	wsProtocol := "ws"
	if r.Header.Get("X-Forwarded-Proto") == "https" {
		wsProtocol = "wss"
	}

	wsUrl := fmt.Sprintf("%s://%s/ws?room=%s", wsProtocol, r.Host, roomID)

	data := ChatPageData{
		RoomID:        roomID,
		MetaTags:      template.HTML(metaTags),
		ResponsiveCSS: template.CSS(responsiveCSS),
		WSUrl:         wsUrl,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8") // <-- FIX: Set header for chat page
	err = chatTemplate.Execute(w, data)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func handleWebSocket(ws *websocket.Conn) {
	cookie, err := ws.Request().Cookie("auth_token")
	if err != nil {
		ws.Close()
		return
	}

	token, err := jwt.ParseWithClaims(cookie.Value, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		ws.Close()
		return
	}

	claims, ok := token.Claims.(*CustomClaims)
	if !ok {
		ws.Close()
		return
	}

	roomID := claims.RoomID
	userID := claims.UserID
	displayName := claims.DisplayName

	client := &Client{
		Conn:        ws,
		UserID:      userID,
		DisplayName: displayName,
	}

	roomsMutex.Lock()
	room, exists := rooms[roomID]
	if !exists {
		room = &Room{Clients: make(map[*Client]bool)}
		rooms[roomID] = room
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
		if len(msg) == 0 || msg == "CHECKMARK_CLICKED" {
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
				log.Printf("Error sending message to client %s: %v", clientConn.DisplayName, err)
			}
		}
		room.Mutex.Unlock()
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", servePage)
	mux.Handle("/ws", websocket.Handler(handleWebSocket))

	finalHandler := compressionHandler(mux)

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, finalHandler))
}
