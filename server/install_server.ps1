# Server install script (for staff PC)

$INSTALL_DIR = "C:\gakudo_server"
$EXE_NAME    = "server.exe"
$VBS_NAME    = "start_server.vbs"
$TASK_NAME   = "GakudoServer"
$EXE_SRC     = Join-Path $PSScriptRoot $EXE_NAME
$VBS_SRC     = Join-Path $PSScriptRoot $VBS_NAME
$EXE_DEST    = Join-Path $INSTALL_DIR  $EXE_NAME
$VBS_DEST    = Join-Path $INSTALL_DIR  $VBS_NAME

Write-Host "=== Installing Server ===" -ForegroundColor Cyan

# 1. Create folder and copy files
if (-not (Test-Path $INSTALL_DIR)) {
    New-Item -ItemType Directory -Path $INSTALL_DIR | Out-Null
}
Copy-Item -Path $EXE_SRC -Destination $EXE_DEST -Force
Copy-Item -Path $VBS_SRC -Destination $VBS_DEST -Force
Write-Host "1. Copied files to $INSTALL_DIR"

# 2. Remove existing task
if (Get-ScheduledTask -TaskName $TASK_NAME -ErrorAction SilentlyContinue) {
    Unregister-ScheduledTask -TaskName $TASK_NAME -Confirm:$false
}

# 3. Register in Task Scheduler (via VBS to hide console window)
$action    = New-ScheduledTaskAction -Execute "wscript.exe" -Argument $VBS_DEST
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

Write-Host "2. Registered in Task Scheduler"

# 4. Open firewall port 8080
netsh advfirewall firewall add rule `
    name="GakudoServer" `
    protocol=TCP `
    dir=in `
    localport=8080 `
    action=allow | Out-Null
Write-Host "3. Opened firewall port 8080"

# 5. Start now
Start-ScheduledTask -TaskName $TASK_NAME
Write-Host "4. Server started"

Write-Host ""
Write-Host "=== Install complete! ===" -ForegroundColor Green
Write-Host "Open http://localhost:8080 in your browser to confirm."
