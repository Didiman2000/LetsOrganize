# Baut LetsOrganize unter Windows.
# Aufruf: powershell -ExecutionPolicy Bypass -File .\Werkzeuge\bauen.ps1
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

go vet ./...
go test ./...

New-Item -ItemType Directory -Force -Path dist | Out-Null
go build -trimpath -ldflags "-s -w" -o dist\LetsOrganize.exe .
Write-Host "Fertig: dist\LetsOrganize.exe"
