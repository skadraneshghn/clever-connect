#!/usr/bin/env bash
# ==============================================================================
# CleverConnect Interactive CLI Manager & TUI Dashboard
# ==============================================================================

# ANSI Color Codes for Premium TUI Layout
CLR_RED='\033[0;31m'
CLR_GREEN='\033[0;32m'
CLR_YELLOW='\033[0;33m'
CLR_BLUE='\033[0;34m'
CLR_PURPLE='\033[0;35m'
CLR_CYAN='\033[0;36m'
CLR_WHITE='\033[1;37m'
CLR_BG_DARK='\033[40m'
CLR_NC='\033[0m' # No Color

# Unicode Icons & Borders
ICON_PLAY="▶"
ICON_STOP="⏹"
ICON_GEAR="⚙"
ICON_STATS="📊"
ICON_KILL="💀"
ICON_CHECK="✔"
ICON_WARN="⚠"
ICON_INFO="ℹ"

# Print TUI Banner
print_banner() {
    clear
    echo -e "${CLR_CYAN}"
    echo "  ██████╗██╗     ███████╗██╗   ██╗███████╗██████╗      ██████╗ ██████╗ ███╗   ██╗███╗   ██╗███████╗ ██████╗████████╗"
    echo " ██╔════╝██║     ██╔════╝██║   ██║██╔════╝██╔══██╗    ██╔════╝██╔═══██╗████╗  ██║████╗  ██║██╔════╝██╔════╝╚══██╔══╝"
    echo " ██║     ██║     █████╗  ██║   ██║█████╗  ██████╔╝    ██║     ██║   ██║██╔██╗ ██║██╔██╗ ██║█████╗  ██║        ██║   "
    echo " ██║     ██║     ██╔══╝  ╚██╗ ██╔╝██╔══╝  ██╔══██╗    ██║     ██║   ██║██║╚██╗██║██║╚██╗██║██╔══╝  ██║        ██║   "
    echo " ╚██████╗███████╗███████╗ ╚████╔╝ ███████╗██║  ██║    ╚██████╗╚██████╔╝██║ ╚████║██║ ╚████║███████╗╚██████╗   ██║   "
    echo "  ╚══════╝╚══════╝╚══════╝  ╚═══╝  ╚══════╝╚═╝  ╚═╝     ╚═════╝ ╚═════╝ ╚═╝  ╚═══╝╚═╝  ╚═══╝╚══════╝ ╚═════╝   ╚═╝   "
    echo -e "${CLR_NC}"
    echo -e "${CLR_WHITE}  ===================================================================================================${CLR_NC}"
    echo -e "  System Orchestrator, Port Multiplexers, and Ehco Tunnel Relayer Control Panel"
    echo -e "  Time: $(date) | User: $(whoami)"
    echo -e "${CLR_WHITE}  ===================================================================================================${CLR_NC}"
    echo ""
}

# Print Diagnostics / Status bar
print_diagnostics() {
    echo -e "${CLR_WHITE}  [ Diagnostics Status Panel ]${CLR_NC}"
    
    # 1. Check Clever Connect Server
    if pgrep -f "clever-connect" > /dev/null; then
        PID_CC=$(pgrep -f "clever-connect" | head -n 1)
        echo -e "    ${CLR_GREEN}${ICON_CHECK} CleverConnect Process:${CLR_NC} Running (PID: $PID_CC)"
    else
        echo -e "    ${CLR_RED}${ICON_WARN} CleverConnect Process:${CLR_NC} Dormant"
    fi

    # 2. Check Ehco Tunnel Relayer
    if pgrep -f "bin/ehco" > /dev/null; then
        PID_EHCO=$(pgrep -f "bin/ehco" | head -n 1)
        echo -e "    ${CLR_GREEN}${ICON_CHECK} Ehco Tunnel Subprocess:${CLR_NC} Running (PID: $PID_EHCO)"
    else
        echo -e "    ${CLR_RED}${ICON_WARN} Ehco Tunnel Subprocess:${CLR_NC} Dormant"
    fi

    # 3. Check Nginx Multiplexer
    if pgrep -x "nginx" > /dev/null; then
        echo -e "    ${CLR_GREEN}${ICON_CHECK} Nginx Port Multiplexer:${CLR_NC} Active"
    else
        echo -e "    ${CLR_RED}${ICON_WARN} Nginx Port Multiplexer:${CLR_NC} Dormant"
    fi

    # 4. Port listeners
    echo -n "    Active Ports Check: "
    for port in 8080 8081 3000 3001; do
        if ss -tln | grep -q ":$port " 2>/dev/null || netstat -an | grep -q "\.$port " 2>/dev/null || ss -tln | grep -q "127.0.0.1:$port" 2>/dev/null; then
            echo -e -n "${CLR_GREEN}$port${CLR_NC} "
        else
            echo -e -n "${CLR_WHITE}$port${CLR_NC} "
        fi
    done
    echo ""
    echo -e "${CLR_WHITE}  ===================================================================================================${CLR_NC}"
    echo ""
}

# Handle stopping everything
kill_all() {
    echo -e "\n${CLR_YELLOW}${ICON_STOP} Gracefully terminating all processes...${CLR_NC}"
    
    # Kill clever-connect
    if pgrep -f "clever-connect" > /dev/null; then
        pkill -f "clever-connect"
        echo -e "  - Terminated CleverConnect processes."
    fi

    # Kill ehco
    if pgrep -f "bin/ehco" > /dev/null; then
        pkill -f "bin/ehco"
        echo -e "  - Terminated Ehco Tunnel subprocesses."
    fi

    # Kill Nginx (local development if running sudo/user)
    if pgrep -x "nginx" > /dev/null; then
        pkill -x "nginx"
        echo -e "  - Terminated Nginx service."
    fi

    echo -e "${CLR_GREEN}${ICON_CHECK} System quieted successfully.${CLR_NC}"
    sleep 1.5
}

# Main management loop
while true; do
    print_banner
    print_diagnostics

    echo -e "  ${CLR_CYAN}Select Orchestration Choice:${CLR_NC}"
    echo -e "    ${CLR_GREEN}1)${CLR_NC} Start ${CLR_WHITE}Server Panel${CLR_NC} Mode (Admin Dashboard on Port :8080)"
    echo -e "    ${CLR_GREEN}2)${CLR_NC} Start ${CLR_WHITE}Client Panel${CLR_NC} Mode (Local Wallet/Tunnel on Port :8081)"
    echo -e "    ${CLR_GREEN}3)${CLR_NC} Compile / Build ${CLR_WHITE}Whole Project${CLR_NC} (React SPAs + Go server + self-compiles Ehco)"
    echo -e "    ${CLR_GREEN}4)${CLR_NC} Terminate / Kill ${CLR_WHITE}All Active Instances${CLR_NC} (Clear ports 8080, 8081, 3000, 3001)"
    echo -e "    ${CLR_GREEN}5)${CLR_NC} System Diagnostic & ${CLR_WHITE}Tail Live Logs${CLR_NC}"
    echo -e "    ${CLR_GREEN}6)${CLR_NC} Exit"
    echo ""
    echo -n "  Enter choice [1-6]: "
    read -r choice

    case $choice in
        1)
            print_banner
            echo -e "${CLR_GREEN}${ICON_PLAY} Starting CleverConnect in SERVER mode...${CLR_NC}"
            echo -e "  - Access UI at: ${CLR_CYAN}http://localhost:8080${CLR_NC}"
            echo -e "  - Press Ctrl+C inside the backend log loop to return to TUI menu."
            echo ""
            APP_MODE=server PORT=8080 go run main.go
            ;;
        2)
            print_banner
            echo -e "${CLR_GREEN}${ICON_PLAY} Starting CleverConnect in CLIENT mode...${CLR_NC}"
            echo -e "  - Access UI at: ${CLR_CYAN}http://localhost:8081${CLR_NC}"
            echo -e "  - Press Ctrl+C inside the backend log loop to return to TUI menu."
            echo ""
            APP_MODE=client PORT=8081 go run main.go
            ;;
        3)
            print_banner
            echo -e "${CLR_YELLOW}${ICON_GEAR} Bootstrapping build pipelines...${CLR_NC}"
            make build
            echo -e "\n${CLR_GREEN}${ICON_CHECK} Compilation finished! Press [Enter] to return.${CLR_NC}"
            read -r
            ;;
        4)
            kill_all
            ;;
        5)
            print_banner
            echo -e "${CLR_WHITE}  [ Real-time Diagnostic Telemetry ]${CLR_NC}"
            echo -e "    Uptime: $(uptime -p)"
            echo -e "    RAM Info:"
            free -h | grep -E "Mem|total"
            echo -e "    Disk Info:"
            df -h / | grep -E "Size|/"
            echo ""
            echo -e "  ${CLR_YELLOW}${ICON_INFO} Displaying tail of application logs (Press Ctrl+C to stop tailing):${CLR_NC}"
            echo -e "  ---------------------------------------------------------------------------------------------------"
            tail -n 25 logs/app.log 2>/dev/null || echo -e "  No logs found under logs/app.log yet."
            echo -e "  ---------------------------------------------------------------------------------------------------"
            sleep 3
            ;;
        6)
            echo -e "\n${CLR_CYAN}Thank you for using CleverConnect. Safe travels!${CLR_NC}\n"
            exit 0
            ;;
        *)
            echo -e "\n${CLR_RED}${ICON_WARN} Invalid option selected. Press [Enter] to retry.${CLR_NC}"
            read -r
            ;;
    esac
done
