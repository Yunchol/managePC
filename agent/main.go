package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// サーバーのアドレス（スタッフPCの固定IPに合わせて変える）
const serverURL = "ws://localhost:8080/ws"

// デフォルトの PC 名（環境変数 PC_NAME で上書き可能）
const defaultPCName = "pc-1"

// ── メッセージ形式 ────────────────────────────────────────────
type Message struct {
	Type      string `json:"type"`
	Seconds   int    `json:"seconds,omitempty"`   // サーバーから受け取る秒数
	Remaining int    `json:"remaining,omitempty"` // サーバーへ送る残り秒数
}

// ── タイマー管理 ──────────────────────────────────────────────
// activeTimer は現在動いているタイマーを表す
// nil のときはタイマーが動いていない
type activeTimerState struct {
	cancelCh  chan struct{} // このチャネルを close するとタイマーが止まる
	mu        sync.Mutex
	remaining int // 残り秒数（goroutine が毎秒更新する）
}

var (
	currentTimer *activeTimerState
	timerMu      sync.Mutex // currentTimer 自体へのアクセスを守る鍵
)

func main() {
	name := getPCName()
	log.Printf("エージェント起動: %s", name)

	// 接続が切れても何度でも再接続し続けるループ
	for {
		if err := connect(name); err != nil {
			log.Println("接続失敗、5秒後に再試行:", err)
		}
		time.Sleep(5 * time.Second)
	}
}

// connect はサーバーに接続して、切断されるまでメッセージを処理する
func connect(pcName string) error {
	log.Println("サーバーに接続中...")
	conn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// 接続したらすぐ PC 名を送る
	if err := conn.WriteMessage(websocket.TextMessage, []byte(pcName)); err != nil {
		return err
	}
	log.Println("サーバーに接続しました")

	// WebSocket への書き込みを複数 goroutine から安全に行うための鍵
	var writeMu sync.Mutex
	send := func(msg Message) {
		data, _ := json.Marshal(msg)
		writeMu.Lock()
		defer writeMu.Unlock()
		conn.WriteMessage(websocket.TextMessage, data)
	}

	// サーバーからのメッセージを受け取るループ
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			// 切断 → タイマーを止めて reconnect へ
			stopTimer()
			return err
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "start":
			// タイマー開始（新しいタイマーが来たら既存のものは止まる）
			log.Printf("タイマー開始: %d 秒", msg.Seconds)
			startTimer(msg.Seconds, send)

		case "pause":
			// タイマーを止めて残り秒数をサーバーに返す
			remaining := stopTimer()
			log.Printf("一時停止: 残り %d 秒", remaining)
			send(Message{Type: "paused", Remaining: remaining})

		case "resume":
			// 保存されていた残り秒数からタイマーを再開
			log.Printf("タイマー再開: 残り %d 秒", msg.Seconds)
			startTimer(msg.Seconds, send)
		}
	}
}

// startTimer は新しいカウントダウン goroutine を起動する
// 既存のタイマーがあれば先に止める
func startTimer(seconds int, send func(Message)) {
	stopTimer()

	cancelCh := make(chan struct{})
	t := &activeTimerState{
		cancelCh:  cancelCh,
		remaining: seconds,
	}

	timerMu.Lock()
	currentTimer = t
	timerMu.Unlock()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-cancelCh:
				// pause や再接続で止められた
				return
			case <-ticker.C:
				t.mu.Lock()
				t.remaining--
				remaining := t.remaining
				t.mu.Unlock()

				// 毎秒、残り時間をサーバーに報告
				send(Message{Type: "tick", Remaining: remaining})

				// 残り 5 分・1 分で警告ダイアログを表示
				if remaining == 5*60 {
					showWarning("残り5分です！")
				}
				if remaining == 60 {
					showWarning("残り1分です！")
				}

				// タイマー終了
				if remaining <= 0 {
					log.Println("タイマー終了")
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

// stopTimer は現在のタイマーを止めて残り秒数を返す
// タイマーがなければ 0 を返す
func stopTimer() int {
	timerMu.Lock()
	t := currentTimer
	currentTimer = nil
	timerMu.Unlock()

	if t == nil {
		return 0
	}

	close(t.cancelCh) // goroutine に止まるよう伝える

	t.mu.Lock()
	remaining := t.remaining
	t.mu.Unlock()

	return remaining
}

// showWarning は警告を表示する
// Windows: PowerShell でダイアログ表示
// Mac/Linux（開発中）: ログに出すだけ
func showWarning(message string) {
	if runtime.GOOS == "windows" {
		// PowerShell でメッセージボックスを表示（子どもの画面に出る）
		script := fmt.Sprintf(
			`Add-Type -AssemblyName PresentationFramework; [System.Windows.MessageBox]::Show('%s','時間のお知らせ')`,
			message,
		)
		exec.Command("powershell", "-Command", script).Start()
	} else {
		// 開発中（Mac）はログに出すだけ
		log.Println("警告:", message)
	}
}

// getPCName は PC 名を返す
// 環境変数 PC_NAME が設定されていればそれを使う（子どもPC ごとに設定する）
func getPCName() string {
	if name := os.Getenv("PC_NAME"); name != "" {
		return name
	}
	return defaultPCName
}
