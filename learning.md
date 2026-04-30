いいまとめになってる 👍
そのまま使えるように、読みやすい形に整えたよ。

---

# 🟦 全体像：何を作ってるの？

```
スタッフのスマホ/PC
      ↕ ブラウザ
  ┌─────────────┐
  │ スタッフPC   │  ← サーバー（司令塔）
  │ server.exe  │
  └──────┬──────┘
         ↕ Wi-Fi (WebSocket)
  ┌──────┴──────┐
  │ 子どもPC×6  │  ← エージェント（手足）
  │ agent.exe   │
  └─────────────┘
```

### 💡 仕組み

* スタッフがブラウザで「30分スタート」を押す
* サーバーが子どもPCに命令を送る
* 時間が来たらアプリを強制終了

---

# 🟩 やったコマンドの意味

## 🟢 Mac：GoでWindows用EXEを作成

```bash
GOOS=windows GOARCH=amd64 go build -o server.exe .
```

| 部分            | 意味              |
| ------------- | --------------- |
| GOOS=windows  | Windows用に作る     |
| GOARCH=amd64  | 64ビットPC用        |
| go build      | Goコードを実行ファイルに変換 |
| -o server.exe | 出力ファイル名         |

👉 **MacでWindows用EXEが作れる（クロスコンパイル）**

---

## 🟣 PowerShell①：実行ポリシー変更

```powershell
Set-ExecutionPolicy RemoteSigned -Scope CurrentUser
```

### 💡 意味

* スクリプト実行制限をゆるめる
* 自分のユーザーだけ対象

👉 Windowsは初期状態だとスクリプト実行が禁止されている

---

## 🟣 PowerShell②：Bypassで実行

```powershell
powershell -ExecutionPolicy Bypass -File .\install_server.ps1
```

| 部分                      | 意味           |
| ----------------------- | ------------ |
| powershell              | PowerShell起動 |
| -ExecutionPolicy Bypass | セキュリティチェック無視 |
| -File                   | 実行ファイル指定     |
| .\                      | 今のフォルダ       |

---

# 🟥 エラーの理由と解決

## ❌ エラー①：スクリプト実行が無効

```
このシステムではスクリプトの実行が無効になっています
```

### 原因

* Windowsのセキュリティ（デフォルト設定）

### 解決

```powershell
Set-ExecutionPolicy RemoteSigned
```

---

## ❌ エラー②：デジタル署名がない

```
このスクリプトはデジタル署名されていません
```

### 原因

* 個人スクリプトには署名がない

### 解決

```powershell
-ExecutionPolicy Bypass
```

---

## ❌ エラー③：文字化け

```
文字列に終端記号 " がありません
```

### 原因

* Mac → UTF-8
* Windows → Shift-JISで読む

👉 日本語が壊れて構文エラーに

### 解決

* 日本語を削除（英語にする）

---

# 🟨 インストールスクリプトの中身

```powershell
# server.exe をコピー
Copy-Item -Path $EXE_SRC -Destination $EXE_DEST

# 自動起動登録
Register-ScheduledTask ...

# ファイアウォール設定
netsh advfirewall firewall add rule ...

# 即起動
Start-ScheduledTask ...
```

---

## 💡 用語解説

### 🟦 タスクスケジューラ

* Windowsの自動実行機能
  👉 ログイン時に `server.exe` を起動

---

### 🟦 ファイアウォール

* 通信をブロックする壁
  👉 ポート8080だけ開けた

---

# 🟪 ネットワークの理解

## 🟢 IPアドレス

```
192.168.0.104 ← スタッフPC
192.168.0.xxx ← 子どもPC
```

👉 同じWi-Fiなら通信できる

---

## 🔴 特殊なIP

```
169.254.x.x
```

### 意味

* ルーターからIPもらえなかった
* 仮のアドレス

👉 通信できないことが多い

---

# 🧠 全体の理解まとめ

* サーバー = 司令塔
* エージェント = 実行役
* WebSocket = 常時接続
* タスクスケジューラ = 自動起動
* ファイアウォール = 通信制御
* IPアドレス = ネットワーク上の住所

---

もし次いくならこれかな👇
👉 「WebSocketがどうやって常時接続を保ってるか」
👉 「強制終了の実装（プロセスkillの仕組み）」

この2つやると一気に“実務レベル”になるよ
