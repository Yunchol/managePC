package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ── メッセージ形式 ──────────────────────────────────────────
// サーバー → エージェント
//   {"type":"start",   "seconds":1800}  タイマー開始
//   {"type":"pause"}                    一時停止
//   {"type":"resume",  "seconds":950}   再開
//   {"type":"block"}                    ブロックモード開始
//   {"type":"unblock"}                  ブロックモード解除
//
// エージェント → サーバー
//   {"type":"tick",    "remaining":1799} 毎秒の残り時間報告
//   {"type":"paused",  "remaining":950}  一時停止完了
//   {"type":"done"}                      タイマー終了

type Message struct {
	Type      string `json:"type"`
	Seconds   int    `json:"seconds,omitempty"`
	Remaining int    `json:"remaining,omitempty"`
}

// ── Client ──────────────────────────────────────────────────
type Client struct {
	name      string
	conn      *websocket.Conn
	mu        sync.Mutex
	remaining int
}

func (c *Client) send(msg Message) error {
	data, _ := json.Marshal(msg)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// ── Hub ──────────────────────────────────────────────────────
type Hub struct {
	mu        sync.Mutex
	clients   map[string]*Client
	paused    map[string]int  // 一時停止中の PC → 残り秒数
	blocked   map[string]bool // ブロック中の PC → true（再接続時にも復元される）
	lastReset time.Time
}

func newHub() *Hub {
	return &Hub{
		clients:   make(map[string]*Client),
		paused:    make(map[string]int),
		blocked:   make(map[string]bool),
		lastReset: time.Now(),
	}
}

func (h *Hub) checkAndReset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	lastY, lastM, lastD := h.lastReset.Date()
	nowY, nowM, nowD := now.Date()
	if nowY != lastY || nowM != lastM || nowD != lastD {
		h.paused = make(map[string]int)
		h.lastReset = now
		log.Println("日付変更を検知: 一時停止データをリセットしました")
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

// ── WebSocket ハンドラー ──────────────────────────────────────
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *Hub) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket アップグレード失敗:", err)
		return
	}

	_, msg, err := conn.ReadMessage()
	if err != nil {
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

	// 再接続時にブロック状態を復元する
	// （子どもが PC を再起動してもブロックが継続される）
	h.mu.Lock()
	wasBlocked := h.blocked[pcName]
	h.mu.Unlock()
	if wasBlocked {
		client.send(Message{Type: "block"})
		log.Printf("[%s] 再接続 → ブロック状態を復元", pcName)
	}

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var m Message
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}

		switch m.Type {
		case "tick":
			h.mu.Lock()
			client.remaining = m.Remaining
			h.mu.Unlock()

		case "done":
			log.Printf("[%s] タイマー終了", pcName)
			h.mu.Lock()
			delete(h.paused, pcName)
			client.remaining = 0
			h.mu.Unlock()

		case "paused":
			log.Printf("[%s] 一時停止: 残り %d 秒", pcName, m.Remaining)
			h.mu.Lock()
			h.paused[pcName] = m.Remaining
			client.remaining = 0
			h.mu.Unlock()
		}
	}
}

// ── HTTP API ─────────────────────────────────────────────────

// POST /api/timer/start  {"pc":"pc-1","minutes":30}
func (h *Hub) handleStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PC      string `json:"pc"`
		Minutes int    `json:"minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "リクエスト形式が不正です", 400)
		return
	}

	h.mu.Lock()
	client, ok := h.clients[req.PC]
	h.mu.Unlock()

	if !ok {
		http.Error(w, "PC が接続されていません", 404)
		return
	}
	if err := client.send(Message{Type: "start", Seconds: req.Minutes * 60}); err != nil {
		http.Error(w, "送信失敗", 500)
		return
	}
	log.Printf("[%s] タイマー開始: %d 分", req.PC, req.Minutes)
	fmt.Fprintln(w, "OK")
}

// POST /api/timer/pause  {"pc":"pc-1"}
func (h *Hub) handlePause(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PC string `json:"pc"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "リクエスト形式が不正です", 400)
		return
	}

	h.mu.Lock()
	client, ok := h.clients[req.PC]
	h.mu.Unlock()

	if !ok {
		http.Error(w, "PC が接続されていません", 404)
		return
	}
	client.send(Message{Type: "pause"})
	fmt.Fprintln(w, "OK")
}

// POST /api/timer/resume  {"pc":"pc-1"}
func (h *Hub) handleResume(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PC string `json:"pc"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "リクエスト形式が不正です", 400)
		return
	}

	h.mu.Lock()
	client, ok := h.clients[req.PC]
	remaining, hasPaused := h.paused[req.PC]
	h.mu.Unlock()

	if !ok {
		http.Error(w, "PC が接続されていません", 404)
		return
	}
	if !hasPaused {
		http.Error(w, "一時停止中のタイマーがありません", 404)
		return
	}

	client.send(Message{Type: "resume", Seconds: remaining})
	h.mu.Lock()
	delete(h.paused, req.PC)
	h.mu.Unlock()

	log.Printf("[%s] タイマー再開: 残り %d 秒", req.PC, remaining)
	fmt.Fprintln(w, "OK")
}

// POST /api/block/start  {"pc":"pc-1"}
func (h *Hub) handleBlockStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PC string `json:"pc"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "リクエスト形式が不正です", 400)
		return
	}

	h.mu.Lock()
	client, ok := h.clients[req.PC]
	h.blocked[req.PC] = true // 再接続時も復元されるように保存
	h.mu.Unlock()

	if !ok {
		// PC が未接続でもブロック状態は保存する（繋がったとき自動で適用される）
		fmt.Fprintln(w, "OK（PC 未接続のためブロック状態を保存しました）")
		return
	}
	client.send(Message{Type: "block"})
	log.Printf("[%s] ブロック開始", req.PC)
	fmt.Fprintln(w, "OK")
}

// POST /api/block/stop  {"pc":"pc-1"}
func (h *Hub) handleBlockStop(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PC string `json:"pc"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "リクエスト形式が不正です", 400)
		return
	}

	h.mu.Lock()
	client, ok := h.clients[req.PC]
	delete(h.blocked, req.PC) // ブロック状態を解除
	h.mu.Unlock()

	if ok {
		client.send(Message{Type: "unblock"})
	}
	log.Printf("[%s] ブロック解除", req.PC)
	fmt.Fprintln(w, "OK")
}

// GET /api/status  → 全 PC の状態を返す
func (h *Hub) handleStatus(w http.ResponseWriter, r *http.Request) {
	h.checkAndReset()

	h.mu.Lock()
	defer h.mu.Unlock()

	type PCStatus struct {
		Name      string `json:"name"`
		Connected bool   `json:"connected"`
		Remaining int    `json:"remaining"`
		Paused    bool   `json:"paused"`
		PausedAt  int    `json:"pausedAt"`
		Blocked   bool   `json:"blocked"`
	}

	status := []PCStatus{}

	for name, c := range h.clients {
		pausedAt, paused := h.paused[name]
		status = append(status, PCStatus{
			Name:      name,
			Connected: true,
			Remaining: c.remaining,
			Paused:    paused,
			PausedAt:  pausedAt,
			Blocked:   h.blocked[name],
		})
	}

	// 接続していないが一時停止中 or ブロック中の PC も表示
	seen := make(map[string]bool)
	for _, s := range status {
		seen[s.Name] = true
	}
	for name, pausedAt := range h.paused {
		if !seen[name] {
			status = append(status, PCStatus{
				Name:     name,
				PausedAt: pausedAt,
				Paused:   true,
				Blocked:  h.blocked[name],
			})
			seen[name] = true
		}
	}
	for name := range h.blocked {
		if !seen[name] {
			status = append(status, PCStatus{
				Name:    name,
				Blocked: true,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// ── main ─────────────────────────────────────────────────────
func main() {
	hub := newHub()

	http.HandleFunc("/ws", hub.wsHandler)
	http.HandleFunc("/api/timer/start", hub.handleStart)
	http.HandleFunc("/api/timer/pause", hub.handlePause)
	http.HandleFunc("/api/timer/resume", hub.handleResume)
	http.HandleFunc("/api/block/start", hub.handleBlockStart)
	http.HandleFunc("/api/block/stop", hub.handleBlockStop)
	http.HandleFunc("/api/status", hub.handleStatus)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "管理サーバー 稼働中")
	})

	addr := ":8080"
	log.Println("サーバー起動:", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
