# 超了么 (Chaoleme)

[![Build and Release](https://github.com/Catker/chaoleme/actions/workflows/release.yml/badge.svg)](https://github.com/Catker/chaoleme/actions/workflows/release.yml)

轻量级 VPS 超售检测工具，通过 CPU Steal Time、I/O Wait、磁盘延迟、负载与历史趋势等指标，基于证据等级评估 VPS 是否存在超售或资源争抢。

## ✨ 特性

- 🔍 **多维度检测**
  - CPU Steal Time - 虚拟化资源争抢的核心指标
  - I/O Wait - 检测存储 I/O 瓶颈
  - 4KB 随机读写延迟 - 使用 O_DIRECT 绕过缓存，测量真实磁盘性能
  - 磁盘繁忙度 - 从 `/proc/diskstats` 采集系统级 I/O 统计
  - Linux PSI 压力指标 - 从 `/proc/pressure/*` 采集 CPU/IO 等待压力
  - cgroup CPU 限额节流 - 识别容器或限额导致的本机 CPU 受限
  - 运行环境上下文 - 识别虚拟化类型、容器环境和 Steal 可解释性
  - 内存可用率
- 📊 **证据判定**：区分强证据、辅助证据和缺失数据，避免把本机压力误判为超售
- 📈 **历史趋势**：用稳健统计和质量评级识别性能退化
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

编辑 `/opt/chaoleme/config/config.yaml`：

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

# 存储配置
storage:
  db_path: "/opt/chaoleme/data/data.db"
  retention_days: 90  # 月报历史趋势至少需要 90 天

# 采集配置
collect:
  cpu_steal_interval: "5m"   # CPU Steal 采集间隔
  cpu_bench_interval: "30m"  # CPU 基准测试间隔
  io_test_interval: "15m"    # I/O 延迟测试间隔
  io_test_size_mb: 4         # I/O 测试文件大小

# AI 分析（可选，修改后会在下一次生成报告时自动加载最新配置）
ai:
  enabled: false
  api_url: "https://api.openai.com/v1/chat/completions"
  api_key: "YOUR_API_KEY"
  model: "gpt-4o-mini"
  daily: true    # 日报启用 AI 评价
  weekly: true   # 周报启用 AI 评价
  monthly: true  # 月报启用 AI 评价
```

## 🚀 使用

```bash
# 测试 Telegram 连接
chaoleme --test-telegram

# 启动服务
systemctl start chaoleme
systemctl enable chaoleme

# 本地预览报告，不发送 Telegram，不调用 AI
chaoleme --report-preview daily
chaoleme --report-preview weekly
chaoleme --report-preview monthly

# 输出机器可读 JSON 报告，便于脚本校验
chaoleme --report-json daily

# 检查报告证据并返回退出码：0 未判定超售，1 存在风险，2 数据不足
chaoleme --report-check daily

# 完整实机验证：环境诊断 + 报告证据检查
chaoleme --verify-evidence daily

# 手动发送报告
chaoleme --report daily
chaoleme --report weekly
chaoleme --report monthly

# 诊断当前环境是否支持超售证据采集
chaoleme --diagnose

# 仅采集一次数据
chaoleme --collect-once

# 连续采样一段时间，用于生成足够样本
chaoleme --collect-for 24h --collect-interval 5m --collect-io-interval 15m
```

连续采样会高频采集 CPU Steal、IOWait、Load、PSI 和 cgroup 节流；
CPU Bench、I/O 测试、内存和磁盘统计按 `--collect-io-interval` 低频采集，减少采样本身对 I/O 的影响。
启动时会输出预计样本数，便于确认样本规模是否足够。

### 自动更新

```bash
# 更新到最新版本
sudo ./update.sh

# 更新到指定版本
sudo ./update.sh v1.3.0

# 强制重新安装当前版本
sudo ./update.sh --force

# 回滚到上一版本
sudo ./update.sh rollback
```

### 卸载

```bash
# 完全卸载 chaoleme（包括配置和数据）
sudo ./uninstall.sh
```

## 📊 评分规则

| 指标 | 权重 | 满分标准 |
|-----|-----|---------| 
| CPU Steal | 35% | < 3% |
| CPU IOWait | 10% | < 5% |
| CPU 稳定性 | 10% | 变异系数 < 0.05 |
| 顺序 I/O 延迟 | 15% | SSD < 20ms / HDD < 50ms (P95) |
| 随机 I/O 延迟 | 10% | SSD < 30ms / HDD < 100ms (P95) |
| 磁盘繁忙度 | 5% | < 30% |
| 内存可用率 | 10% | > 90% |
| 历史趋势 | 5% | 可参考且偏离 < 10% |

**历史趋势数据要求**：
- 日报：当前 1 天 + 前 14 天历史窗口，至少保留 15 天
- 周报：当前 7 天 + 前 28 天历史窗口，至少保留 35 天
- 月报：当前约 30 天 + 前 60 天历史窗口，至少保留 90 天

**历史趋势质量**：
- `usable`：可参考，可作为辅助趋势证据
- `weak`：弱参考，只影响健康分展示，不参与超售判定
- `contaminated`：历史窗口疑似已有异常，不作为趋势证据
- `unavailable`：样本不足或查询失败

**健康等级**：
- 90-100: ✅ 优秀
- 70-89: 🟢 良好
- 50-69: ⚠️ 中等
- 0-49: 🔴 严重

**超售判定**：
- 🔴 高度可能超售：CPU Steal 达到强证据阈值，且运行环境支持直接解释 Steal
- ⚠️ 可能存在超售或资源争抢：存在 Steal、I/O 或可参考的历史趋势异常，但证据链不完整
- 📊 更像本机负载导致：Load 较高且 Steal 正常
- ✅ 暂无明显超售证据：核心指标未达到阈值
- ⚪ 数据不足：核心样本不足，不能判定

**核心样本要求**：
- 日报：至少 12 个 CPU Steal/IOWait 样本
- 周报：至少 36 个 CPU Steal/IOWait 样本
- 月报：至少 72 个 CPU Steal/IOWait 样本
- 核心样本时间覆盖率至少 50%，避免短时间密集采样误代表整个周期

## 📋 报告示例

```
📊 超了么日报 [Tokyo-VPS-01]
📅 2025-12-25

━━━━━━━━━━━━━━━━━━
🧭 超售判定: ⚠️ 可能存在超售或资源争抢
🔎 证据等级: 中

🖥️ CPU 争抢证据: ⚠️ 中等
   • Steal Time 平均: 3.2%
   • Steal Time 峰值: 18.7%
   • IOWait 平均: 2.1%
   • 性能波动系数: 0.23

🧪 样本覆盖:
   • CPU Steal/IOWait 样本: 288/288
   • 核心覆盖: 23.9小时 / 99.6%

🧱 运行环境:
   • 虚拟化类型: kvm
   • Steal 可直接解释: true

💾 存储争抢证据: ✅ 低
   • 顺序写延迟 P95: 8.3ms
   • 随机写延迟 P95: 3.2ms
   • 随机读延迟 P95: 2.8ms
   • 磁盘繁忙度: 12%

📈 历史趋势: 正常
   • 性能偏离: +5%

━━━━━━━━━━━━━━━━━━
📈 健康评分: 72/100
📋 健康等级: 🟢 良好

🤖 AI 分析:
CPU Steal 有持续异常，存在资源争抢风险，建议继续观察高峰时段样本。
━━━━━━━━━━━━━━━━━━
```

## 🔧 技术细节

### 磁盘测试

- **测试目录选择**：自动避开 tmpfs（内存盘），确保测试真实磁盘
- **O_DIRECT 模式**：4KB 随机读写使用 O_DIRECT 绕过页缓存
- **O_DIRECT 有效样本**：不可用时随机 I/O 仅作参考，不作为强 I/O 证据
- **存储类型检测**：自动识别 SSD/HDD 并应用不同评分阈值

### 环境诊断

部署后建议先运行：

```bash
chaoleme --diagnose
```

诊断会检查 `/proc/stat`、Linux PSI、cgroup CPU 节流、磁盘统计和运行环境上下文。
处于容器内或 CPU Steal 不能直接解释时，`--diagnose` / `--verify-evidence` 会返回失败，避免把弱证据当成可靠结论。

连续采样后可用本地预览检查规则判定：

```bash
chaoleme --report-preview daily
```

需要手动积累样本时，可直接运行：

```bash
chaoleme --collect-for 24h --collect-interval 5m --collect-io-interval 15m
```

也可以输出机器可读 JSON，便于自动校验：

```bash
chaoleme --report-json daily
```

也可以直接检查报告证据：

```bash
chaoleme --verify-evidence daily
```

`--verify-evidence` 的退出码：

- `0`：报告证据有效，未判定超售
- `1`：报告证据有效，存在超售或资源争抢风险
- `2`：数据不足、查询错误或证据等级偏低，不能判定

`--report-check` 只检查报告数据；`--verify-evidence` 会先做环境诊断。
运行环境无法可靠解释 CPU Steal 时，检查命令会返回 `2`。
这些本地报告命令都不会发送 Telegram，也不会调用 AI，适合用于实机验证。

项目也提供了完整实机验证脚本：

```bash
CHAOLEME_BIN=chaoleme CHAOLEME_CONFIG=/opt/chaoleme/config/config.yaml \
  scripts/verify_oversell_evidence.sh daily
```

脚本内部调用的是 `chaoleme --verify-evidence`，会执行环境诊断和报告证据检查，不依赖 Python 或 jq。

### 超售检测原理

| 指标 | 检测目标 | 说明 |
|-----|---------|-----|
| CPU Steal | 强证据 | 宿主机 CPU 资源不足时，虚拟机被等待的 CPU 时间 |
| IOWait | 辅助证据 | 进程等待 I/O 完成的时间，需结合 Load 与磁盘指标判断 |
| 随机 I/O | 辅助证据 | 4KB 随机读写延迟是存储争抢的敏感指标 |
| 磁盘繁忙度 | 辅助证据 | 基于 `/proc/diskstats` 累计 IO 时间增量计算 |
| Linux PSI | 辅助证据 | 衡量任务因 CPU/IO 资源不足产生等待的比例 |
| cgroup CPU 节流 | 排除证据 | 节流明显且 Steal 正常时，更像本机限额而非宿主超售 |
| 运行环境上下文 | 解释边界 | 容器或未知虚拟化环境会降低 Steal 结论强度，超过 7 天的环境记录会被忽略 |
| 历史趋势 | 趋势证据 | 使用 median/P75/P95 与质量评级检测性能是否逐渐恶化 |

## 📄 License

MIT License
