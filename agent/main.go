package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const defaultPCName = "pc-1"
const defaultServerURL = "ws://localhost:8080/ws"

// getServerURL は環境変数 SERVER_URL があればそれを使う
// なければデフォルト値（インストール時に設定される）
func getServerURL() string {
	if url := os.Getenv("SERVER_URL"); url != "" {
		return url
	}
	return defaultServerURL
}

// ── メッセージ形式 ────────────────────────────────────────────
type Message struct {
	Type      string `json:"type"`
	Seconds   int    `json:"seconds,omitempty"`
	Remaining int    `json:"remaining,omitempty"`
}

// ── タイマー管理 ──────────────────────────────────────────────
type activeTimerState struct {
	cancelCh  chan struct{}
	mu        sync.Mutex
	remaining int
}

var (
	currentTimer *activeTimerState
	timerMu      sync.Mutex
)

// ── ブロックモード管理 ────────────────────────────────────────
// ブロック中は 1 秒ごとに全 UI アプリを kill し続ける
var (
	blockCancel chan struct{} // close するとブロックが止まる
	blockMu     sync.Mutex
)

func main() {
	name := getPCName()
	log.Printf("エージェント起動: %s", name)

	for {
		if err := connect(name, getServerURL()); err != nil {
			log.Println("接続失敗、5秒後に再試行:", err)
		}
		time.Sleep(5 * time.Second)
	}
}

func connect(pcName string, serverURL string) error {
	log.Println("サーバーに接続中...", serverURL)
	conn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
	if err != nil {
		return err
	}
	defer func() {
		stopBlock() // 切断時にブロックモードも止める
		conn.Close()
	}()

	if err := conn.WriteMessage(websocket.TextMessage, []byte(pcName)); err != nil {
		return err
	}
	log.Println("サーバーに接続しました")

	var writeMu sync.Mutex
	send := func(msg Message) {
		data, _ := json.Marshal(msg)
		writeMu.Lock()
		defer writeMu.Unlock()
		conn.WriteMessage(websocket.TextMessage, data)
	}

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			stopTimer()
			return err
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "start":
			log.Printf("タイマー開始: %d 秒", msg.Seconds)
			startTimer(msg.Seconds, send)

		case "pause":
			remaining := stopTimer()
			log.Printf("一時停止: 残り %d 秒", remaining)
			send(Message{Type: "paused", Remaining: remaining})

		case "resume":
			log.Printf("タイマー再開: 残り %d 秒", msg.Seconds)
			startTimer(msg.Seconds, send)

		case "stop":
			// タイマーをリセット（ブロックはそのまま）
			log.Println("タイマーリセット")
			stopTimer()

		case "block":
			// ブロックモード開始（PC ごとにサーバーから指示が来る）
			log.Println("ブロックモード開始")
			startBlock()

		case "unblock":
			// ブロックモード解除
			log.Println("ブロックモード解除")
			stopBlock()
		}
	}
}

// ── タイマー処理 ──────────────────────────────────────────────

func startTimer(seconds int, send func(Message)) {
	stopTimer()

	cancelCh := make(chan struct{})
	t := &activeTimerState{cancelCh: cancelCh, remaining: seconds}

	timerMu.Lock()
	currentTimer = t
	timerMu.Unlock()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-cancelCh:
				return
			case <-ticker.C:
				t.mu.Lock()
				t.remaining--
				remaining := t.remaining
				t.mu.Unlock()

				send(Message{Type: "tick", Remaining: remaining})

				if remaining == 5*60 {
					showWarning("残り5分です！")
				}
				if remaining == 60 {
					showWarning("残り1分です！")
				}

				if remaining <= 0 {
					log.Println("タイマー終了 → 全UIアプリを終了します")
					killAllUIApps() // ← 時間切れで全アプリを閉じる
					send(Message{Type: "done"})
					timerMu.Lock()
					currentTimer = nil
					timerMu.Unlock()
					return
				}
			}
		}
	}()
}

func stopTimer() int {
	timerMu.Lock()
	t := currentTimer
	currentTimer = nil
	timerMu.Unlock()

	if t == nil {
		return 0
	}
	close(t.cancelCh)
	t.mu.Lock()
	remaining := t.remaining
	t.mu.Unlock()
	return remaining
}

// ── ブロックモード処理 ────────────────────────────────────────

// startBlock はブロックモードを開始する
// 1 秒ごとに全 UI アプリを kill し続ける goroutine を起動する
func startBlock() {
	blockMu.Lock()
	defer blockMu.Unlock()

	if blockCancel != nil {
		return // すでにブロック中
	}

	cancelCh := make(chan struct{})
	blockCancel = cancelCh

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-cancelCh:
				return
			case <-ticker.C:
				killAllUIApps()
			}
		}
	}()
}

// stopBlock はブロックモードを解除する
func stopBlock() {
	blockMu.Lock()
	defer blockMu.Unlock()

	if blockCancel == nil {
		return
	}
	close(blockCancel)
	blockCancel = nil
}

// ── アプリ終了処理 ────────────────────────────────────────────

// killAllUIApps は画面に窓を持つ全アプリを終了する
// エージェント自身（PID で除外）とシステムプロセス（窓なし）は残る
func killAllUIApps() {
	if runtime.GOOS == "windows" {
		pid := os.Getpid()
		script := fmt.Sprintf(
			`Get-Process | Where-Object { $_.MainWindowHandle -ne 0 -and $_.Id -ne %d } | Stop-Process -Force`,
			pid,
		)
		if err := newPSCmd("-WindowStyle", "Hidden", "-NonInteractive", "-Command", script).Run(); err != nil {
			log.Println("アプリ終了エラー:", err)
		}
	} else {
		log.Println("[開発中] 全UIアプリを終了します（実際の kill はスキップ）")
	}
}

// showWarning は右下にトースト通知を表示する（ゲームを中断しない）
func showWarning(message string) {
	if runtime.GOOS == "windows" {
		script := fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=WindowsRuntime] | Out-Null
$t = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$t.GetElementsByTagName('text')[0].AppendChild($t.CreateTextNode('Time Notice')) | Out-Null
$t.GetElementsByTagName('text')[1].AppendChild($t.CreateTextNode('%s')) | Out-Null
$app = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($app).Show([Windows.UI.Notifications.ToastNotification]::new($t))
`, message)
		newPSCmd("-WindowStyle", "Hidden", "-NonInteractive", "-Command", script).Start()
	} else {
		log.Println("警告:", message)
	}
}

func getPCName() string {
	if name := os.Getenv("PC_NAME"); name != "" {
		return name
	}
	return defaultPCName
}
