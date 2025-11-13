const state = {
  token: localStorage.getItem("bark_token") || "",
  username: "",
  currentView: "dashboard",
  devices: [],
};

const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => document.querySelectorAll(selector);

document.addEventListener("DOMContentLoaded", () => {
  bindEvents();
  bootstrap();
});

async function bootstrap() {
  if (await tryAutoLoginFromQuery()) {
    return;
  }
  if (!state.token) {
    showAuth();
    return;
  }
  try {
    const profile = await api("/auth/profile");
    if (profile?.enabled === false) {
      state.username = "guest";
      showPortal();
      refreshAll();
      return;
    }
    state.username = profile.username;
    showPortal();
    refreshAll();
  } catch (err) {
    console.warn("profile failed", err);
    logout(true);
  }
}

function bindEvents() {
  $("#login-form").addEventListener("submit", handleLogin);
  $("#logout-btn").addEventListener("click", () => logout(false));
  $$("#nav .nav-link").forEach((btn) =>
    btn.addEventListener("click", () => switchView(btn.dataset.view))
  );
  $("#refresh-dashboard").addEventListener("click", loadDashboard);
  $("#refresh-devices").addEventListener("click", loadDevices);
  $("#refresh-sender").addEventListener("click", loadDevices);
  $("#refresh-logs").addEventListener("click", () => loadLogs(true));
  $("#device-form").addEventListener("submit", handleDeviceSubmit);
  $("#notice-form").addEventListener("submit", handleNoticeSubmit);
  $("#log-filter").addEventListener("submit", (e) => {
    e.preventDefault();
    loadLogs(true);
  });
  $("#notice-all").addEventListener("change", (e) => {
    $("#notice-device-keys").disabled = e.target.checked;
  });
  $$(".chip").forEach((chip) =>
    chip.addEventListener("click", () => applyTemplate(chip.dataset.template))
  );
  const snippetForm = $("#snippet-form");
  if (snippetForm) {
    $("#snippet-endpoint").addEventListener("change", updateSnippet);
    snippetForm.addEventListener("input", (e) => {
      if (e.target.name === "endpoint") return;
      updateSnippet();
    });
    $("#copy-snippet").addEventListener("click", copySnippet);
    const snippetAll = $("#snippet-all");
    if (snippetAll) {
      snippetAll.addEventListener("change", () => {
        const select = $("#snippet-device-keys");
        if (select) select.disabled = snippetAll.checked;
        updateSnippet();
      });
    }
    const hostInput = $("#snippet-host");
    if (hostInput && !hostInput.value) {
      hostInput.value = `${location.protocol}//${location.host}`;
    }
    updateSnippet();
  }
}

function showAuth() {
  $("#auth-panel").classList.remove("hidden");
  $("#portal").classList.add("hidden");
}

function showPortal() {
  $("#auth-panel").classList.add("hidden");
  $("#portal").classList.remove("hidden");
  const sidebarUser = $("#sidebar-user");
  if (sidebarUser) {
    sidebarUser.textContent = `欢迎，${state.username || "管理员"}`;
  }
}

function switchView(view) {
  state.currentView = view;
  $$("#nav .nav-link").forEach((btn) =>
    btn.classList.toggle("active", btn.dataset.view === view)
  );
  $$(".view-section").forEach((section) =>
    section.classList.toggle("active", section.dataset.view === view)
  );
  if (view === "dashboard") loadDashboard();
  if (view === "devices") loadDevices();
  if (view === "sender") loadDevices();
  if (view === "logs") loadLogs(false);
}

async function handleLogin(event) {
  event.preventDefault();
  const payload = Object.fromEntries(new FormData(event.currentTarget).entries());
  await performLogin(payload, { silent: false });
}

function logout(isExpired) {
  state.token = "";
  state.username = "";
  localStorage.removeItem("bark_token");
  showAuth();
  if (isExpired) {
    showToast("登录已失效，请重新登录", true);
  }
}

async function tryAutoLoginFromQuery() {
  const params = new URLSearchParams(window.location.search);
  if (!params.has("username") || !params.has("password")) {
    return false;
  }
  const payload = {
    username: params.get("username"),
    password: params.get("password"),
  };
  history.replaceState({}, "", window.location.pathname);
  const success = await performLogin(payload, { silent: true });
  return success;
}

async function performLogin(payload, { silent }) {
  try {
    const res = await api("/auth/login", {
      method: "POST",
      body: JSON.stringify(payload),
      skipAuth: true,
    });
    state.token = res.token || "";
    state.username = res.username || payload.username;
    localStorage.setItem("bark_token", state.token);
    showPortal();
    refreshAll();
    if (!silent) {
      showToast("登录成功");
    }
    return true;
  } catch (err) {
    $("#login-hint").textContent = err.message;
    showToast(err.message, true);
    showAuth();
    return false;
  }
}

function refreshAll() {
  switchView("dashboard");
  loadDashboard();
  loadDevices();
  loadLogs(false);
  updateSnippet();
}

async function loadDashboard() {
  try {
    const data = await api("/admin/summary");
    $("#card-status").textContent = data.status || "--";
    $("#card-active").textContent = `${data.active || 0}/${data.total || 0}`;
    $("#card-today").textContent = data.todaySent ?? "--";
    $("#card-success").textContent = data.todaySuccess ?? "--";
    renderRecentLogs(data.recentLogs || []);
  } catch (err) {
    showToast(err.message, true);
  }
}

function renderRecentLogs(logs) {
  const body = $("#dashboard-logs");
  if (!logs.length) {
    body.innerHTML = `<tr><td colspan="5" class="empty">暂无数据</td></tr>`;
    return;
  }
  body.innerHTML = logs
    .map(
      (log) => `<tr>
        <td>${log.time}</td>
        <td>${escapeHtml(log.deviceKey || "-")}</td>
        <td>${escapeHtml(log.title || "-")}</td>
        <td>${escapeHtml(log.group || "-")}</td>
        <td>${log.status}</td>
      </tr>`
    )
    .join("");
}

async function loadDevices() {
  try {
    const devices = await api("/admin/devices");
    state.devices = devices || [];
    renderDeviceTable();
    renderDeviceOptions();
  } catch (err) {
    showToast(err.message, true);
  }
}

function renderDeviceTable() {
  const body = $("#devices-table-body");
  if (!state.devices.length) {
    body.innerHTML = `<tr><td colspan="5" class="empty">暂无数据</td></tr>`;
    return;
  }
  body.innerHTML = state.devices
    .map(
      (d) => `<tr>
        <td>${escapeHtml(d.name || "-")}</td>
        <td class="mono" title="${d.deviceToken}">${mask(d.deviceToken)}</td>
        <td class="mono" title="${d.deviceKey}">${mask(d.deviceKey)}</td>
        <td>${d.status || "ACTIVE"}</td>
        <td>${formatTime(d.updatedAt)}</td>
      </tr>`
    )
    .join("");
}

function renderDeviceOptions() {
  const select = $("#notice-device-keys");
  if (!select) return;
  select.innerHTML = "";
  const activeDevices = state.devices.filter((d) => (d.status || "ACTIVE") === "ACTIVE");
  activeDevices
    .filter((d) => (d.status || "ACTIVE") === "ACTIVE")
    .forEach((device) => {
      const option = document.createElement("option");
      option.value = device.deviceKey;
      option.textContent = device.name ? `${device.name} (${device.deviceKey})` : device.deviceKey;
      select.appendChild(option);
    });
  select.disabled = $("#notice-all").checked;
  const snippetSelect = $("#snippet-device-keys");
  if (snippetSelect) {
    snippetSelect.innerHTML = "";
    activeDevices.forEach((device) => {
      const option = document.createElement("option");
      option.value = device.deviceKey;
      option.textContent = device.name ? `${device.name} (${device.deviceKey})` : device.deviceKey;
      snippetSelect.appendChild(option);
    });
    const snippetAll = $("#snippet-all");
    snippetSelect.disabled = snippetAll ? snippetAll.checked : false;
  }
}

async function handleDeviceSubmit(event) {
  event.preventDefault();
  const payload = Object.fromEntries(new FormData(event.currentTarget).entries());
  try {
    const res = await api("/admin/devices", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    $("#device-feedback").textContent = `保存成功，encodeKey: ${res.encodeKey}, IV: ${res.iv}`;
    event.currentTarget.reset();
    loadDevices();
    showToast("设备已保存");
  } catch (err) {
    $("#device-feedback").textContent = err.message;
    showToast(err.message, true);
  }
}

async function handleNoticeSubmit(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const payload = {
    title: form.title.value.trim(),
    subtitle: form.subtitle.value.trim(),
    body: form.body.value.trim(),
    group: form.group.value.trim(),
    url: form.url.value.trim(),
    icon: form.icon.value.trim(),
    image: form.image.value.trim(),
  };
  if (!payload.body) {
    showToast("正文不能为空", true);
    return;
  }
  if (!$("#notice-all").checked) {
    payload.deviceKeys = Array.from($("#notice-device-keys").selectedOptions, (opt) => opt.value).filter(Boolean);
  }
  try {
    const summary = await api("/notice", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    $("#notice-feedback").textContent = `已完成：${summary.successNum}/${summary.sendNum}`;
    showToast("通知发送中");
    form.reset();
    $("#notice-all").checked = true;
    $("#notice-device-keys").disabled = true;
    loadDashboard();
    loadLogs(false);
  } catch (err) {
    $("#notice-feedback").textContent = err.message;
    showToast(err.message, true);
  }
}

function applyTemplate(type) {
  const form = $("#notice-form");
  if (!form) return;
  if (type === "jellyfin") {
    form.title.value = "🎬 Jellyfin 播放";
    form.group.value = "jellyfin";
    form.body.value = "用户：{{NotificationUsername}}\n影片：{{Name}}\n终端：{{DeviceName}}";
  } else if (type === "simple") {
    form.title.value = "提示";
    form.group.value = "default";
    form.body.value = "这是一条来自 Bark Secure Proxy 的测试消息。";
  }
}

async function loadLogs(force) {
  const body = $("#logs-table-body");
  if (!force && body.dataset.loaded === "true") {
    return;
  }
  const form = $("#log-filter");
  const params = new URLSearchParams({
    page: "1",
    pageSize: "20",
  });
  if (form.group.value) params.set("group", form.group.value.trim());
  if (form.status.value) params.set("status", form.status.value);
  if (form.beginTime.value) params.set("beginTime", form.beginTime.value);
  if (form.endTime.value) params.set("endTime", form.endTime.value);
  try {
    const res = await api(`/api/notice/log/list?${params.toString()}`);
    renderLogTable(res.data || []);
    body.dataset.loaded = "true";
  } catch (err) {
    showToast(err.message, true);
  }
}

function renderLogTable(logs) {
  const body = $("#logs-table-body");
  if (!logs.length) {
    body.innerHTML = `<tr><td colspan="5" class="empty">暂无数据</td></tr>`;
    return;
  }
  body.innerHTML = logs
    .map(
      (log) => `<tr>
        <td>${formatTime(log.createdAt)}</td>
        <td>${escapeHtml(log.title || "-")}</td>
        <td>${escapeHtml(log.group || "-")}</td>
        <td>${mask(log.deviceKey)}</td>
        <td>${log.status}</td>
      </tr>`
    )
    .join("");
}

function updateSnippet() {
  const form = $("#snippet-form");
  if (!form) return;
  const params = Object.fromEntries(new FormData(form).entries());
  params.host = (params.host && params.host.trim() ? params.host.trim() : `${location.protocol}//${location.host}`)
    .replace(/\/$/, "");
  const snippetSelect = $("#snippet-device-keys");
  params.deviceKeys = snippetSelect
    ? Array.from(snippetSelect.selectedOptions, (opt) => opt.value).filter(Boolean)
    : [];
  params.broadcastAll = $("#snippet-all") ? $("#snippet-all").checked : true;
  params.subtitle = params.subtitle?.trim();
  params.group = params.group?.trim();
  params.url = params.url?.trim();
  params.icon = params.icon?.trim();
  params.image = params.image?.trim();
  params.body = params.body || "";
  const snippet = buildSnippetParts(params.endpoint, params);
  $("#snippet-method-output").value = snippet.method || "";
  $("#snippet-url-output").value = snippet.url || "";
  $("#snippet-headers-output").value = (snippet.headers || []).join("\n");
  $("#snippet-body-output").value = snippet.body || "";
}

function copySnippet() {
  const method = $("#snippet-method-output").value || "";
  const url = $("#snippet-url-output").value || "";
  const headers = $("#snippet-headers-output").value || "";
  const body = $("#snippet-body-output").value || "";
  const content = `Method: ${method}\nURL: ${url}\nHeaders:\n${headers || "(none)"}\nBody:\n${body || "(none)"}`;
  navigator.clipboard.writeText(content).then(() => showToast("已复制结果"));
}

function buildSnippetParts(endpoint, params) {
  switch (endpoint) {
    case "notice-post":
      return buildNoticePostSnippet(params);
    case "notice-get":
      return buildNoticeGetSnippet(params);
    case "device-gen":
      return buildDeviceGenSnippet(params);
    case "log-list":
      return buildLogListSnippet(params);
    default:
      return {};
  }
}

function buildNoticePostSnippet(params) {
  const payload = {
    title: params.title || "提示",
    body: params.body || "你好 Bark",
  };
  if (params.subtitle) payload.subtitle = params.subtitle;
  if (params.group) payload.group = params.group;
  if (params.url) payload.url = params.url;
  if (params.icon) payload.icon = params.icon;
  if (params.image) payload.image = params.image;
  if (!params.broadcastAll && params.deviceKeys && params.deviceKeys.length) {
    payload.deviceKeys = params.deviceKeys;
  }
  return {
    method: "POST",
    url: `${params.host}/notice`,
    headers: ["Content-Type: application/json"],
    body: JSON.stringify(payload, null, 2),
  };
}

function buildNoticeGetSnippet(params) {
  const title = params.title || "测试";
  const body = params.body || "你好";
  const query = `title=${encodeURIComponent(title)}&body=${encodeURIComponent(body)}`;
  return {
    method: "GET",
    url: `${params.host}/notice?${query}`,
    headers: [],
    body: "",
  };
}

function buildDeviceGenSnippet(params) {
  const payload = {
    deviceToken: params.deviceToken || "DEVICE_TOKEN",
    deviceKey: params.deviceKey || "DEVICE_KEY",
    name: params.title || "我的设备",
  };
  return {
    method: "POST",
    url: `${params.host}/device/gen`,
    headers: ["Content-Type: application/json"],
    body: JSON.stringify(payload, null, 2),
  };
}

function buildLogListSnippet(params) {
  const token = params.token || "<token>";
  return {
    method: "GET",
    url: `${params.host}/api/notice/log/list?page=1&pageSize=20`,
    headers: token ? [`Authorization: Bearer ${token}`] : [],
    body: "",
  };
}

async function api(path, options = {}) {
  const headers = {
    "Content-Type": "application/json",
    ...(options.headers || {}),
  };
  if (!options.skipAuth && state.token) {
    headers.Authorization = `Bearer ${state.token}`;
  }
  const res = await fetch(path, {
    method: options.method || "GET",
    headers,
    body: options.body,
  });
  let data;
  const contentType = res.headers.get("content-type") || "";
  if (contentType.includes("application/json")) {
    data = await res.json();
  } else {
    data = await res.text();
  }
  if (!res.ok) {
    const message =
      typeof data === "object" && data !== null
        ? data.error || data.message || res.statusText
        : data || res.statusText;
    throw new Error(message);
  }
  if (data && typeof data === "object" && "code" in data && "msg" in data) {
    if (data.code !== "000000") {
      throw new Error(data.msg || "请求失败");
    }
    return data.data;
  }
  return data;
}

function showToast(message, isError = false) {
  const toast = $("#toast");
  toast.textContent = message;
  toast.classList.toggle("error", isError);
  toast.classList.remove("hidden");
  toast.classList.add("visible");
  clearTimeout(showToast.timeout);
  showToast.timeout = setTimeout(() => {
    toast.classList.remove("visible");
  }, 3000);
}

function mask(value) {
  if (!value) return "-";
  if (value.length <= 4) return value;
  return `${value.slice(0, 4)}***${value.slice(-2)}`;
}

function escapeHtml(value = "") {
  return value.replace(/[&<>'"]/g, (char) => {
    const map = {
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      "'": "&#39;",
      '"': "&quot;",
    };
    return map[char];
  });
}

function formatTime(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}
