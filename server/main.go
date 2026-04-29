package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Client は接続中の子どもPC 1台を表す
type Client struct {
	name string
	conn *websocket.Conn
}

// Hub は接続中の全クライアントを管理する
type Hub struct {
	mu      sync.Mutex
	clients map[string]*Client // PC名 → Client
}

func newHub() *Hub {
	return &Hub{
		clients: make(map[string]*Client),
	}
}

func (h *Hub) register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client.name] = client
	log.Printf("[接続] %s (現在 %d 台接続中)", client.name, len(h.clients))
}

func (h *Hub) unregister(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, name)
	log.Printf("[切断] %s (現在 %d 台接続中)", name, len(h.clients))
}

var upgrader = websocket.Upgrader{
	// 開発中は全オリジンを許可（本番では絞る）
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *Hub) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket アップグレード失敗:", err)
		return
	}

	// 最初のメッセージで PC 名を受け取る
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Println("PC 名受信失敗:", err)
		conn.Close()
		return
	}
	pcName := string(msg)

	client := &Client{name: pcName, conn: conn}
	h.register(client)
	defer func() {
		h.unregister(pcName)
		conn.Close()
	}()

	// メッセージ受信ループ（今後タイマー報告などを受け取る）
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		log.Printf("[%s] %s", pcName, string(msg))
	}
}

func main() {
	hub := newHub()

	http.HandleFunc("/ws", hub.wsHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "管理サーバー 稼働中")
	})

	addr := ":8080"
	log.Println("サーバー起動:", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
