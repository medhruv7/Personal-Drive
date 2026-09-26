"use strict";

const $ = (sel) => document.querySelector(sel);

const state = {
  path: "/",
  entries: [],
  sort: { key: "name", desc: false },
  loadSeq: 0,
  loaded: false,
};

const ICONS = {
  folder: '<path d="M3 6a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
  file: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>',
  image: '<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="M21 15l-5-5L5 21"/>',
  video: '<rect x="3" y="5" width="14" height="14" rx="2"/><path d="M17 10l4-2v8l-4-2"/>',
  audio: '<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>',
  archive: '<path d="M21 8v13H3V8M1 3h22v5H1zM10 12h4"/>',
  doc: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M9 13h6M9 17h6"/>',
  download: '<path d="M12 4v12M7 11l5 5 5-5M4 20h16"/>',
};

const EXT_KIND = {
  image: "jpg jpeg png gif webp heic heif bmp svg tiff",
  video: "mp4 mov mkv avi webm m4v",
  audio: "mp3 wav aac m4a flac ogg",
  archive: "zip gz tgz bz2 xz 7z rar tar",
  doc: "pdf doc docx xls xlsx ppt pptx txt md csv json rtf pages numbers key",
};
const kindByExt = {};
for (const [kind, exts] of Object.entries(EXT_KIND)) for (const e of exts.split(" ")) kindByExt[e] = kind;

function icon(name) {
  return `<svg viewBox="0 0 24 24" aria-hidden="true">${ICONS[name]}</svg>`;
}

function fileKind(name) {
  const dot = name.lastIndexOf(".");
  return (dot > 0 && kindByExt[name.slice(dot + 1).toLowerCase()]) || "file";
}

function esc(s) {
  return s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
}

function joinPath(dir, name) {
  return (dir === "/" ? "" : dir) + "/" + name;
}

function hashFor(path) {
  return "#" + path.split("/").map(encodeURIComponent).join("/");
}

function pathFromHash() {
  const raw = location.hash.slice(1) || "/";
  try {
    return "/" + raw.split("/").filter(Boolean).map(decodeURIComponent).join("/");
  } catch {
    return "/";
  }
}

function api(endpoint, path) {
  return `/api/${endpoint}?path=${encodeURIComponent(path)}`;
}

function formatSize(n) {
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let i = -1;
  do { n /= 1024; i++; } while (n >= 1024 && i < units.length - 1);
  return `${n < 10 ? n.toFixed(1) : Math.round(n)} ${units[i]}`;
}

function formatDate(iso) {
  const d = new Date(iso);
  const now = new Date();
  if (d.toDateString() === now.toDateString()) {
    return d.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
  }
  const opts = { month: "short", day: "numeric" };
  if (d.getFullYear() !== now.getFullYear()) opts.year = "numeric";
  return d.toLocaleDateString(undefined, opts);
}

let toastTimer;
function toast(msg) {
  const el = $("#toast");
  el.textContent = msg;
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (el.hidden = true), 4000);
}

async function errorMessage(res) {
  try {
    return (await res.json()).error || res.statusText;
  } catch {
    return res.statusText || `HTTP ${res.status}`;
  }
}

// ---- listing ---------------------------------------------------------------

async function load() {
  const path = pathFromHash();
  const seq = ++state.loadSeq;
  if (path !== state.path) {
    state.entries = [];
    state.loaded = false;
    renderList();
  }
  state.path = path;
  renderCrumbs();
  const loadingTimer = setTimeout(() => ($("#loading").hidden = false), 200);
  try {
    const res = await fetch(api("list", path));
    if (seq !== state.loadSeq) return;
    if (!res.ok) {
      const msg = await errorMessage(res);
      toast(msg);
      if (res.status === 404 && path !== "/") location.hash = "#/";
      return;
    }
    const data = await res.json();
    state.entries = data.entries;
    state.loaded = true;
    renderList();
  } catch {
    if (seq === state.loadSeq) toast("Could not reach the server");
  } finally {
    clearTimeout(loadingTimer);
    if (seq === state.loadSeq) $("#loading").hidden = true;
  }
}

function renderCrumbs() {
  const parts = state.path.split("/").filter(Boolean);
  let html = `<a href="#/">My Drive</a>`;
  let acc = "";
  for (const p of parts) {
    acc += "/" + p;
    html += `<span class="sep">›</span><a href="${hashFor(acc)}">${esc(p)}</a>`;
  }
  const nav = $("#crumbs");
  nav.innerHTML = html;
  nav.scrollLeft = nav.scrollWidth;
  document.title = parts.length ? `${parts[parts.length - 1]} – Local Drive` : "Local Drive";
}

function sortedEntries() {
  const { key, desc } = state.sort;
  const dir = desc ? -1 : 1;
  return [...state.entries].sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
    let c;
    if (key === "size") c = a.size - b.size;
    else if (key === "modTime") c = new Date(a.modTime) - new Date(b.modTime);
    else c = 0;
    if (c === 0) c = a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });
    return c * dir;
  });
}

function renderList() {
  for (const btn of document.querySelectorAll(".sort")) {
    const active = btn.dataset.key === state.sort.key;
    btn.classList.toggle("active", active);
    btn.classList.toggle("desc", active && state.sort.desc);
  }

  const rows = sortedEntries().map((e) => {
    const full = joinPath(state.path, e.name);
    const name = esc(e.name);
    const date = formatDate(e.modTime);
    if (e.isDir) {
      return `<li class="row dir">
        <a class="name" href="${hashFor(full)}">${icon("folder")}<div><span>${name}</span><small class="meta">${date}</small></div></a>
        <span class="size">—</span>
        <span class="date">${date}</span>
        <a class="icon-btn" href="${api("zip", full)}" title="Download folder as zip" aria-label="Download ${name} as zip">${icon("download")}</a>
      </li>`;
    }
    const size = formatSize(e.size);
    const href = api("download", full);
    return `<li class="row">
      <a class="name" href="${href}" download>${icon(fileKind(e.name))}<div><span>${name}</span><small class="meta">${size} · ${date}</small></div></a>
      <span class="size">${size}</span>
      <span class="date">${date}</span>
      <a class="icon-btn" href="${href}" download title="Download" aria-label="Download ${name}">${icon("download")}</a>
    </li>`;
  });

  $("#list").innerHTML = rows.join("");
  $("#empty").hidden = state.entries.length > 0 || !state.loaded;
}

for (const btn of document.querySelectorAll(".sort")) {
  btn.addEventListener("click", () => {
    const key = btn.dataset.key;
    if (state.sort.key === key) state.sort.desc = !state.sort.desc;
    else state.sort = { key, desc: key !== "name" }; // newest/largest first
    renderList();
  });
}

// ---- new folder ------------------------------------------------------------

const mkdirDialog = $("#mkdir-dialog");

$("#btn-mkdir").addEventListener("click", () => {
  $("#mkdir-name").value = "";
  $("#mkdir-error").hidden = true;
  mkdirDialog.showModal();
});

$("#mkdir-form").addEventListener("submit", async (ev) => {
  if (ev.submitter && ev.submitter.value === "cancel") return; // let dialog close
  ev.preventDefault();
  const name = $("#mkdir-name").value.trim();
  if (!name) return;
  const res = await fetch("/api/mkdir", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path: state.path, name }),
  }).catch(() => null);
  if (!res) {
    showMkdirError("Could not reach the server");
  } else if (!res.ok) {
    showMkdirError(await errorMessage(res));
  } else {
    mkdirDialog.close();
    load();
  }
});

function showMkdirError(msg) {
  const el = $("#mkdir-error");
  el.textContent = msg;
  el.hidden = false;
}

// ---- upload ----------------------------------------------------------------

const MAX_PARALLEL = 3;
const queue = [];
let active = 0;
let panelTasks = [];

$("#btn-upload").addEventListener("click", () => $("#file-input").click());
$("#fab").addEventListener("click", () => $("#file-input").click());
$("#file-input").addEventListener("change", (ev) => {
  enqueue([...ev.target.files]);
  ev.target.value = "";
});

$("#uploads-close").addEventListener("click", () => {
  if (active > 0 || queue.length > 0) {
    if (!confirm("Cancel the remaining uploads?")) return;
    queue.length = 0;
    for (const t of panelTasks) t.xhr?.abort();
  }
  panelTasks = [];
  $("#uploads-list").innerHTML = "";
  $("#uploads").hidden = true;
});

function enqueue(files) {
  if (!files.length) return;
  const dir = state.path;
  for (const file of files) {
    const li = document.createElement("li");
    li.innerHTML = `<div class="up-top"><span class="up-name"></span><span class="up-status">Waiting</span></div><div class="bar"><div></div></div>`;
    li.querySelector(".up-name").textContent = file.name;
    $("#uploads-list").append(li);
    const task = { file, dir, li, xhr: null, done: false };
    panelTasks.push(task);
    queue.push(task);
  }
  $("#uploads").hidden = false;
  updatePanelTitle();
  pump();
}

function pump() {
  while (active < MAX_PARALLEL && queue.length) {
    active++;
    upload(queue.shift()).finally(() => {
      active--;
      updatePanelTitle();
      pump();
    });
  }
}

function upload(task) {
  return new Promise((resolve) => {
    const status = task.li.querySelector(".up-status");
    const bar = task.li.querySelector(".bar");
    const fill = bar.firstElementChild;
    const finish = (ok, msg) => {
      task.done = true;
      status.textContent = msg;
      status.className = "up-status " + (ok ? "ok" : "err");
      bar.classList.add(ok ? "ok" : "err");
      fill.style.width = "100%";
      if (ok && task.dir === state.path) refreshSoon();
      resolve();
    };

    const form = new FormData();
    form.append("files", task.file, task.file.name);
    const xhr = new XMLHttpRequest();
    task.xhr = xhr;
    xhr.open("POST", api("upload", task.dir));
    xhr.upload.onprogress = (ev) => {
      if (!ev.lengthComputable) return;
      const pct = Math.round((ev.loaded / ev.total) * 100);
      fill.style.width = pct + "%";
      status.textContent = pct < 100 ? `${pct}%` : "Saving…";
    };
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        let saved = task.file.name;
        try { saved = JSON.parse(xhr.responseText).uploaded[0]; } catch {}
        if (saved !== task.file.name) task.li.querySelector(".up-name").textContent = saved;
        finish(true, "Done");
      } else {
        let msg = `Failed (${xhr.status})`;
        try { msg = JSON.parse(xhr.responseText).error || msg; } catch {}
        finish(false, msg);
      }
    };
    xhr.onerror = () => finish(false, "Network error");
    xhr.onabort = () => finish(false, "Cancelled");
    status.textContent = "0%";
    xhr.send(form);
  });
}

let refreshTimer;
function refreshSoon() {
  clearTimeout(refreshTimer);
  refreshTimer = setTimeout(load, 300);
}

function updatePanelTitle() {
  const remaining = panelTasks.filter((t) => !t.done).length;
  const failed = panelTasks.filter((t) => t.done && t.li.querySelector(".err")).length;
  let title;
  if (remaining) title = `Uploading ${remaining} file${remaining === 1 ? "" : "s"}`;
  else if (failed) title = `${failed} upload${failed === 1 ? "" : "s"} failed`;
  else title = `${panelTasks.length} upload${panelTasks.length === 1 ? "" : "s"} complete`;
  $("#uploads-title").textContent = title;
}

window.addEventListener("beforeunload", (ev) => {
  if (active > 0 || queue.length > 0) ev.preventDefault();
});

// ---- drag & drop -----------------------------------------------------------

let dragDepth = 0;
const hasFiles = (ev) => [...(ev.dataTransfer?.types || [])].includes("Files");

document.addEventListener("dragenter", (ev) => {
  if (!hasFiles(ev)) return;
  ev.preventDefault();
  dragDepth++;
  $("#drop").hidden = false;
});
document.addEventListener("dragover", (ev) => {
  if (hasFiles(ev)) ev.preventDefault();
});
document.addEventListener("dragleave", (ev) => {
  if (!hasFiles(ev)) return;
  if (--dragDepth <= 0) {
    dragDepth = 0;
    $("#drop").hidden = true;
  }
});
document.addEventListener("drop", (ev) => {
  if (!hasFiles(ev)) return;
  ev.preventDefault();
  dragDepth = 0;
  $("#drop").hidden = true;

  const files = [];
  let skippedFolders = 0;
  const items = [...(ev.dataTransfer.items || [])];
  if (items.length && items[0].webkitGetAsEntry) {
    for (const item of items) {
      const entry = item.webkitGetAsEntry();
      if (entry?.isDirectory) skippedFolders++;
      else if (item.kind === "file") files.push(item.getAsFile());
    }
  } else {
    files.push(...ev.dataTransfer.files);
  }
  if (skippedFolders) toast("Folder upload isn't supported — drop the files inside instead");
  enqueue(files.filter(Boolean));
});

// ---- boot ------------------------------------------------------------------

window.addEventListener("hashchange", load);
load();
