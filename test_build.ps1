#!/usr/bin/env pwsh
Write-Host "Testing Go build..."
Set-Location "c:\dev\go\astro-ai-archiver"
$result = go build -v ./cmd/astro-ai-archiver 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "Build successful!" -ForegroundColor Green
} else {
    Write-Host "Build failed:" -ForegroundColor Red
    Write-Host $result
}