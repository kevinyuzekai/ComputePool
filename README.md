# ComputePool（设备算力聚合）

让 **MacBook Pro** 把同一局域网里的 **iPhone / iPad** 当成可拆分任务的算力 Worker —— Mac 负责编排，手机 / 平板负责执行分片。包装风格对齐 [LinkPool](https://github.com/kevinyuzekai/LinkPool)（Go 单二进制 + 嵌入式中文 Web UI）。

> **诚实边界（请先读）**  
> ComputePool **不是**透明的操作系统 CPU / GPU 融合，也**不会**让任意 App 自动变快，**不会**劫持 Photoshop / Final Cut 等 Mac 应用。  
> 它是一个 **LAN 任务编排器**：把**可拆分**的作业切成 shard，调度到 Mac 本机 worker 协程 + 手机 / 平板 Worker。  
> **v0.2.0** 起支持真实批处理：`image_resize`（缩放 / 压缩图片）。演示作业 `cpu_hash` 仍可用。不要指望「插上 iPad，系统就多了几个核」。

**English:** ComputePool is a LAN orchestrator that shards *splittable* jobs (now including real **image resize/compress**) across the Mac and iPhone/iPad workers. It is **not** transparent OS-level CPU/GPU fusion and does **not** hijack arbitrary Mac apps. Version **0.2.0**, MIT.

---

## 功能一览

| 能力 | 状态 |
|--|--|
| Mac Hub 监听 LAN（默认 `0.0.0.0:9797`） | ✅ |
| Bonjour/mDNS `_computepool._tcp`（Darwin + `dns-sd`） | ✅ 尽力 |
| Worker 注册表（名称 / 平台 / 核心 / 心跳 / 吞吐） | ✅ |
| 本机 local worker（无手机也能跑） | ✅ |
| **图片批处理 `image_resize`（Inbox → iPad/本机 → Outbox）** | ✅ **0.2.0** |
| 演示作业 `cpu_hash`（SHA-256 迭代）+ 算力对比 | ✅ |
| 协议测试 `echo` / `sleep` | ✅ |
| Web UI「图片任务」+「自定义任务」 | ✅ |
| 嵌入式 zh-CN Web UI + 复制加入 URL | ✅ |
| iOS / iPadOS SwiftUI Worker（含图片） | ✅ |
| `.app` / `.dmg` 双架构脚本 | ✅（DMG 需在 Mac 上打） |
| 透明 GPU 融合 / 越狱 / App Store / Windows / 劫持任意 Mac App | ❌ 不做 |

---

## 架构

```
┌──────────────────┐     HTTP/JSON 长轮询      ┌─────────────────────┐
│  Web UI :9797    │◄────────────────────────►│  Mac Hub (Go)       │
│  图片任务 / 对比  │                           │  Inbox · Outbox     │
└──────────────────┘                           │  注册表 · 分片队列   │
                                               └──────────┬──────────┘
                     ┌────────────────────────────────────┼────────────────┐
                     ▼                                    ▼                ▼
            ┌────────────────┐                 ┌──────────────┐   ┌──────────────┐
            │ Mac goroutine  │                 │ iPhone Worker│   │ iPad Worker  │
            │ image_resize   │                 │ UIKit/ImageIO│   │ UIKit/ImageIO│
            └────────────────┘                 └──────────────┘   └──────────────┘
```

**图片流：** 用户把文件丢进 `~/ComputePool-Inbox`（或 Web 多选上传）→ Hub 一图一分片（base64 payload）→ Worker 缩放压缩 → Hub 写入 `~/ComputePool-Outbox/<jobId>/`。

**算力对比：** 同一总迭代量先只调度 **Mac local**，再调度 **Mac + 全部在线设备**，对比墙钟时间。

---

## 图片批处理（v0.2.0）

### 目录

| 路径 | 作用 |
|--|--|
| `~/ComputePool-Inbox` | 默认输入（启动时自动创建） |
| `~/ComputePool-Outbox/<jobId>/` | 处理后的图片 |

可用 `-inbox` / `-outbox` 覆盖。

### Web UI

1. 打开控制面板 → **「图片任务（扔到 iPad）」**
2. 确认 Inbox 路径 → **扫描 Inbox**，或用文件多选上传
3. 设置 `maxEdge`（默认 1920）、`quality`（默认 80）、`format`（jpeg / png；webp 在 iOS 尽力，Mac local 回退 jpeg）
4. **提交图片批处理** → 看进度；完成后打开 Outbox 路径取结果

无 iPad 时：勾选「仅本机」仍可跑通；接上 iPad Worker 后分片会自动分给平板。

### API

```bash
# 扫描 Inbox
curl -s 'http://127.0.0.1:9797/api/images/inbox' | jq .

# 批处理（扫描 Inbox）
curl -s -X POST http://127.0.0.1:9797/api/images/batch \
  -H 'Content-Type: application/json' \
  -d '{"maxEdge":1920,"quality":80,"format":"jpeg"}'

# 或 multipart 上传
curl -s -X POST http://127.0.0.1:9797/api/images/batch \
  -F 'maxEdge=1280' -F 'quality=75' -F 'format=jpeg' \
  -F 'files=@/path/to/a.jpg' -F 'files=@/path/to/b.png'
```

也可用通用 `POST /api/jobs`，`type: "image_resize"`，但需自备 `imageBase64` 且 `shards` 必须为 1；文件夹场景请用 `/api/images/batch`。

### 大小与协议限制（请读）

| 限制 | 说明 |
|--|--|
| **单张 ≤ ~12MB 原始字节** | HTTP JSON + base64 会再膨胀约 4/3；过大则跳过/失败 |
| 建议单张 **8–12MB** 以下 | iPad 回传大图可能需数十秒；Worker 需保持前台 |
| 每批最多 **256** 张 | |
| 输入格式 | jpeg / png / gif（Mac）；iOS 还可试 HEIC 等 UIImage 能解的 |
| 输出格式 | jpeg / png；webp 在 iOS ImageIO 尽力，Mac local **回退 jpeg** |
| 不会劫持 Mac App | 只处理你主动提交 / Inbox 里的文件 |

架构上可继续加更多 job type（视频抽帧、音频转码等），但本版只落地图片。

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
  "result": { "...": "..." },
  "metrics": { "elapsedMs": 42 }
}
```

### 作业类型

| type | payload | 说明 |
|--|--|--|
| `image_resize` | `{ imageBase64, fileName, maxEdge, quality, format }` | 缩放压缩；结果含 `imageBase64` + 尺寸指标 |
| `cpu_hash` | `{ "seed", "iterations" }` | CPU 密集 SHA-256 链式迭代 |
| `echo` | `{ "message" }` | 协议连通性 |
| `sleep` | `{ "ms" }` | 协议连通性 |

### UI / 管理 API（摘要）

- `GET /api/status` · `POST /api/hub/start` · `POST /api/hub/stop`
- `GET /api/workers` · `GET /api/jobs` · `GET /api/jobs/:id`
- `POST /api/jobs` `{type,payload,shards,localOnly,label}`
- `GET /api/images/paths` · `GET /api/images/inbox` · `POST /api/images/batch`
- `POST /api/benchmark` · `GET /api/benchmark/:compareId`
- `POST /api/test/echo` · `POST /api/test/sleep`

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
ARCH=arm64 ./scripts/package-dmg.sh   # → build/macos/arm64/ComputePool-0.2.0-arm64.dmg（需 macOS + hdiutil）

# Intel Mac
ARCH=amd64 ./scripts/build-macos.sh
ARCH=amd64 ./scripts/package-dmg.sh   # → build/macos/amd64/ComputePool-0.2.0-amd64.dmg
```

Darwin 上脚本会对 `.app` 做 **ad-hoc codesign**。Linux 可交叉编译出二进制与 `.app` 布局 / zip，但不能生成 DMG。

---

## 用 iPad 跑真实图片批处理（端到端）

1. Mac：`./bin/computepool -open`（或打开 `.app`）
2. 控制面板复制「Worker 加入」URL
3. **Xcode 重新编译并安装** [`ios/ComputePoolWorker`](ios/ComputePoolWorker/)（0.2.0 含 `image_resize`；旧 App 不认识该类型会失败）  
   → Signing 选 Personal Team → Run 到 iPad / iPhone（详见 [`ios/ComputePoolWorker/README.md`](ios/ComputePoolWorker/README.md)）
4. iPad：粘贴 URL → 连接，**保持前台**
5. Mac：把图片放进 `~/ComputePool-Inbox` → UI「扫描 Inbox」→「提交图片批处理」
6. 完成后打开 `~/ComputePool-Outbox/<jobId>/` 查看结果

没有平板时，仅 Mac local workers 也能处理 Inbox（勾选「仅本机」或不勾选均可；无远程设备时全由本机吃掉）。

---

## 自定义任务（Web UI）

**「自定义任务」** 仍用于 `cpu_hash` / `echo` / `sleep`。图片请用 **「图片任务」**。

算力对比完成后，「最近任务」列表现在会自动刷新。

---

## 目录结构

```
cmd/computepool/          # Hub 入口
internal/hub/             # 注册表 · 分片队列 · 图片批处理 · 算力对比
internal/jobs/            # cpu_hash / echo / sleep / image_resize
internal/paths/           # Inbox / Outbox 路径
internal/localworker/     # Mac 本机 worker 协程
internal/mdns/            # Bonjour（darwin）
internal/api/             # HTTP JSON API
web/static/               # 嵌入式中文 UI
ios/ComputePoolWorker/    # SwiftUI Worker（含 image_resize）
scripts/build-macos.sh
scripts/package-dmg.sh
```

---

## 局限与非目标

- 不是 MPTCP / 系统扩展 / 透明多核融合
- **不会**把任意 Mac App 的 CPU 工作卸载到 iPad
- Worker 需保持 App 前台；锁屏后任务会停
- 图片走 base64 JSON：单张约 **12MB** 上限；超大图请先本地缩小或自行切分
- 无鉴权（默认信任局域网）；勿暴露到公网
- Windows 客户端不在本版范围

---

## License

MIT © 2026 kevinyuzekai
