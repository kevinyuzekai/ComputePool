# ComputePool（设备算力聚合）

让 **MacBook Pro** 把同一局域网里的 **iPhone / iPad** 当成可拆分任务的算力 Worker —— Mac 负责编排，手机 / 平板负责执行分片。包装风格对齐 [LinkPool](https://github.com/kevinyuzekai/LinkPool)（Go 单二进制 + 嵌入式中文 Web UI）。

> **诚实边界（请先读）**  
> ComputePool **不是**透明的操作系统 CPU / GPU 融合，也**不会**让任意 App 自动变快。  
> 它是一个 **LAN 任务编排器**：把**可拆分**的作业切成 shard，调度到 Mac 本机 worker 协程 + 手机 / 平板 Worker。  
> 适合演示与批处理（如 `cpu_hash`）；不要指望「插上 iPad，系统就多了几个核」。

**English:** ComputePool is a LAN orchestrator that shards *splittable* CPU jobs across the Mac (local goroutine workers) and iPhone/iPad workers. It is **not** transparent OS-level CPU/GPU fusion. Version **0.1.0**, MIT.

---

## 功能一览

| 能力 | 状态 |
|--|--|
| Mac Hub 监听 LAN（默认 `0.0.0.0:9797`） | ✅ |
| Bonjour/mDNS `_computepool._tcp`（Darwin + `dns-sd`） | ✅ 尽力 |
| Worker 注册表（名称 / 平台 / 核心 / 心跳 / 吞吐） | ✅ |
| 本机 local worker（无手机也能跑） | ✅ |
| 演示作业 `cpu_hash`（SHA-256 迭代）+ 算力对比 | ✅ |
| 协议测试 `echo` / `sleep` | ✅ |
| 嵌入式 zh-CN Web UI + 复制加入 URL | ✅ |
| iOS / iPadOS SwiftUI Worker | ✅ |
| `.app` / `.dmg` 双架构脚本 | ✅（DMG 需在 Mac 上打） |
| 透明 GPU 融合 / 越狱 / App Store / Windows | ❌ 不做 |

---

## 架构

```
┌──────────────────┐     HTTP/JSON 长轮询      ┌─────────────────────┐
│  Web UI :9797    │◄────────────────────────►│  Mac Hub (Go)       │
│  算力对比 / 设备  │                           │  注册表 · 分片队列   │
└──────────────────┘                           │  local workers ×N   │
                                               └──────────┬──────────┘
                     ┌────────────────────────────────────┼────────────────┐
                     ▼                                    ▼                ▼
            ┌────────────────┐                 ┌──────────────┐   ┌──────────────┐
            │ Mac goroutine  │                 │ iPhone Worker│   │ iPad Worker  │
            │ cpu_hash shard │                 │ SwiftUI app  │   │ SwiftUI app  │
            └────────────────┘                 └──────────────┘   └──────────────┘
```

**算力对比：** 同一总迭代量先只调度 **Mac local** 分片，再调度 **Mac + 全部在线设备**，对比墙钟时间得到加速比。

---

## 协议（Worker ↔ Hub）

Base URL：`http://<mac-lan-ip>:9797`

### `POST /api/worker/register`

```json
{ "id": "uuid", "name": "Jack’s iPhone", "platform": "iOS/18.0", "cores": 6 }
```

### `GET /api/worker/poll?id=<workerId>&timeout=25`

- 有任务 → `200` + `{ "jobId", "shardId", "type", "payload" }`
- 空闲 → `204`

### `POST /api/worker/result`

```json
{
  "jobId": "job-1",
  "shardId": "s-1",
  "workerId": "uuid",
  "result": { "digest": "...", "iterations": 100000, "elapsedMs": 42 },
  "metrics": { "elapsedMs": 42, "hashesPerSec": 2.3e6 }
}
```

### 作业类型

| type | payload | 说明 |
|--|--|--|
| `cpu_hash` | `{ "seed", "iterations" }` | CPU 密集 SHA-256 链式迭代 |
| `echo` | `{ "message" }` | 协议连通性 |
| `sleep` | `{ "ms" }` | 协议连通性 |

### UI / 管理 API（摘要）

- `GET /api/status` · `POST /api/hub/start` · `POST /api/hub/stop`
- `GET /api/workers` · `GET /api/jobs` · `GET /api/jobs/:id`
- `POST /api/benchmark` `{iterations,shards}` → `GET /api/benchmark/:compareId`
- `POST /api/test/echo` · `POST /api/test/sleep`

本 MVP 使用 HTTP 长轮询；WebSocket keepalive 可后续加。

---

## 在 Mac 上构建 Hub

需要 Go 1.22+。

```bash
git clone https://github.com/kevinyuzekai/ComputePool.git
cd ComputePool
make test
make build          # → bin/computepool
./bin/computepool -open
```

浏览器打开 `http://127.0.0.1:9797`。控制面板会显示 LAN 加入 URL（如 `http://192.168.1.23:9797`）。

### 打出 .app / .dmg（双架构）

```bash
# Apple Silicon
ARCH=arm64 ./scripts/build-macos.sh
ARCH=arm64 ./scripts/package-dmg.sh   # → build/macos/arm64/ComputePool-0.1.0-arm64.dmg（需 macOS + hdiutil）

# Intel Mac
ARCH=amd64 ./scripts/build-macos.sh
ARCH=amd64 ./scripts/package-dmg.sh   # → build/macos/amd64/ComputePool-0.1.0-amd64.dmg
```

Darwin 上脚本会对 `.app` 做 **ad-hoc codesign**。Linux 可交叉编译出二进制与 `.app` 布局，但不能生成 DMG。

---

## 用 1 部 iPhone 做端到端测试

1. Mac：`./bin/computepool -open`（或打开 `.app`）
2. 控制面板复制「Worker 加入」URL
3. Xcode 打开 [`ios/ComputePoolWorker/ComputePoolWorker.xcodeproj`](ios/ComputePoolWorker/)  
   → Signing 选 Personal Team → Run 到真机（详见 [`ios/ComputePoolWorker/README.md`](ios/ComputePoolWorker/README.md)）
4. iPhone：粘贴 URL → 连接，**保持前台**
5. Mac UI 设备表应出现该 iPhone
6. 点「算力对比」：先看 Mac 单机耗时，再看 Mac+设备耗时与加速比

没有手机时，仅 Mac local workers 也能跑通对比（加速比接近 1×，用于自检）。

---

## 目录结构

```
cmd/computepool/          # Hub 入口
internal/hub/             # 注册表 · 分片队列 · 算力对比
internal/jobs/            # cpu_hash / echo / sleep
internal/localworker/     # Mac 本机 worker 协程
internal/mdns/            # Bonjour（darwin）
internal/api/             # HTTP JSON API
web/static/               # 嵌入式中文 UI
ios/ComputePoolWorker/    # SwiftUI Worker
scripts/build-macos.sh
scripts/package-dmg.sh
```

---

## 局限与非目标

- 不是 MPTCP / 系统扩展 / 透明多核融合
- Worker 需保持 App 前台；锁屏后任务会停
- 仅适合可拆分、可验证的作业；共享可变状态的任务需自行设计
- 无鉴权（默认信任局域网）；勿暴露到公网
- Windows 客户端不在本版范围

---

## License

MIT © 2026 kevinyuzekai
