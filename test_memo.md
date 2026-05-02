# 実機テストメモ

## 進捗サマリー（2026/04/30 時点）

### 完了したこと
- [x] スタッフPCに server.exe をインストール（`C:\gakudo_server\server.exe`）
- [x] スタッフPCの IP を固定（192.168.0.119）
- [x] 子どもPC（pc-1）に agent.exe をインストール（`C:\gakudo`）
- [x] 管理画面（http://localhost:8080）で pc-1 の接続を確認
- [x] 警告をトースト通知に変更（ゲームが止まらない）
- [x] PowerShell ウィンドウを非表示に（ターミナルが出なくなった）
- [x] 管理画面 UI を最新版に更新
- [x] 強制終了中に Explorer を除外してチラつきを修正
- [x] 通知に残り時間を大きく表示するよう改善

### 残っている作業
- [ ] 子どもPC（pc-2〜pc-6）へのエージェントインストール
- [ ] 全テスト項目の動作確認（下記）
- [ ] QRコードのipアドレスが違う
---

## 基本動作
- [ ] タイマー開始 → カウントダウンが管理画面で見える
- [ ] 残り5分・1分で子どもPCにトースト通知が出る
- [ ] 時間切れで子どもPCのアプリが強制終了される
- [ ] 一時停止 → 再開ができる
- [ ] リセットが動く
- [ ] 強制終了ボタンでアプリが閉じる・解除で戻る

## 接続が切れたとき
- [x] 子どもPCをスリープ → 復帰後にタイマーが継続する ✅
- [x] 子どもPCを再起動 → ログイン後にエージェントが自動起動する ✅
  - 切断中にタイマーが切れた場合は終了扱い（正常動作）
  - 素早く再起動した場合は残り時間から継続（正常動作）
- [ ] スタッフPCをスリープ → 復帰後に子どもPCが再接続する
- [ ] Wi-Fiが一時的に切れる → 再接続後に復帰する

## スタッフPCの問題
- [ ] スタッフPCを再起動したらサーバーが自動起動する
- [ ] スタッフPCが再起動してもタイマーが再開する（再接続後）

## 実際の使用シナリオ
- [ ] タイマー中に子どもがPCを強制再起動してもブロックが継続する
- [ ] 複数PC同時にタイマーを動かす
- [ ] 強制終了中に子どもがアプリを起動しようとしても1秒で閉じられる

---

## よく使うコマンド

**子どもPC（agent インストール）**
```
cd "C:\Users\USER\Downloads\managePC-main\managePC-main\agent"
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

**スタッフPC（server インストール）**
```
cd "C:\Users\USER\Downloads\managePC-main\managePC-main\server"
powershell -ExecutionPolicy Bypass -File .\install_server.ps1
```

**手動起動（緊急用）**
- スタッフPC: `C:\gakudo_server\server.exe`
- 子どもPC: `C:\gakudo\agent.exe`
