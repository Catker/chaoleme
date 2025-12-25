#!/bin/bash
# 超了么 (chaoleme) 一键安装脚本
# 适用于 Debian/Ubuntu 系统

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查是否为 root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}请使用 root 用户运行此脚本${NC}"
    exit 1
fi

echo -e "${GREEN}=== 超了么 (chaoleme) 安装脚本 ===${NC}"
echo ""

# 检测系统架构
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

echo -e "${YELLOW}系统架构: $ARCH${NC}"

# 定义路径
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/chaoleme"
DATA_DIR="/var/lib/chaoleme"
BINARY_NAME="chaoleme"
SERVICE_NAME="chaoleme"

# 检查二进制文件
if [ ! -f "./$BINARY_NAME" ]; then
    echo -e "${RED}未找到 $BINARY_NAME 二进制文件${NC}"
    echo "请先编译项目或下载对应架构的二进制文件"
    exit 1
fi

# 停止现有服务（如果存在）
if systemctl is-active --quiet $SERVICE_NAME 2>/dev/null; then
    echo -e "${YELLOW}停止现有服务...${NC}"
    systemctl stop $SERVICE_NAME
fi

# 创建目录
echo -e "${YELLOW}创建目录...${NC}"
mkdir -p $CONFIG_DIR
mkdir -p $DATA_DIR

# 复制二进制文件
echo -e "${YELLOW}安装二进制文件...${NC}"
cp ./$BINARY_NAME $INSTALL_DIR/$BINARY_NAME
chmod +x $INSTALL_DIR/$BINARY_NAME

# 生成配置文件（如果不存在）
if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
    echo -e "${YELLOW}生成配置文件模板...${NC}"
    cat > $CONFIG_DIR/config.yaml << 'EOF'
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
  db_path: "/var/lib/chaoleme/data.db"
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
    echo -e "${GREEN}配置文件已生成: $CONFIG_DIR/config.yaml${NC}"
else
    echo -e "${YELLOW}配置文件已存在，跳过...${NC}"
fi

# 创建 systemd 服务
echo -e "${YELLOW}创建 systemd 服务...${NC}"
cat > /etc/systemd/system/$SERVICE_NAME.service << EOF
[Unit]
Description=Chaoleme VPS Overselling Detector
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/$BINARY_NAME --config $CONFIG_DIR/config.yaml
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
echo -e "${GREEN}=== 安装完成 ===${NC}"
echo ""
echo -e "请编辑配置文件: ${YELLOW}$CONFIG_DIR/config.yaml${NC}"
echo ""
echo -e "配置完成后，运行以下命令："
echo -e "  1. 测试 Telegram 连接: ${YELLOW}$BINARY_NAME --test-telegram${NC}"
echo -e "  2. 启动服务: ${YELLOW}systemctl start $SERVICE_NAME${NC}"
echo -e "  3. 开机自启: ${YELLOW}systemctl enable $SERVICE_NAME${NC}"
echo -e "  4. 查看日志: ${YELLOW}journalctl -u $SERVICE_NAME -f${NC}"
echo ""
echo -e "其他命令："
echo -e "  - 仅采集一次: ${YELLOW}$BINARY_NAME --collect-once${NC}"
echo -e "  - 立即发送日报: ${YELLOW}$BINARY_NAME --report daily${NC}"
echo -e "  - 立即发送周报: ${YELLOW}$BINARY_NAME --report weekly${NC}"
echo -e "  - 立即发送月报: ${YELLOW}$BINARY_NAME --report monthly${NC}"
echo ""
