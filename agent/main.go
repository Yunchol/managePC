package main

import (
	"log"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

// サーバーのアドレス（スタッフPCの固定IPに合わせて後で変える）
const serverURL = "ws://localhost:8080/ws"

// この PC の名前（子どもPC ごとに変える → pc-1, pc-2 ... pc-6）
const pcName = "pc-1"

func main() {
	log.Printf("エージェント起動: %s", pcName)

	// 接続が切れても何度でも再接続し続けるループ
	for {
		err := connect()
		if err != nil {
			log.Println("接続失敗、5秒後に再試行:", err)
		}
		// 接続が切れたら 5秒待ってから再接続する
		time.Sleep(5 * time.Second)
	}
}

// connect = サーバーに接続して、切断されるまでメッセージを待つ関数
// 切断されたら err を返す → main のループで再接続される
func connect() error {

	// ① サーバーに WebSocket で接続する
	log.Println("サーバーに接続中...")
	conn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
	if err != nil {
		return err // 接続失敗 → main に返して再試行
	}
	defer conn.Close() // この関数が終わるとき（切断時）に自動でコネクションを閉じる

	// ② 接続したらすぐ PC 名をサーバーに送る（「俺は pc-1 です」）
	err = conn.WriteMessage(websocket.TextMessage, []byte(pcName))
	if err != nil {
		return err
	}
	log.Println("サーバーに接続しました")

	// ③ サーバーからのメッセージを待ち続けるループ
	// 今はログに出すだけ（Step 4 でタイマー処理を追加する）
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			// サーバーとの接続が切れた → ループを抜けて再接続へ
			log.Println("接続が切れました:", err)
			return err
		}
		log.Printf("[サーバーから] %s", string(msg))
	}
}

// pcNameFromEnv は環境変数 PC_NAME が設定されていればそれを使う（任意）
// 設定されていなければデフォルト値を使う
func pcNameFromEnv(defaultName string) string {
	if name := os.Getenv("PC_NAME"); name != "" {
		return name
	}
	return defaultName
}
