#!/bin/bash
# chaoleme 自动更新脚本
# 用法: ./update.sh [版本号]
# 示例: ./update.sh         # 更新到最新版本
#       ./update.sh v1.2.0  # 更新到指定版本

set -e

# ========== 配置 ==========
REPO="Catker/chaoleme"
INSTALL_DIR="/opt/chaoleme"
BINARY_NAME="chaoleme"
SERVICE_NAME="chaoleme"
BACKUP_DIR="/opt/chaoleme/backup"

# ========== 颜色输出 ==========
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ========== 检测系统架构 ==========
detect_arch() {
    local arch=$(uname -m)
    case "$arch" in
        x86_64)  echo "amd64" ;;
        aarch64) echo "arm64" ;;
        armv7l)  echo "arm" ;;
        i686|i386) echo "386" ;;
        *)
            log_error "不支持的架构: $arch"
            exit 1
            ;;
    esac
}

# ========== 获取当前版本 ==========
get_current_version() {
    if [[ -x "$INSTALL_DIR/$BINARY_NAME" ]]; then
        "$INSTALL_DIR/$BINARY_NAME" -version 2>/dev/null | awk '{print $2}' | sed 's/^v//'
    else
        echo "0.0.0"
    fi
}

# ========== 获取最新版本 ==========
get_latest_version() {
    local url="https://api.github.com/repos/$REPO/releases/latest"
    local version
    
    if command -v curl &> /dev/null; then
        version=$(curl -fsSL "$url" 2>/dev/null | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/' | sed 's/^v//')
    elif command -v wget &> /dev/null; then
        version=$(wget -qO- "$url" 2>/dev/null | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/' | sed 's/^v//')
    else
        log_error "需要 curl 或 wget"
        exit 1
    fi
    
    if [[ -z "$version" ]]; then
        log_error "无法获取最新版本信息"
        exit 1
    fi
    
    echo "$version"
}

# ========== 版本比较 ==========
# 返回: 0=相等, 1=v1>v2, 2=v1<v2
compare_versions() {
    local v1="$1"
    local v2="$2"
    
    if [[ "$v1" == "$v2" ]]; then
        return 0
    fi
    
    # 将版本号拆分为数组
    IFS='.' read -ra V1 <<< "$v1"
    IFS='.' read -ra V2 <<< "$v2"
    
    for i in 0 1 2; do
        local n1=${V1[$i]:-0}
        local n2=${V2[$i]:-0}
        
        if (( n1 > n2 )); then
            return 1
        elif (( n1 < n2 )); then
            return 2
        fi
    done
    
    return 0
}

# ========== 下载二进制文件 ==========
download_binary() {
    local version="$1"
    local arch="$2"
    local filename="chaoleme-linux-${arch}.tar.gz"
    local url="https://github.com/$REPO/releases/download/v${version}/${filename}"
    local tmp_dir=$(mktemp -d)
    
    log_info "下载 $filename ..."
    
    if command -v curl &> /dev/null; then
        curl -fsSL "$url" -o "$tmp_dir/$filename" || {
            log_error "下载失败: $url"
            rm -rf "$tmp_dir"
            exit 1
        }
    else
        wget -q "$url" -O "$tmp_dir/$filename" || {
            log_error "下载失败: $url"
            rm -rf "$tmp_dir"
            exit 1
        }
    fi
    
    log_info "解压文件..."
    tar -xzf "$tmp_dir/$filename" -C "$tmp_dir" || {
        log_error "解压失败"
        rm -rf "$tmp_dir"
        exit 1
    }
    
    echo "$tmp_dir"
}

# ========== 备份当前版本 ==========
backup_current() {
    local current_version="$1"
    
    if [[ -x "$INSTALL_DIR/$BINARY_NAME" ]]; then
        mkdir -p "$BACKUP_DIR"
        local backup_file="$BACKUP_DIR/${BINARY_NAME}-${current_version}-$(date +%Y%m%d%H%M%S)"
        cp "$INSTALL_DIR/$BINARY_NAME" "$backup_file"
        log_info "已备份当前版本到: $backup_file"
    fi
}

# ========== 安装新版本 ==========
install_binary() {
    local tmp_dir="$1"
    local arch="$2"
    local binary_file="$tmp_dir/chaoleme-linux-${arch}"
    
    if [[ ! -f "$binary_file" ]]; then
        log_error "找不到二进制文件: $binary_file"
        rm -rf "$tmp_dir"
        exit 1
    fi
    
    # 停止服务
    if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
        log_info "停止服务..."
        systemctl stop "$SERVICE_NAME"
    fi
    
    # 替换二进制
    log_info "安装新版本..."
    cp "$binary_file" "$INSTALL_DIR/$BINARY_NAME"
    chmod +x "$INSTALL_DIR/$BINARY_NAME"
    
    # 启动服务
    if systemctl is-enabled --quiet "$SERVICE_NAME" 2>/dev/null; then
        log_info "启动服务..."
        systemctl start "$SERVICE_NAME"
    fi
    
    # 清理临时文件
    rm -rf "$tmp_dir"
}

# ========== 回滚 ==========
rollback() {
    local backup_file=$(ls -t "$BACKUP_DIR"/${BINARY_NAME}-* 2>/dev/null | head -1)
    
    if [[ -z "$backup_file" ]]; then
        log_error "没有可用的备份文件"
        exit 1
    fi
    
    log_info "回滚到: $backup_file"
    
    if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
        systemctl stop "$SERVICE_NAME"
    fi
    
    cp "$backup_file" "$INSTALL_DIR/$BINARY_NAME"
    chmod +x "$INSTALL_DIR/$BINARY_NAME"
    
    if systemctl is-enabled --quiet "$SERVICE_NAME" 2>/dev/null; then
        systemctl start "$SERVICE_NAME"
    fi
    
    log_info "回滚完成"
}

# ========== 主逻辑 ==========
main() {
    # 检查 root 权限
    if [[ $EUID -ne 0 ]]; then
        log_error "请使用 root 权限运行此脚本"
        exit 1
    fi
    
    # 解析参数
    local target_version="$1"
    
    # 回滚命令
    if [[ "$target_version" == "rollback" ]]; then
        rollback
        exit 0
    fi
    
    # 获取架构
    local arch=$(detect_arch)
    log_info "检测到架构: $arch"
    
    # 获取当前版本
    local current_version=$(get_current_version)
    log_info "当前版本: $current_version"
    
    # 获取目标版本
    if [[ -z "$target_version" ]]; then
        target_version=$(get_latest_version)
    else
        target_version=$(echo "$target_version" | sed 's/^v//')
    fi
    log_info "目标版本: $target_version"
    
    # 版本比较
    compare_versions "$current_version" "$target_version"
    local cmp_result=$?
    
    if [[ $cmp_result -eq 0 ]]; then
        log_info "已是最新版本，无需更新"
        exit 0
    elif [[ $cmp_result -eq 1 ]]; then
        log_warn "目标版本 ($target_version) 低于当前版本 ($current_version)"
        read -p "确认降级? [y/N] " confirm
        if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
            log_info "取消操作"
            exit 0
        fi
    fi
    
    # 备份
    backup_current "$current_version"
    
    # 下载
    local tmp_dir=$(download_binary "$target_version" "$arch")
    
    # 安装
    install_binary "$tmp_dir" "$arch"
    
    # 验证
    local new_version=$(get_current_version)
    if [[ "$new_version" == "$target_version" ]]; then
        log_info "✅ 更新成功: $current_version -> $new_version"
    else
        log_error "版本验证失败，期望 $target_version，实际 $new_version"
        log_warn "尝试回滚..."
        rollback
        exit 1
    fi
}

main "$@"
