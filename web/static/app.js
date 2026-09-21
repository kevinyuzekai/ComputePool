const $ = (id) => document.getElementById(id);

let compareId = null;
let pollTimer = null;

async function api(path, opts = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(opts.headers || {}) },
    ...opts,
  });
  if (res.status === 204) return null;
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { raw: text }; }
  if (!res.ok) throw new Error((data && data.error) || res.statusText);
  return data;
}

function fmtThroughput(v) {
  if (!v || v <= 0) return "—";
  if (v >= 1e6) return (v / 1e6).toFixed(2) + " Mhash/s";
  if (v >= 1e3) return (v / 1e3).toFixed(1) + " khash/s";
  return v.toFixed(0) + " hash/s";
}

function ago(ts) {
  if (!ts) return "—";
  const t = new Date(ts).getTime();
  const s = Math.max(0, Math.round((Date.now() - t) / 1000));
  if (s < 5) return "刚刚";
  if (s < 60) return s + "s 前";
  return Math.floor(s / 60) + "m 前";
}

function isOnline(w) {
  if (!w.lastSeen) return false;
  return Date.now() - new Date(w.lastSeen).getTime() < 45000;
}

async function refreshStatus() {
  const info = await api("/api/status");
  const pill = $("statusPill");
  if (info.running) {
    pill.textContent = "Hub 运行中";
    pill.classList.add("running");
  } else {
    pill.textContent = "已停止";
    pill.classList.remove("running");
  }
  $("listenLabel").textContent = info.listenAddr + ` · 在线 ${info.onlineCount}/${info.workerCount} · 本机 ${info.localWorkers}`;
  $("joinURL").textContent = info.joinURL;
  $("ver").textContent = info.version || "0.1.0";
  $("qrText").textContent =
    `加入地址（可扫码/手输）\n${info.joinURL}\n\n` +
    `Bonjour: ${info.serviceType} · LAN ${info.lanIP}:${info.port}\n` +
    `在同一 Wi‑Fi 打开 ComputePoolWorker，粘贴 URL 后保持前台运行。`;
  return info;
}

async function refreshWorkers() {
  const data = await api("/api/workers");
  const body = $("workerBody");
  body.innerHTML = "";
  const list = (data.workers || []).slice().sort((a, b) => {
    const ao = isOnline(a) ? 0 : 1;
    const bo = isOnline(b) ? 0 : 1;
    if (ao !== bo) return ao - bo;
    if (a.local !== b.local) return a.local ? -1 : 1;
    return (a.name || "").localeCompare(b.name || "");
  });
  if (!list.length) {
    body.innerHTML = `<tr><td colspan="7" class="muted">暂无 Worker（Hub 启动后本机 local worker 会自动注册）</td></tr>`;
    return;
  }
  for (const w of list) {
    const online = isOnline(w);
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>${escapeHtml(w.name || w.id)}</td>
      <td>${escapeHtml(w.platform || "")}</td>
      <td>${w.cores || 1}</td>
      <td><span class="badge ${online ? "up" : "down"}">${online ? "在线" : "离线"}</span> ${ago(w.lastSeen)}</td>
      <td>${w.jobsDone || 0}</td>
      <td>${fmtThroughput(w.throughput)}</td>
      <td>${w.local ? '<span class="badge local">本机</span>' : '<span class="badge">远程</span>'}</td>`;
    body.appendChild(tr);
  }
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  })[c]);
}

async function startBench() {
  const iterations = Number($("iterations").value) || 2000000;
  const shards = Number($("shards").value) || 0;
  $("benchMetrics").style.display = "grid";
  $("benchLog").style.display = "block";
  $("localMs").textContent = "运行中…";
  $("poolMs").textContent = "等待…";
  $("speedup").textContent = "—";
  $("benchBar").style.width = "5%";
  $("benchLog").textContent = "提交算力对比…";
  const res = await api("/api/benchmark", {
    method: "POST",
    body: JSON.stringify({ iterations, shards }),
  });
  compareId = res.compareId;
  $("benchLog").textContent = JSON.stringify(res, null, 2);
  if (pollTimer) clearInterval(pollTimer);
  pollTimer = setInterval(pollBench, 400);
}

async function pollBench() {
  if (!compareId) return;
  const res = await api("/api/benchmark/" + compareId);
  $("benchLog").textContent = JSON.stringify(res, null, 2);
  const localDone = res.localJob && res.localJob.status === "done";
  const poolDone = res.poolJob && res.poolJob.status === "done";
  if (res.localWallMs) $("localMs").textContent = res.localWallMs + " ms";
  else if (res.localJob) {
    const d = res.localJob.doneShards || 0;
    const t = res.localJob.totalShards || 1;
    $("localMs").textContent = `分片 ${d}/${t}`;
    $("benchBar").style.width = Math.min(45, (d / t) * 45) + "%";
  }
  if (localDone && !poolDone) {
    $("poolMs").textContent = "运行中…";
    if (res.poolJob) {
      const d = res.poolJob.doneShards || 0;
      const t = res.poolJob.totalShards || 1;
      $("poolMs").textContent = `分片 ${d}/${t}`;
      $("benchBar").style.width = 45 + Math.min(50, (d / t) * 50) + "%";
    }
  }
  if (res.poolWallMs) $("poolMs").textContent = res.poolWallMs + " ms";
  if (res.status === "done") {
    $("speedup").textContent = (res.speedup || 0).toFixed(2) + "×";
    $("benchBar").style.width = "100%";
    clearInterval(pollTimer);
    pollTimer = null;
    refreshWorkers();
  }
  if (res.status === "failed") {
    $("speedup").textContent = "失败";
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

$("btnStart").onclick = async () => {
  await api("/api/hub/start", { method: "POST", body: "{}" });
  await refreshAll();
};
$("btnStop").onclick = async () => {
  await api("/api/hub/stop", { method: "POST", body: "{}" });
  await refreshAll();
};
$("btnRefresh").onclick = () => refreshAll();
$("btnCopy").onclick = async () => {
  const url = $("joinURL").textContent.trim();
  try {
    await navigator.clipboard.writeText(url);
    $("btnCopy").textContent = "已复制";
    setTimeout(() => { $("btnCopy").textContent = "复制 URL"; }, 1500);
  } catch {
    prompt("复制加入地址：", url);
  }
};
$("btnBench").onclick = () => startBench().catch((e) => alert(e.message));
$("btnEcho").onclick = async () => {
  const job = await api("/api/test/echo", { method: "POST", body: JSON.stringify({ message: "hello-from-ui" }) });
  alert("已提交 echo job: " + job.id);
};
$("btnSleep").onclick = async () => {
  const job = await api("/api/test/sleep", { method: "POST", body: JSON.stringify({ ms: 300, shards: 2 }) });
  alert("已提交 sleep job: " + job.id);
};

async function refreshAll() {
  await refreshStatus();
  await refreshWorkers();
}

refreshAll().catch(console.error);
setInterval(() => refreshAll().catch(() => {}), 3000);
