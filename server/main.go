package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	qrcode "github.com/skip2/go-qrcode"
)

// ui/index.html をバイナリに埋め込む（配布時に別ファイル不要）
//
//go:embed ui/index.html
var indexHTML []byte

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

// timerRecord はタイマーの開始情報を記録する
// 再接続時に「残り時間 = 合計 - 経過時間」で復元するために使う
type timerRecord struct {
	startedAt time.Time // タイマーを開始した時刻
	total     int       // 合計秒数
}

// ── Hub ──────────────────────────────────────────────────────
type Hub struct {
	mu        sync.Mutex
	clients   map[string]*Client
	paused    map[string]int        // 一時停止中の PC → 残り秒数
	blocked   map[string]bool       // ブロック中の PC → true（再接続時にも復元される）
	timers    map[string]*timerRecord // 稼働中タイマーの記録（再接続時の復元用）
	lastReset time.Time
}

func newHub() *Hub {
	return &Hub{
		clients:   make(map[string]*Client),
		paused:    make(map[string]int),
		blocked:   make(map[string]bool),
		timers:    make(map[string]*timerRecord),
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

	h.mu.Lock()
	wasBlocked := h.blocked[pcName]
	timer := h.timers[pcName]
	h.mu.Unlock()

	// 再接続時: 稼働中タイマーがあれば残り時間を計算して自動再開
	if timer != nil {
		elapsed := int(time.Since(timer.startedAt).Seconds())
		remaining := timer.total - elapsed
		if remaining > 0 {
			client.send(Message{Type: "start", Seconds: remaining})
			log.Printf("[%s] 再接続 → タイマー継続: 残り %d 秒", pcName, remaining)
		} else {
			// タイマーが切れていた → 終了扱い
			h.mu.Lock()
			delete(h.timers, pcName)
			h.mu.Unlock()
			log.Printf("[%s] 再接続 → タイマー切れ（切断中に終了）", pcName)
		}
	}

	// 再接続時: ブロック状態を復元（子どもが再起動してもブロック継続）
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
			delete(h.timers, pcName) // タイマー記録も削除
			client.remaining = 0
			h.mu.Unlock()

		case "paused":
			log.Printf("[%s] 一時停止: 残り %d 秒", pcName, m.Remaining)
			h.mu.Lock()
			h.paused[pcName] = m.Remaining
			delete(h.timers, pcName) // 一時停止中はタイマー記録を消す（再開時に新たに記録）
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
	seconds := req.Minutes * 60
	if err := client.send(Message{Type: "start", Seconds: seconds}); err != nil {
		http.Error(w, "送信失敗", 500)
		return
	}
	// タイマー記録を保存（再接続時の自動復元に使う）
	h.mu.Lock()
	h.timers[req.PC] = &timerRecord{startedAt: time.Now(), total: seconds}
	h.mu.Unlock()
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
	// 再開時のタイマー記録を新たに保存（再接続時に継続できるよう）
	h.timers[req.PC] = &timerRecord{startedAt: time.Now(), total: remaining}
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

// ── LAN IP 検出 ───────────────────────────────────────────────
// スタッフPCの LAN 上の IP アドレスを自動で取得する
func getLANIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "localhost"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}
	return "localhost"
}

// ── main ─────────────────────────────────────────────────────
func main() {
	hub := newHub()
	ip := getLANIP()
	serverURL := fmt.Sprintf("http://%s:8080", ip)

	// WebSocket（エージェント用）
	http.HandleFunc("/ws", hub.wsHandler)

	// タイマー API
	http.HandleFunc("/api/timer/start", hub.handleStart)
	http.HandleFunc("/api/timer/pause", hub.handlePause)
	http.HandleFunc("/api/timer/resume", hub.handleResume)

	// ブロック API
	http.HandleFunc("/api/block/start", hub.handleBlockStart)
	http.HandleFunc("/api/block/stop", hub.handleBlockStop)

	// ステータス API
	http.HandleFunc("/api/status", hub.handleStatus)

	// QR コード画像を返す（スタッフのスマホが読むとこのページに飛ぶ）
	http.HandleFunc("/qr", func(w http.ResponseWriter, r *http.Request) {
		png, err := qrcode.Encode(serverURL, qrcode.Medium, 256)
		if err != nil {
			http.Error(w, "QR 生成失敗", 500)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(png)
	})

	// スタッフ管理画面（HTML を配信）
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	addr := ":8080"
	log.Printf("サーバー起動: %s", serverURL)
	log.Println("スタッフはこの URL にアクセス →", serverURL)
	log.Fatal(http.ListenAndServe(addr, nil))
}
