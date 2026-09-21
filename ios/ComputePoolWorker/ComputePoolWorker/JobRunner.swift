import Foundation
import CryptoKit
import UIKit
import ImageIO
import UniformTypeIdentifiers

enum JobRunner {
    /// Keep under Hub limit (~12MB raw). Base64 expands ~4/3 → JSON body may approach ~16–20MB.
    static let maxImageBytes = 12 * 1024 * 1024

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

    struct ImageResizePayload: Decodable {
        let imageBase64: String?
        let fileName: String?
        let maxEdge: Int?
        let quality: Int?
        let format: String?
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
        case "image_resize":
            return try runImageResize(payload)
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

    private static func runImageResize(_ payload: Data) throws -> RunOutput {
        let p = try JSONDecoder().decode(ImageResizePayload.self, from: payload)
        guard var b64 = p.imageBase64?.trimmingCharacters(in: .whitespacesAndNewlines), !b64.isEmpty else {
            throw NSError(domain: "ComputePool", code: 10, userInfo: [NSLocalizedDescriptionKey: "imageBase64 required"])
        }
        if let comma = b64.firstIndex(of: ","), b64[..<comma].lowercased().contains("base64") {
            b64 = String(b64[b64.index(after: comma)...])
        }
        guard let raw = Data(base64Encoded: b64) else {
            throw NSError(domain: "ComputePool", code: 11, userInfo: [NSLocalizedDescriptionKey: "invalid imageBase64"])
        }
        if raw.count > maxImageBytes {
            throw NSError(domain: "ComputePool", code: 12, userInfo: [NSLocalizedDescriptionKey: "image too large (\(raw.count) bytes; limit \(maxImageBytes))"])
        }

        let start = Date()
        guard let uiImage = UIImage(data: raw) else {
            throw NSError(domain: "ComputePool", code: 13, userInfo: [NSLocalizedDescriptionKey: "cannot decode image (unsupported or corrupt)"])
        }

        var maxEdge = p.maxEdge ?? 1920
        if maxEdge <= 0 { maxEdge = 1920 }
        if maxEdge > 8192 { maxEdge = 8192 }
        var quality = p.quality ?? 80
        if quality <= 0 { quality = 80 }
        if quality > 100 { quality = 100 }
        var format = (p.format ?? "jpeg").lowercased()
        if format == "jpg" { format = "jpeg" }

        let px = uiImage.size.width * uiImage.scale
        let py = uiImage.size.height * uiImage.scale
        var outImage = uiImage
        var skipped = true
        var outW = Int(px.rounded())
        var outH = Int(py.rounded())
        let longest = max(px, py)
        if longest > CGFloat(maxEdge) {
            skipped = false
            let scale = CGFloat(maxEdge) / longest
            outW = max(1, Int((px * scale).rounded()))
            outH = max(1, Int((py * scale).rounded()))
            let size = CGSize(width: outW, height: outH)
            let renderer = UIGraphicsImageRenderer(size: size)
            outImage = renderer.image { _ in
                uiImage.draw(in: CGRect(origin: .zero, size: size))
            }
        }

        var note = ""
        let encoded: Data
        let outFormat: String
        switch format {
        case "png":
            guard let d = outImage.pngData() else {
                throw NSError(domain: "ComputePool", code: 14, userInfo: [NSLocalizedDescriptionKey: "png encode failed"])
            }
            encoded = d
            outFormat = "png"
        case "webp":
            // Prefer ImageIO webp when available (iOS 14+); else jpeg.
            if let d = encodeWebP(outImage, quality: quality) {
                encoded = d
                outFormat = "webp"
            } else if let d = outImage.jpegData(compressionQuality: CGFloat(quality) / 100.0) {
                encoded = d
                outFormat = "jpeg"
                note = "webp encode unavailable; used jpeg"
            } else {
                throw NSError(domain: "ComputePool", code: 15, userInfo: [NSLocalizedDescriptionKey: "encode failed"])
            }
        default:
            guard let d = outImage.jpegData(compressionQuality: CGFloat(quality) / 100.0) else {
                throw NSError(domain: "ComputePool", code: 16, userInfo: [NSLocalizedDescriptionKey: "jpeg encode failed"])
            }
            encoded = d
            outFormat = "jpeg"
        }

        if encoded.count > maxImageBytes {
            throw NSError(domain: "ComputePool", code: 17, userInfo: [NSLocalizedDescriptionKey: "output too large (\(encoded.count) bytes)"])
        }

        let fileName = sanitizeFileName(p.fileName, format: outFormat)
        let elapsedMs = max(Int64(Date().timeIntervalSince(start) * 1000), 1)
        var obj: [String: Any] = [
            "imageBase64": encoded.base64EncodedString(),
            "fileName": fileName,
            "format": outFormat,
            "width": outW,
            "height": outH,
            "bytesIn": raw.count,
            "bytesOut": encoded.count,
            "elapsedMs": elapsedMs,
            "skippedScale": skipped,
        ]
        if !note.isEmpty { obj["note"] = note }
        return RunOutput(
            resultJSON: try JSONSerialization.data(withJSONObject: obj),
            elapsedMs: elapsedMs,
            hashesPerSec: 0
        )
    }

    private static func encodeWebP(_ image: UIImage, quality: Int) -> Data? {
        guard let cg = image.cgImage else { return nil }
        let data = NSMutableData()
        guard let dest = CGImageDestinationCreateWithData(data as CFMutableData, UTType.webP.identifier as CFString, 1, nil) else {
            return nil
        }
        let q = max(0.01, min(1.0, Double(quality) / 100.0))
        let props: [CFString: Any] = [
            kCGImageDestinationLossyCompressionQuality: q,
        ]
        CGImageDestinationAddImage(dest, cg, props as CFDictionary)
        guard CGImageDestinationFinalize(dest) else { return nil }
        return data as Data
    }

    private static func sanitizeFileName(_ name: String?, format: String) -> String {
        var n = (name ?? "image").trimmingCharacters(in: .whitespacesAndNewlines)
        if let slash = n.lastIndex(of: "/") {
            n = String(n[n.index(after: slash)...])
        }
        if n.isEmpty || n == "." || n == ".." { n = "image" }
        if let dot = n.lastIndex(of: "."), dot != n.startIndex {
            n = String(n[..<dot])
        }
        let ext: String
        switch format {
        case "png": ext = ".png"
        case "webp": ext = ".webp"
        default: ext = ".jpg"
        }
        return n + ext
    }
}
