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
  $("ver").textContent = info.version || "0.1.1";
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

/* ---- 自定义任务 ---- */
let customJobId = null;
let customPollTimer = null;
let payloadSyncLock = false; // avoid feedback loops

function defaultPayload(type) {
  if (type === "echo") return { message: "hello-from-ui" };
  if (type === "sleep") return { ms: 300 };
  return { seed: "demo", iterations: 500000 };
}

function showTypeFields(type) {
  $("cjFieldsCpu").style.display = type === "cpu_hash" ? "" : "none";
  $("cjFieldsEcho").style.display = type === "echo" ? "" : "none";
  $("cjFieldsSleep").style.display = type === "sleep" ? "" : "none";
}

function fieldsToPayloadObj() {
  const type = $("cjType").value;
  if (type === "echo") return { message: $("cjMessage").value };
  if (type === "sleep") return { ms: Number($("cjMs").value) || 0 };
  return {
    seed: $("cjSeed").value,
    iterations: Number($("cjIterations").value) || 1,
  };
}

function writePayloadFromFields() {
  if (payloadSyncLock) return;
  payloadSyncLock = true;
  try {
    $("cjPayload").value = JSON.stringify(fieldsToPayloadObj(), null, 2);
  } finally {
    payloadSyncLock = false;
  }
}

function applyPayloadToFields(obj) {
  if (!obj || typeof obj !== "object") return;
  const type = $("cjType").value;
  if (type === "cpu_hash") {
    if (obj.seed != null) $("cjSeed").value = String(obj.seed);
    if (obj.iterations != null) $("cjIterations").value = Number(obj.iterations);
  } else if (type === "echo") {
    if (obj.message != null) $("cjMessage").value = String(obj.message);
  } else if (type === "sleep") {
    if (obj.ms != null) $("cjMs").value = Number(obj.ms);
  }
}

function readPayloadFromTextarea() {
  const raw = $("cjPayload").value.trim();
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

function onTypeChange() {
  const type = $("cjType").value;
  showTypeFields(type);
  const cur = readPayloadFromTextarea();
  // Prefer current fields defaults for new type
  const next = defaultPayload(type);
  if (cur && typeof cur === "object") {
    if (type === "cpu_hash") {
      if (cur.seed != null) next.seed = cur.seed;
      if (cur.iterations != null) next.iterations = cur.iterations;
    } else if (type === "echo" && cur.message != null) {
      next.message = cur.message;
    } else if (type === "sleep" && cur.ms != null) {
      next.ms = cur.ms;
    }
  }
  payloadSyncLock = true;
  try {
    applyPayloadToFields(next);
    $("cjPayload").value = JSON.stringify(next, null, 2);
  } finally {
    payloadSyncLock = false;
  }
}

function onFriendlyFieldChange() {
  writePayloadFromFields();
}

function onPayloadTextChange() {
  if (payloadSyncLock) return;
  const obj = readPayloadFromTextarea();
  if (!obj) return;
  payloadSyncLock = true;
  try {
    applyPayloadToFields(obj);
  } finally {
    payloadSyncLock = false;
  }
}

function summarizeJob(job) {
  const lines = [];
  lines.push(`status: ${job.status}`);
  lines.push(`type: ${job.type} · shards ${job.doneShards || 0}/${job.totalShards || 0}`);
  if (job.wallMs) lines.push(`wallMs: ${job.wallMs}`);
  if (job.label) lines.push(`label: ${job.label}`);
  if (Array.isArray(job.shards) && job.shards.length) {
    const sample = job.shards.slice(0, 4).map((s) => {
      let r = s.result;
      if (typeof r === "string") {
        try { r = JSON.parse(r); } catch { /* keep */ }
      }
      return {
        shardId: s.shardId,
        status: s.status,
        workerId: s.workerId,
        error: s.error || undefined,
        result: r,
        metrics: s.metrics,
      };
    });
    lines.push("shards sample:");
    lines.push(JSON.stringify(sample, null, 2));
    if (job.shards.length > 4) lines.push(`… 另有 ${job.shards.length - 4} 个分片`);
  }
  return lines.join("\n");
}

function renderJobStatus(job) {
  $("cjStatus").style.display = "block";
  $("cjProgressWrap").style.display = "block";
  $("cjJobId").textContent = job.id || "—";
  $("cjJobStatus").textContent = job.status || "—";
  const done = job.doneShards || 0;
  const total = job.totalShards || 1;
  const wall = job.wallMs != null && job.wallMs > 0 ? job.wallMs + " ms" : "—";
  $("cjJobMeta").textContent = `${done}/${total} · ${wall}`;
  const pct = Math.min(100, Math.round((done / total) * 100));
  $("cjBar").style.width = pct + "%";
  $("cjJobLog").textContent = summarizeJob(job);
}

async function submitCustomJob() {
  const type = $("cjType").value;
  let payload = readPayloadFromTextarea();
  if (!payload) {
    // fall back to friendly fields
    payload = fieldsToPayloadObj();
    writePayloadFromFields();
  }
  const shards = Number($("cjShards").value) || 4;
  const label = ($("cjLabel").value || "").trim();
  const localOnly = !!$("cjLocalOnly").checked;
  $("cjStatus").style.display = "block";
  $("cjProgressWrap").style.display = "block";
  $("cjJobId").textContent = "提交中…";
  $("cjJobStatus").textContent = "—";
  $("cjJobMeta").textContent = "—";
  $("cjBar").style.width = "2%";
  $("cjJobLog").textContent = "POST /api/jobs …";
  const job = await api("/api/jobs", {
    method: "POST",
    body: JSON.stringify({ type, payload, shards, localOnly, label }),
  });
  customJobId = job.id;
  renderJobStatus(job);
  if (customPollTimer) clearInterval(customPollTimer);
  customPollTimer = setInterval(() => pollCustomJob().catch(console.error), 400);
  refreshJobsList().catch(console.error);
}

async function pollCustomJob() {
  if (!customJobId) return;
  const job = await api("/api/jobs/" + customJobId);
  renderJobStatus(job);
  if (job.status === "done" || job.status === "failed" || job.status === "cancelled") {
    clearInterval(customPollTimer);
    customPollTimer = null;
    refreshJobsList().catch(console.error);
    refreshWorkers().catch(console.error);
  }
}

async function refreshJobsList() {
  const data = await api("/api/jobs");
  const body = $("jobsBody");
  if (!body) return;
  body.innerHTML = "";
  const list = data.jobs || [];
  if (!list.length) {
    body.innerHTML = `<tr><td colspan="6" class="muted">暂无任务</td></tr>`;
    return;
  }
  for (const j of list) {
    const tr = document.createElement("tr");
    tr.className = "job-row";
    tr.title = "点击查看详情";
    tr.innerHTML = `
      <td class="mono" style="padding:8px 6px;background:transparent">${escapeHtml(j.id)}</td>
      <td>${escapeHtml(j.type || "")}</td>
      <td>${escapeHtml(j.label || "—")}</td>
      <td><span class="badge ${j.status === "done" ? "up" : j.status === "failed" ? "down" : ""}">${escapeHtml(j.status || "")}</span></td>
      <td>${j.doneShards || 0}/${j.totalShards || 0}</td>
      <td>${j.wallMs ? j.wallMs + " ms" : "—"}</td>`;
    tr.onclick = () => {
      customJobId = j.id;
      api("/api/jobs/" + j.id).then((full) => {
        renderJobStatus(full);
        if (full.status === "running" || full.status === "pending") {
          if (customPollTimer) clearInterval(customPollTimer);
          customPollTimer = setInterval(() => pollCustomJob().catch(console.error), 400);
        }
      }).catch((e) => alert(e.message));
    };
    body.appendChild(tr);
  }
}

function initCustomJobUI() {
  if (!$("cjType")) return;
  $("cjType").onchange = onTypeChange;
  ["cjSeed", "cjIterations", "cjMessage", "cjMs"].forEach((id) => {
    const el = $(id);
    if (!el) return;
    el.addEventListener("input", onFriendlyFieldChange);
    el.addEventListener("change", onFriendlyFieldChange);
  });
  $("cjPayload").addEventListener("input", onPayloadTextChange);
  $("btnSubmitJob").onclick = () => submitCustomJob().catch((e) => {
    $("cjJobLog").textContent = "错误: " + e.message;
    $("cjJobStatus").textContent = "失败";
    alert(e.message);
  });
  $("btnJobsRefresh").onclick = () => refreshJobsList().catch((e) => alert(e.message));
  onTypeChange();
}

const _refreshAllOrig = refreshAll;
refreshAll = async function () {
  await _refreshAllOrig();
  await refreshJobsList().catch(() => {});
};

initCustomJobUI();
