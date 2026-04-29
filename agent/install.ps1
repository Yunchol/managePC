# Agent install script (for children's PCs)
# Edit the two lines below before running

# ===== SETTINGS (change per PC) =====
$PC_NAME   = "pc-1"          # PC name: pc-1, pc-2, pc-3 ...
$SERVER_IP = "192.168.0.110" # Staff PC's IP address
# ====================================

$SERVER_URL  = "ws://${SERVER_IP}:8080/ws"
$INSTALL_DIR = "C:\gakudo"
$EXE_NAME    = "agent.exe"
$TASK_NAME   = "GakudoAgent"
$EXE_SRC     = Join-Path $PSScriptRoot $EXE_NAME
$EXE_DEST    = Join-Path $INSTALL_DIR  $EXE_NAME

Write-Host "=== Installing Agent ===" -ForegroundColor Cyan
Write-Host "PC Name:    $PC_NAME"
Write-Host "Server URL: $SERVER_URL"
Write-Host ""

# 1. Create install folder and copy exe
if (-not (Test-Path $INSTALL_DIR)) {
    New-Item -ItemType Directory -Path $INSTALL_DIR | Out-Null
}
Copy-Item -Path $EXE_SRC -Destination $EXE_DEST -Force
Write-Host "1. Copied agent.exe to $EXE_DEST"

# 2. Set system environment variables (persist after reboot)
[System.Environment]::SetEnvironmentVariable("PC_NAME",    $PC_NAME,   "Machine")
[System.Environment]::SetEnvironmentVariable("SERVER_URL", $SERVER_URL, "Machine")
Write-Host "2. Set environment variables (PC_NAME=$PC_NAME)"

# 3. Remove existing task
if (Get-ScheduledTask -TaskName $TASK_NAME -ErrorAction SilentlyContinue) {
    Unregister-ScheduledTask -TaskName $TASK_NAME -Confirm:$false
    Write-Host "3. Removed existing task"
}

# 4. Register in Task Scheduler (auto-start at logon, hidden)
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

Write-Host "4. Registered in Task Scheduler (auto-start at logon)"

# 5. Start now
Start-ScheduledTask -TaskName $TASK_NAME
Write-Host "5. Agent started"

Write-Host ""
Write-Host "=== Install complete! ===" -ForegroundColor Green
Write-Host "Check Task Manager > Details tab to confirm agent.exe is running."
