# 超了么 (Chaoleme)

[![Build and Release](https://github.com/Catker/chaoleme/actions/workflows/release.yml/badge.svg)](https://github.com/Catker/chaoleme/actions/workflows/release.yml)

轻量级 VPS 超售检测工具，通过监控 CPU Steal Time、I/O Wait、磁盘延迟等多维指标，精准评估 VPS 是否存在超售问题。

## ✨ 特性

- 🔍 **多维度检测**
  - CPU Steal Time - 虚拟化资源争抢的核心指标
  - I/O Wait - 检测存储 I/O 瓶颈
  - 4KB 随机读写延迟 - 使用 O_DIRECT 绕过缓存，测量真实磁盘性能
  - 磁盘繁忙度 - 从 `/proc/diskstats` 采集系统级 I/O 统计
  - 内存可用率
- 📊 **智能评分**：加权评分系统，自动检测 SSD/HDD 并适配阈值
- 📈 **基线对比**：与历史数据对比，检测性能退化
- 🤖 **AI 分析**：可选接入 OpenAI 兼容 API 生成智能评价
- 📱 **Telegram 通知**：支持日报/周报/月报，多主机标识
- 💾 **低资源消耗**：内存 < 10MB，CPU < 0.1%
- 🚀 **单二进制部署**：无依赖，下载即用

## 📦 快速安装

### 下载预编译版本

从 [Releases](https://github.com/Catker/chaoleme/releases) 下载对应架构的二进制文件：

```bash
# amd64
wget https://github.com/Catker/chaoleme/releases/latest/download/chaoleme-linux-amd64.tar.gz
tar -xzf chaoleme-linux-amd64.tar.gz

# arm64
wget https://github.com/Catker/chaoleme/releases/latest/download/chaoleme-linux-arm64.tar.gz
tar -xzf chaoleme-linux-arm64.tar.gz
```

```bash
chmod +x install.sh
sudo ./install.sh
```

### 从源码编译

```bash
git clone https://github.com/Catker/chaoleme.git
cd chaoleme
go build -ldflags="-s -w" -o chaoleme .
```

## ⚙️ 配置

编辑 `/etc/chaoleme/config.yaml`：

```yaml
# 主机标识（可选，用于多机器推送区分）
hostname: "Tokyo-VPS-01"

# Telegram 通知配置
telegram:
  bot_token: "YOUR_BOT_TOKEN"  # 从 @BotFather 获取
  chat_id: "YOUR_CHAT_ID"

# 报告配置
report:
  daily: true
  daily_time: "09:00"
  weekly: true
  weekly_day: 0     # 0=周日
  monthly: true
  monthly_day: 1

# 采集配置
collect:
  cpu_steal_interval: "5m"   # CPU 采集间隔
  io_test_interval: "15m"    # I/O 延迟测试间隔
  io_test_size_mb: 4         # I/O 测试文件大小

# AI 分析（可选）
ai:
  enabled: false
  api_url: "https://api.openai.com/v1/chat/completions"
  api_key: "YOUR_API_KEY"
  model: "gpt-4o-mini"
```

## 🚀 使用

```bash
# 测试 Telegram 连接
chaoleme --test-telegram

# 启动服务
systemctl start chaoleme
systemctl enable chaoleme

# 手动发送报告
chaoleme --report daily
chaoleme --report weekly
chaoleme --report monthly

# 仅采集一次数据
chaoleme --collect-once
```

## 📊 评分规则

| 指标 | 权重 | 满分标准 |
|-----|-----|---------| 
| CPU Steal | 35% | < 3% |
| CPU IOWait | 10% | < 5% |
| CPU 稳定性 | 10% | 变异系数 < 0.05 |
| 顺序 I/O 延迟 | 10% | SSD < 20ms / HDD < 50ms (P95) |
| 随机 I/O 延迟 | 15% | SSD < 5ms / HDD < 30ms (P95) |
| 磁盘繁忙度 | 5% | < 20% |
| 内存可用率 | 10% | > 90% |
| 基线偏离 | 5% | < 10% |

**风险等级**：
- 90-100: ✅ 优秀
- 70-89: 🟢 良好
- 50-69: ⚠️ 中等（可能超售）
- 0-49: 🔴 严重超售

## 📋 报告示例

```
📊 超了么日报 [Tokyo-VPS-01]
📅 2025-12-25

━━━━━━━━━━━━━━━━━━
🖥️ CPU 超售风险: ⚠️ 中等
   • Steal Time 平均: 3.2%
   • Steal Time 峰值: 18.7%
   • IOWait 平均: 2.1%
   • 性能波动系数: 0.23

💾 I/O 超售风险: ✅ 低
   • 顺序写延迟 P95: 8.3ms
   • 随机写延迟 P95: 3.2ms
   • 随机读延迟 P95: 2.8ms
   • 磁盘繁忙度: 12%

📈 基线对比: 正常
   • 性能偏离: +5%

━━━━━━━━━━━━━━━━━━
📈 综合评分: 72/100

🤖 AI 分析:
该 VPS 存在轻度 CPU 超售，建议关注高峰期表现。
━━━━━━━━━━━━━━━━━━
```

## 🔧 技术细节

### 磁盘测试

- **测试目录选择**：自动避开 tmpfs（内存盘），确保测试真实磁盘
- **O_DIRECT 模式**：4KB 随机读写使用 O_DIRECT 绕过页缓存
- **存储类型检测**：自动识别 SSD/HDD 并应用不同评分阈值

### 超售检测原理

| 指标 | 检测目标 | 说明 |
|-----|---------|-----|
| CPU Steal | CPU 超售 | 宿主机 CPU 资源不足时，虚拟机被"偷走"的 CPU 时间 |
| IOWait | I/O 瓶颈 | 进程等待 I/O 完成的时间，可能指示共享存储超售 |
| 随机 I/O | 存储超售 | 4KB 随机读写延迟是 SSD/HDD 性能的敏感指标 |
| 基线对比 | 性能退化 | 与历史数据对比，检测性能是否逐渐恶化 |

## 📄 License

MIT License
