# 学童エージェント インストールスクリプト
# PowerShell を「管理者として実行」してから実行する
#
# 使い方:
#   1. このスクリプトと agent.exe を同じフォルダに置く
#   2. PC_NAME を各 PC に合わせて変更する（pc-1 〜 pc-6）
#   3. PowerShell を管理者として実行し、このスクリプトを実行する

# ── 設定（PC ごとに変える） ────────────────────────────
$PC_NAME    = "pc-1"          # この PC の名前（pc-1 〜 pc-6）
$INSTALL_DIR = "C:\gakudo"    # インストール先フォルダ
# ──────────────────────────────────────────────────────

$SERVICE_NAME = "GakudoAgent"
$EXE_NAME     = "agent.exe"
$EXE_SRC      = Join-Path $PSScriptRoot $EXE_NAME
$EXE_DEST     = Join-Path $INSTALL_DIR  $EXE_NAME

# インストール先フォルダを作る
if (-not (Test-Path $INSTALL_DIR)) {
    New-Item -ItemType Directory -Path $INSTALL_DIR | Out-Null
    Write-Host "フォルダを作成しました: $INSTALL_DIR"
}

# exe をコピー
Copy-Item -Path $EXE_SRC -Destination $EXE_DEST -Force
Write-Host "コピー完了: $EXE_DEST"

# 既存サービスがあれば削除
if (Get-Service -Name $SERVICE_NAME -ErrorAction SilentlyContinue) {
    Stop-Service -Name $SERVICE_NAME -Force -ErrorAction SilentlyContinue
    sc.exe delete $SERVICE_NAME | Out-Null
    Start-Sleep -Seconds 1
    Write-Host "既存サービスを削除しました"
}

# Windows サービスとして登録
# PC_NAME を環境変数でエージェントに渡す
$binPath = "$EXE_DEST"
New-Service `
    -Name        $SERVICE_NAME `
    -BinaryPathName $binPath `
    -DisplayName "学童 PC エージェント" `
    -StartupType Automatic | Out-Null

# 環境変数 PC_NAME をサービスに設定
reg add "HKLM\SYSTEM\CurrentControlSet\Services\$SERVICE_NAME" `
    /v Environment /t REG_MULTI_SZ /d "PC_NAME=$PC_NAME" /f | Out-Null

# サービスを起動
Start-Service -Name $SERVICE_NAME
Write-Host ""
Write-Host "インストール完了！"
Write-Host "  サービス名: $SERVICE_NAME"
Write-Host "  PC 名:      $PC_NAME"
Write-Host "  PC を再起動すると自動で起動します"
