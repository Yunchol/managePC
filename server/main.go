package main

// このファイルはメインプログラム。Go では必ず書く。

import (
	// 使う道具を宣言する（Python の import、JS の require と同じ）
	"fmt"      // 文字を表示する道具
	"log"      // ログを出す道具
	"net/http" // HTTP サーバーを作る道具
	"sync"     // 並列処理を安全にする道具

	"github.com/gorilla/websocket" // WebSocket の道具（go get で追加したやつ）
)

// Client は子どもPC 1台分の情報をまとめた箱（struct）
// struct = 関連するデータをセットで持つ入れ物
type Client struct {
	name string           // PC の名前（例："pc-1"）
	conn *websocket.Conn  // その PC との通信回線
}

// Hub は接続中の全 PC を管理する受付
// map は辞書みたいなもの → "pc-1": Client, "pc-2": Client ...
type Hub struct {
	mu      sync.Mutex          // 鍵。複数PCが同時に接続してきたとき辞書が壊れないようにする
	clients map[string]*Client  // PC名 → Client の対応表
}

// Hub を新しく作って返す関数
func newHub() *Hub {
	return &Hub{
		clients: make(map[string]*Client), // 空の辞書を用意
	}
}

// register = PC が接続してきたとき、辞書に追加する（出席簿に名前を書くイメージ）
func (h *Hub) register(client *Client) {
	h.mu.Lock()         // 鍵をかける（他の処理が辞書を触れないようにする）
	defer h.mu.Unlock() // この関数が終わったら鍵を外す（defer = 後で実行）

	h.clients[client.name] = client
	log.Printf("[接続] %s (現在 %d 台接続中)", client.name, len(h.clients))
}

// unregister = PC が切断したとき、辞書から削除する
func (h *Hub) unregister(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.clients, name)
	log.Printf("[切断] %s (現在 %d 台接続中)", name, len(h.clients))
}

// upgrader = HTTP 接続を WebSocket に切り替えるための道具
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // 開発中は全オリジンを許可
}

// wsHandler = /ws にアクセスが来たときに呼ばれる関数（接続の窓口）
// 流れ: 接続 → 名前を聞く → 登録 → メッセージを待ち続ける → 切断したら削除
func (h *Hub) wsHandler(w http.ResponseWriter, r *http.Request) {

	// ① HTTP 接続を WebSocket に切り替える
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket アップグレード失敗:", err)
		return
	}

	// ② 最初のメッセージで PC 名を受け取る（エージェントが「pc-1 です」と送ってくる）
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Println("PC 名受信失敗:", err)
		conn.Close()
		return
	}
	pcName := string(msg) // バイト列を文字列に変換（例: "pc-1"）

	// ③ Client を作って Hub に登録する
	client := &Client{name: pcName, conn: conn}
	h.register(client)

	// この関数が終わるとき（＝切断時）に自動で後片付けする
	defer func() {
		h.unregister(pcName)
		conn.Close()
	}()

	// ④ メッセージを待ち続けるループ（切断されたら err が出てループを抜ける）
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break // 切断 → ループを抜ける → defer の後片付けが走る
		}
		log.Printf("[%s] %s", pcName, string(msg))
	}
}

// main = プログラムが起動したときに最初に呼ばれる関数。ここから全部始まる。
func main() {
	hub := newHub() // Hub（受付）を作る

	// URL ごとに呼ぶ関数を登録する
	http.HandleFunc("/ws", hub.wsHandler) // /ws → WebSocket の窓口
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "管理サーバー 稼働中") // / → 動作確認用
	})

	// 8080番ポートで待ち始める（ここで止まってずっと接続を待ち続ける）
	addr := ":8080"
	log.Println("サーバー起動:", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
