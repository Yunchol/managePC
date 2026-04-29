# 学童エージェント インストールスクリプト
# 使い方:
#   1. このスクリプトと agent.exe を同じフォルダに置く
#   2. 下の「設定」欄を各 PC に合わせて変更する
#   3. PowerShell を右クリック →「管理者として実行」→ このファイルをドラッグ＆ドロップ → Enter

# ════════════════════════════════════════
#  設定（PC ごとに変える）
# ════════════════════════════════════════
$PC_NAME   = "pc-1"                   # この PC の名前（pc-1 〜 pc-6）
$SERVER_IP = "192.168.0.110"          # スタッフPCの固定IP（実際のIPに変える）
# ════════════════════════════════════════

$SERVER_URL  = "ws://${SERVER_IP}:8080/ws"
$INSTALL_DIR = "C:\gakudo"
$EXE_NAME    = "agent.exe"
$TASK_NAME   = "GakudoAgent"
$EXE_SRC     = Join-Path $PSScriptRoot $EXE_NAME
$EXE_DEST    = Join-Path $INSTALL_DIR  $EXE_NAME

Write-Host "=== 学童エージェント インストール ===" -ForegroundColor Cyan
Write-Host "PC 名:       $PC_NAME"
Write-Host "サーバーURL: $SERVER_URL"
Write-Host ""

# ① インストール先フォルダを作る
if (-not (Test-Path $INSTALL_DIR)) {
    New-Item -ItemType Directory -Path $INSTALL_DIR | Out-Null
}
Copy-Item -Path $EXE_SRC -Destination $EXE_DEST -Force
Write-Host "① agent.exe をコピーしました → $EXE_DEST"

# ② システム環境変数を設定する（再起動後も有効）
[System.Environment]::SetEnvironmentVariable("PC_NAME",    $PC_NAME,   "Machine")
[System.Environment]::SetEnvironmentVariable("SERVER_URL", $SERVER_URL, "Machine")
Write-Host "② 環境変数を設定しました（PC_NAME=$PC_NAME）"

# ③ 既存タスクがあれば削除
if (Get-ScheduledTask -TaskName $TASK_NAME -ErrorAction SilentlyContinue) {
    Unregister-ScheduledTask -TaskName $TASK_NAME -Confirm:$false
    Write-Host "③ 既存タスクを削除しました"
}

# ④ タスクスケジューラに登録（ログイン時に自動起動・非表示）
$action    = New-ScheduledTaskAction -Execute $EXE_DEST
$trigger   = New-ScheduledTaskTrigger -AtLogOn
$settings  = New-ScheduledTaskSettingsSet -Hidden $true -ExecutionTimeLimit 0
$principal = New-ScheduledTaskPrincipal -UserId (whoami) -LogonType Interactive -RunLevel Highest

Register-ScheduledTask `
    -TaskName  $TASK_NAME `
    -Action    $action `
    -Trigger   $trigger `
    -Settings  $settings `
    -Principal $principal `
    -Force | Out-Null

Write-Host "④ タスクスケジューラに登録しました（ログイン時に自動起動）"

# ⑤ 今すぐ起動
Start-ScheduledTask -TaskName $TASK_NAME
Write-Host "⑤ エージェントを起動しました"

Write-Host ""
Write-Host "=== インストール完了！ ===" -ForegroundColor Green
Write-Host "PC を再起動するとログイン後に自動で起動します。"
Write-Host "タスクマネージャーの「詳細」タブで agent.exe が動いているか確認できます。"
