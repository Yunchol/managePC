# Server install script (for staff PC)

$INSTALL_DIR = "C:\gakudo_server"
$EXE_NAME    = "server.exe"
$TASK_NAME   = "GakudoServer"
$EXE_SRC     = Join-Path $PSScriptRoot $EXE_NAME
$EXE_DEST    = Join-Path $INSTALL_DIR  $EXE_NAME

Write-Host "=== Installing Server ===" -ForegroundColor Cyan

# 1. Create folder and copy exe
if (-not (Test-Path $INSTALL_DIR)) {
    New-Item -ItemType Directory -Path $INSTALL_DIR | Out-Null
}
Copy-Item -Path $EXE_SRC -Destination $EXE_DEST -Force
Write-Host "1. Copied server.exe to $EXE_DEST"

# 2. Remove existing task
if (Get-ScheduledTask -TaskName $TASK_NAME -ErrorAction SilentlyContinue) {
    Unregister-ScheduledTask -TaskName $TASK_NAME -Confirm:$false
}

# 3. Register in Task Scheduler
$action    = New-ScheduledTaskAction -Execute $EXE_DEST
$trigger   = New-ScheduledTaskTrigger -AtLogOn
$settings  = New-ScheduledTaskSettingsSet -Hidden -ExecutionTimeLimit 0
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
