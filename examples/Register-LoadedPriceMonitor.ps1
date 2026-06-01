# Register a scheduled task that runs the loaded.com price monitor every 6 hours.
# Edit the values below, then run from an ELEVATED PowerShell:
#   .\examples\Register-LoadedPriceMonitor.ps1
#
# To remove later:
#   Unregister-ScheduledTask -TaskName "Loaded Price Monitor" -Confirm:$false

param(
    [string]$RepoRoot     = "C:\dev\tools\loaded-price-tracker",
    [string]$Executable   = "C:\dev\tools\loaded-price-tracker\rust\target\release\loaded-rs.exe",
    [string]$Arguments    = "--no-banner check",
    [int]   $IntervalHours = 6,

    # Notification secrets — at least one channel required.
    [string]$DiscordWebhookUrl = "",
    [string]$TelegramBotToken  = "",
    [string]$TelegramChatId    = ""
)

$ErrorActionPreference = "Stop"

$action = New-ScheduledTaskAction `
    -Execute $Executable `
    -Argument $Arguments `
    -WorkingDirectory $RepoRoot

$trigger = New-ScheduledTaskTrigger `
    -Once -At (Get-Date).AddMinutes(1) `
    -RepetitionInterval (New-TimeSpan -Hours $IntervalHours)

$settings = New-ScheduledTaskSettingsSet `
    -StartWhenAvailable `
    -DontStopIfGoingOnBatteries `
    -RestartCount 2 -RestartInterval (New-TimeSpan -Minutes 5) `
    -ExecutionTimeLimit (New-TimeSpan -Minutes 30)

$principal = New-ScheduledTaskPrincipal `
    -UserId "$env:USERDOMAIN\$env:USERNAME" `
    -LogonType S4U -RunLevel Limited

if ($DiscordWebhookUrl) { [Environment]::SetEnvironmentVariable("DISCORD_WEBHOOK_URL", $DiscordWebhookUrl, "User") }
if ($TelegramBotToken)  { [Environment]::SetEnvironmentVariable("TELEGRAM_BOT_TOKEN",  $TelegramBotToken,  "User") }
if ($TelegramChatId)    { [Environment]::SetEnvironmentVariable("TELEGRAM_CHAT_ID",    $TelegramChatId,    "User") }

Register-ScheduledTask `
    -TaskName "Loaded Price Monitor" `
    -Description "Checks loaded.com prices every $IntervalHours hours." `
    -Action $action -Trigger $trigger -Settings $settings -Principal $principal `
    -Force | Out-Null

Write-Host "Registered task 'Loaded Price Monitor' — runs every $IntervalHours hour(s)."
Write-Host "View it in Task Scheduler, or with:  Get-ScheduledTask -TaskName 'Loaded Price Monitor'"
