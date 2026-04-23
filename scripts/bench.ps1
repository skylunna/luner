# scripts/bench.ps1 - Performance benchmark suite for luner (Windows)
# Supports: PowerShell 5.1+ / PowerShell 7+

param(
    [string]$GatewayUrl = "http://localhost:8080",
    [string]$Model = "qwen-turbo",
    [int]$Concurrency = 50,
    [int]$Requests = 1000,
    [switch]$Help
)

# Payloads
$PayloadCache = @"
{"model":"$Model","messages":[{"role":"user","content":"Explain Go context in 1 sentence"}],"temperature":0}
"@
$PayloadCold = @"
{"model":"$Model","messages":[{"role":"user","content":"What is the meaning of life?"}],"temperature":0.7}
"@

# Colors
$Green = "Green"; $Yellow = "Yellow"; $Red = "Red"
function Write-Info { param($msg) Write-Host "[INFO] $msg" -ForegroundColor $Green }
function Write-Warn { param($msg) Write-Host "[WARN] $msg" -ForegroundColor $Yellow }
function Write-Error { param($msg) Write-Host "[ERROR] $msg" -ForegroundColor $Red }

# Check dependencies
function Check-Deps {
    if (-not (Get-Command hey -ErrorAction SilentlyContinue)) {
        Write-Error "hey not found. Install: go install github.com/rakyll/hey@latest"
        exit 1
    }
}

# Wait for gateway health
function Wait-Gateway {
    Write-Info "Waiting for luner gateway at $GatewayUrl..."
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $resp = Invoke-WebRequest -Uri "$GatewayUrl/health" -UseBasicParsing -TimeoutSec 2
            if ($resp.StatusCode -eq 200) {
                Write-Info "Gateway is healthy"
                return
            }
        } catch {}
        Start-Sleep -Seconds 1
    }
    Write-Error "Gateway not ready after 30s"
    exit 1
}

# Warm up cache
function Warm-Cache {
    Write-Info "Warming up cache..."
    for ($i = 0; $i -lt 3; $i++) {
        try {
            Invoke-WebRequest -Uri "$GatewayUrl/v1/chat/completions" -Method POST `
                -ContentType "application/json" -Body $PayloadCache -UseBasicParsing | Out-Null
        } catch {}
    }
}

# Run benchmark and extract metrics (FIXED)
function Run-Bench {
    param($Name, $Payload)
    $OutputFile = "bench_$Name.txt"
    
    Write-Info "Running $Name benchmark ($Concurrency concurrent, $Requests requests)..."
    
    # Write payload to temp file
    $TempFile = [System.IO.Path]::GetTempFileName()
    $Payload | Out-File -FilePath $TempFile -Encoding utf8
    
    # 移除了 -q 0 (它不控制进度条，而是取消所有速率限制，容易导致本地网关崩溃)
    hey -c $Concurrency -n $Requests -m POST `
        -H "Content-Type: application/json" `
        -D $TempFile `
        "$GatewayUrl/v1/chat/completions" > $OutputFile 2>&1
    
    Remove-Item $TempFile -Force
    
    # Read and parse output
    $Content = Get-Content $OutputFile -Raw
    
    # Extract QPS
    $Qps = "N/A"
    if ($Content -match "Requests/sec:\s*([\d,]+\.?\d*)") {
        $Qps = $Matches[1] -replace ',', ''
    }
    
    # Extract P50/P99
    $P50 = "N/A"
    $P99 = "N/A"
    
    if ($Content -match "50%%?\s+in\s+([\d.]+)\s*secs?") {
        $P50 = $Matches[1]
    }
    if ($Content -match "99%%?\s+in\s+([\d.]+)\s*secs?") {
        $P99 = $Matches[1]
    }
    
    Write-Host "  QPS: $Qps | P50: ${P50}s | P99: ${P99}s"
    
    if ($P50 -eq "N/A") {
        Write-Host "  [DEBUG] Raw output snippet (Look for Error distribution):" -ForegroundColor Gray
        $Content -split "`n" | Select-Object -First 30 | ForEach-Object { 
            Write-Host "    $_" -ForegroundColor Gray 
        }
    }

    return "$Qps $P50 $P99"
}

function Collect-Metrics {
    Write-Info "Collecting Prometheus metrics..."
    try {
        $Metrics = Invoke-WebRequest -Uri "$GatewayUrl/metrics" -UseBasicParsing -TimeoutSec 5
        Write-Host ""
        Write-Host " Key Metrics:"
        $Metrics.Content -split "`n" | Where-Object { $_ -match "luner_requests_total|luner_tokens_used" -and $_ -notmatch "^#" } | Select-Object -First 10 | ForEach-Object { Write-Host $_ }
    } catch {
        Write-Warn "Failed to collect metrics: $_"
    }
}

function Print-Summary {
    Write-Host ""
    Write-Host " Benchmark Summary" -ForegroundColor Green
    Write-Host "==================="
    printf "%-20s %-12s %-12s %-12s`n" "Scenario", "QPS", "P50", "P99"
    printf "%-20s %-12s %-12s %-12s`n" "--------", "---", "---", "---"
    
    $CacheResult = Run-Bench "cache_hit" $PayloadCache
    printf "%-20s %-12s %-12s %-12s`n" " Cache Hit", ($CacheResult -split " ")
    
    $ColdResult = Run-Bench "cold_start" $PayloadCold
    printf "%-20s %-12s %-12s %-12s`n" " Cold Start", ($ColdResult -split " ")
    
    Write-Host ""
    Write-Host " Tip: Cache Hit QPS is theoretical max for identical requests." -ForegroundColor Yellow
    Write-Host "   Real-world workloads will see lower QPS but still benefit from repeated prompts."
}

if ($Help) {
    Write-Host "Usage: .\bench.ps1 [OPTIONS]"
    Write-Host ""
    Write-Host "Performance benchmark suite for luner."
    Write-Host ""
    Write-Host "Parameters:"
    Write-Host "  -GatewayUrl   Gateway endpoint (default: http://localhost:8080)"
    Write-Host "  -Model        Model name to test (default: qwen-turbo)"
    Write-Host "  -Concurrency  Concurrent clients (default: 50)"
    Write-Host "  -Requests     Total requests (default: 1000)"
    Write-Host "  -Help         Show this help"
    exit 0
}

Write-Info "Starting luner benchmark suite"
Check-Deps
Wait-Gateway
Warm-Cache
Print-Summary
Collect-Metrics
Write-Info " Benchmark complete!"