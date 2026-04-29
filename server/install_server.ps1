# 学童サーバー インストールスクリプト（スタッフPC用）
# 使い方:
#   1. このスクリプトと server.exe を同じフォルダに置く
#   2. PowerShell を右クリック →「管理者として実行」→ このファイルをドラッグ＆ドロップ

$INSTALL_DIR = "C:\gakudo_server"
$EXE_NAME    = "server.exe"
$TASK_NAME   = "GakudoServer"
$EXE_SRC     = Join-Path $PSScriptRoot $EXE_NAME
$EXE_DEST    = Join-Path $INSTALL_DIR  $EXE_NAME

Write-Host "=== 学童サーバー インストール ===" -ForegroundColor Cyan

# ① フォルダ作成 & コピー
if (-not (Test-Path $INSTALL_DIR)) {
    New-Item -ItemType Directory -Path $INSTALL_DIR | Out-Null
}
Copy-Item -Path $EXE_SRC -Destination $EXE_DEST -Force
Write-Host "① server.exe をコピーしました → $EXE_DEST"

# ② 既存タスク削除
if (Get-ScheduledTask -TaskName $TASK_NAME -ErrorAction SilentlyContinue) {
    Unregister-ScheduledTask -TaskName $TASK_NAME -Confirm:$false
}

# ③ タスクスケジューラに登録
$action    = New-ScheduledTaskAction -Execute $EXE_DEST
$trigger   = New-ScheduledTaskTrigger -AtLogOn
$settings  = New-ScheduledTaskSettingsSet -ExecutionTimeLimit 0
$principal = New-ScheduledTaskPrincipal -UserId (whoami) -LogonType Interactive -RunLevel Highest

Register-ScheduledTask `
    -TaskName  $TASK_NAME `
    -Action    $action `
    -Trigger   $trigger `
    -Settings  $settings `
    -Principal $principal `
    -Force | Out-Null

Write-Host "② タスクスケジューラに登録しました"

# ④ ファイアウォールで 8080 ポートを開放
netsh advfirewall firewall add rule `
    name="GakudoServer" `
    protocol=TCP `
    dir=in `
    localport=8080 `
    action=allow | Out-Null
Write-Host "③ ファイアウォールでポート 8080 を開放しました"

# ⑤ 今すぐ起動
Start-ScheduledTask -TaskName $TASK_NAME
Write-Host "④ サーバーを起動しました"

Write-Host ""
Write-Host "=== インストール完了！ ===" -ForegroundColor Green
Write-Host "ブラウザで http://localhost:8080 を開いて確認してください。"
