<#
.SYNOPSIS
One throwaway update rehearsal on Windows: the last public HTML Clay (1.x)
updates itself to this checkout's build through its own updater, and the
supervisor that judges the new version is the one 1.x shipped, unchanged.

.DESCRIPTION
The Windows twin of scripts/update-rehearsal-linux.sh. It runs in the Windows
VM only, from an elevated PowerShell window on the signed-in desktop:

    $env:HTMLCLAY_REHEARSAL_VM = '1'
    powershell -ExecutionPolicy Bypass -File scripts\update-rehearsal-windows.ps1 first

The old side is the public tag's source with one file replaced at build time
(internal/update/keys.go, so it trusts a throwaway feed key). It finds the
update on a loopback HTTPS feed on its own 60 s check, and restarts when this
script sends its tray the click on "Restart to Update", which is the only
trigger 1.x has. Three documents are open and saving once a second the whole
time (scripts/upgrade-ledger.mjs).

Cases:
  first             the plain update: the new version serves and the supervisor keeps it
  rollback-version  the new build reports another version than the feed promised, so it
                    runs and never counts as serving; after 20 s the supervisor kills it
                    and puts 1.x back
  rollback-early    the new "build" exits at once, and the supervisor puts 1.x back
  next              2.0 to the version after it: both sides are this checkout, the old
                    side serves a folder with its service running, and the update is
                    started with `htmlclay update-now`, with no click
  next-start        the same 2.0 pair, installed at the next start: a browser tab presses
                    Check for updates, the update downloads and waits, the app quits
                    the way its tray's Quit does, and starting HTML Clay again with a
                    document installs the update and opens that document
  next-button       the same 2.0 pair, started by a browser tab pressing Check for
                    updates and then the Update button, the way the dashboard does

Why the VM only, and why elevated:
  - Go on Windows asks Windows whether a certificate is trusted and ignores
    SSL_CERT_FILE, so the feed's throwaway certificate goes into the machine's
    Root store for the run and is removed by thumbprint at the end. Its key
    never leaves the feed process and it is valid for one day.
  - The app registers its file types and its start-on-login entry for the
    signed-in user. No environment variable moves those.
  - 1.x holds one machine-wide single-instance name, so no other HTML Clay may
    be running.
  - The tray lives on the signed-in desktop. A session with no desktop (ssh, a
    service) has no tray window to click.

The app's settings, documents and updates folder are in a scratch folder: it
runs with USERPROFILE, APPDATA and LOCALAPPDATA pointed there. 1.x opens each
of the three documents in the VM's browser; those tabs can be closed after.

  HTMLCLAY_REHEARSAL_OLD_SRC  a checkout or export of the old tag (default: clones the public tag)
  HTMLCLAY_REHEARSAL_DIR      the run folder, as a full long path (default: a new folder under C:\htmlclay-rehearsal)

Exit codes: 0 every check passed, 1 one of them did not, 2 usage or setup.
The last line on stdout is "REHEARSAL OK <case> ..." or "REHEARSAL FAILED <case>: ...".
#>
param(
  [Parameter(Mandatory = $true, Position = 0)]
  [ValidateSet('first', 'rollback-version', 'rollback-early', 'next', 'next-start', 'next-button')]
  [string]$Case,
  [string]$OldTag = 'v1.16.4',
  # The number of the "Restart to Update" row in the old app's tray, counted
  # from 1 in the order the app makes its rows. 1.x makes it first.
  [int]$UpdateRowId = 1
)

Set-StrictMode -Version 2
$ErrorActionPreference = 'Stop'

function Say([string]$Message) { [Console]::Error.WriteLine("[rehearsal] $Message") }

if ($env:OS -ne 'Windows_NT') { Say 'this rehearsal runs on Windows only'; exit 2 }
if ($env:HTMLCLAY_REHEARSAL_VM -ne '1') {
  Say 'this rehearsal changes the machine it runs on (a root certificate for the run, file types, start on login).'
  Say 'Run it in the Windows VM only, and say so first: $env:HTMLCLAY_REHEARSAL_VM = ''1'''
  exit 2
}
$principal = New-Object Security.Principal.WindowsPrincipal ([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  Say 'run this from an elevated PowerShell window: the feed''s certificate goes into the machine''s Root store for the run'
  exit 2
}
foreach ($tool in 'go', 'node', 'git') {
  if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { Say "$tool is missing"; exit 2 }
}
if (Get-Process -Name htmlclay -ErrorAction SilentlyContinue) {
  Say 'an HTML Clay is already running. Quit it first: 1.x holds one single-instance name for the whole machine'
  exit 2
}

$Go = (Get-Command go).Source
$Node = (Get-Command node).Source
$Git = (Get-Command git).Source
$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$OldVersion = '9.0.0'
$NewVersion = '9.0.1'
$Next = $Case -in 'next', 'next-start', 'next-button'
if ($Next) {
  $OldVersion = '9.0.1'
  $NewVersion = '9.0.2'
}
$PublicUrl = 'https://github.com/panphora/htmlclay.git'
$Arch = (& $Go env GOARCH).Trim()

# The run folder is not under %TEMP%: that path is often a short 8.3 name
# (C:\Users\ADMINI~1\...), and the app, Go and this script would then spell
# the same folder two ways.
if ($env:HTMLCLAY_REHEARSAL_DIR) {
  $Lab = $env:HTMLCLAY_REHEARSAL_DIR
} else {
  $Lab = Join-Path "$env:SystemDrive\" ('htmlclay-rehearsal\' + [IO.Path]::GetRandomFileName().Replace('.', ''))
}
New-Item -ItemType Directory -Force -Path $Lab | Out-Null
$Lab = (Resolve-Path $Lab).Path
$Logs = "$Lab\logs"
$HomeDir = "$Lab\home"
$AppData = "$HomeDir\AppData\Roaming"
$LocalAppData = "$HomeDir\AppData\Local"
$Config = "$AppData\htmlclay"
$Updates = "$Config\updates"
$Install = "$Lab\install\htmlclay.exe"
$Previous = "$Install.old"
foreach ($dir in "$Lab\bin", "$Lab\keys", "$Lab\old", "$Lab\new", "$Lab\install", "$Lab\feed\update", $Logs, $Config, $LocalAppData,
    "$HomeDir\docs", "$HomeDir\docs2", "$HomeDir\loose", "$HomeDir\site") {
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
}
Say "case $Case, run folder $Lab"

$Utf8 = New-Object System.Text.UTF8Encoding $false
$Failed = New-Object System.Collections.Generic.List[string]
$Facts = New-Object System.Collections.Generic.List[string]
$Result = $null
$FeedProc = $null
$LedgerProc = $null
$RootThumb = ''
$Port = 0
$SiteHost = ''

function NowMs { [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds() }
function Fail([string]$Message) { $script:Failed.Add($Message); Say "FAILED: $Message" }
function Pass([string]$Message) { Say "ok: $Message" }
function Note([string]$Message) { $script:Facts.Add($Message) }
function Write-Text([string]$File, [string]$Text) { [IO.File]::WriteAllText($File, $Text, $script:Utf8) }

# Stop-Rehearsal ends the run from anywhere: the catch at the bottom prints the
# line and the finally there cleans up.
function Stop-Rehearsal([int]$Code, [string]$Line) {
  $script:Result = @{ Code = $Code; Line = $Line }
  throw 'rehearsal-stop'
}
function Die([string]$Message) {
  Say $Message
  Stop-Rehearsal 2 "REHEARSAL FAILED ${Case}: setup: $Message"
}
function Finish {
  if ($script:Failed.Count -eq 0) {
    Stop-Rehearsal 0 ("REHEARSAL OK ${Case}:" + ($script:Facts -join ','))
  }
  Stop-Rehearsal 1 ("REHEARSAL FAILED ${Case}: " + ($script:Failed -join '; '))
}

# Join-Arguments quotes a list the way a Windows program splits its command line.
function Join-Arguments([string[]]$List) {
  $parts = foreach ($item in $List) {
    if ($item -ne '' -and $item -notmatch '[\s"]') {
      $item
    } else {
      '"' + (($item -replace '(\\*)"', '$1$1\"') -replace '(\\+)$', '$1$1') + '"'
    }
  }
  return ($parts -join ' ')
}

# Invoke-Tool runs a program to its end and returns its exit code and output.
# PowerShell's own redirection is never used for a program here: Windows
# PowerShell 5 writes it as UTF-16 and turns stderr lines into errors.
function Invoke-Tool {
  param([string]$Exe, [string[]]$Arguments = @(), [hashtable]$Vars = @{}, [string]$Dir = '', [int]$TimeoutSec = 900)
  $info = New-Object System.Diagnostics.ProcessStartInfo
  $info.FileName = $Exe
  $info.Arguments = Join-Arguments $Arguments
  $info.UseShellExecute = $false
  $info.CreateNoWindow = $true
  $info.RedirectStandardOutput = $true
  $info.RedirectStandardError = $true
  $info.StandardOutputEncoding = $script:Utf8
  $info.StandardErrorEncoding = $script:Utf8
  if ($Dir) { $info.WorkingDirectory = $Dir }
  foreach ($name in $Vars.Keys) { $info.EnvironmentVariables[$name] = [string]$Vars[$name] }
  $proc = [System.Diagnostics.Process]::Start($info)
  $outTask = $proc.StandardOutput.ReadToEndAsync()
  $errTask = $proc.StandardError.ReadToEndAsync()
  if (-not $proc.WaitForExit($TimeoutSec * 1000)) {
    try { $proc.Kill() } catch { }
    return [pscustomobject]@{ Code = -1; Out = ''; Err = "no answer within $TimeoutSec s" }
  }
  $proc.WaitForExit()
  return [pscustomobject]@{ Code = $proc.ExitCode; Out = $outTask.Result; Err = $errTask.Result }
}

# Start-Detached starts a program that keeps running, with its output in files.
# The variables are set for the moment of the start only, so the child gets them
# and this script's own tools do not.
function Start-Detached {
  param([string]$Exe, [string[]]$Arguments = @(), [hashtable]$Vars = @{}, [string]$OutFile)
  $saved = @{}
  foreach ($name in $Vars.Keys) {
    $saved[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
    [Environment]::SetEnvironmentVariable($name, [string]$Vars[$name], 'Process')
  }
  try {
    $start = @{ FilePath = $Exe; RedirectStandardOutput = $OutFile; RedirectStandardError = "$OutFile.err"; WindowStyle = 'Hidden'; PassThru = $true }
    if ($Arguments.Count -gt 0) { $start['ArgumentList'] = (Join-Arguments $Arguments) }
    return (Start-Process @start)
  } finally {
    foreach ($name in $saved.Keys) { [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process') }
  }
}

function Read-Json([string]$File) {
  try { return (Get-Content -LiteralPath $File -Raw -ErrorAction Stop | ConvertFrom-Json) } catch { return $null }
}
# JGet reads one value of a JSON file by a dotted path, or $null.
function JGet([string]$File, [string]$Path) {
  $value = Read-Json $File
  foreach ($key in $Path.Split('.')) {
    if ($null -eq $value) { return $null }
    $prop = $value.PSObject.Properties[$key]
    if ($null -eq $prop) { return $null }
    $value = $prop.Value
  }
  return $value
}
function Get-Slugs {
  $slugs = JGet "$Config\config.json" 'siteSlugs'
  if ($null -eq $slugs) { return $null }
  return ($slugs | ConvertTo-Json -Compress)
}
function Count-Slugs {
  $slugs = JGet "$Config\config.json" 'siteSlugs'
  if ($null -eq $slugs) { return 0 }
  return @($slugs.PSObject.Properties).Count
}
# To-UnixMs reads a time the app wrote. PowerShell 7 hands back a date and
# Windows PowerShell 5 the text, with more digits of a second than .NET reads.
function To-UnixMs($Value) {
  if ($null -eq $Value) { return $null }
  if ($Value -is [DateTime]) { return ([DateTimeOffset]$Value.ToUniversalTime()).ToUnixTimeMilliseconds() }
  $text = [regex]::Replace([string]$Value, '\.(\d{3})\d+', '.$1')
  $parsed = [DateTimeOffset]::MinValue
  $styles = [Globalization.DateTimeStyles]::AssumeUniversal
  if ([DateTimeOffset]::TryParse($text, [Globalization.CultureInfo]::InvariantCulture, $styles, [ref]$parsed)) {
    return $parsed.ToUnixTimeMilliseconds()
  }
  return $null
}

function Test-Alive($ProcId) {
  if (-not $ProcId) { return $false }
  return [bool](Get-Process -Id ([int]$ProcId) -ErrorAction SilentlyContinue)
}
function Get-AppProcesses {
  @(Get-CimInstance Win32_Process -Filter "Name = 'htmlclay.exe'" | Where-Object { $_.ExecutablePath -and ($_.ExecutablePath -ieq $Install) })
}
function Get-ServiceProcesses {
  $under = "$Config\service-runtime\"
  @(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -and $_.ExecutablePath.StartsWith($under, [StringComparison]::OrdinalIgnoreCase) })
}
function Test-Port {
  $client = New-Object Net.Sockets.TcpClient
  try { $client.Connect('127.0.0.1', $Port); return $true } catch { return $false } finally { $client.Close() }
}
# Get-Loopback asks the app's port for a page under a host name, as a browser
# does for http://<name>.localhost:<port>/, and returns the whole answer.
function Get-Loopback([string]$HostName, [string]$Path) {
  $client = New-Object Net.Sockets.TcpClient
  try {
    $client.ReceiveTimeout = 5000
    $client.SendTimeout = 5000
    $client.Connect('127.0.0.1', $Port)
    $stream = $client.GetStream()
    $request = [Text.Encoding]::ASCII.GetBytes("GET $Path HTTP/1.1`r`nHost: $HostName`r`nConnection: close`r`n`r`n")
    $stream.Write($request, 0, $request.Length)
    $reader = New-Object IO.StreamReader($stream, [Text.Encoding]::UTF8)
    return $reader.ReadToEnd()
  } catch {
    return ''
  } finally {
    $client.Close()
  }
}
function Test-Site { return ($SiteHost -ne '' -and (Get-Loopback $SiteHost '/index.html').Contains('<p>site</p>')) }
function Count-ServiceStarts {
  if (-not (Test-Path "$Config\htmlclay.log")) { return 0 }
  return @(Select-String -LiteralPath "$Config\htmlclay.log" -SimpleMatch 'The live-room service started in').Count
}
function Get-RuntimeVersions {
  if (-not (Test-Path -LiteralPath "$Config\service-runtime")) { return '' }
  return ((Get-ChildItem -LiteralPath "$Config\service-runtime" -Directory | Where-Object { -not $_.Name.StartsWith('.') } |
    Sort-Object Name | ForEach-Object { $_.Name }) -join ' ')
}
function Same-File([string]$One, [string]$Two) {
  if (-not (Test-Path -LiteralPath $One) -or -not (Test-Path -LiteralPath $Two)) { return $false }
  return ((Get-FileHash -LiteralPath $One -Algorithm SHA256).Hash -eq (Get-FileHash -LiteralPath $Two -Algorithm SHA256).Hash)
}
function Keep-Logs {
  if (-not $env:TESTBOX_OUT) { return }
  $out = Join-Path $env:TESTBOX_OUT "update-rehearsal-$Case"
  New-Item -ItemType Directory -Force -Path $out | Out-Null
  Copy-Item -Recurse -Force $Logs $out -ErrorAction SilentlyContinue
  Copy-Item -Force "$Config\htmlclay.log", "$Config\config.json" $out -ErrorAction SilentlyContinue
  Copy-Item -Recurse -Force $Updates "$out\updates" -ErrorAction SilentlyContinue
}

function Cleanup {
  if ($script:LedgerProc -and -not $script:LedgerProc.HasExited) { try { $script:LedgerProc.Kill() } catch { } }
  foreach ($proc in @(Get-CimInstance Win32_Process | Where-Object {
        $_.ExecutablePath -and ($_.ExecutablePath.StartsWith("$Lab\install\", [StringComparison]::OrdinalIgnoreCase) -or
          $_.ExecutablePath.StartsWith("$Config\service-runtime\", [StringComparison]::OrdinalIgnoreCase)) })) {
    Stop-Process -Id $proc.ProcessId -Force -ErrorAction SilentlyContinue
  }
  if ($script:FeedProc -and -not $script:FeedProc.HasExited) { try { $script:FeedProc.Kill() } catch { } }
  if ($script:RootThumb) {
    Remove-Item -LiteralPath "Cert:\LocalMachine\Root\$($script:RootThumb)" -Force -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath "Cert:\LocalMachine\Root\$($script:RootThumb)") {
      Say "COULD NOT REMOVE the rehearsal's certificate from the machine's Root store: thumbprint $($script:RootThumb). Remove it by hand."
    }
  }
  Keep-Logs
}

function Stop-Ledger {
  Write-Text "$Logs\ledger.stop" ''
  for ($i = 0; $i -lt 120 -and -not $script:LedgerProc.HasExited; $i++) { Start-Sleep -Milliseconds 250 }
  $line = ''
  if (Test-Path "$Logs\ledger.out") { $line = [string](Get-Content "$Logs\ledger.out" -Tail 1) }
  if (Test-Path "$Logs\ledger.out.err") {
    Get-Content "$Logs\ledger.out.err" | Where-Object { $_ -like '`[ledger`]*' } | ForEach-Object { [Console]::Error.WriteLine($_) }
  }
  if ($line -like 'LEDGER OK*') {
    Pass $line
    Note " $line"
  } elseif ($line) {
    Fail $line
  } else {
    Fail 'the ledger printed nothing'
  }
}

# Serve-Site serves a folder through the installed binary's command line, the
# way a person would, retrying while the app is not up yet. Serving is what
# makes the app start its service. Never call it while the installed binary is
# 1.x: 1.x takes "serve" as a file to open.
function Serve-Site {
  Write-Text "$HomeDir\site\index.html" "<!DOCTYPE html><html><body><p>site</p></body></html>`n"
  $answer = $null
  for ($i = 0; $i -lt 40; $i++) {
    $answer = Invoke-Tool $Install @('serve', "$HomeDir\site", '--name', 'site') $AppVars '' 30
    if ($answer.Code -eq 0) { break }
    Start-Sleep -Milliseconds 250
  }
  return $answer
}

try {
  $BuildVars = @{ CGO_ENABLED = '0' }

  # --- builds -----------------------------------------------------------------

  Say 'building the helpers'
  foreach ($name in 'updaterehearsal', 'mkfeed', 'signfeed') {
    $built = Invoke-Tool $Go @('build', '-o', "$Lab\bin\$name.exe", "./cmd/$name") $BuildVars $Root
    Add-Content -LiteralPath "$Logs\build-helpers.log" -Value ($built.Out + $built.Err)
    if ($built.Code -ne 0) { Die "the helper $name did not build, see $Logs\build-helpers.log" }
  }
  $made = Invoke-Tool "$Lab\bin\updaterehearsal.exe" @('keys', '-seed', "$Lab\keys\seed", '-go', "$Lab\keys\keys.go")
  if ($made.Code -ne 0) { Die "could not make the throwaway feed key: $($made.Err)" }

  # The key goes in through -overlay, so no source tree is ever edited.
  function Write-Overlay([string]$Source, [string]$File) {
    $replace = @{}
    $replace["$Source\internal\update\keys.go"] = "$Lab\keys\keys.go"
    Write-Text $File (@{ Replace = $replace } | ConvertTo-Json -Compress)
  }

  $Blob = "$Root\internal\service\blobs\windows-$Arch"
  if (-not (Test-Path "$Blob.tar.gz") -or (Get-Item "$Blob.tar.gz").Length -eq 0) {
    if (-not (Test-Path "$Root\service\node_modules")) {
      Say 'installing the service''s packages (npm ci)'
      $ci = Invoke-Tool $env:ComSpec @('/c', 'npm', 'ci') @{} "$Root\service" 1800
      Write-Text "$Logs\npm-ci.log" ($ci.Out + $ci.Err)
      if ($ci.Code -ne 0) { Die "npm ci failed in service, see $Logs\npm-ci.log" }
    }
    # service/build.mjs bundles the service with esbuild, a root dev dependency. Install
    # scripts stay off: wrangler's workerd has no Windows ARM64 build and its script fails
    # there, and esbuild finds its own platform package without its script.
    if (-not (Test-Path "$Root\node_modules\esbuild")) {
      Say 'installing the root packages (npm ci)'
      $rootCi = Invoke-Tool $env:ComSpec @('/c', 'npm', 'ci', '--ignore-scripts', '--no-audit', '--no-fund') @{} $Root 1800
      Write-Text "$Logs\npm-ci-root.log" ($rootCi.Out + $rootCi.Err)
      if ($rootCi.Code -ne 0) { Die "npm ci failed at the repo root, see $Logs\npm-ci-root.log" }
    }
    Say 'building the service bundle'
    $bundle = Invoke-Tool $Node @('service/build.mjs', '--goos', 'windows', '--goarch', $Arch) @{} $Root 1800
    Write-Text "$Logs\build-service.log" ($bundle.Out + $bundle.Err)
    if ($bundle.Code -ne 0) { Die "the service did not build, see $Logs\build-service.log" }
  }

  if ($Next) {
    # The old side is this checkout too. Its service bundle is the same files
    # packed again at another compression level, so the two versions unpack
    # into two folders, as two real releases do.
    Say "building the old app (this checkout as $OldVersion) with the service inside"
    Write-Text "$Lab\keys\repack.cjs" @'
const fs = require('fs'), zlib = require('zlib'), crypto = require('crypto');
const [src, dst, sum] = process.argv.slice(2);
const original = fs.readFileSync(src);
const tar = zlib.gunzipSync(original);
let out = zlib.gzipSync(tar, { level: 1 });
if (out.equals(original)) out = zlib.gzipSync(tar, { level: 9 });
if (out.equals(original)) { console.error('the repacked service bundle is the same bytes as the original'); process.exit(1); }
fs.writeFileSync(dst, out);
fs.writeFileSync(sum, crypto.createHash('sha256').update(out).digest('hex') + ' ' + out.length + '\n');
'@
    $repack = Invoke-Tool $Node @("$Lab\keys\repack.cjs", "$Blob.tar.gz", "$Lab\keys\old-service.tar.gz", "$Lab\keys\old-service.sha256") @{} $Root 120
    if ($repack.Code -ne 0) { Die "could not repack the service bundle: $($repack.Err.Trim())" }
    $replace = @{}
    $replace["$Root\internal\update\keys.go"] = "$Lab\keys\keys.go"
    $replace["$Blob.tar.gz"] = "$Lab\keys\old-service.tar.gz"
    $replace["$Blob.sha256"] = "$Lab\keys\old-service.sha256"
    Write-Text "$Lab\keys\overlay-old.json" (@{ Replace = $replace } | ConvertTo-Json -Compress)
    $old = Invoke-Tool $Go @('build', '-overlay', "$Lab\keys\overlay-old.json", '-tags', 'service_bundle', '-ldflags', "-X main.version=$OldVersion -X main.updateChannel=stable",
      '-o', "$Lab\old\htmlclay.exe", './cmd/htmlclay') $BuildVars $Root
    Write-Text "$Logs\build-old.log" ($old.Out + $old.Err)
    if ($old.Code -ne 0) { Die "the old app did not build, see $Logs\build-old.log" }
  } else {
    $OldSrc = $env:HTMLCLAY_REHEARSAL_OLD_SRC
    if (-not $OldSrc) {
      $OldSrc = "$Lab\old-src"
      Say "cloning the public $OldTag"
      $clone = Invoke-Tool $Git @('clone', '--quiet', '--depth', '1', '--branch', $OldTag, $PublicUrl, $OldSrc)
      Write-Text "$Logs\clone.log" ($clone.Out + $clone.Err)
      if ($clone.Code -ne 0) { Die "could not clone $OldTag from $PublicUrl" }
    }
    $OldSrc = (Resolve-Path $OldSrc).Path
    if (-not (Test-Path "$OldSrc\internal\update\keys.go")) { Die "$OldSrc is not an HTML Clay source tree" }
    if (Test-Path "$OldSrc\.git") {
      $dirty = (Invoke-Tool $Git @('-C', $OldSrc, 'status', '--porcelain')).Out.Trim()
      if ($dirty) { Die "$OldSrc differs from its tag: $dirty" }
    }

    Say "building the old app ($OldTag as $OldVersion)"
    Write-Overlay $OldSrc "$Lab\keys\overlay-old.json"
    $old = Invoke-Tool $Go @('build', '-overlay', "$Lab\keys\overlay-old.json", '-ldflags', "-X main.version=$OldVersion -X main.updateChannel=stable",
      '-o', "$Lab\old\htmlclay.exe", './cmd/htmlclay') $BuildVars $OldSrc
    Write-Text "$Logs\build-old.log" ($old.Out + $old.Err)
    if ($old.Code -ne 0) { Die "the old app did not build, see $Logs\build-old.log" }
  }

  $BuildVersion = $NewVersion
  if ($Case -eq 'rollback-version') { $BuildVersion = '9.0.2' }
  Say "building the new app (this checkout as $BuildVersion) with the service inside"
  Write-Overlay $Root "$Lab\keys\overlay-new.json"
  $new = Invoke-Tool $Go @('build', '-overlay', "$Lab\keys\overlay-new.json", '-tags', 'service_bundle', '-ldflags',
    "-X main.version=$BuildVersion -X main.updateChannel=stable", '-o', "$Lab\new\htmlclay.exe", './cmd/htmlclay') $BuildVars $Root
  Write-Text "$Logs\build-new.log" ($new.Out + $new.Err)
  if ($new.Code -ne 0) { Die "the new app did not build, see $Logs\build-new.log" }
  $info = (Invoke-Tool "$Lab\new\htmlclay.exe" @('update-info') @{} '' 30).Out.Trim()
  if ($info.Contains("`"version`":`"$BuildVersion`"") -and $info.Contains('"service":true')) {
    Say "the new app: $info"
  } else {
    Die "the new app does not carry the service: $info"
  }
  if ($Case -eq 'rollback-early') {
    # Any program that exits at once will do: the helper prints its usage and exits 2.
    Copy-Item "$Lab\bin\updaterehearsal.exe" "$Lab\new\artifact.exe"
  } else {
    Copy-Item "$Lab\new\htmlclay.exe" "$Lab\new\artifact.exe"
  }

  # --- the feed ---------------------------------------------------------------

  $FeedProc = Start-Detached "$Lab\bin\updaterehearsal.exe" @('feed', '-dir', "$Lab\feed", '-cert', "$Lab\keys\cert.cer", '-addr', "$Lab\keys\addr") @{} "$Logs\feed.log"
  for ($i = 0; $i -lt 50 -and -not (Test-Path "$Lab\keys\addr"); $i++) { Start-Sleep -Milliseconds 100 }
  $FeedAddr = ''
  if (Test-Path "$Lab\keys\addr") { $FeedAddr = ([string](Get-Content "$Lab\keys\addr" -Raw)).Trim() }
  if (-not $FeedAddr) { Die 'the feed server did not start' }
  $RootThumb = (Import-Certificate -FilePath "$Lab\keys\cert.cer" -CertStoreLocation 'Cert:\LocalMachine\Root').Thumbprint
  Say "the feed is at https://$FeedAddr, its certificate $RootThumb is trusted until this run ends"

  $Art = "$Lab\feed\update\$NewVersion"
  New-Item -ItemType Directory -Force -Path $Art | Out-Null
  foreach ($target in 'linux-amd64', 'linux-arm64', 'windows-amd64', 'windows-arm64') {
    $ext = '.gz'
    if ($target -like 'windows-*') { $ext = '.exe' }
    Write-Text "$Art\htmlclay-$NewVersion-$target$ext" "not built for the rehearsal`n"
  }
  Copy-Item -Force "$Lab\new\artifact.exe" "$Art\htmlclay-$NewVersion-windows-$Arch.exe"
  $SourceSha = (Invoke-Tool $Git @('-C', $Root, 'rev-parse', 'HEAD')).Out.Trim()
  if ($SourceSha -notmatch '^[0-9a-f]{40}$') { $SourceSha = '0' * 40 }
  $payload = Invoke-Tool "$Lab\bin\mkfeed.exe" @('payload', '-version', $NewVersion, '-source-sha', $SourceSha, '-base-url', "https://$FeedAddr/update", $Art)
  if ($payload.Code -ne 0) { Die "could not build the feed payload: $($payload.Err)" }
  Write-Text "$Lab\keys\payload.json" $payload.Out
  $seed = ([string](Get-Content "$Lab\keys\seed" -Raw)).Trim()
  $signed = Invoke-Tool "$Lab\bin\signfeed.exe" @('sign', '-key-id', 'rehearsal', "$Lab\keys\payload.json") @{ HTMLCLAY_UPDATE_SEED = $seed }
  if ($signed.Code -ne 0) { Die "could not sign the feed: $($signed.Err)" }
  Write-Text "$Lab\feed\update\stable.json" $signed.Out

  # --- the old app, with three documents open ---------------------------------

  $AppVars = @{
    USERPROFILE             = $HomeDir
    APPDATA                 = $AppData
    LOCALAPPDATA            = $LocalAppData
    HTMLCLAY_LAB            = $Lab
    HTMLCLAY_LOCAL_USERDATA = "$HomeDir\no-hyperclay-local"
    HTMLCLAY_UPDATE_FEED    = "https://$FeedAddr/update/stable.json"
  }

  $listener = New-Object Net.Sockets.TcpListener ([Net.IPAddress]::Loopback, 0)
  $listener.Start()
  $Port = $listener.LocalEndpoint.Port
  $listener.Stop()

  $Documents = @("$HomeDir\docs\a.htmlclay", "$HomeDir\docs2\b.htmlclay", "$HomeDir\loose\c.htmlclay")
  Write-Text $Documents[0] "<!DOCTYPE html><html><body><p>rehearsal</p></body></html>`n"
  Write-Text $Documents[1] "<!DOCTYPE html><html><body><p>second</p></body></html>`n"
  Write-Text $Documents[2] "<!DOCTYPE html><html><body><p>loose</p></body></html>`n"
  # A trusted folder with no identity is given one when the app loads its config.
  $settings = @{ port = $Port; workspaceFolders = @(@{ path = "$HomeDir\docs" }, @{ path = "$HomeDir\docs2" }) }
  Write-Text "$Config\config.json" ($settings | ConvertTo-Json -Depth 5 -Compress)
  Copy-Item "$Lab\old\htmlclay.exe" $Install

  Say "starting the old app on port $Port"
  $OldApp = Start-Detached $Install @() $AppVars "$Logs\app.out"
  $OldPid = $OldApp.Id
  for ($i = 0; $i -lt 100 -and -not (Test-Port); $i++) { Start-Sleep -Milliseconds 200 }
  if (-not (Test-Port)) { Die "the old app never served port $Port, see $Logs\app.out" }
  if (-not (Test-Alive $OldPid)) { Die 'the old app started and exited' }
  $StartedAt = Get-Date

  foreach ($document in $Documents) {
    $opened = Invoke-Tool $Install @($document) $AppVars '' 30
    Add-Content -LiteralPath "$Logs\open.out" -Value ($opened.Out + $opened.Err)
    if ($opened.Code -ne 0) { Die "the old app did not take $document (exit $($opened.Code))" }
  }
  for ($i = 0; $i -lt 80 -and (Count-Slugs) -lt 3; $i++) { Start-Sleep -Milliseconds 250 }
  if ((Count-Slugs) -lt 3) { Die "the old app opened fewer than three sites: $(Get-Slugs)" }
  $SlugsBefore = Get-Slugs
  # 1.x gives its first site the name the dashboard takes in 2.0; 2.0 never does.
  if (-not $Next -and -not $SlugsBefore.Contains(':"htmlclay"')) { Die "no first site named htmlclay: $SlugsBefore" }
  Say "sites before the update: $SlugsBefore"

  # In the next cases the old version is 2.0: it serves a folder before the
  # update, so its service runs and its unpacked copy is on disk when the new
  # version starts.
  $OldRuntime = ''
  if ($Next) {
    $served = Serve-Site
    if ($served.Code -ne 0) { Die "the old app could not serve a folder: $(($served.Out + $served.Err).Trim())" }
    $match = [regex]::Match($served.Out, 'http://([^/ ]+)/')
    if ($match.Success) { $SiteHost = $match.Groups[1].Value }
    for ($i = 0; $i -lt 240 -and (Count-ServiceStarts) -lt 1; $i++) { Start-Sleep -Milliseconds 500 }
    if ((Count-ServiceStarts) -lt 1) { Die "the old app's service never started, see $Config\htmlclay.log" }
    for ($i = 0; $i -lt 20 -and -not (Test-Site); $i++) { Start-Sleep -Milliseconds 500 }
    if (-not (Test-Site)) { Die "the old app does not answer its served folder at '$SiteHost'" }
    $OldRuntime = Get-RuntimeVersions
    Say "the old app serves $SiteHost, its service runs from service-runtime\$OldRuntime"
  }

  $LedgerMode = 'rollback'
  if ($Case -eq 'first') { $LedgerMode = 'first' }
  if ($Next) { $LedgerMode = 'next' }
  # After a quit the tabs' save tokens are gone (they live in the running app's
  # memory), so next-start asks only that every save acked before the quit survives.
  if ($Case -eq 'next-start') { $LedgerMode = 'quit' }
  $ledgerArgs = @("$Root\scripts\upgrade-ledger.mjs", '--port', "$Port", '--config', $Config, '--out', $Logs, '--mode', $LedgerMode,
    '--stop-file', "$Logs\ledger.stop", '--times', $Logs, '--home', $HomeDir) + $Documents
  $LedgerProc = Start-Detached $Node $ledgerArgs @{} "$Logs\ledger.out"
  for ($i = 0; $i -lt 120 -and -not (Test-Path "$Logs\ledger-ready"); $i++) { Start-Sleep -Milliseconds 250 }
  if (-not (Test-Path "$Logs\ledger-ready")) { Die "the ledger's tabs never saved, see $Logs\ledger.out and $Logs\ledger.out.err" }

  # --- the update -------------------------------------------------------------

  # A browser tab's requests: the host answers a first load of / with a ticket
  # redirect, /enter spends the ticket and sets the session cookie, and the page
  # carries the token every action needs (internal/dashboard/server.go). curl.exe
  # maps *.localhost to loopback itself; --resolve makes it explicit.
  $PageToken = ''
  function Invoke-Page([string[]]$CurlArgs) {
    $base = @('-sS', '--max-time', '10', '--resolve', "htmlclay.localhost:${Port}:127.0.0.1", '-b', "$Logs\page.cookies")
    return Invoke-Tool "$env:SystemRoot\System32\curl.exe" ($base + $CurlArgs) @{} '' 30
  }
  function Page-SignIn {
    $r = Invoke-Page @('-L', '-c', "$Logs\page.cookies", '-o', "$Logs\page.html", "http://htmlclay.localhost:$Port/")
    if ($r.Code -ne 0 -or -not (Test-Path -LiteralPath "$Logs\page.html")) { return $false }
    $m = [regex]::Match([string](Get-Content -LiteralPath "$Logs\page.html" -Raw), '<meta name="workspace-token" content="([^"]*)"')
    if (-not $m.Success -or -not $m.Groups[1].Value) { return $false }
    $script:PageToken = $m.Groups[1].Value
    return $true
  }
  # Page-Action sends what a page button sends, and is true when the host ran it
  # (it answers with the new snapshot) rather than refusing it (a code).
  function Page-Action([string]$Type) {
    Write-Text "$Logs\action-body.json" (@{ id = [guid]::NewGuid().ToString(); type = $Type } | ConvertTo-Json -Compress)
    $r = Invoke-Page @('-H', "X-Workspace-Token: $script:PageToken", '-H', 'Content-Type: application/json', '-X', 'POST',
      '--data-binary', "@$Logs\action-body.json", "http://htmlclay.localhost:$Port/api/app/actions")
    Write-Text "$Logs\action-$Type.json" $r.Out
    try { $d = $r.Out | ConvertFrom-Json } catch { return $false }
    if ($null -eq $d) { return $false }
    $names = @($d.PSObject.Properties.Name)
    return (($names -contains 'snapshot') -and -not ($names -contains 'code') -and -not (($names -contains 'pending') -and $d.pending))
  }
  # The page's Update button shows when the snapshot has an update ready, none
  # finishing, and update.start allowed.
  Write-Text "$Lab\keys\offered.cjs" @'
const s = JSON.parse(require('fs').readFileSync(process.argv[2], 'utf8'));
const u = s.update || {};
process.exit(u.ready === process.argv[3] && !u.finishing && ((s.host || {}).can || {})['update.start'] === true ? 0 : 1);
'@
  function Wait-UpdateOffered {
    for ($i = 0; $i -lt 120; $i++) {
      $r = Invoke-Page @('-H', "X-Workspace-Token: $script:PageToken", '-o', "$Logs\page-snapshot.json", "http://htmlclay.localhost:$Port/api/app/snapshot")
      if ($r.Code -eq 0 -and (Invoke-Tool $Node @("$Lab\keys\offered.cjs", "$Logs\page-snapshot.json", $NewVersion) @{} '' 30).Code -eq 0) {
        Say "the page offers Update to $NewVersion after $i s"
        return $true
      }
      Start-Sleep -Seconds 1
    }
    return $false
  }

  if ($Case -eq 'next') {
    # 2.0 takes update-now. "checking" means it is fetching the version now.
    Say "asking the old app to update to $NewVersion with update-now"
    $answer = ''
    for ($i = 0; $i -lt 240; $i++) {
      $r = Invoke-Tool $Install @('update-now', '--version', $NewVersion) $AppVars '' 30
      $answer = [string](@((($r.Out + "`n" + $r.Err).Trim()) -split "`r?`n")[-1]).Trim()
      if ($answer -eq 'accepted') { break }
      Start-Sleep -Milliseconds 500
    }
    if ($answer -ne 'accepted') {
      Fail "update-now was never accepted: $answer"
      Finish
    }
    $ClickMs = NowMs
    Say "update-now accepted after $([int]((Get-Date) - $StartedAt).TotalSeconds) s"
  } elseif ($Case -in 'next-start', 'next-button') {
    if (-not (Page-SignIn)) {
      Fail "could not sign in as a browser tab, see $Logs\page.html"
      Finish
    }
    # update.check is refused as not yet until the updater has started.
    $checked = $false
    for ($i = 0; $i -lt 30 -and -not $checked; $i++) {
      $checked = Page-Action 'update.check'
      if (-not $checked) { Start-Sleep -Seconds 1 }
    }
    if (-not $checked) {
      Fail "Check for updates was refused: $([string](Get-Content -LiteralPath "$Logs\action-update.check.json" -Raw))"
      Finish
    }
    Say "pressed Check for updates after $([int]((Get-Date) - $StartedAt).TotalSeconds) s"
    if (-not (Wait-UpdateOffered)) {
      Fail "the page never offered $NewVersion, see $Logs\page-snapshot.json"
      Finish
    }
    if ($Case -eq 'next-button') {
      if (-not (Page-Action 'update.start')) {
        Fail "Update was refused: $([string](Get-Content -LiteralPath "$Logs\action-update.start.json" -Raw))"
        Finish
      }
      $ClickMs = NowMs
      Pass "pressed Update after $([int]((Get-Date) - $StartedAt).TotalSeconds) s"
    } else {
      if (Test-Path -LiteralPath "$Updates\waiting.json") { Pass 'the downloaded update waits in waiting.json' } else { Fail 'no waiting.json after the download' }
      # Closing the tray's window is the path its own Quit takes (fyne systray).
      $closed = Invoke-Tool "$Lab\bin\updaterehearsal.exe" @('close', '-pid', "$OldPid", '-wait', '20') @{} '' 60
      Write-Text "$Logs\close.out" ($closed.Out + $closed.Err)
      if ($closed.Code -ne 0) {
        Fail "could not quit the old app: $($closed.Err.Trim())"
        Finish
      }
      for ($i = 0; $i -lt 100 -and (Test-Alive $OldPid); $i++) { Start-Sleep -Milliseconds 200 }
      if (Test-Alive $OldPid) {
        Fail 'the old app did not quit'
        Finish
      }
      Pass 'the old app quit with the update still waiting'
      $ClickMs = NowMs
      $again = Start-Detached $Install @($Documents[0]) $AppVars "$Logs\start-again.out"
      # Windows PowerShell reports ExitCode only for a process whose handle was read while it ran.
      $null = $again.Handle
      $null = $again.WaitForExit(20000)
      if (-not $again.HasExited) {
        Fail 'starting again ran the old version instead of installing the waiting update'
      } elseif ($again.ExitCode -eq 0) {
        Pass "starting again handed over to the update and exited after $((NowMs) - $ClickMs) ms"
      } else {
        Fail "starting again exited $($again.ExitCode), see $Logs\start-again.out"
      }
    }
  } else {
    Say "waiting for the old app to download $NewVersion (its first check runs 60 s after it starts)"
    $staged = $false
    for ($i = 0; $i -lt 300 -and -not $staged; $i++) {
      $staged = (Test-Path "$Config\htmlclay.log") -and [bool](Select-String -LiteralPath "$Config\htmlclay.log" -SimpleMatch "Update $NewVersion is downloaded and staged" -Quiet)
      if (-not $staged) { Start-Sleep -Milliseconds 500 }
    }
    if (-not $staged) {
      $tail = ''
      if (Test-Path "$Config\htmlclay.log") { $tail = (Get-Content "$Config\htmlclay.log" -Tail 5) -join ' | ' }
      # A wrong path in an overlay builds without a word, with the real feed key
      # inside. The log then says the feed's key is not one the app trusts.
      Fail "the old app never staged the update (if its log says the feed's key is not trusted, the overlay did not match keys.go): $tail"
      Finish
    }
    # The tray row changes a moment after the log line. The click runs whatever the row means then.
    Start-Sleep -Seconds 3
    # 1.x refuses Restart to update while a save is in flight ("not applied ... requests were
    # still in flight"), and the three tabs save every second, so a refused click is clicked
    # again, as a person would.
    $refusals = { @(Select-String -LiteralPath "$Config\htmlclay.log" -SimpleMatch 'not applied, still running' -ErrorAction SilentlyContinue).Count }
    for ($try = 1; $try -le 5; $try++) {
      $before = & $refusals
      $click = Invoke-Tool "$Lab\bin\updaterehearsal.exe" @('click', '-pid', "$OldPid", '-id', "$UpdateRowId", '-wait', '20') @{} '' 60
      Write-Text "$Logs\click.out" ($click.Out + $click.Err)
      if ($click.Code -ne 0) {
        Fail "the click did not reach the old app's tray: $($click.Err.Trim())"
        Finish
      }
      $ClickMs = NowMs
      Say "$($click.Out.Trim()) after $([int]((Get-Date) - $StartedAt).TotalSeconds) s"
      for ($i = 0; $i -lt 80 -and -not (Test-Path -LiteralPath $Previous) -and -not (JGet "$Updates\state.json" 'phase') -and (& $refusals) -le $before; $i++) {
        Start-Sleep -Milliseconds 100
      }
      if ((& $refusals) -le $before) { break }
      Say "the old app refused the update while a save was in flight (try $try); clicking again"
      Start-Sleep -Milliseconds 700
    }
  }

  # The moment the binary changed, which the ledger reads to tell the old version's saves from the new one's.
  for ($i = 0; $i -lt 1500 -and -not (Test-Path -LiteralPath $Previous); $i++) { Start-Sleep -Milliseconds 20 }
  if (Test-Path -LiteralPath $Previous) {
    Write-Text "$Logs\version-changed.ms" "$(NowMs)`n"
  } elseif (-not (JGet "$Updates\state.json" 'phase')) {
    Fail "nothing happened within 30 s of the click on row $UpdateRowId. If the old app's rows are in another order, pass -UpdateRowId"
    Finish
  }

  # In rollback-version the new version runs for the supervisor's whole 20 s.
  # Serve a folder in that window so its service runs, and the checks below can
  # see that the rollback takes the service down with the app.
  if ($Case -eq 'rollback-version') {
    $served = Serve-Site
    $judged = @()
    if ($served.Code -eq 0) {
      for ($i = 0; $i -lt 60 -and $judged.Count -eq 0; $i++) {
        $judged = @(Get-ServiceProcesses)
        if ($judged.Count -eq 0) { Start-Sleep -Milliseconds 250 }
      }
    }
    $unpacking = @(Get-ChildItem -LiteralPath "$Config\service-runtime" -Directory -Force -Filter '.extract-*' -ErrorAction SilentlyContinue)
    if ($judged.Count -gt 0) {
      Pass "the judged version's service was running as pid $($judged[0].ProcessId) ($($served.Out.Trim()))"
    } elseif ($served.Code -eq 0 -and $unpacking.Count -gt 0) {
      # A fresh 70 MB unpack can outlast the supervisor's 20 s on Windows (the scanner reads every
      # file written), and nothing runs from service-runtime until it ends. The rollback then has
      # no service to stop; the next 2.0 start removes the partial unpack (internal/service/extract.go, prune).
      Note " the judged version was still unpacking its service ($($unpacking[0].Name)) when the rollback came"
    } else {
      Fail "the judged version's service never ran, so the rollback did not have to stop it: $(($served.Out + $served.Err).Trim())"
    }
  }

  $Phase = ''
  for ($i = 0; $i -lt 300; $i++) {
    $Phase = [string](JGet "$Updates\state.json" 'phase')
    if ($Phase -in 'done', 'rolledBack', 'rollbackFailed') { break }
    Start-Sleep -Milliseconds 300
  }
  $VerdictMs = NowMs
  Say "the supervisor's verdict: $Phase after $([int](($VerdictMs - $ClickMs) / 1000)) s"
  if (Test-Path "$Updates\supervisor.log") { Get-Content "$Updates\supervisor.log" | ForEach-Object { [Console]::Error.WriteLine("[supervisor] $_") } }

  if ($Case -eq 'first' -or $Next) {
    if ($Phase -eq 'done') { Pass 'state.json says done' } else { Fail "state.json says '$Phase', not done" }
    $HVersion = [string](JGet "$Updates\health.json" 'version')
    $HPhase = [string](JGet "$Updates\health.json" 'phase')
    $HPid = JGet "$Updates\health.json" 'pid'
    $HTxid = [string](JGet "$Updates\health.json" 'txid')
    $STxid = [string](JGet "$Updates\state.json" 'txid')
    if ($HVersion -eq $NewVersion -and $HPhase -eq 'serving' -and $STxid -and $HTxid -eq $STxid) {
      Pass "health.json: $NewVersion serving for this update"
    } else {
      Fail "health.json says version '$HVersion' phase '$HPhase' txid '$HTxid' (state txid '$STxid')"
    }
    if ((Test-Alive $HPid) -and $HPid -ne $OldPid -and -not (Test-Alive $OldPid)) {
      Pass "the new process $HPid is running and the old one ($OldPid) is gone"
    } else {
      Fail "the serving pid '$HPid' is not a new live process, or the old one ($OldPid) is still running"
    }
    if (Same-File $Install "$Lab\new\htmlclay.exe") { Pass 'the installed binary is the new build' } else { Fail 'the installed binary is not the new build' }
    if (Same-File $Previous "$Lab\old\htmlclay.exe") { Pass 'the old build is kept beside it as htmlclay.exe.old' } else { Fail "$Previous is not the old build" }
    $ServingMs = To-UnixMs (JGet "$Updates\health.json" 'time')
    if ($null -ne $ServingMs) {
      Note " $OldVersion -> $NewVersion serving $($ServingMs - $ClickMs) ms after the click"
    } else {
      Fail 'health.json holds no time'
    }

    $WantStarts = 2
    if ($Case -eq 'first') {
      # Nothing 1.x leaves behind needs the service, so serve a folder: that is what starts it.
      $served = Serve-Site
      if ($served.Code -eq 0) {
        Pass "htmlclay serve: $($served.Out.Trim())"
      } else {
        Fail "htmlclay serve failed: $(($served.Out + $served.Err).Trim())"
      }
      $match = [regex]::Match($served.Out, 'http://([^/ ]+)/')
      if ($match.Success) { $SiteHost = $match.Groups[1].Value }
      $WantStarts = 1
    }
    for ($i = 0; $i -lt 240 -and (Count-ServiceStarts) -lt $WantStarts; $i++) { Start-Sleep -Milliseconds 500 }
    if ((Count-ServiceStarts) -ge $WantStarts) {
      $line = [string](@(Select-String -LiteralPath "$Config\htmlclay.log" -SimpleMatch 'The live-room service started in')[-1].Line)
      $line = $line.Substring($line.IndexOf('The live-room service ') + 22)
      Pass "the new version's service runs: $line"
      Note " the service $line"
    } else {
      $tail = (@(Select-String -LiteralPath "$Config\htmlclay.log" -Pattern 'live-room') | Select-Object -Last 3 | ForEach-Object { $_.Line }) -join ' | '
      Fail "the new version's service never started: $tail"
    }
    for ($i = 0; $i -lt 20 -and -not (Test-Site); $i++) { Start-Sleep -Milliseconds 500 }
    if (Test-Site) { Pass "the served folder answers at $SiteHost" } else { Fail "the served folder does not answer at '$SiteHost'" }
    if (Test-Port) { Pass "port $Port answers" } else { Fail "port $Port does not answer" }
    if (-not (Test-Alive $HPid)) { Fail 'the new app died after it reported serving' }

    if ($Case -eq 'next-start') {
      if (Test-Path -LiteralPath "$Updates\waiting.json") { Fail 'waiting.json is still there after the update' } else { Pass 'waiting.json is gone' }
      # The "Opening" line comes before the open runs; a "Serving <name> at" line after it is the
      # open succeeding (cmd/htmlclay/open.go).
      $opened = $false
      $name = Split-Path -Leaf $Documents[0]
      for ($i = 0; $i -lt 40 -and -not $opened; $i++) {
        $lines = @(Get-Content -LiteralPath "$Config\htmlclay.log" -ErrorAction SilentlyContinue)
        $at = -1
        for ($j = $lines.Count - 1; $j -ge 0; $j--) { if ($lines[$j].Contains("Opening file from before the update: $($Documents[0])")) { $at = $j; break } }
        if ($at -ge 0) { $opened = [bool](@($lines[($at + 1)..($lines.Count - 1)] | Where-Object { $_.Contains("Serving $name at http") }).Count) }
        if (-not $opened) { Start-Sleep -Milliseconds 250 }
      }
      if ($opened) { Pass 'the document HTML Clay was started with opened after the update' } else { Fail "the new version never opened $($Documents[0])" }
      if (Test-Path -LiteralPath "$Config\open-after-update.json") { Fail 'open-after-update.json is still there' } else { Pass 'open-after-update.json is gone' }
    }
    if ($Next) {
      Start-Sleep -Seconds 8
      Stop-Ledger
      $NowRuntime = Get-RuntimeVersions
      if ($NowRuntime -and $NowRuntime -ne $OldRuntime -and -not $NowRuntime.Contains(' ')) {
        Pass "the old version's service copy was removed after the verdict (service-runtime holds $NowRuntime)"
      } else {
        Fail "service-runtime holds '$NowRuntime' after the verdict; the old copy was $OldRuntime"
      }
      if ([string](Get-Slugs) -eq $SlugsBefore) { Pass 'the sites are the ones from before the update' } else { Fail "the sites changed across the update: $(Get-Slugs)" }
      Finish
    }

    # The first site moves only after the verdict, and the ledger needs saves on both sides of it.
    Start-Sleep -Seconds 30
    Stop-Ledger
    $SlugsAfter = [string](Get-Slugs)
    if ($SlugsAfter.Contains(':"htmlclay"') -or -not $SlugsAfter) {
      Fail "the first site still has the dashboard's name after the update: $SlugsAfter"
    } else {
      Pass "the first site was renamed after the verdict: $SlugsAfter"
    }
    Finish
  }

  # The two rollback cases.
  if ($Phase -eq 'rolledBack') { Pass 'state.json says rolledBack' } else { Fail "state.json says '$Phase', not rolledBack" }
  $FVersion = [string](JGet "$Updates\state.json" 'failed.version')
  $FReason = [string](JGet "$Updates\state.json" 'failed.reason')
  if ($FVersion -eq $NewVersion) { Pass "the failure is recorded: $FReason" } else { Fail "state.json records no failure of $NewVersion" }
  Note " rolled back $([int](($VerdictMs - $ClickMs) / 1000)) s after the click ($FReason)"

  $Back = $null
  for ($i = 0; $i -lt 75 -and -not $Back; $i++) {
    $Back = Get-AppProcesses | Where-Object { $_.ProcessId -ne $OldPid } | Select-Object -First 1
    if (-not $Back -or -not (Test-Port)) {
      $Back = $null
      Start-Sleep -Milliseconds 200
    }
  }
  if ($Back) { Pass "the old app is back as pid $($Back.ProcessId) on port $Port" } else { Fail "the old app did not come back on port $Port within 15 s" }
  if (Same-File $Install "$Lab\old\htmlclay.exe") { Pass 'the installed binary is the old build again' } else { Fail 'the installed binary is not the old build' }
  if (-not (Test-Path -LiteralPath $Previous)) { Pass 'no previous binary is left beside the install' } else { Fail "$Previous is still there" }

  $HTxid = [string](JGet "$Updates\health.json" 'txid')
  $STxid = [string](JGet "$Updates\state.json" 'txid')
  $HPhase = [string](JGet "$Updates\health.json" 'phase')
  if ($STxid -and $HTxid -eq $STxid -and $HPhase -eq 'serving') {
    Fail 'the new version reported serving and was rolled back anyway'
  } else {
    Pass 'the new version never reported serving for this update'
  }

  Start-Sleep -Seconds 10
  $left = @(Get-ServiceProcesses)
  if ($left.Count -eq 0) {
    Pass 'no service process of the new version is left'
  } else {
    Fail "a service process of the new version is still running: $(($left | ForEach-Object { $_.ProcessId }) -join ' ')"
  }
  $configText = [string](Get-Content -LiteralPath "$Config\config.json" -Raw)
  if ($configText.Contains('"servedFolders"') -and $configText.Contains('site')) {
    Fail 'the rolled-back version wrote its served folder into the config the old app reads'
  } else {
    Pass 'nothing the judged version did reached the config'
  }
  $SlugsAfter = [string](Get-Slugs)
  if ($SlugsAfter -eq $SlugsBefore) {
    Pass 'the sites are the ones from before the update'
  } else {
    Fail "the sites changed across a rolled-back update: $SlugsBefore became $SlugsAfter"
  }
  Stop-Ledger
  Finish
} catch {
  if (-not $Result) {
    Say "the script itself failed: $($_.Exception.Message)"
    Say $_.ScriptStackTrace
    $Result = @{ Code = 2; Line = "REHEARSAL FAILED ${Case}: setup: $($_.Exception.Message)" }
  }
} finally {
  Cleanup
}

Write-Output $Result.Line
exit $Result.Code
