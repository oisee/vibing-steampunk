#!/bin/bash
# Demo script for vsp debug-daemon
# Showcases the HTTP API for ABAP debugging

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

DAEMON_PORT=${DAEMON_PORT:-9999}
DAEMON_URL="http://localhost:${DAEMON_PORT}"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}   vsp Debug Daemon Demo${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Check if daemon is running
check_daemon() {
    if curl -s "${DAEMON_URL}/health" > /dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

# Pretty print JSON (fallback if jq not available)
pretty_json() {
    if command -v jq &> /dev/null; then
        jq .
    elif command -v python3 &> /dev/null; then
        python3 -m json.tool
    else
        cat
    fi
}

# Start daemon if not running
if ! check_daemon; then
    echo -e "${YELLOW}Starting debug daemon...${NC}"
    echo "Required environment variables: SAP_URL, SAP_USER, SAP_PASSWORD, SAP_CLIENT"
    echo ""

    if [ -z "$SAP_URL" ]; then
        echo -e "${RED}Error: SAP_URL not set${NC}"
        echo "Usage: SAP_URL=http://host:port SAP_USER=user SAP_PASSWORD=pass SAP_CLIENT=001 $0"
        exit 1
    fi

    ./vsp debug-daemon --port ${DAEMON_PORT} --verbose &
    DAEMON_PID=$!
    echo "Daemon started with PID: ${DAEMON_PID}"
    sleep 2

    if ! check_daemon; then
        echo -e "${RED}Failed to start daemon${NC}"
        exit 1
    fi
fi

echo -e "${GREEN}Daemon running at ${DAEMON_URL}${NC}"
echo ""

# Demo functions
demo_health() {
    echo -e "${BLUE}[1] Health Check${NC}"
    echo "GET /health"
    curl -s "${DAEMON_URL}/health" | pretty_json
    echo ""
}

demo_session_status() {
    echo -e "${BLUE}[2] Session Status${NC}"
    echo "GET /session"
    curl -s "${DAEMON_URL}/session" | pretty_json
    echo ""
}

demo_set_exception_breakpoint() {
    echo -e "${BLUE}[3] Set Exception Breakpoint${NC}"
    echo "POST /breakpoint - CX_SY_ZERODIVIDE"
    curl -s -X POST "${DAEMON_URL}/breakpoint" \
        -H "Content-Type: application/json" \
        -d '{"kind":"exception","exception":"CX_SY_ZERODIVIDE"}' | pretty_json
    echo ""
}

demo_set_line_breakpoint() {
    local uri=$1
    local line=$2
    echo -e "${BLUE}[4] Set Line Breakpoint${NC}"
    echo "POST /breakpoint - ${uri}:${line}"
    curl -s -X POST "${DAEMON_URL}/breakpoint" \
        -H "Content-Type: application/json" \
        -d "{\"kind\":\"line\",\"uri\":\"${uri}\",\"line\":${line}}" | pretty_json
    echo ""
}

demo_list_breakpoints() {
    echo -e "${BLUE}[5] List Breakpoints${NC}"
    echo "GET /breakpoints"
    curl -s "${DAEMON_URL}/breakpoints" | pretty_json
    echo ""
}

demo_delete_breakpoint() {
    local id=$1
    echo -e "${BLUE}[6] Delete Breakpoint${NC}"
    echo "DELETE /breakpoint?id=${id}"
    curl -s -X DELETE "${DAEMON_URL}/breakpoint?id=${id}" | pretty_json
    echo ""
}

demo_start_session() {
    local timeout=${1:-30}
    echo -e "${BLUE}[7] Start Debug Session${NC}"
    echo "POST /session - timeout: ${timeout}s"
    curl -s -X POST "${DAEMON_URL}/session" \
        -H "Content-Type: application/json" \
        -d "{\"timeout\":${timeout}}" | pretty_json
    echo ""
}

demo_stop_session() {
    echo -e "${BLUE}[8] Stop Session${NC}"
    echo "DELETE /session"
    curl -s -X DELETE "${DAEMON_URL}/session" | pretty_json
    echo ""
}

# Interactive menu
show_menu() {
    echo -e "${YELLOW}========================================${NC}"
    echo -e "${YELLOW}   Demo Menu${NC}"
    echo -e "${YELLOW}========================================${NC}"
    echo "1) Health check"
    echo "2) Session status"
    echo "3) Set exception breakpoint (CX_SY_ZERODIVIDE)"
    echo "4) Set line breakpoint (enter URI and line)"
    echo "5) List breakpoints"
    echo "6) Delete breakpoint (enter ID)"
    echo "7) Start debug session"
    echo "8) Stop session"
    echo "9) Run full demo sequence"
    echo "0) Exit"
    echo ""
}

run_full_demo() {
    echo -e "${GREEN}Running full demo sequence...${NC}"
    echo ""

    demo_health
    sleep 1

    demo_session_status
    sleep 1

    demo_set_exception_breakpoint
    sleep 1

    demo_list_breakpoints
    sleep 1

    echo -e "${YELLOW}Starting debug listener (10s timeout)...${NC}"
    echo "The listener will wait for a debuggee to hit a breakpoint."
    echo "In a real scenario, you would trigger ABAP code execution now."
    echo ""
    demo_start_session 10

    echo -e "${YELLOW}Checking session status...${NC}"
    demo_session_status

    echo -e "${YELLOW}Waiting for timeout...${NC}"
    sleep 12

    demo_session_status

    demo_stop_session

    echo -e "${GREEN}Demo complete!${NC}"
}

# Main loop
if [ "$1" == "--auto" ]; then
    run_full_demo
    exit 0
fi

while true; do
    show_menu
    read -p "Select option: " choice
    echo ""

    case $choice in
        1) demo_health ;;
        2) demo_session_status ;;
        3) demo_set_exception_breakpoint ;;
        4)
            read -p "Enter URI (e.g., /sap/bc/adt/programs/programs/ZTEST): " uri
            read -p "Enter line number: " line
            demo_set_line_breakpoint "$uri" "$line"
            ;;
        5) demo_list_breakpoints ;;
        6)
            read -p "Enter breakpoint ID: " bp_id
            demo_delete_breakpoint "$bp_id"
            ;;
        7)
            read -p "Enter timeout in seconds (default 30): " timeout
            demo_start_session "${timeout:-30}"
            ;;
        8) demo_stop_session ;;
        9) run_full_demo ;;
        0)
            echo "Goodbye!"
            exit 0
            ;;
        *) echo -e "${RED}Invalid option${NC}" ;;
    esac
done
