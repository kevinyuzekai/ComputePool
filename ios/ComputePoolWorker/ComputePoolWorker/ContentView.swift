import SwiftUI

struct ContentView: View {
    @StateObject private var client = WorkerClient()
    @State private var hubURL: String = UserDefaults.standard.string(forKey: "hubURL") ?? "http://192.168.1.1:9797"
    @State private var deviceName: String = UIDevice.current.name

    var body: some View {
        NavigationStack {
            Form {
                Section("Mac Hub") {
                    TextField("http://<mac-lan-ip>:9797", text: $hubURL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                    TextField("设备名称", text: $deviceName)
                    HStack {
                        Circle()
                            .fill(client.connected ? Color.green : Color.orange)
                            .frame(width: 10, height: 10)
                        Text(client.statusText)
                            .foregroundStyle(.secondary)
                    }
                }

                Section("控制") {
                    Button(client.running ? "断开" : "连接并领取任务") {
                        if client.running {
                            client.stop()
                        } else {
                            UserDefaults.standard.set(hubURL, forKey: "hubURL")
                            client.start(hubURL: hubURL, name: deviceName)
                        }
                    }
                    .disabled(hubURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }

                Section("统计") {
                    LabeledContent("已完成分片", value: "\(client.jobsDone)")
                    LabeledContent("最近吞吐", value: client.lastThroughput)
                    LabeledContent("平台", value: client.platformLabel)
                    LabeledContent("核心数", value: "\(ProcessInfo.processInfo.activeProcessorCount)")
                }

                Section("说明") {
                    Text("保持 App 在前台。Worker 通过 HTTP 长轮询向 Mac Hub 领取 cpu_hash / echo / sleep 分片并回传结果。需与 Mac 同一局域网；首次需在系统设置允许本地网络。")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            .navigationTitle("ComputePool Worker")
        }
    }
}

#Preview {
    ContentView()
}
