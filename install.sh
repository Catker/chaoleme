#!/bin/bash
# 超了么 (chaoleme) 一键安装脚本
# 适用于 Debian/Ubuntu 系统
# 卸载请使用 uninstall.sh

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 默认配置
DEFAULT_INSTALL_DIR="/opt/chaoleme"
BINARY_NAME="chaoleme"
SERVICE_NAME="chaoleme"

# 显示帮助信息
show_help() {
    echo -e "${GREEN}超了么 (chaoleme) 安装脚本${NC}"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  install [目录]    安装到指定目录 (默认: $DEFAULT_INSTALL_DIR)"
    echo "  --help, -h        显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 install                    # 安装到默认目录 /opt/chaoleme"
    echo "  $0 install /home/chaoleme     # 安装到自定义目录"
    echo ""
    echo "卸载请使用: ./uninstall.sh"
    echo ""
}

# 检查是否为 root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        echo -e "${RED}请使用 root 用户运行此脚本${NC}"
        exit 1
    fi
}

# 检测系统架构
detect_arch() {
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)
            ARCH="amd64"
            ;;
        aarch64)
            ARCH="arm64"
            ;;
        *)
            echo -e "${RED}不支持的系统架构: $ARCH${NC}"
            exit 1
            ;;
    esac
    echo -e "${BLUE}系统架构: $ARCH${NC}"
}

# 查找二进制文件
find_binary() {
    # 按优先级查找二进制文件
    # 1. chaoleme-linux-{arch} (GitHub Release 格式)
    # 2. chaoleme (本地编译格式)
    
    if [ -f "./chaoleme-linux-$ARCH" ]; then
        BINARY_SOURCE="./chaoleme-linux-$ARCH"
        echo -e "${GREEN}找到二进制文件: $BINARY_SOURCE${NC}"
    elif [ -f "./$BINARY_NAME" ]; then
        BINARY_SOURCE="./$BINARY_NAME"
        echo -e "${GREEN}找到二进制文件: $BINARY_SOURCE${NC}"
    else
        echo -e "${RED}未找到二进制文件${NC}"
        echo ""
        echo "请确保以下文件之一存在于当前目录："
        echo "  - chaoleme-linux-$ARCH (从 GitHub Release 下载)"
        echo "  - chaoleme (本地编译)"
        echo ""
        echo "下载地址: https://github.com/lvvdev/chaoleme/releases"
        exit 1
    fi
}

# 安装函数
do_install() {
    local INSTALL_DIR="${1:-$DEFAULT_INSTALL_DIR}"
    
    echo -e "${GREEN}=== 超了么 (chaoleme) 安装脚本 ===${NC}"
    echo ""
    
    check_root
    detect_arch
    find_binary
    
    # 定义路径
    local BIN_DIR="$INSTALL_DIR/bin"
    local CONFIG_DIR="$INSTALL_DIR/config"
    local DATA_DIR="$INSTALL_DIR/data"
    
    echo ""
    echo -e "${BLUE}安装目录: $INSTALL_DIR${NC}"
    echo -e "${BLUE}  - 二进制: $BIN_DIR${NC}"
    echo -e "${BLUE}  - 配置:   $CONFIG_DIR${NC}"
    echo -e "${BLUE}  - 数据:   $DATA_DIR${NC}"
    echo ""
    
    # 停止现有服务（如果存在）
    if systemctl is-active --quiet $SERVICE_NAME 2>/dev/null; then
        echo -e "${YELLOW}停止现有服务...${NC}"
        systemctl stop $SERVICE_NAME
    fi
    
    # 创建目录
    echo -e "${YELLOW}创建目录结构...${NC}"
    mkdir -p "$BIN_DIR"
    mkdir -p "$CONFIG_DIR"
    mkdir -p "$DATA_DIR"
    
    # 复制二进制文件
    echo -e "${YELLOW}安装二进制文件...${NC}"
    cp "$BINARY_SOURCE" "$BIN_DIR/$BINARY_NAME"
    chmod +x "$BIN_DIR/$BINARY_NAME"
    
    # 创建软链接到 /usr/local/bin
    echo -e "${YELLOW}创建命令链接...${NC}"
    ln -sf "$BIN_DIR/$BINARY_NAME" "/usr/local/bin/$BINARY_NAME"
    
    # 生成配置文件（如果不存在）
    if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
        echo -e "${YELLOW}生成配置文件模板...${NC}"
        cat > "$CONFIG_DIR/config.yaml" << 'EOF'
# 超了么 (chaoleme) 配置文件

telegram:
  bot_token: "YOUR_BOT_TOKEN"
  chat_id: "YOUR_CHAT_ID"

report:
  daily: true
  daily_time: "09:00"
  weekly: true
  weekly_day: 0
  monthly: true
  monthly_day: 1

storage:
  db_path: "${DATA_DIR}/data.db"
  retention_days: 30

collect:
  cpu_steal_interval: "5m"
  cpu_bench_interval: "30m"
  io_test_interval: "15m"
  io_test_size_mb: 4

ai:
  enabled: false
  api_url: "https://api.openai.com/v1/chat/completions"
  api_key: "YOUR_API_KEY"
  model: "gpt-4o-mini"
  daily: true
  weekly: true
  monthly: true
EOF
        # 替换配置文件中的路径变量
        sed -i "s|\${DATA_DIR}|$DATA_DIR|g" "$CONFIG_DIR/config.yaml"
        echo -e "${GREEN}配置文件已生成: $CONFIG_DIR/config.yaml${NC}"
    else
        echo -e "${YELLOW}配置文件已存在，跳过生成...${NC}"
    fi
    
    # 保存安装路径（用于卸载）
    echo "$INSTALL_DIR" > /etc/chaoleme_install_path
    
    # 创建 systemd 服务
    echo -e "${YELLOW}创建 systemd 服务...${NC}"
    cat > /etc/systemd/system/$SERVICE_NAME.service << EOF
[Unit]
Description=Chaoleme VPS Overselling Detector
After=network.target

[Service]
Type=simple
ExecStart=$BIN_DIR/$BINARY_NAME --config $CONFIG_DIR/config.yaml
WorkingDirectory=$INSTALL_DIR
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
    
    # 重新加载 systemd
    systemctl daemon-reload
    
    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                    安装完成!                              ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "安装目录: ${YELLOW}$INSTALL_DIR${NC}"
    echo -e "配置文件: ${YELLOW}$CONFIG_DIR/config.yaml${NC}"
    echo ""
    echo -e "${BLUE}[下一步操作]${NC}"
    echo -e "  1. 编辑配置文件:"
    echo -e "     ${YELLOW}nano $CONFIG_DIR/config.yaml${NC}"
    echo ""
    echo -e "  2. 配置 Telegram Bot Token 和 Chat ID"
    echo ""
    echo -e "  3. 测试 Telegram 连接:"
    echo -e "     ${YELLOW}$BINARY_NAME --config $CONFIG_DIR/config.yaml --test-telegram${NC}"
    echo ""
    echo -e "  4. 启动服务:"
    echo -e "     ${YELLOW}systemctl start $SERVICE_NAME${NC}"
    echo ""
    echo -e "  5. 设置开机自启:"
    echo -e "     ${YELLOW}systemctl enable $SERVICE_NAME${NC}"
    echo ""
    echo -e "${BLUE}[其他命令]${NC}"
    echo -e "  - 查看服务状态: ${YELLOW}systemctl status $SERVICE_NAME${NC}"
    echo -e "  - 查看日志:     ${YELLOW}journalctl -u $SERVICE_NAME -f${NC}"
    echo -e "  - 仅采集一次:   ${YELLOW}$BINARY_NAME --collect-once${NC}"
    echo -e "  - 发送日报:     ${YELLOW}$BINARY_NAME --report daily${NC}"
    echo -e "  - 发送周报:     ${YELLOW}$BINARY_NAME --report weekly${NC}"
    echo -e "  - 发送月报:     ${YELLOW}$BINARY_NAME --report monthly${NC}"
    echo -e "  - 卸载程序:     ${YELLOW}./uninstall.sh${NC}"
    echo ""
}

# 主程序
case "${1:-}" in
    install)
        do_install "${2:-}"
        ;;
    --help|-h)
        show_help
        ;;
    "")
        # 默认行为：安装
        do_install
        ;;
    *)
        echo -e "${RED}未知选项: $1${NC}"
        echo ""
        show_help
        exit 1
        ;;
esac
