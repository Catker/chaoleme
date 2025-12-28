#!/bin/bash
# chaoleme 自动更新脚本
# 用法: ./update.sh [选项] [版本号]
# 示例: ./update.sh           # 更新到最新版本
#       ./update.sh v1.2.0    # 更新到指定版本
#       ./update.sh --force   # 强制重新安装当前版本
#       ./update.sh rollback  # 回滚到上一版本
# ========== 配置 ==========
REPO="Catker/chaoleme"
INSTALL_DIR="/opt/chaoleme/bin"
BINARY_NAME="chaoleme"
SERVICE_NAME="chaoleme"
BACKUP_DIR="/opt/chaoleme/backup"

# ========== 颜色输出 ==========
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info()  { echo -e "${GREEN}[INFO]${NC} $1" >&2; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1" >&2; }
log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }

# ========== 检测系统架构 ==========
detect_arch() {
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64)  echo "amd64" ;;
        aarch64) echo "arm64" ;;
        armv7l)  echo "arm" ;;
        i686|i386) echo "386" ;;
        *)
            log_error "不支持的架构: $arch"
            return 1
            ;;
    esac
}

# ========== 获取当前版本 ==========
get_current_version() {
    local version=""
    # 优先使用 PATH 中的命令
    if command -v "$BINARY_NAME" &> /dev/null; then
        version=$("$BINARY_NAME" -version 2>/dev/null | awk '{print $2}')
    # fallback 到固定安装路径
    elif [[ -x "$INSTALL_DIR/$BINARY_NAME" ]]; then
        version=$("$INSTALL_DIR/$BINARY_NAME" -version 2>/dev/null | awk '{print $2}')
    fi
    
    # 去掉 v 前缀
    version=${version#v}
    
    if [[ -z "$version" ]]; then
        echo "0.0.0"
    else
        echo "$version"
    fi
}

# ========== 获取最新版本 ==========
get_latest_version() {
    local url="https://api.github.com/repos/$REPO/releases/latest"
    local version=""
    
    if command -v curl &> /dev/null; then
        version=$(curl -fsSL "$url" 2>/dev/null | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/' | sed 's/^v//')
    elif command -v wget &> /dev/null; then
        version=$(wget -qO- "$url" 2>/dev/null | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/' | sed 's/^v//')
    else
        log_error "需要 curl 或 wget"
        return 1
    fi
    
    if [[ -z "$version" ]]; then
        log_error "无法获取最新版本信息"
        return 1
    fi
    
    echo "$version"
}

# ========== 版本比较 ==========
# 输出: equal, greater, less
compare_versions() {
    # 先去掉 v 前缀
    local v1="${1#v}"
    local v2="${2#v}"
    
    if [[ "$v1" == "$v2" ]]; then
        echo "equal"
        return
    fi
    
    # 将版本号拆分为数组
    IFS='.' read -ra V1 <<< "$v1"
    IFS='.' read -ra V2 <<< "$v2"
    
    for i in 0 1 2; do
        local n1=${V1[$i]:-0}
        local n2=${V2[$i]:-0}
        
        if (( n1 > n2 )); then
            echo "greater"
            return
        elif (( n1 < n2 )); then
            echo "less"
            return
        fi
    done
    
    echo "equal"
}

# ========== 下载二进制文件 ==========
# 成功返回临时目录路径，失败返回空
download_binary() {
    local version="$1"
    local arch="$2"
    local filename="chaoleme-linux-${arch}.tar.gz"
    local url="https://github.com/$REPO/releases/download/v${version}/${filename}"
    local tmp_dir
    tmp_dir=$(mktemp -d)
    
    log_info "下载 $filename ..."
    
    local download_ok=false
    if command -v curl &> /dev/null; then
        if curl -fsSL "$url" -o "$tmp_dir/$filename" 2>/dev/null; then
            download_ok=true
        fi
    elif command -v wget &> /dev/null; then
        if wget -q "$url" -O "$tmp_dir/$filename" 2>/dev/null; then
            download_ok=true
        fi
    fi
    
    if [[ "$download_ok" != "true" ]]; then
        log_error "下载失败: $url"
        rm -rf "$tmp_dir"
        return 1
    fi
    
    log_info "解压文件..."
    if ! tar -xzf "$tmp_dir/$filename" -C "$tmp_dir" 2>/dev/null; then
        log_error "解压失败"
        rm -rf "$tmp_dir"
        return 1
    fi
    
    echo "$tmp_dir"
}

# ========== 备份当前版本 ==========
backup_current() {
    local current_version="$1"
    
    if [[ -x "$INSTALL_DIR/$BINARY_NAME" ]]; then
        mkdir -p "$BACKUP_DIR"
        local backup_file="$BACKUP_DIR/${BINARY_NAME}-${current_version}-$(date +%Y%m%d%H%M%S)"
        if cp "$INSTALL_DIR/$BINARY_NAME" "$backup_file" 2>/dev/null; then
            log_info "已备份当前版本到: $backup_file"
        else
            log_warn "备份失败，继续更新..."
        fi
    fi
}

# ========== 停止服务 ==========
stop_service() {
    if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
        log_info "停止服务..."
        systemctl stop "$SERVICE_NAME" 2>/dev/null || true
    fi
}

# ========== 启动服务 ==========
start_service() {
    if systemctl is-enabled --quiet "$SERVICE_NAME" 2>/dev/null; then
        log_info "启动服务..."
        systemctl start "$SERVICE_NAME" 2>/dev/null || true
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
        return 1
    fi
    
    stop_service
    
    # 替换二进制
    log_info "安装新版本..."
    if ! cp "$binary_file" "$INSTALL_DIR/$BINARY_NAME"; then
        log_error "复制二进制文件失败"
        rm -rf "$tmp_dir"
        return 1
    fi
    chmod +x "$INSTALL_DIR/$BINARY_NAME"
    
    start_service
    
    # 清理临时文件
    rm -rf "$tmp_dir"
}

# ========== 回滚 ==========
rollback() {
    local backup_file
    backup_file=$(ls -t "$BACKUP_DIR"/${BINARY_NAME}-* 2>/dev/null | head -1)
    
    if [[ -z "$backup_file" || ! -f "$backup_file" ]]; then
        log_error "没有可用的备份文件"
        return 1
    fi
    
    log_info "回滚到: $backup_file"
    
    stop_service
    
    if ! cp "$backup_file" "$INSTALL_DIR/$BINARY_NAME"; then
        log_error "回滚失败"
        return 1
    fi
    chmod +x "$INSTALL_DIR/$BINARY_NAME"
    
    start_service
    
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
        if rollback; then
            exit 0
        else
            exit 1
        fi
    fi
    
    # 强制更新标志
    local force_update=false
    if [[ "$target_version" == "--force" || "$target_version" == "-f" ]]; then
        force_update=true
        target_version=""
    fi
    
    # 获取架构
    local arch
    arch=$(detect_arch) || exit 1
    log_info "检测到架构: $arch"
    
    # 获取当前版本
    local current_version
    current_version=$(get_current_version)
    log_info "当前版本: $current_version"
    
    # 获取目标版本
    if [[ -z "$target_version" ]]; then
        target_version=$(get_latest_version) || exit 1
    else
        target_version=${target_version#v}  # 去掉 v 前缀
    fi
    log_info "目标版本: $target_version"
    
    # 版本比较（强制更新时跳过）
    if [[ "$force_update" == "true" ]]; then
        log_info "强制更新: $current_version -> $target_version"
    else
        local cmp_result
        cmp_result=$(compare_versions "$current_version" "$target_version")
        
        case "$cmp_result" in
            equal)
                log_info "已是最新版本，无需更新"
                exit 0
                ;;
            greater)
                log_warn "目标版本 ($target_version) 低于当前版本 ($current_version)"
                read -p "确认降级? [y/N] " confirm
                if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
                    log_info "取消操作"
                    exit 0
                fi
                ;;
            less)
                log_info "准备升级: $current_version -> $target_version"
                ;;
        esac
    fi
    
    # 备份
    backup_current "$current_version"
    
    # 下载
    local tmp_dir
    tmp_dir=$(download_binary "$target_version" "$arch") || exit 1
    
    # 安装
    install_binary "$tmp_dir" "$arch" || exit 1
    
    # 等待一下让服务启动
    sleep 1
    
    # 验证
    local new_version
    new_version=$(get_current_version)
    # 比较时统一去掉 v 前缀
    if [[ "${new_version#v}" == "${target_version#v}" ]]; then
        log_info "✅ 更新成功: $current_version -> ${new_version#v}"
    else
        log_error "版本验证失败，期望 $target_version，实际 $new_version"
        log_warn "尝试回滚..."
        if rollback; then
            log_info "回滚成功"
        else
            log_error "回滚失败，请手动处理"
        fi
        exit 1
    fi
}

main "$@"
