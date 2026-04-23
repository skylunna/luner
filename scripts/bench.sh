#!/bin/bash
# Performance benchmark suite for luner (macOS/Linux)
# Supports: Bash 4.0+, macOS 10.15+, Linux

set -euo pipefail

# Default parameters
GATEWAY_URL="http://localhost:8080"
MODEL="qwen-turbo"
CONCURRENCY=50
REQUESTS=1000

# Colors
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
GRAY='\033[0;90m'
NC='\033[0m' # No Color

# Payloads (using printf for variable substitution)
get_payload_cache() {
    printf '{"model":"%s","messages":[{"role":"user","content":"Explain Go context in 1 sentence"}],"temperature":0}' "$MODEL"
}

get_payload_cold() {
    printf '{"model":"%s","messages":[{"role":"user","content":"What is the meaning of life?"}],"temperature":0.7}' "$MODEL"
}

# Logging functions
write_info() {
    echo -e "${GREEN}[INFO] $1${NC}"
}

write_warn() {
    echo -e "${YELLOW}[WARN] $1${NC}"
}

write_error() {
    echo -e "${RED}[ERROR] $1${NC}"
}

# Usage help
show_help() {
    cat << EOF
Usage: ./bench.sh [OPTIONS]

Performance benchmark suite for luner.

Parameters:
  -g, --gateway-url   Gateway endpoint (default: http://localhost:8080)
  -m, --model         Model name to test (default: qwen-turbo)
  -c, --concurrency   Concurrent clients (default: 50)
  -n, --requests      Total requests (default: 1000)
  -h, --help          Show this help

Examples:
  ./bench.sh
  ./bench.sh -g http://127.0.0.1:8080 -c 100 -n 5000
  ./bench.sh --model qwen-plus --concurrency 25

EOF
    exit 0
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -g|--gateway-url)
                GATEWAY_URL="$2"
                shift 2
                ;;
            -m|--model)
                MODEL="$2"
                shift 2
                ;;
            -c|--concurrency)
                CONCURRENCY="$2"
                shift 2
                ;;
            -n|--requests)
                REQUESTS="$2"
                shift 2
                ;;
            -h|--help)
                show_help
                ;;
            *)
                write_error "Unknown option: $1"
                echo "Use --help for usage information"
                exit 1
                ;;
        esac
    done
}

# Check dependencies
check_deps() {
    if ! command -v hey >/dev/null 2>&1; then
        write_error "hey not found. Install: go install github.com/rakyll/hey@latest"
        exit 1
    fi
    if ! command -v curl >/dev/null 2>&1; then
        write_error "curl not found. Please install curl."
        exit 1
    fi
}

# Wait for gateway health
wait_gateway() {
    write_info "Waiting for luner gateway at $GATEWAY_URL..."
    for i in {1..30}; do
        if curl -sf --max-time 2 "${GATEWAY_URL}/health" >/dev/null 2>&1; then
            write_info "Gateway is healthy"
            return 0
        fi
        sleep 1
    done
    write_error "Gateway not ready after 30s"
    exit 1
}

# Warm up cache
warm_cache() {
    write_info "Warming up cache..."
    for i in {1..3}; do
        curl -sf --max-time 10 \
            -X POST "${GATEWAY_URL}/v1/chat/completions" \
            -H "Content-Type: application/json" \
            -d "$(get_payload_cache)" \
            >/dev/null 2>&1 || true
    done
}

# Run benchmark and extract metrics
run_bench() {
    local name="$1"
    local payload="$2"
    local output_file="bench_${name}.txt"
    
    write_info "Running $name benchmark ($CONCURRENCY concurrent, $REQUESTS requests)..."
    
    # Create temp file for payload
    local temp_file
    temp_file=$(mktemp)
    printf '%s\n' "$payload" > "$temp_file"
    
    # Run hey benchmark (removed -q 0 to avoid overwhelming local gateway)
    hey -c "$CONCURRENCY" -n "$REQUESTS" -m POST \
        -H "Content-Type: application/json" \
        -D "$temp_file" \
        "${GATEWAY_URL}/v1/chat/completions" > "$output_file" 2>&1 || true
    
    rm -f "$temp_file"
    
    # Read and parse output
    local content
    content=$(cat "$output_file")
    
    # Extract QPS
    local qps="N/A"
    if [[ "$content" =~ Requests/sec:[[:space:]]*([0-9,]+\.?[0-9]*) ]]; then
        qps="${BASH_REMATCH[1]//,/}"
    fi
    
    # Extract P50/P99 (hey output format: "50% in 0.12 secs")
    local p50="N/A"
    local p99="N/A"
    
    if [[ "$content" =~ 50%[[:space:]]+in[[:space:]]+([0-9.]+)[[:space:]]*secs? ]]; then
        p50="${BASH_REMATCH[1]}"
    fi
    if [[ "$content" =~ 99%[[:space:]]+in[[:space:]]+([0-9.]+)[[:space:]]*secs? ]]; then
        p99="${BASH_REMATCH[1]}"
    fi
    
    echo -e "  QPS: $qps | P50: ${p50}s | P99: ${p99}s"
    
    # Debug output if parsing failed
    if [[ "$p50" == "N/A" ]]; then
        echo -e "${GRAY}  [DEBUG] Raw output snippet (Look for Error distribution):${NC}"
        echo "$content" | head -30 | while IFS= read -r line; do
            echo -e "${GRAY}    $line${NC}"
        done
    fi
    
    # Return values for caller
    echo "$qps $p50 $p99"
}

# Collect Prometheus metrics
collect_metrics() {
    write_info "Collecting Prometheus metrics..."
    if curl -sf --max-time 5 "${GATEWAY_URL}/metrics" -o /tmp/luner_metrics.txt 2>/dev/null; then
        echo ""
        echo -e " Key Metrics:"
        grep -E "luner_requests_total|luner_tokens_used" /tmp/luner_metrics.txt | grep -v "^#" | head -10 | while IFS= read -r line; do
            echo "  $line"
        done
        rm -f /tmp/luner_metrics.txt
    else
        write_warn "Failed to collect metrics"
    fi
}

# Print formatted summary
print_summary() {
    echo ""
    echo -e "${GREEN} Benchmark Summary${NC}"
    echo "==================="
    printf "%-20s %-12s %-12s %-12s\n" "Scenario" "QPS" "P50" "P99"
    printf "%-20s %-12s %-12s %-12s\n" "--------" "---" "---" "---"
    
    # Cache Hit benchmark
    local cache_result
    cache_result=$(run_bench "cache_hit" "$(get_payload_cache)")
    read -r qps p50 p99 <<< "$cache_result"
    printf "%-20s %-12s %-12s %-12s\n" " Cache Hit" "$qps" "${p50}s" "${p99}s"
    
    # Cold Start benchmark
    local cold_result
    cold_result=$(run_bench "cold_start" "$(get_payload_cold)")
    read -r qps p50 p99 <<< "$cold_result"
    printf "%-20s %-12s %-12s %-12s\n" " Cold Start" "$qps" "${p50}s" "${p99}s"
    
    echo ""
    echo -e "${YELLOW} Tip: Cache Hit QPS is theoretical max for identical requests.${NC}"
    echo -e "   Real-world workloads will see lower QPS but still benefit from repeated prompts."
}

# Main execution
main() {
    parse_args "$@"
    
    write_info "Starting luner benchmark suite"
    check_deps
    wait_gateway
    warm_cache
    print_summary
    collect_metrics
    write_info "Benchmark complete!"
}

# Run main function with all arguments
main "$@"