#!/bin/bash
# 超了么 (chaoleme) 卸载脚本
# 用法: ./uninstall.sh

set -e

# ========== 颜色定义 ==========
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# ========== 配置 ==========
BINARY_NAME="chaoleme"
SERVICE_NAME="chaoleme"

# ========== 检查 root 权限 ==========
check_root() {
    if [ "$EUID" -ne 0 ]; then
        echo -e "${RED}请使用 root 用户运行此脚本${NC}"
        exit 1
    fi
}

# ========== 主函数 ==========
main() {
    echo -e "${RED}=== 超了么 (chaoleme) 卸载脚本 ===${NC}"
    echo ""
    
    check_root
    
    # 读取安装路径
    local INSTALL_DIR=""
    if [ -f /etc/chaoleme_install_path ]; then
        INSTALL_DIR=$(cat /etc/chaoleme_install_path)
    fi
    
    # 确认卸载
    echo -e "${YELLOW}此操作将完全卸载 chaoleme，包括：${NC}"
    echo "  - 停止并禁用 systemd 服务"
    echo "  - 删除二进制文件和命令链接"
    echo "  - 删除 systemd 服务配置"
    if [ -n "$INSTALL_DIR" ] && [ -d "$INSTALL_DIR" ]; then
        echo "  - 删除安装目录: $INSTALL_DIR"
    fi
    echo ""
    echo -e "${RED}注意: 配置文件和数据库将被一并删除！${NC}"
    echo ""
    read -p "确定要卸载吗？[y/N] " confirm
    
    if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
        echo -e "${YELLOW}已取消卸载${NC}"
        exit 0
    fi
    
    echo ""
    
    # 停止并禁用服务
    if systemctl is-active --quiet $SERVICE_NAME 2>/dev/null; then
        echo -e "${YELLOW}停止服务...${NC}"
        systemctl stop $SERVICE_NAME
    fi
    
    if systemctl is-enabled --quiet $SERVICE_NAME 2>/dev/null; then
        echo -e "${YELLOW}禁用服务...${NC}"
        systemctl disable $SERVICE_NAME
    fi
    
    # 删除 systemd 服务文件
    if [ -f /etc/systemd/system/$SERVICE_NAME.service ]; then
        echo -e "${YELLOW}删除 systemd 服务...${NC}"
        rm -f /etc/systemd/system/$SERVICE_NAME.service
        systemctl daemon-reload
    fi
    
    # 删除命令链接
    if [ -L /usr/local/bin/$BINARY_NAME ]; then
        echo -e "${YELLOW}删除命令链接...${NC}"
        rm -f /usr/local/bin/$BINARY_NAME
    fi
    
    # 删除安装目录
    if [ -n "$INSTALL_DIR" ] && [ -d "$INSTALL_DIR" ]; then
        echo -e "${YELLOW}删除安装目录: $INSTALL_DIR${NC}"
        rm -rf "$INSTALL_DIR"
    fi
    
    # 删除安装路径记录
    rm -f /etc/chaoleme_install_path
    
    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                    卸载完成!                              ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "感谢使用超了么 (chaoleme)！"
    echo ""
}

main "$@"
