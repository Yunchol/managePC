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
//   {"type":"start",  "seconds":1800}   タイマー開始
//   {"type":"pause"}                    一時停止
//   {"type":"resume", "seconds":950}    再開（残り秒数を指定）
//
// エージェント → サーバー
//   {"type":"tick",   "remaining":1799} 毎秒の残り時間報告
//   {"type":"paused", "remaining":950}  一時停止完了の報告
//   {"type":"done"}                     タイマー終了の報告

type Message struct {
	Type      string `json:"type"`
	Seconds   int    `json:"seconds,omitempty"`   // サーバー→エージェント用
	Remaining int    `json:"remaining,omitempty"` // エージェント→サーバー用
}

// ── Client ──────────────────────────────────────────────────
// 子どもPC 1台分の接続情報
type Client struct {
	name      string
	conn      *websocket.Conn
	mu        sync.Mutex // WriteMessage を複数 goroutine から安全に呼ぶための鍵
	remaining int        // 現在の残り秒数（tick で更新される）
}

// send はスレッドセーフに WebSocket へメッセージを送る
func (c *Client) send(msg Message) error {
	data, _ := json.Marshal(msg)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// ── Hub ──────────────────────────────────────────────────────
// 全 PC の接続・一時停止データを管理する中心部
type Hub struct {
	mu        sync.Mutex
	clients   map[string]*Client // 接続中の PC
	paused    map[string]int     // 一時停止中の PC → 残り秒数
	lastReset time.Time          // 最後にリセットした日付（0時リセット判定に使う）
}

func newHub() *Hub {
	return &Hub{
		clients:   make(map[string]*Client),
		paused:    make(map[string]int),
		lastReset: time.Now(),
	}
}

// checkAndReset は日付が変わっていたら一時停止データをリセットする
// スタッフが画面を開くたびに呼ぶ（0時以降に画面を開いた瞬間にリセット）
func (h *Hub) checkAndReset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	// 日付（年・月・日）が変わっていたらリセット
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

	// 最初のメッセージで PC 名を受け取る
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

	// エージェントからのメッセージを受け取るループ
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
			// エージェントからの毎秒報告 → クライアントの残り時間を更新
			h.mu.Lock()
			client.remaining = m.Remaining
			h.mu.Unlock()

		case "done":
			// タイマー終了 → 一時停止データを消す
			log.Printf("[%s] タイマー終了", pcName)
			h.mu.Lock()
			delete(h.paused, pcName)
			client.remaining = 0
			h.mu.Unlock()

		case "paused":
			// 一時停止完了 → 残り時間を保存
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
	delete(h.paused, req.PC) // 保存データを消す（再開したので）
	h.mu.Unlock()

	log.Printf("[%s] タイマー再開: 残り %d 秒", req.PC, remaining)
	fmt.Fprintln(w, "OK")
}

// GET /api/status  → 全 PC の状態を返す
func (h *Hub) handleStatus(w http.ResponseWriter, r *http.Request) {
	h.checkAndReset() // 日付変更チェック（画面を開くたびに呼ばれる）

	h.mu.Lock()
	defer h.mu.Unlock()

	type PCStatus struct {
		Name      string `json:"name"`
		Connected bool   `json:"connected"`
		Remaining int    `json:"remaining"` // カウント中の残り秒数
		Paused    bool   `json:"paused"`
		PausedAt  int    `json:"pausedAt"` // 一時停止時の残り秒数
	}

	status := []PCStatus{}

	// 接続中の PC
	for name, c := range h.clients {
		pausedAt, paused := h.paused[name]
		status = append(status, PCStatus{
			Name:      name,
			Connected: true,
			Remaining: c.remaining,
			Paused:    paused,
			PausedAt:  pausedAt,
		})
	}

	// 一時停止中だが今は切断している PC
	for name, pausedAt := range h.paused {
		if _, connected := h.clients[name]; !connected {
			status = append(status, PCStatus{
				Name:     name,
				Paused:   true,
				PausedAt: pausedAt,
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
	http.HandleFunc("/api/status", hub.handleStatus)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "管理サーバー 稼働中")
	})

	addr := ":8080"
	log.Println("サーバー起動:", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
