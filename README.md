# 超了么 (Chaoleme)

[![Build and Release](https://github.com/Catker/chaoleme/actions/workflows/release.yml/badge.svg)](https://github.com/Catker/chaoleme/actions/workflows/release.yml)

轻量级 VPS 超售检测工具，通过监控 CPU Steal Time、I/O 延迟等指标评估 VPS 是否存在超售问题。

## ✨ 特性

- 🔍 **多维度检测**：CPU Steal、I/O 延迟、内存可用率
- 📊 **智能评分**：加权评分系统，自动适配 SSD/HDD
- 🤖 **AI 分析**：可选接入 OpenAI 兼容 API 生成智能评价
- 📱 **Telegram 通知**：支持日报/周报/月报
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

### 一键安装

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
telegram:
  bot_token: "YOUR_BOT_TOKEN"  # 从 @BotFather 获取
  chat_id: "YOUR_CHAT_ID"

report:
  daily: true
  daily_time: "09:00"
  weekly: true
  monthly: true

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
| CPU Steal | 50% | < 3% |
| CPU 稳定性 | 15% | 变异系数 < 0.05 |
| I/O 延迟 | 25% | SSD < 20ms / HDD < 50ms (P95) |
| 内存可用率 | 10% | > 90% |

**风险等级**：
- 90-100: ✅ 优秀
- 70-89: 🟢 良好
- 50-69: ⚠️ 中等（可能超售）
- 0-49: 🔴 严重超售

## 📋 报告示例

```
📊 超了么日报
📅 2025-12-25

━━━━━━━━━━━━━━━━━━
🖥️ CPU 超售风险: ⚠️ 中等
   • Steal Time 平均: 3.2%
   • Steal Time 峰值: 18.7%
   • 性能波动系数: 0.23

💾 I/O 超售风险: ✅ 低
   • 写延迟 P95: 8.3ms

━━━━━━━━━━━━━━━━━━
📈 综合评分: 68/100

🤖 AI 分析:
该 VPS 存在轻度 CPU 超售，建议关注高峰期表现。
━━━━━━━━━━━━━━━━━━
```

## 📄 License

MIT License
