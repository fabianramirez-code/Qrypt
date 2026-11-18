<#
.SYNOPSIS
Build and package a Windows release of Qrypt.
.DESCRIPTION
Uses Go from PATH or downloads and verifies an official local Go zip. Runs vet and
unit tests before packaging the executable and complete dependency licenses.
.PARAMETER Version
Release version, for example v0.1.0 or v0.1.0-rc1.
.EXAMPLE
.\build.ps1 -Version v0.1.0
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9][A-Za-z0-9.-]*)?$')]
    [string]$Version
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot

function Get-Sha256([string]$Path) {
    $sha = [System.Security.Cryptography.SHA256]::Create()
    $stream = [System.IO.File]::OpenRead($Path)
    try { return ([BitConverter]::ToString($sha.ComputeHash($stream))).Replace('-', '').ToLowerInvariant() }
    finally { $stream.Dispose(); $sha.Dispose() }
}

function Install-LocalGo([string]$Root) {
    $releases = Invoke-RestMethod -Uri 'https://go.dev/dl/?mode=json'
    $stable = $releases | Where-Object { $_.stable -eq $true } | Select-Object -First 1
    if ($null -eq $stable) { throw 'No stable Go release in the official index.' }
    $archive = $stable.files | Where-Object { $_.filename -eq ($stable.version + '.windows-amd64.zip') } | Select-Object -First 1
    if ($null -eq $archive -or $archive.sha256 -notmatch '^[0-9a-fA-F]{64}$') { throw 'No verified windows-amd64 Go zip in the official index.' }
    $download = [System.IO.Path]::GetTempFileName()
    try {
        Invoke-WebRequest -UseBasicParsing -Uri ('https://go.dev/dl/' + $archive.filename) -OutFile $download
        if ((Get-Sha256 $download) -ne $archive.sha256.ToLowerInvariant()) { throw 'Official Go zip SHA-256 mismatch.' }
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        [System.IO.Compression.ZipFile]::ExtractToDirectory($download, $Root)
        $executable = Join-Path $Root 'go\bin\go.exe'
        if (-not [System.IO.File]::Exists($executable)) { throw 'Official Go zip has no go.exe.' }
        return $executable
    } finally {
        [System.IO.File]::Delete($download)
    }
}

# The symbols-only Nerd Font, for the optional icon set. Pinned by version and SHA-256:
# a different archive is refused, never used.
$fontVersion = 'v3.5.1'
$fontSha256 = 'fdca3682534f6f65e1ccb2345b0362ccf67d9b8eca7c8025330946e93e2473bc'
$fontFile = 'SymbolsNerdFontMono-Regular.ttf'

function Get-SymbolsFont([string]$Directory) {
    $download = [System.IO.Path]::GetTempFileName()
    try {
        $uri = 'https://github.com/ryanoasis/nerd-fonts/releases/download/' + $script:fontVersion + '/NerdFontsSymbolsOnly.zip'
        Invoke-WebRequest -UseBasicParsing -Uri $uri -OutFile $download
        if ((Get-Sha256 $download) -ne $script:fontSha256) { throw 'Symbols font archive SHA-256 mismatch.' }
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $archive = [System.IO.Compression.ZipFile]::OpenRead($download)
        try {
            foreach ($name in @($script:fontFile, 'LICENSE')) {
                $entry = $archive.GetEntry($name)
                if ($null -eq $entry) { throw ('Symbols font archive has no ' + $name) }
                [System.IO.Compression.ZipFileExtensions]::ExtractToFile($entry, (Join-Path $Directory $name), $true)
            }
        } finally { $archive.Dispose() }
    } finally {
        [System.IO.File]::Delete($download)
    }
}

function Invoke-Go([string[]]$GoArguments) {
    & $script:goExe @GoArguments
    if ($LASTEXITCODE -ne 0) { throw ('go ' + ($GoArguments -join ' ') + ' failed (exit ' + $LASTEXITCODE + ').') }
}

function Add-Text([System.IO.Stream]$Stream, [string]$Text) {
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($Text)
    $Stream.Write($bytes, 0, $bytes.Length)
}

function Add-License([System.IO.Stream]$Stream, [string]$Heading, [string]$Path) {
    if (-not [System.IO.File]::Exists($Path)) { throw ('Missing license for ' + $Heading) }
    Add-Text $Stream ("===== $Heading =====`n")
    $inputFile = [System.IO.File]::OpenRead($Path)
    try { $inputFile.CopyTo($Stream) } finally { $inputFile.Dispose() }
    Add-Text $Stream "`n`n"
}

function Get-PrivateMarkers([string]$Root, [string]$Go) {
    $markers = New-Object 'System.Collections.Generic.List[string]'
    $profile = $env:USERPROFILE
    $user = $env:USERNAME
    $cache = (& $Go env GOMODCACHE)
    if ($LASTEXITCODE -ne 0) { throw 'Could not determine Go module cache.' }
    $gopath = (& $Go env GOPATH)
    if ($LASTEXITCODE -ne 0) { throw 'Could not determine GOPATH.' }
    foreach ($value in @($profile, $Root, $cache, $gopath)) {
        if (-not [string]::IsNullOrWhiteSpace($value)) {
            $markers.Add($value)
            $markers.Add($value.Replace('\', '/'))
            $markers.Add($value.Replace('/', '\'))
        }
    }
    if (-not [string]::IsNullOrWhiteSpace($user)) {
        $markers.Add('Users\' + $user)
        $markers.Add('Users/' + $user)
        $markers.Add('home/' + $user)
    }
    if ($env:COMPUTERNAME -and $env:COMPUTERNAME.Length -ge 4) { $markers.Add($env:COMPUTERNAME) }
    return $markers | Select-Object -Unique
}

function Assert-NoPrivateMarkers([string]$File, [string[]]$Markers) {
    # Reject builder profile, user-directory components, project and Go cache paths,
    # and sufficiently long computer names in ASCII and UTF-16LE, ignoring case.
    # Bare user-name substrings in public project identifiers are not private paths.
    $latin = [System.Text.Encoding]::GetEncoding(28591)
    $content = $latin.GetString([System.IO.File]::ReadAllBytes($File))
    foreach ($marker in $Markers) {
        foreach ($encoding in @([System.Text.Encoding]::ASCII, [System.Text.Encoding]::Unicode)) {
            $encoded = $latin.GetString($encoding.GetBytes($marker))
            if ($content.IndexOf($encoded, [StringComparison]::OrdinalIgnoreCase) -ge 0) {
                throw ('Builder information detected in ' + [System.IO.Path]::GetFileName($File))
            }
        }
    }
}

Push-Location -LiteralPath $root
try {
$command = Get-Command go -ErrorAction SilentlyContinue
$localGo = Join-Path $root 'go\bin\go.exe'
if ($null -ne $command) { $goExe = $command.Source }
elseif ([System.IO.File]::Exists($localGo)) { $goExe = $localGo }
elseif ([System.IO.Directory]::Exists((Join-Path $root 'go'))) {
    throw 'Local Go folder is incomplete. Delete the go folder and retry.'
} else {
    Write-Host 'Downloading the official Go zip into the local go folder.'
    $goExe = Install-LocalGo $root
}
if ($goExe -eq $localGo) {
    # Keep the local toolchain out of the application's ./... package pattern.
    $boundary = Join-Path $root 'go\go.mod'
    if (-not [System.IO.File]::Exists($boundary)) {
        [System.IO.File]::WriteAllText($boundary, "module qrypt-local-go-toolchain`ngo 1.26.7`n", (New-Object System.Text.UTF8Encoding($false)))
    }
}
$goVersion = (& $goExe version)
if ($LASTEXITCODE -ne 0) { throw 'Go version check failed.' }
$folder = 'qrypt-' + $Version + '-windows-amd64'
$dist = Join-Path $root 'dist'
$stage = Join-Path $dist $folder
# A failed prior build may have left partial output. Recreate only this version's files.
if ([System.IO.Directory]::Exists($stage)) {
    $existing = @([System.IO.Directory]::GetFileSystemEntries($stage))
    foreach ($item in $existing) {
        if ([System.IO.Path]::GetFileName($item) -notin @('qrypt.exe', 'LICENSE', 'THIRD_PARTY_LICENSES.txt', 'README.txt', $fontFile)) {
            throw 'Unexpected file in release staging directory.'
        }
    }
}

Invoke-Go -GoArguments @('vet', './...')
Write-Host 'go vet ./...: passed'
Invoke-Go -GoArguments @('test', './...', '-count=1')
Write-Host 'go test ./... -count=1: passed'
[System.IO.Directory]::CreateDirectory($stage) | Out-Null
$exe = Join-Path $stage 'qrypt.exe'
$previousCgo = $env:CGO_ENABLED
$previousGoos = $env:GOOS
$previousGoarch = $env:GOARCH
try {
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    Invoke-Go -GoArguments @('build', '-trimpath', '-buildvcs=true', '-ldflags', '-s -w', '-o', $exe, './cmd/qrypt')
} finally {
    $env:CGO_ENABLED = $previousCgo
    $env:GOOS = $previousGoos
    $env:GOARCH = $previousGoarch
}
$metadata = @(& $goExe version -m $exe)
if ($LASTEXITCODE -ne 0) { throw 'Could not inspect built executable.' }
$metadataText = $metadata -join "`n"
if ($metadataText -notmatch '(?m)^\s*build\s+-trimpath=true\s*$' -or
    $metadataText -notmatch '(?m)^\s*build\s+CGO_ENABLED=0\s*$') {
    throw 'Executable is missing -trimpath or CGO_ENABLED=0.'
}

[System.IO.File]::Copy((Join-Path $root 'LICENSE'), (Join-Path $stage 'LICENSE'), $true)
$fontTemp = Join-Path ([System.IO.Path]::GetTempPath()) ('qrypt-font-' + [System.Guid]::NewGuid().ToString('N'))
[System.IO.Directory]::CreateDirectory($fontTemp) | Out-Null
try {
Get-SymbolsFont $fontTemp
[System.IO.File]::Copy((Join-Path $fontTemp $fontFile), (Join-Path $stage $fontFile), $true)
$licenses = Join-Path $stage 'THIRD_PARTY_LICENSES.txt'
$output = [System.IO.File]::Create($licenses)
try {
    $goroot = (& $goExe env GOROOT)
    if ($LASTEXITCODE -ne 0) { throw 'Could not determine GOROOT.' }
    Add-License $output ('Go ' + (($goVersion -split ' ')[2])) (Join-Path $goroot 'LICENSE')
    $dependencies = @($metadata | Where-Object { $_ -match '^\s*dep\s+\S+\s+\S+' })
    foreach ($line in $dependencies) {
        if ($line -notmatch '^\s*dep\s+(\S+)\s+(\S+)') { throw 'Malformed dependency metadata.' }
        $module = $Matches[1]
        $moduleVersion = $Matches[2]
        $json = (& $goExe mod download -json ($module + '@' + $moduleVersion)) -join "`n"
        if ($LASTEXITCODE -ne 0) { throw ('Cannot download module ' + $module) }
        $downloaded = $json | ConvertFrom-Json
        if ($downloaded.PSObject.Properties['Error'] -or -not $downloaded.Dir) { throw ('Cannot locate module ' + $module) }
        $licenseFile = $null
        foreach ($name in @('LICENSE', 'LICENSE.txt', 'LICENSE.md', 'LICENCE', 'COPYING')) {
            $candidate = Join-Path $downloaded.Dir $name
            if ([System.IO.File]::Exists($candidate)) { $licenseFile = $candidate; break }
        }
        if ($null -eq $licenseFile) { throw ('Missing license for ' + $module) }
        Add-License $output ($module + ' ' + $moduleVersion) $licenseFile
    }
    Add-Text $output "===== BIP39 English word list =====`nSource: https://github.com/bitcoin/bips/blob/master/bip-0039/english.txt`nLicense: MIT (https://github.com/bitcoin/bips/blob/master/bip-0039.mediawiki)`nThe BIP-0039 document states: `"This BIP falls under the MIT License.`"`n`n"
    Add-License $output ('Symbols Nerd Font Mono ' + $fontVersion + ' (' + $fontFile + ', https://github.com/ryanoasis/nerd-fonts)') (Join-Path $fontTemp 'LICENSE')
} finally { $output.Dispose() }
} finally { [System.IO.Directory]::Delete($fontTemp, $true) }

$readme = @"
Qrypt is an early-stage post-quantum terminal secrets manager.
It has not been audited. Do not use it for real secrets.
Start by double-clicking qrypt.exe, or run it from Windows Terminal, PowerShell or cmd.
Windows may show "Windows protected your PC" because the program is not signed.
To continue, choose "More info", then "Run anyway".
To look at the interface without creating anything, run: qrypt.exe --preview
Icons: the first screen chooses the icon set with the key I. The default (emoji folders)
needs no font. If they look misaligned in your terminal, choose "unicode".
The "nerd" set needs a Nerd Font in your terminal. One is included:
  1. Double-click $fontFile and choose Install.
  2. In Windows Terminal: Settings, your profile, Appearance, Font face. Keep your
     font and add the new one after a comma, for example:
     Cascadia Mono, Symbols Nerd Font Mono
  3. Restart the terminal. If the icons show as boxes, choose another set with I.
Identity file: %AppData%\qrypt\identity
Project: https://github.com/fabianramirez-code/Qrypt
Version: $Version
"@
[System.IO.File]::WriteAllText((Join-Path $stage 'README.txt'), $readme + "`r`n", [System.Text.Encoding]::ASCII)
$markers = @(Get-PrivateMarkers $root $goExe)
foreach ($file in [System.IO.Directory]::GetFiles($stage)) { Assert-NoPrivateMarkers $file $markers }

Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = Join-Path $dist ($folder + '.zip')
$zipStream = [System.IO.File]::Create($zip)
try {
    $archive = New-Object System.IO.Compression.ZipArchive($zipStream, [System.IO.Compression.ZipArchiveMode]::Create, $true)
    try {
        foreach ($file in [System.IO.Directory]::GetFiles($stage)) {
            $entry = $archive.CreateEntry(($folder + '/' + [System.IO.Path]::GetFileName($file)), [System.IO.Compression.CompressionLevel]::Optimal)
            $source = [System.IO.File]::OpenRead($file)
            $destination = $entry.Open()
            try { $source.CopyTo($destination) } finally { $source.Dispose(); $destination.Dispose() }
        }
    } finally { $archive.Dispose() }
} finally { $zipStream.Dispose() }
Assert-NoPrivateMarkers $zip $markers
$hash = Get-Sha256 $zip
$checksum = $zip + '.sha256'
[System.IO.File]::WriteAllText($checksum, ($hash + '  ' + [System.IO.Path]::GetFileName($zip) + "`n"), (New-Object System.Text.UTF8Encoding($false)))
Assert-NoPrivateMarkers $checksum $markers
Write-Host ('Go used: ' + $goVersion)
foreach ($file in @([System.IO.Directory]::GetFiles($stage)) + @($zip, $checksum)) {
    Write-Host ([System.IO.Path]::GetFileName($file) + ': ' + (New-Object System.IO.FileInfo($file)).Length + ' bytes')
}
Write-Host ('ZIP SHA-256: ' + $hash)
} finally {
    Pop-Location
}
