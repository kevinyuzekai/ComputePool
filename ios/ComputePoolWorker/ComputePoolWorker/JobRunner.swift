import Foundation
import CryptoKit

enum JobRunner {
    struct CPUHashPayload: Decodable {
        let seed: String
        let iterations: Int64
    }

    struct EchoPayload: Decodable {
        let message: String
    }

    struct SleepPayload: Decodable {
        let ms: Int64
    }

    struct RunOutput {
        let resultJSON: Data
        let elapsedMs: Int64
        let hashesPerSec: Double
    }

    static func run(type: String, payload: Data) throws -> RunOutput {
        switch type {
        case "cpu_hash":
            return try runCPUHash(payload)
        case "echo":
            let p = try JSONDecoder().decode(EchoPayload.self, from: payload)
            let obj: [String: String] = ["echo": p.message]
            return RunOutput(resultJSON: try JSONSerialization.data(withJSONObject: obj), elapsedMs: 0, hashesPerSec: 0)
        case "sleep":
            let p = try JSONDecoder().decode(SleepPayload.self, from: payload)
            let ms = min(max(p.ms, 0), 60_000)
            let start = Date()
            Thread.sleep(forTimeInterval: Double(ms) / 1000.0)
            let elapsed = Int64(Date().timeIntervalSince(start) * 1000)
            let obj: [String: Int64] = ["sleptMs": elapsed]
            return RunOutput(resultJSON: try JSONSerialization.data(withJSONObject: obj), elapsedMs: elapsed, hashesPerSec: 0)
        default:
            throw NSError(domain: "ComputePool", code: 1, userInfo: [NSLocalizedDescriptionKey: "unknown type \(type)"])
        }
    }

    private static func runCPUHash(_ payload: Data) throws -> RunOutput {
        let p = try JSONDecoder().decode(CPUHashPayload.self, from: payload)
        var iterations = p.iterations
        if iterations <= 0 { iterations = 1 }
        let start = Date()
        var digest = SHA256.hash(data: Data(p.seed.utf8))
        if iterations > 0 {
            for _ in 0..<iterations {
                digest = SHA256.hash(data: Data(digest))
            }
        }
        let elapsed = max(Date().timeIntervalSince(start), 0.000001)
        let elapsedMs = Int64(elapsed * 1000)
        let hps = Double(iterations) / elapsed
        let hex = digest.map { String(format: "%02x", $0) }.joined()
        let obj: [String: Any] = [
            "digest": hex,
            "iterations": iterations,
            "elapsedMs": elapsedMs,
        ]
        return RunOutput(
            resultJSON: try JSONSerialization.data(withJSONObject: obj),
            elapsedMs: max(elapsedMs, 1),
            hashesPerSec: hps
        )
    }
}
