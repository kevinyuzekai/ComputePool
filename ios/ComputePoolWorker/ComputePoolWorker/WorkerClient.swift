import Foundation
import UIKit

@MainActor
final class WorkerClient: ObservableObject {
    @Published var running = false
    @Published var connected = false
    @Published var statusText = "未连接"
    @Published var jobsDone = 0
    @Published var lastThroughput = "—"

    let platformLabel: String = {
        let idiom = UIDevice.current.userInterfaceIdiom
        let kind = idiom == .pad ? "iPadOS" : "iOS"
        return "\(kind)/\(UIDevice.current.systemVersion)"
    }()

    private var workerID = UUID().uuidString
    private var task: Task<Void, Never>?
    private var baseURL: URL?

    func start(hubURL: String, name: String) {
        stop()
        var raw = hubURL.trimmingCharacters(in: .whitespacesAndNewlines)
        if raw.hasSuffix("/") { raw = String(raw.dropLast()) }
        guard let url = URL(string: raw), url.scheme == "http" || url.scheme == "https" else {
            statusText = "URL 无效"
            return
        }
        baseURL = url
        running = true
        statusText = "正在注册…"
        let cores = ProcessInfo.processInfo.activeProcessorCount
        let id = workerID
        task = Task.detached { [weak self] in
            await self?.loop(base: url, id: id, name: name, cores: cores)
        }
    }

    func stop() {
        task?.cancel()
        task = nil
        running = false
        connected = false
        statusText = "已断开"
    }

    private func loop(base: URL, id: String, name: String, cores: Int) async {
        while !Task.isCancelled {
            do {
                try await register(base: base, id: id, name: name, cores: cores)
                await MainActor.run {
                    self.connected = true
                    self.statusText = "已连接，等待任务…"
                }
                if let asg = try await poll(base: base, id: id) {
                    await MainActor.run { self.statusText = "执行 \(asg.type)…" }
                    try await execute(base: base, id: id, asg: asg)
                }
            } catch is CancellationError {
                break
            } catch {
                await MainActor.run {
                    self.connected = false
                    self.statusText = "重试中：\(error.localizedDescription)"
                }
                try? await Task.sleep(nanoseconds: 1_500_000_000)
            }
        }
    }

    private struct Assignment: Decodable {
        let jobId: String
        let shardId: String
        let type: String
        let payload: AnyCodableJSON
    }

    private func register(base: URL, id: String, name: String, cores: Int) async throws {
        let url = base.appendingPathComponent("api/worker/register")
        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        let body: [String: Any] = [
            "id": id,
            "name": name,
            "platform": platformLabel,
            "cores": cores,
            "local": false,
        ]
        req.httpBody = try JSONSerialization.data(withJSONObject: body)
        let (_, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw NSError(domain: "ComputePool", code: 2, userInfo: [NSLocalizedDescriptionKey: "register failed"])
        }
    }

    private func poll(base: URL, id: String) async throws -> Assignment? {
        var comp = URLComponents(url: base.appendingPathComponent("api/worker/poll"), resolvingAgainstBaseURL: false)!
        comp.queryItems = [
            URLQueryItem(name: "id", value: id),
            URLQueryItem(name: "timeout", value: "20"),
        ]
        var req = URLRequest(url: comp.url!)
        req.timeoutInterval = 30
        let (data, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse else { return nil }
        if http.statusCode == 204 { return nil }
        guard (200..<300).contains(http.statusCode) else {
            throw NSError(domain: "ComputePool", code: 3, userInfo: [NSLocalizedDescriptionKey: "poll \(http.statusCode)"])
        }
        if data.isEmpty { return nil }
        return try JSONDecoder().decode(Assignment.self, from: data)
    }

    private func execute(base: URL, id: String, asg: Assignment) async throws {
        let payloadData = asg.payload.raw
        let out: JobRunner.RunOutput
        do {
            out = try JobRunner.run(type: asg.type, payload: payloadData)
        } catch {
            try await postResult(base: base, id: id, jobId: asg.jobId, shardId: asg.shardId, result: Data("{}".utf8), metrics: [:], error: error.localizedDescription)
            throw error
        }
        var metrics: [String: Any] = ["elapsedMs": out.elapsedMs]
        if out.hashesPerSec > 0 {
            metrics["hashesPerSec"] = out.hashesPerSec
        }
        try await postResult(base: base, id: id, jobId: asg.jobId, shardId: asg.shardId, result: out.resultJSON, metrics: metrics, error: nil)
        await MainActor.run {
            self.jobsDone += 1
            if out.hashesPerSec > 0 {
                if out.hashesPerSec >= 1_000_000 {
                    self.lastThroughput = String(format: "%.2f Mhash/s", out.hashesPerSec / 1_000_000)
                } else if out.hashesPerSec >= 1000 {
                    self.lastThroughput = String(format: "%.1f khash/s", out.hashesPerSec / 1000)
                } else {
                    self.lastThroughput = String(format: "%.0f hash/s", out.hashesPerSec)
                }
            }
            self.statusText = "已连接，等待任务…"
        }
    }

    private func postResult(base: URL, id: String, jobId: String, shardId: String, result: Data, metrics: [String: Any], error: String?) async throws {
        let url = base.appendingPathComponent("api/worker/result")
        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        let resultObj = try JSONSerialization.jsonObject(with: result)
        var body: [String: Any] = [
            "jobId": jobId,
            "shardId": shardId,
            "workerId": id,
            "result": resultObj,
            "metrics": metrics,
        ]
        if let error { body["error"] = error }
        req.httpBody = try JSONSerialization.data(withJSONObject: body)
        let (_, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw NSError(domain: "ComputePool", code: 4, userInfo: [NSLocalizedDescriptionKey: "result failed"])
        }
    }
}

/// Keeps arbitrary JSON payload as raw Data for JobRunner.
struct AnyCodableJSON: Decodable {
    let raw: Data
    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        let obj = try container.decode(JSONValue.self)
        raw = try JSONEncoder().encode(obj)
    }
}

enum JSONValue: Codable {
    case string(String), number(Double), bool(Bool), object([String: JSONValue]), array([JSONValue]), null
    init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null; return }
        if let v = try? c.decode(Bool.self) { self = .bool(v); return }
        if let v = try? c.decode(Double.self) { self = .number(v); return }
        if let v = try? c.decode(String.self) { self = .string(v); return }
        if let v = try? c.decode([String: JSONValue].self) { self = .object(v); return }
        if let v = try? c.decode([JSONValue].self) { self = .array(v); return }
        self = .null
    }
    func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        case .string(let v): try c.encode(v)
        case .number(let v): try c.encode(v)
        case .bool(let v): try c.encode(v)
        case .object(let v): try c.encode(v)
        case .array(let v): try c.encode(v)
        case .null: try c.encodeNil()
        }
    }
}
