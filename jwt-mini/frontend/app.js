const TOKEN_KEY = "jwt_mini_token";
const API_BASE = (window.APP_CONFIG && window.APP_CONFIG.API_BASE) || "http://localhost:8080";

const loginSection = document.getElementById("login-section");
const registerSection = document.getElementById("register-section");
const mainSection = document.getElementById("main-section");
const loginForm = document.getElementById("login-form");
const registerForm = document.getElementById("register-form");
const goRegisterBtn = document.getElementById("go-register-btn");
const goLoginBtn = document.getElementById("go-login-btn");
const logoutBtn = document.getElementById("logout-btn");
const navFilesBtn = document.getElementById("nav-files-btn");
const navProfileBtn = document.getElementById("nav-profile-btn");
const filesPanel = document.getElementById("files-panel");
const profilePanel = document.getElementById("profile-panel");
const profileRefreshBtn = document.getElementById("profile-refresh-btn");
const uploadBtn = document.getElementById("upload-btn");
const refreshBtn = document.getElementById("refresh-btn");
const fileInput = document.getElementById("file-input");
const fileList = document.getElementById("file-list");
const statusBox = document.getElementById("status");
const welcomeText = document.getElementById("welcome-text");

function getToken() { return localStorage.getItem(TOKEN_KEY); }
function setToken(token) { localStorage.setItem(TOKEN_KEY, token); }
function clearToken() { localStorage.removeItem(TOKEN_KEY); }

function showStatus(message, type = "info") {
  if (!statusBox) return;
  statusBox.textContent = message;
  statusBox.className = `status ${type}`;
  statusBox.classList.remove("hidden");
}

function hideStatus() {
  if (statusBox) statusBox.classList.add("hidden");
}

async function api(path, options = {}) {
  const headers = { ...(options.headers || {}) };
  if (options.auth !== false) {
    const token = getToken();
    if (token) headers.Authorization = `Bearer ${token}`;
  }
  if (options.body && !(options.body instanceof FormData)) {
    headers["Content-Type"] = "application/json";
  }

  const url = path.startsWith("http") ? path : `${API_BASE}${path}`;
  const res = await fetch(url, { ...options, headers });
  const text = await res.text();
  let data = {};
  if (text) {
    try { data = JSON.parse(text); } catch { data = { raw: text }; }
  }
  if (!res.ok) {
    throw new Error(data.error || data.message || `请求失败 (${res.status})`);
  }
  return data;
}

function showMain() {
  loginSection.classList.add("hidden");
  registerSection.classList.add("hidden");
  mainSection.classList.remove("hidden");
  showFilesPanel();
}

function showFilesPanel() {
  filesPanel.classList.remove("hidden");
  profilePanel.classList.add("hidden");
  navFilesBtn.classList.add("active");
  navProfileBtn.classList.remove("active");
}

function showProfilePanel() {
  filesPanel.classList.add("hidden");
  profilePanel.classList.remove("hidden");
  navFilesBtn.classList.remove("active");
  navProfileBtn.classList.add("active");
}

function formatPermissions(perms) {
  const labels = [];
  if (perms & 1) labels.push("读取");
  if (perms & 2) labels.push("写入");
  if (perms & 4) labels.push("删除");
  if (perms & 8) labels.push("分享");
  return labels.length ? labels.join("、") : "无";
}

async function loadProfile() {
  const data = await api("/api/profile");

  document.getElementById("profile-username").textContent = data.username;
  document.getElementById("profile-userid").textContent = `ID: ${data.user_id}`;
  document.getElementById("profile-phone").textContent = data.phone || "未绑定";
  document.getElementById("profile-created").textContent = formatTime(data.created_at);
  document.getElementById("profile-file-count").textContent = String(data.file_count ?? 0);
  document.getElementById("profile-perms").textContent = formatPermissions(data.permissions ?? 0);
  document.getElementById("profile-avatar").textContent =
    (data.username && data.username[0]) ? data.username[0].toUpperCase() : "U";

  welcomeText.textContent = `欢迎，${data.username}`;
}

function showLogin() {
  mainSection.classList.add("hidden");
  registerSection.classList.add("hidden");
  loginSection.classList.remove("hidden");
}

function showRegister() {
  mainSection.classList.add("hidden");
  loginSection.classList.add("hidden");
  registerSection.classList.remove("hidden");
  hideStatus();
}

function formatSize(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function formatTime(value) {
  return new Date(value).toLocaleString("zh-CN");
}

function escapeHtml(str) {
  return String(str)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

async function login(username, password) {
  const data = await api("/login", {
    method: "POST",
    auth: false,
    body: JSON.stringify({ username, password }),
  });
  setToken(data.token_access);
  welcomeText.textContent = `欢迎，${username}`;
  showMain();
  await loadFiles();
}

async function register(username, password, phone) {
  const body = { username, password };
  if (phone) body.phone = phone;

  await api("/register", {
    method: "POST",
    auth: false,
    body: JSON.stringify(body),
  });
  registerForm.reset();
  document.getElementById("login-username").value = username;
  document.getElementById("login-password").value = "";
  showLogin();
  showStatus("注册成功，请登录", "success");
}

async function loadFiles() {
  const data = await api("/api/files?page=1&size=50");
  const files = data.files || [];

  if (files.length === 0) {
    fileList.className = "file-list empty";
    fileList.textContent = "暂无文件";
    return;
  }

  fileList.className = "file-list";
  fileList.innerHTML = "";

  files.forEach((file) => {
    const item = document.createElement("div");
    item.className = "file-item";
    item.innerHTML = `
      <div class="file-meta">
        <div class="file-name">${escapeHtml(file.filename)}</div>
        <div class="file-sub">#${file.file_id} · ${formatSize(file.size)} · ${formatTime(file.created_at)}</div>
      </div>
      <div class="file-actions">
        <button class="btn ghost" data-action="download" data-id="${file.file_id}">下载</button>
        <button class="btn danger" data-action="delete" data-id="${file.file_id}">删除</button>
      </div>
    `;
    fileList.appendChild(item);
  });
}

async function uploadFile(file) {
  showStatus("正在申请上传链接...", "info");

  const presign = await api("/api/files/presign/upload", {
    method: "POST",
    body: JSON.stringify({ filename: file.name }),
  });

  showStatus("正在上传到 MinIO...", "info");

  const putRes = await fetch(presign.upload_url, {
    method: "PUT",
    body: await file.arrayBuffer(),
  });
  if (!putRes.ok) {
    throw new Error(`上传到 MinIO 失败 (${putRes.status})`);
  }

  showStatus("正在确认入库...", "info");

  await api("/api/files/confirm", {
    method: "POST",
    body: JSON.stringify({
      filename: file.name,
      object_key: presign.object_key,
      size: file.size,
    }),
  });

  showStatus("上传成功", "success");
  await loadFiles();
}

async function downloadFile(fileId) {
  showStatus("正在获取下载链接...", "info");
  const data = await api(`/api/files/presign/download/${fileId}`);
  window.open(data.download_url, "_blank");
  hideStatus();
}

async function deleteFile(fileId) {
  if (!confirm("确定删除这个文件吗？")) return;
  await api(`/api/files/remove/${fileId}`, { method: "DELETE" });
  showStatus("删除成功", "success");
  await loadFiles();
}

loginForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  hideStatus();
  try {
    await login(
      document.getElementById("login-username").value.trim(),
      document.getElementById("login-password").value
    );
  } catch (err) {
    showStatus(err.message, "error");
  }
});

registerForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  hideStatus();

  const username = document.getElementById("reg-username").value.trim();
  const phone = document.getElementById("reg-phone").value.trim();
  const password = document.getElementById("reg-password").value;
  const password2 = document.getElementById("reg-password2").value;

  if (password !== password2) {
    showStatus("两次输入的密码不一致", "error");
    return;
  }
  if (phone && !/^1[3-9]\d{9}$/.test(phone)) {
    showStatus("手机号格式不正确", "error");
    return;
  }

  try {
    await register(username, password, phone);
  } catch (err) {
    showStatus(err.message, "error");
  }
});

goRegisterBtn.addEventListener("click", () => showRegister());
goLoginBtn.addEventListener("click", () => {
  registerForm.reset();
  showLogin();
});

navFilesBtn.addEventListener("click", () => {
  hideStatus();
  showFilesPanel();
});

navProfileBtn.addEventListener("click", async () => {
  hideStatus();
  showProfilePanel();
  try {
    await loadProfile();
  } catch (err) {
    showStatus(err.message, "error");
  }
});

profileRefreshBtn.addEventListener("click", async () => {
  try {
    await loadProfile();
    showStatus("资料已刷新", "success");
  } catch (err) {
    showStatus(err.message, "error");
  }
});

logoutBtn.addEventListener("click", () => {
  clearToken();
  showLogin();
  hideStatus();
});

uploadBtn.addEventListener("click", async () => {
  const file = fileInput.files[0];
  if (!file) {
    showStatus("请先选择文件", "error");
    return;
  }
  uploadBtn.disabled = true;
  try {
    await uploadFile(file);
    fileInput.value = "";
  } catch (err) {
    showStatus(err.message, "error");
  } finally {
    uploadBtn.disabled = false;
  }
});

refreshBtn.addEventListener("click", async () => {
  try {
    await loadFiles();
    showStatus("列表已刷新", "success");
  } catch (err) {
    showStatus(err.message, "error");
  }
});

fileList.addEventListener("click", async (e) => {
  const btn = e.target.closest("button");
  if (!btn) return;
  try {
    if (btn.dataset.action === "download") await downloadFile(btn.dataset.id);
    if (btn.dataset.action === "delete") await deleteFile(btn.dataset.id);
  } catch (err) {
    showStatus(err.message, "error");
  }
});

document.getElementById("api-hint").textContent = `API: ${API_BASE}`;

(async function init() {
  if (!getToken()) return;
  try {
    await loadProfile();
    showMain();
    await loadFiles();
  } catch {
    clearToken();
    showLogin();
  }
})();
