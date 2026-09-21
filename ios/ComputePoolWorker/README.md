# ComputePoolWorker（iOS / iPadOS）

SwiftUI Worker：连接 Mac 上的 ComputePool Hub，领取并执行 `cpu_hash` / `echo` / `sleep` / `image_resize` 分片。

## 在 Mac 上运行（个人 Team 即可）

1. 用 Xcode 打开本目录的 `ComputePoolWorker.xcodeproj`
2. 选中 Target → **Signing & Capabilities** → 勾选 Automatically manage signing  
   选择你的 **Personal Team**（免费 Apple ID 即可，只能装到自己的设备）
3. 用数据线连接 iPhone / iPad，信任此电脑
4. 顶部设备选你的真机 → 点 **Run**
5. 首次在设备上：设置 → 通用 → VPN 与设备管理 → 信任开发者证书
6. 若弹出「本地网络」权限，点允许

**不需要**上架 App Store；MVP 仅侧载到自己的设备。

## 使用

1. Mac 上启动 ComputePool Hub，控制面板复制加入 URL（形如 `http://192.168.x.x:9797`）
2. 在 Worker App 粘贴 URL →「连接并领取任务」
3. **保持 App 前台**（后台会被系统挂起，长轮询会停）
4. 回到 Mac 控制面板点「算力对比」，或用「图片任务」把 Inbox 图片扔到 iPad 处理

### image_resize 限制

- 单张原始体积建议 **≤ 12MB**（base64 JSON 协议；过大分片会失败并跳过）
- 使用 UIKit / ImageIO：解码 → 按 `maxEdge` 等比缩放 → JPEG/PNG（WebP 尽力）
- **保持 App 前台**；大图回传可能需数十秒

## 要求

- iOS / iPadOS 16+
- 与 Mac 同一 Wi‑Fi / 局域网
- Hub 默认端口 `9797`

## 协议摘要

与仓库根 README 一致：

- `POST /api/worker/register` `{id,name,platform,cores}`
- `GET  /api/worker/poll?id=…&timeout=20` → 分片 JSON 或 HTTP 204
- `POST /api/worker/result` `{jobId,shardId,workerId,result,metrics}`
