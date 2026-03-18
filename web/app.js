const state = {
  selectedNodeId: "",
  searchItems: [],
  graph: null,
  details: null,
  status: null,
  upload: null,
};

const els = {
  rootInput: document.getElementById("root-input"),
  indexBtn: document.getElementById("index-btn"),
  uploadFilesInput: document.getElementById("upload-files-input"),
  uploadDirInput: document.getElementById("upload-dir-input"),
  uploadZipInput: document.getElementById("upload-zip-input"),
  snippetBtn: document.getElementById("snippet-btn"),
  snippetPanel: document.getElementById("snippet-panel"),
  snippetCloseBtn: document.getElementById("snippet-close-btn"),
  snippetFileName: document.getElementById("snippet-file-name"),
  snippetCode: document.getElementById("snippet-code"),
  snippetSubmitBtn: document.getElementById("snippet-submit-btn"),
  statusBox: document.getElementById("status-box"),
  uploadBox: document.getElementById("upload-box"),
  searchInput: document.getElementById("search-input"),
  searchResults: document.getElementById("search-results"),
  resultCount: document.getElementById("result-count"),
  direction: document.getElementById("direction-select"),
  depth: document.getElementById("depth-input"),
  limit: document.getElementById("limit-input"),
  bodyToggle: document.getElementById("body-toggle"),
  graphSummary: document.getElementById("graph-summary"),
  graphTitle: document.getElementById("graph-title"),
  graphSVG: document.getElementById("graph-svg"),
  graphEmpty: document.getElementById("graph-empty"),
  detailBox: document.getElementById("detail-box"),
  sourceView: document.getElementById("source-view"),
  sourcePath: document.getElementById("source-path"),
};

init();

async function init() {
  bindEvents();
  await refreshStatus();
  await runSearch("");
}

function bindEvents() {
  els.indexBtn.addEventListener("click", handleIndex);
  els.uploadFilesInput.addEventListener("change", (event) => handleUpload(event, "文件"));
  els.uploadDirInput.addEventListener("change", (event) => handleUpload(event, "Maven/源码目录"));
  els.uploadZipInput.addEventListener("change", (event) => handleUpload(event, "ZIP 项目包"));
  els.snippetBtn.addEventListener("click", () => toggleSnippetPanel(true));
  els.snippetCloseBtn.addEventListener("click", () => toggleSnippetPanel(false));
  els.snippetSubmitBtn.addEventListener("click", handleSnippetSubmit);
  els.searchInput.addEventListener("input", debounce((event) => runSearch(event.target.value), 180));
  els.direction.addEventListener("change", rerenderGraph);
  els.depth.addEventListener("change", rerenderGraph);
  els.limit.addEventListener("change", rerenderGraph);
  els.bodyToggle.addEventListener("change", rerenderGraph);
}

async function handleIndex() {
  setStatus("正在索引目录...");
  clearUploadInfo();
  const payload = { root: els.rootInput.value.trim() };
  try {
    const res = await fetchJSON("/api/index", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    state.status = res;
    renderStatus();
    await afterIndexUpdated();
  } catch (error) {
    setStatus(renderError(error.message));
  }
}

async function handleUpload(event, sourceLabel) {
  const files = Array.from(event.target.files || []);
  if (!files.length) {
    return;
  }

  const form = new FormData();
  for (const file of files) {
    form.append("files", file, file.webkitRelativePath || file.name);
  }

  setUploadInfo(`正在上传${sourceLabel}，共 ${files.length} 个文件，并自动索引...`);
  try {
    const res = await fetchUpload("/api/upload", form);
    state.upload = res.upload || null;
    state.status = res.status || null;
    if (state.status && state.status.root) {
      els.rootInput.value = state.status.root;
    }
    renderStatus();
    renderUploadInfo();
    await afterIndexUpdated();
  } catch (error) {
    setUploadInfo(renderError(`上传失败: ${error.message}`));
  } finally {
    event.target.value = "";
  }
}

async function handleSnippetSubmit() {
  const code = els.snippetCode.value.trim();
  const fileName = els.snippetFileName.value.trim();
  if (!code) {
    setUploadInfo(renderError("请先粘贴 Java 源码"));
    return;
  }

  setUploadInfo("正在保存源码并索引...");
  try {
    const res = await fetchJSON("/api/snippet", {
      method: "POST",
      body: JSON.stringify({
        file_name: fileName,
        code,
      }),
    });
    state.upload = res.upload || null;
    state.status = res.status || null;
    if (state.status && state.status.root) {
      els.rootInput.value = state.status.root;
    }
    renderStatus();
    renderUploadInfo();
    toggleSnippetPanel(false);
    els.snippetCode.value = "";
    els.snippetFileName.value = "";
    await afterIndexUpdated();
  } catch (error) {
    setUploadInfo(renderError(`源码索引失败: ${error.message}`));
  }
}

async function afterIndexUpdated() {
  await runSearch(els.searchInput.value.trim());
  state.selectedNodeId = "";
  state.graph = null;
  state.details = null;
  renderGraph();
  renderDetails();
}

async function refreshStatus() {
  try {
    state.status = await fetchJSON("/api/status");
    renderStatus();
    renderUploadInfo();
    if (state.status && state.status.root) {
      els.rootInput.value = state.status.root;
    }
  } catch (error) {
    setStatus(renderError(error.message));
  }
}

function renderStatus() {
  if (!state.status) {
    setStatus("尚未索引项目");
    return;
  }
  const stats = state.status.stats || {};
  const parts = [];
  if (state.status.project_ready) {
    parts.push(`<div><strong>当前项目:</strong> ${escapeHTML(state.status.root || "")}</div>`);
    parts.push(`<div>${stats.java_files || 0} 个 Java 文件，${stats.classes || 0} 个类，${stats.interfaces || 0} 个接口，${stats.methods || 0} 个方法，${stats.fields || 0} 个字段</div>`);
    parts.push(`<div>${stats.call_edges || 0} 条调用边，${stats.access_edges || 0} 条访问边，耗时 ${stats.duration_ms || state.status.duration_ms || 0} ms</div>`);
  } else if (state.status.running) {
    parts.push("<div><strong>索引中...</strong></div>");
  } else if (state.status.error) {
    parts.push(renderError(state.status.error));
  } else {
    parts.push("<div>尚未索引项目</div>");
  }
  setStatus(parts.join(""));
}

function renderUploadInfo() {
  if (!state.upload) {
    clearUploadInfo();
    return;
  }
  const files = state.upload.files || [];
  const preview = files.slice(0, 3).map((item) => escapeHTML(item)).join("<br>");
  const suffix = files.length > 3 ? `<br>... 共 ${files.length} 个文件` : "";
  setUploadInfo(`<div><strong>最近上传:</strong> ${escapeHTML(state.upload.root || "")}</div><div>${preview}${suffix}</div>`);
}

function clearUploadInfo() {
  els.uploadBox.innerHTML = "支持上传 `.java` 文件、Maven/源码目录、`.zip` 项目包，或直接粘贴 Java 源码。";
}

function toggleSnippetPanel(visible) {
  els.snippetPanel.classList.toggle("visible", visible);
}

function setStatus(html) {
  els.statusBox.innerHTML = html;
}

function setUploadInfo(html) {
  els.uploadBox.innerHTML = html;
}

async function runSearch(query) {
  try {
    const res = await fetchJSON(`/api/search?q=${encodeURIComponent(query || "")}&limit=80`);
    state.searchItems = res.items || [];
    renderSearchResults();
  } catch (error) {
    els.searchResults.innerHTML = renderError(error.message);
  }
}

function renderSearchResults() {
  els.resultCount.textContent = String(state.searchItems.length);
  if (!state.searchItems.length) {
    els.searchResults.innerHTML = '<p class="muted">没有匹配节点</p>';
    return;
  }
  els.searchResults.innerHTML = state.searchItems.map((item) => {
    const active = item.id === state.selectedNodeId ? "active" : "";
    const desc = item.signature || item.description || item.file_path || "";
    return `
      <button class="result-item ${active}" data-node-id="${escapeAttr(item.id)}">
        <span class="result-kind ${escapeAttr(item.kind)}">${escapeHTML(item.kind)}</span>
        <strong>${escapeHTML(item.label)}</strong>
        <small>${escapeHTML(desc)}</small>
      </button>
    `;
  }).join("");
  els.searchResults.querySelectorAll("[data-node-id]").forEach((node) => {
    node.addEventListener("click", () => selectNode(node.dataset.nodeId));
  });
}

async function selectNode(nodeId) {
  state.selectedNodeId = nodeId;
  renderSearchResults();
  await Promise.all([loadGraph(nodeId), loadDetails(nodeId)]);
}

async function rerenderGraph() {
  if (!state.selectedNodeId) {
    return;
  }
  await loadGraph(state.selectedNodeId);
}

async function loadGraph(nodeId) {
  const params = new URLSearchParams({
    node: nodeId,
    direction: els.direction.value,
    depth: els.depth.value,
    limit: els.limit.value,
    include_body: String(els.bodyToggle.checked),
  });
  try {
    state.graph = await fetchJSON(`/api/graph?${params.toString()}`);
    renderGraph();
  } catch (error) {
    els.graphSVG.innerHTML = "";
    els.graphEmpty.textContent = error.message;
    els.graphEmpty.style.display = "grid";
  }
}

async function loadDetails(nodeId) {
  try {
    state.details = await fetchJSON(`/api/node?id=${encodeURIComponent(nodeId)}`);
    renderDetails();
  } catch (error) {
    els.detailBox.innerHTML = renderError(error.message);
  }
}

function renderGraph() {
  const graph = state.graph;
  if (!graph || !graph.nodes || !graph.nodes.length) {
    els.graphEmpty.style.display = "grid";
    els.graphSVG.innerHTML = "";
    els.graphTitle.textContent = "选择节点开始分析";
    renderSummary({});
    return;
  }

  els.graphEmpty.style.display = "none";
  els.graphTitle.textContent = graph.focus_id;
  renderSummary(graph.summary || {});

  const layout = computeLayout(graph.nodes, graph.focus_id);
  const width = 1400;
  const height = Math.max(900, layout.totalHeight + 80);
  els.graphSVG.setAttribute("viewBox", `0 0 ${width} ${height}`);

  const defs = `
    <defs>
      <marker id="arrow" markerWidth="10" markerHeight="10" refX="8" refY="5" orient="auto" markerUnits="strokeWidth">
        <path d="M0,0 L10,5 L0,10 z" fill="#8c8c8c"></path>
      </marker>
    </defs>
  `;

  const edgeSVG = graph.edges.map((edge) => {
    const from = layout.map.get(edge.source);
    const to = layout.map.get(edge.target);
    if (!from || !to) {
      return "";
    }
    const path = curveBetween(from, to);
    const labelX = (from.cx + to.cx) / 2;
    const labelY = (from.cy + to.cy) / 2 - 8;
    return `
      <g>
        <path class="edge ${escapeAttr(edge.kind)}" d="${path}" marker-end="url(#arrow)"></path>
        <text class="edge-label" x="${labelX}" y="${labelY}">${escapeHTML(edge.kind)}</text>
      </g>
    `;
  }).join("");

  const nodeSVG = graph.nodes.map((node) => {
    const point = layout.map.get(node.id);
    if (!point) {
      return "";
    }
    const active = node.id === state.selectedNodeId ? "active" : "";
    const meta = node.signature || node.description || `${node.kind}`;
    return `
      <g class="node ${escapeAttr(node.kind)} ${active}" data-node-id="${escapeAttr(node.id)}" transform="translate(${point.x}, ${point.y})">
        <rect width="${point.width}" height="${point.height}" rx="8" ry="8"></rect>
        <text class="label" x="14" y="24">${escapeHTML(truncate(node.label, 38))}</text>
        <text class="meta" x="14" y="44">${escapeHTML(truncate(meta, 48))}</text>
      </g>
    `;
  }).join("");

  els.graphSVG.innerHTML = defs + edgeSVG + nodeSVG;
  els.graphSVG.querySelectorAll("[data-node-id]").forEach((el) => {
    el.addEventListener("click", () => selectNode(el.dataset.nodeId));
  });
}

function renderSummary(summary) {
  const items = [
    `节点 ${summary.node_count || 0}`,
    `边 ${summary.edge_count || 0}`,
    `上游 ${summary.upstream_count || 0}`,
    `下游 ${summary.downstream_count || 0}`,
    `方法体 ${summary.body_count || 0}`,
  ];
  els.graphSummary.innerHTML = items.map((text) => `<span class="summary-item">${escapeHTML(text)}</span>`).join("");
}

function computeLayout(nodes, focusId) {
  const lanes = {
    upstream: 220,
    focus: 700,
    downstream: 1180,
    body: 700,
  };
  const buckets = {
    upstream: [],
    focus: [],
    downstream: [],
    body: [],
  };

  nodes.forEach((node) => {
    const lane = buckets[node.lane] ? node.lane : "focus";
    buckets[lane].push(node);
  });

  Object.values(buckets).forEach((list) => list.sort((a, b) => {
    if (a.depth !== b.depth) return a.depth - b.depth;
    return a.label.localeCompare(b.label);
  }));

  const map = new Map();
  let totalHeight = 0;
  for (const [lane, list] of Object.entries(buckets)) {
    const startY = lane === "body" ? 520 : 80;
    const gap = lane === "body" ? 88 : 110;
    list.forEach((node, index) => {
      const width = lane === "body" ? 360 : 260;
      const height = 58;
      const x = lanes[lane] - width / 2;
      const y = startY + index * gap;
      map.set(node.id, {
        x,
        y,
        width,
        height,
        cx: x + width / 2,
        cy: y + height / 2,
      });
      totalHeight = Math.max(totalHeight, y + height + 40);
    });
  }

  if (focusId && map.has(focusId)) {
    const point = map.get(focusId);
    point.x = lanes.focus - point.width / 2;
    point.cx = lanes.focus;
    point.y = 190;
    point.cy = point.y + point.height / 2;
  }

  return { map, totalHeight };
}

function curveBetween(from, to) {
  const dx = Math.abs(to.cx - from.cx);
  const curve = Math.max(60, dx * 0.35);
  return `M ${from.cx} ${from.cy} C ${from.cx + curve} ${from.cy}, ${to.cx - curve} ${to.cy}, ${to.cx} ${to.cy}`;
}

function renderDetails() {
  const details = state.details;
  if (!details) {
    els.detailBox.innerHTML = '<p class="muted">暂无节点详情</p>';
    els.sourceView.innerHTML = "";
    els.sourcePath.textContent = "";
    return;
  }

  const meta = details.meta || {};
  const rows = [
    ["ID", details.id],
    ["类型", details.kind],
    ["标签", details.label],
    ["签名", details.signature || ""],
    ["文件", details.file_path || ""],
    ["位置", details.start_line ? `${details.start_line}-${details.end_line || details.start_line}` : ""],
    ["说明", details.description || ""],
  ];
  for (const [key, value] of Object.entries(meta)) {
    rows.push([key, typeof value === "string" ? value : JSON.stringify(value)]);
  }

  els.detailBox.innerHTML = `<dl>${
    rows
      .filter(([, value]) => value !== "" && value !== undefined && value !== null)
      .map(([label, value]) => `<div><dt>${escapeHTML(label)}</dt><dd>${escapeHTML(String(value))}</dd></div>`)
      .join("")
  }</dl>`;

  els.sourcePath.textContent = details.file_path || "";
  const sourceLines = details.source || [];
  els.sourceView.innerHTML = sourceLines.map((line) => `
    <div class="source-line">
      <span class="ln">${line.number}</span>
      <span>${escapeHTML(line.text)}</span>
    </div>
  `).join("");
}

async function fetchJSON(url, options = {}) {
  const res = await fetch(url, {
    headers: {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
    ...options,
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : {};
  if (!res.ok) {
    throw new Error(data.error || `HTTP ${res.status}`);
  }
  return data;
}

async function fetchUpload(url, formData) {
  const res = await fetch(url, {
    method: "POST",
    body: formData,
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : {};
  if (!res.ok) {
    throw new Error(data.error || `HTTP ${res.status}`);
  }
  return data;
}

function debounce(fn, wait) {
  let timer = null;
  return (...args) => {
    window.clearTimeout(timer);
    timer = window.setTimeout(() => fn(...args), wait);
  };
}

function truncate(text, length) {
  if (!text) return "";
  if (text.length <= length) return text;
  return `${text.slice(0, length - 3)}...`;
}

function renderError(message) {
  return `<span class="error">${escapeHTML(message)}</span>`;
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function escapeAttr(value) {
  return escapeHTML(value);
}
