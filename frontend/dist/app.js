"use strict";

// Leggio: la barra laterale con i file e il documento impaginato.
// Tutto il lavoro sui file lo fa Go (window.go.main.App); qui si disegna.

const $ = (id) => document.getElementById(id);
const api = () => window.go.main.App;
const rt = () => window.runtime;

const tree = $("tree");
const content = $("content");
const scroller = $("scroller");
const filterInput = $("filter");

let state = { files: [], folders: [], current: "", ui: {}, sep: "/" };
let ui = { theme: "auto", sidebarWidth: 260, sidebarHidden: false, zoom: 1, expanded: [] };
let current = "";
let currentDoc = null;
let expanded = new Set();
const scrollMemory = new Map();
const darkQuery = window.matchMedia("(prefers-color-scheme: dark)");

const ICONS = {
  chev: '<svg viewBox="0 0 16 16"><path d="M6 4l4 4-4 4"/></svg>',
  md: '<svg viewBox="0 0 16 16"><path d="M2.5 4.5h11v7h-11z"/><path d="M4.5 9.5v-3l1.5 1.7 1.5-1.7v3M10.5 6.5v3M9.3 8.4l1.2 1.2 1.2-1.2"/></svg>',
  dir: '<svg viewBox="0 0 16 16"><path d="M1.5 12.5v-9a1 1 0 0 1 1-1h3.6l1.6 1.8h5.8a1 1 0 0 1 1 1v7.2a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1z"/></svg>',
  x: '<svg viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"/></svg>',
};
const THEME_ICONS = {
  auto: '<svg viewBox="0 0 16 16"><circle cx="8" cy="8" r="5.5"/><path class="fill" d="M8 2.5a5.5 5.5 0 0 1 0 11z"/></svg>',
  light: '<svg viewBox="0 0 16 16"><circle cx="8" cy="8" r="3"/><path d="M8 1.5v1.5M8 13v1.5M1.5 8H3M13 8h1.5M3.4 3.4l1.1 1.1M11.5 11.5l1.1 1.1M3.4 12.6l1.1-1.1M11.5 4.5l1.1-1.1"/></svg>',
  dark: '<svg viewBox="0 0 16 16"><path d="M13.5 9.8A5.8 5.8 0 0 1 6.2 2.5a5.8 5.8 0 1 0 7.3 7.3z"/></svg>',
};
const THEME_NAMES = { auto: "automatico", light: "chiaro", dark: "scuro" };

// ---------- avvio ----------

async function start() {
  rt().EventsOn("change", (ch) => applyChange(ch));
  rt().EventsOn("doc-changed", (path) => {
    if (path === current) show(current, { keepScroll: true });
  });
  rt().OnFileDrop((_x, _y, paths) => {
    document.body.classList.remove("dragging");
    if (paths && paths.length) api().AddPaths(paths).then(applyChange);
  }, false);
  darkQuery.addEventListener("change", () => ui.theme === "auto" && applyTheme(true));
  window.addEventListener("focus", () => api().Rescan().then(setState));

  const s = await api().GetState();
  ui = Object.assign(ui, s.ui || {});
  expanded = new Set(ui.expanded || []);
  setState(s);
  applyTheme();
  applyLayout();
  if (s.current) {
    if (!current) await show(s.current);
  } else if (!current) {
    showWelcome();
  }
}

function applyChange(ch) {
  if (!ch) return;
  if (ch.state) setState(ch.state);
  if (ch.show) show(ch.show);
}

let stateKey = "";
function setState(s) {
  state = s;
  state.files = state.files || [];
  state.folders = state.folders || [];
  // Il ritorno in primo piano riscansiona: se non è cambiato niente non si ridisegna.
  const key = JSON.stringify([state.files, state.folders]);
  if (key === stateKey) return;
  stateKey = key;
  renderTree();
}

// ---------- barra laterale ----------

function renderTree() {
  const top = tree.scrollTop;
  fillTree();
  tree.scrollTop = top;
}

function fillTree() {
  const q = filterInput.value.trim();
  tree.textContent = "";
  if (q) return renderFiltered(q);

  if (state.files.length) {
    const key = "!files";
    const open = !expanded.has(key);
    tree.append(sectionHead("Aperti", state.files.length, open, () => toggle(key, true), null));
    if (open) {
      for (const f of state.files) {
        const row = fileRow(f.path, f.name, 0, () => api().CloseFile(f.path).then(onClosed(f.path)));
        if (f.missing) row.classList.add("missing");
        row.title = f.path;
        tree.append(row);
      }
    }
  }
  for (const folder of state.folders) {
    const key = "!" + folder.path;
    const open = !expanded.has(key);
    const head = sectionHead(folder.name, folder.count, open, () => toggle(key, true),
      () => api().CloseFolder(folder.path).then(onClosed(folder.path, true)));
    head.title = folder.path;
    tree.append(head);
    if (!open) continue;
    if (folder.missing) {
      tree.append(note("Cartella non trovata"));
    } else if (!folder.tree || !folder.tree.length) {
      tree.append(note("Nessun file markdown"));
    } else {
      renderNodes(folder.tree, 0);
      if (folder.truncated) tree.append(note("Cartella molto grande: mostrati solo i primi file"));
    }
  }
  if (!state.files.length && !state.folders.length) {
    tree.append(note("Nessun file aperto"));
  }
}

function renderNodes(nodes, depth) {
  for (const n of nodes) {
    if (n.dir) {
      const open = expanded.has(n.path) || containsCurrent(n);
      const row = document.createElement("div");
      row.className = "row dir" + (open ? " open" : "");
      row.style.paddingLeft = 8 + depth * 12 + "px";
      row.dataset.path = n.path;
      row.dataset.dir = "1";
      row.innerHTML = `<span class="chev">${ICONS.chev}</span><span class="ficon f-dir">${ICONS.dir}</span><span class="label"></span>`;
      row.querySelector(".label").textContent = n.name;
      row.addEventListener("click", () => toggle(n.path, false, open));
      tree.append(row);
      if (open) renderNodes(n.children || [], depth + 1);
    } else {
      tree.append(fileRow(n.path, n.name, depth, null));
    }
  }
}

function fileRow(path, name, depth, onClose) {
  const row = document.createElement("div");
  row.className = "row file" + (samePath(path, current) ? " current" : "");
  row.style.paddingLeft = 8 + depth * 12 + 16 + "px";
  row.dataset.path = path;
  row.innerHTML = `<span class="ficon f-md">${ICONS.md}</span><span class="label"></span>`;
  row.querySelector(".label").textContent = name;
  row.addEventListener("click", () => show(path));
  if (onClose) row.append(closeButton("Togli dalla lista", onClose));
  return row;
}

function sectionHead(name, count, open, onToggle, onClose) {
  const head = document.createElement("div");
  head.className = "section-head" + (open ? " open" : "");
  head.innerHTML = `<span class="chev">${ICONS.chev}</span><span class="name"></span>`;
  head.querySelector(".name").textContent = name;
  if (count) {
    const c = document.createElement("span");
    c.className = "count";
    c.textContent = count;
    head.querySelector(".name").append(c);
  }
  head.addEventListener("click", onToggle);
  if (onClose) head.append(closeButton("Chiudi cartella", onClose));
  return head;
}

function closeButton(title, onClose) {
  const b = document.createElement("button");
  b.className = "x";
  b.title = title;
  b.innerHTML = ICONS.x;
  b.addEventListener("click", (e) => {
    e.stopPropagation();
    onClose();
  });
  return b;
}

function note(text) {
  const d = document.createElement("div");
  d.className = "tree-note";
  d.textContent = text;
  return d;
}

// Le sezioni (chiave con "!") sono aperte finché non le si chiude; le
// cartelle dentro sono chiuse finché non le si apre.
function toggle(key, isSection, wasOpen) {
  if (isSection) {
    expanded.has(key) ? expanded.delete(key) : expanded.add(key);
  } else if (wasOpen) {
    expanded.delete(key);
    collapsedByUser.add(key);
  } else {
    expanded.add(key);
    collapsedByUser.delete(key);
  }
  saveUI();
  renderTree();
}

// Una cartella si apre da sola se contiene il file mostrato, a meno che
// l'utente non l'abbia appena chiusa.
const collapsedByUser = new Set();
function containsCurrent(n) {
  if (!current || collapsedByUser.has(n.path)) return false;
  return inDir(current, n.path);
}

function onClosed(path, isFolder) {
  return (s) => {
    setState(s);
    const gone = isFolder ? inDir(current, path) : samePath(current, path);
    if (!gone) return;
    const next = allFiles()[0];
    if (next) show(next.path);
    else showWelcome();
  };
}

function allFiles() {
  const out = state.files.map((f) => ({ path: f.path, name: f.name, rel: dirOf(f.path) }));
  const walk = (nodes, folder, rel) => {
    for (const n of nodes || []) {
      if (n.dir) walk(n.children, folder, rel ? rel + "/" + n.name : n.name);
      else out.push({ path: n.path, name: n.name, rel: rel ? folder.name + "/" + rel : folder.name });
    }
  };
  for (const f of state.folders) walk(f.tree, f, "");
  return out;
}

function fold(s) {
  return s.normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase();
}

function renderFiltered(q) {
  const terms = fold(q).split(/\s+/);
  const hits = allFiles().filter((f) => {
    const hay = fold(f.rel + "/" + f.name);
    return terms.every((t) => hay.includes(t));
  });
  hits.sort((a, b) => {
    const an = fold(a.name).includes(terms[0]) ? 0 : 1;
    const bn = fold(b.name).includes(terms[0]) ? 0 : 1;
    return an - bn || a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });
  });
  if (!hits.length) return tree.append(note("Nessun file corrisponde"));
  for (const h of hits.slice(0, 400)) {
    const row = fileRow(h.path, h.name, 0, null);
    const dim = document.createElement("span");
    dim.className = "dim";
    dim.textContent = h.rel;
    row.querySelector(".label").after(dim);
    row.title = h.path;
    tree.append(row);
  }
}

function markCurrent() {
  for (const r of tree.querySelectorAll(".row.file")) {
    r.classList.toggle("current", samePath(r.dataset.path, current));
  }
  const row = tree.querySelector(".row.current");
  if (row) row.scrollIntoView({ block: "nearest" });
}

// ---------- documento ----------

async function show(path, opts = {}) {
  if (current && !opts.keepScroll) scrollMemory.set(current, scroller.scrollTop);
  const keep = opts.keepScroll ? scroller.scrollTop : null;
  let doc;
  try {
    doc = await api().Render(path);
  } catch (e) {
    toast("Non riesco ad aprire il file: " + e);
    return;
  }
  const switched = !samePath(path, current);
  current = path;
  currentDoc = doc;
  $("welcome").hidden = true;
  scroller.classList.remove("hidden-doc");
  $("docbar").classList.remove("empty");
  const refind = opts.keepScroll && !$("findbar").hidden;
  if (!refind) closeFind(false);

  if (doc.missing) {
    content.innerHTML = `<div class="missing"><h2>File non trovato</h2><p><code></code></p><button>Togli dalla lista</button></div>`;
    content.querySelector("code").textContent = path;
    content.querySelector("button").addEventListener("click", () => api().CloseFile(path).then(onClosed(path)));
  } else {
    content.innerHTML = doc.html;
    enhance();
  }
  renderCrumbs(path);
  if (switched) {
    // Il file mostrato potrebbe stare in una cartella chiusa.
    for (const d of collapsedByUser) if (inDir(path, d)) collapsedByUser.delete(d);
    renderTree();
  }
  markCurrent();

  if (keep !== null) {
    scroller.scrollTop = keep;
    if (refind) runFind(true);
  } else if (opts.fragment) {
    scrollToFragment(opts.fragment);
  } else {
    scroller.scrollTop = scrollMemory.get(path) || 0;
  }
}

function showWelcome() {
  current = "";
  currentDoc = null;
  content.textContent = "";
  scroller.classList.add("hidden-doc");
  $("welcome").hidden = false;
  $("docbar").classList.add("empty");
  $("crumbs").textContent = "";
  markCurrent();
  api().Clear();
}

function renderCrumbs(path) {
  const crumbs = $("crumbs");
  crumbs.textContent = "";
  let parts;
  const folder = state.folders.find((f) => inDir(path, f.path));
  if (folder) {
    parts = [folder.name, ...path.slice(withSep(folder.path).length).split(state.sep)];
  } else {
    const p = path.split(state.sep).filter(Boolean);
    parts = p.slice(-2);
  }
  parts.forEach((p, i) => {
    if (i) {
      const s = document.createElement("span");
      s.className = "sep";
      s.textContent = "›";
      crumbs.append(s);
    }
    const seg = document.createElement("span");
    seg.className = "seg";
    seg.textContent = p;
    crumbs.append(seg);
  });
  crumbs.title = path;
}

// enhance aggiunge ciò che l'HTML da solo non fa: copia del codice e diagrammi.
function enhance() {
  for (const pre of content.querySelectorAll("pre")) {
    const box = document.createElement("div");
    box.className = "code-block";
    pre.replaceWith(box);
    box.append(pre);
    const b = document.createElement("button");
    b.className = "copy";
    b.textContent = "Copia";
    b.addEventListener("click", () => {
      copyText(pre.innerText.replace(/\n$/, ""));
      b.textContent = "Copiato";
      setTimeout(() => (b.textContent = "Copia"), 1200);
    });
    box.append(b);
  }
  if (currentDoc && currentDoc.mermaid) renderMermaid();
}

let mermaidLoading = null;
function loadMermaid() {
  if (window.mermaid) return Promise.resolve();
  if (!mermaidLoading) {
    mermaidLoading = new Promise((ok, fail) => {
      const s = document.createElement("script");
      s.src = "vendor/mermaid.min.js";
      s.onload = ok;
      s.onerror = fail;
      document.head.append(s);
    });
  }
  return mermaidLoading;
}

async function renderMermaid() {
  const nodes = [...content.querySelectorAll(".mermaid")];
  if (!nodes.length) return;
  const doc = currentDoc;
  try {
    await loadMermaid();
  } catch {
    return;
  }
  if (doc !== currentDoc) return;
  const dark = document.documentElement.dataset.theme === "dark";
  window.mermaid.initialize({ startOnLoad: false, theme: dark ? "dark" : "default", securityLevel: "strict", fontFamily: "inherit" });
  let i = 0;
  for (const el of nodes) {
    const src = el.textContent;
    const id = "mmd-" + Date.now() + "-" + i++;
    try {
      const { svg } = await window.mermaid.render(id, src);
      if (doc !== currentDoc) return;
      el.innerHTML = svg;
      el.classList.add("done");
    } catch (e) {
      // mermaid lascia nella pagina il suo riquadro d'errore: va tolto.
      document.getElementById("d" + id)?.remove();
      document.getElementById(id)?.remove();
      el.classList.add("mermaid-error");
      el.textContent = "Diagramma non valido: " + (e && e.message ? e.message : e) + "\n\n" + src;
    }
  }
}

function scrollToFragment(frag) {
  let id = frag;
  try {
    id = decodeURIComponent(frag);
  } catch {}
  const el = document.getElementById(id) || document.getElementById(id.toLowerCase()) || document.getElementsByName(id)[0];
  if (el && content.contains(el)) {
    el.closest("details")?.setAttribute("open", "");
    el.scrollIntoView({ block: "start" });
  }
}

// Ogni clic su un collegamento passa da qui: la vista non deve mai
// navigare via dalla pagina.
function onLinkClick(e) {
  const a = e.target.closest("a");
  if (!a || !content.contains(a)) return;
  e.preventDefault();
  if (e.type === "auxclick" && e.button !== 1) return;
  const href = a.getAttribute("href");
  if (!href) return;
  if (href.startsWith("#")) return scrollToFragment(href.slice(1));
  api().Follow(current, href).then((t) => {
    if (!t) return;
    if (t.kind === "md") {
      if (t.change && t.change.state) setState(t.change.state);
      show(t.path, { fragment: t.fragment });
    } else if (t.kind === "none") {
      toast(t.path ? "File non trovato: " + t.path : "Collegamento non apribile: " + href);
    }
  });
}

// ---------- cerca nel documento ----------

let marks = [];
let markIndex = -1;

function openFind() {
  if (!currentDoc) return;
  const bar = $("findbar");
  bar.hidden = false;
  const sel = window.getSelection().toString().trim();
  if (sel && !sel.includes("\n")) $("find").value = sel;
  $("find").focus();
  $("find").select();
  runFind();
}

function closeFind(focusDoc = true) {
  $("findbar").hidden = true;
  clearMarks();
  if (focusDoc) scroller.focus();
}

function clearMarks() {
  const parents = new Set();
  for (const m of content.querySelectorAll("mark.find")) {
    parents.add(m.parentNode);
    m.replaceWith(document.createTextNode(m.textContent));
  }
  for (const p of parents) p.normalize();
  marks = [];
  markIndex = -1;
}

function runFind(stay) {
  clearMarks();
  const q = $("find").value;
  const bar = $("findbar");
  if (!q) {
    $("find-count").textContent = "";
    bar.classList.remove("none");
    return;
  }
  const needle = q.toLowerCase();
  const walker = document.createTreeWalker(content, NodeFilter.SHOW_TEXT, {
    acceptNode: (n) => (n.parentElement.closest(".mermaid.done, button, script, style") ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT),
  });
  const nodes = [];
  while (walker.nextNode()) nodes.push(walker.currentNode);
  for (const node of nodes) {
    const text = node.data;
    const lower = text.toLowerCase();
    let i = lower.indexOf(needle);
    if (i < 0 || lower.length !== text.length) continue;
    const frag = document.createDocumentFragment();
    let last = 0;
    while (i >= 0) {
      frag.append(text.slice(last, i));
      const m = document.createElement("mark");
      m.className = "find";
      m.textContent = text.slice(i, i + q.length);
      frag.append(m);
      marks.push(m);
      last = i + q.length;
      i = lower.indexOf(needle, last);
    }
    frag.append(text.slice(last));
    node.replaceWith(frag);
  }
  bar.classList.toggle("none", marks.length === 0);
  if (!marks.length) {
    $("find-count").textContent = "Nessuno";
    return;
  }
  // Parte dal primo risultato visibile, non dall'inizio del documento.
  const top = scroller.getBoundingClientRect().top;
  markIndex = Math.max(0, marks.findIndex((m) => m.getBoundingClientRect().top >= top));
  focusMark(stay);
}

function stepFind(dir) {
  if (!marks.length) return;
  markIndex = (markIndex + dir + marks.length) % marks.length;
  focusMark();
}

function focusMark(stay) {
  marks.forEach((m, i) => m.classList.toggle("now", i === markIndex));
  const m = marks[markIndex];
  if (!stay) {
    m.closest("details")?.setAttribute("open", "");
    m.scrollIntoView({ block: "center" });
  }
  $("find-count").textContent = `${markIndex + 1} di ${marks.length}`;
}

// ---------- tema, zoom, larghezza ----------

function resolvedTheme() {
  if (ui.theme === "light" || ui.theme === "dark") return ui.theme;
  return darkQuery.matches || state.systemDark ? "dark" : "light";
}

function applyTheme(rerender) {
  const t = resolvedTheme();
  const changed = document.documentElement.dataset.theme !== t;
  document.documentElement.dataset.theme = t;
  const btn = $("btn-theme");
  btn.innerHTML = THEME_ICONS[ui.theme] || THEME_ICONS.auto;
  btn.title = "Tema: " + (THEME_NAMES[ui.theme] || THEME_NAMES.auto);
  if (rerender && changed && currentDoc && currentDoc.mermaid) show(current, { keepScroll: true });
}

function cycleTheme() {
  const order = ["auto", "light", "dark"];
  ui.theme = order[(order.indexOf(ui.theme) + 1) % order.length];
  applyTheme(true);
  saveUI();
  toast("Tema " + THEME_NAMES[ui.theme]);
}

function applyLayout() {
  document.documentElement.style.setProperty("--side-w", ui.sidebarWidth + "px");
  document.body.classList.toggle("side-hidden", !!ui.sidebarHidden);
  content.style.setProperty("--zoom", ui.zoom);
}

function zoom(step) {
  ui.zoom = step === 0 ? 1 : Math.min(2.5, Math.max(0.6, Math.round((ui.zoom + step) * 10) / 10));
  applyLayout();
  saveUI();
  toast("Zoom " + Math.round(ui.zoom * 100) + "%");
}

let saveTimer = 0;
function saveUI() {
  ui.expanded = [...expanded];
  clearTimeout(saveTimer);
  saveTimer = setTimeout(() => api().SaveUI(ui), 300);
}

function setupResizer() {
  const r = $("resizer");
  r.addEventListener("mousedown", (e) => {
    e.preventDefault();
    r.classList.add("drag");
    const move = (ev) => {
      ui.sidebarWidth = Math.min(Math.max(ev.clientX, 160), Math.max(200, window.innerWidth - 300));
      applyLayout();
    };
    const up = () => {
      r.classList.remove("drag");
      window.removeEventListener("mousemove", move);
      window.removeEventListener("mouseup", up);
      saveUI();
    };
    window.addEventListener("mousemove", move);
    window.addEventListener("mouseup", up);
  });
  r.addEventListener("dblclick", () => {
    ui.sidebarWidth = 260;
    applyLayout();
    saveUI();
  });
}

// ---------- varie ----------

function samePath(a, b) {
  return !!a && !!b && keyOf(a) === keyOf(b);
}

// Su Windows i percorsi non distinguono le maiuscole.
function keyOf(p) {
  return state.sep === "\\" ? p.toLowerCase() : p;
}

function withSep(dir) {
  return dir.endsWith(state.sep) ? dir : dir + state.sep;
}

function inDir(p, dir) {
  return !!p && keyOf(p).startsWith(keyOf(withSep(dir)));
}

function dirOf(p) {
  const parts = p.split(state.sep);
  return parts.length > 1 ? parts[parts.length - 2] : "";
}

function copyText(text) {
  if (rt().ClipboardSetText) rt().ClipboardSetText(text);
  else navigator.clipboard?.writeText(text);
}

let toastTimer = 0;
function toast(msg) {
  const t = $("toast");
  t.textContent = msg;
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (t.hidden = true), 2200);
}

function openFile() {
  api().OpenFileDialog().then(applyChange).catch((e) => toast(String(e)));
}

function openFolder() {
  api().OpenFolderDialog().then(applyChange).catch((e) => toast(String(e)));
}

function refresh() {
  api().Rescan().then((s) => {
    setState(s);
    if (current) show(current, { keepScroll: true });
  });
}

// Frecce nella lista: su e giù scorrono i file, destra e sinistra aprono e chiudono le cartelle.
function onTreeKey(e) {
  const rows = [...tree.querySelectorAll(".row")];
  if (!rows.length) return;
  let i = rows.findIndex((r) => r.classList.contains("cursor"));
  if (i < 0) i = rows.findIndex((r) => r.classList.contains("current"));
  const move = (j) => {
    j = Math.max(0, Math.min(rows.length - 1, j));
    const r = rows[j];
    setCursor(r.dataset.dir ? r : null);
    r.scrollIntoView({ block: "nearest" });
    if (!r.dataset.dir) show(r.dataset.path);
  };
  const row = rows[i];
  switch (e.key) {
    case "ArrowDown": e.preventDefault(); move(i + 1); break;
    case "ArrowUp": e.preventDefault(); move(i < 0 ? 0 : i - 1); break;
    case "Home": e.preventDefault(); move(0); break;
    case "End": e.preventDefault(); move(rows.length - 1); break;
    case "ArrowRight":
    case "ArrowLeft":
    case "Enter":
      if (row && row.dataset.dir) {
        e.preventDefault();
        const open = row.classList.contains("open");
        if (e.key === "Enter" || (e.key === "ArrowRight") !== open) {
          const p = row.dataset.path;
          toggle(p, false, open);
          setCursor([...tree.querySelectorAll(".row.dir")].find((r) => r.dataset.path === p));
        }
      }
      break;
  }
}

// Il cursore sta su una cartella; sui file basta il file mostrato.
function setCursor(row) {
  tree.querySelectorAll(".row.cursor").forEach((r) => r.classList.remove("cursor"));
  if (row) row.classList.add("cursor");
  tree.classList.toggle("has-cursor", !!row);
}

function onKey(e) {
  const mod = e.ctrlKey || e.metaKey;
  const k = e.key.toLowerCase();
  const inField = e.target instanceof HTMLInputElement;

  if (mod && k === "o") { e.preventDefault(); return e.shiftKey ? openFolder() : openFile(); }
  if (mod && k === "p") { e.preventDefault(); filterInput.focus(); filterInput.select(); return; }
  if (mod && k === "f") { e.preventDefault(); return openFind(); }
  if (mod && k === "b") { e.preventDefault(); ui.sidebarHidden = !ui.sidebarHidden; applyLayout(); saveUI(); return; }
  if (mod && (k === "=" || k === "+")) { e.preventDefault(); return zoom(0.1); }
  if (mod && k === "-") { e.preventDefault(); return zoom(-0.1); }
  if (mod && k === "0") { e.preventDefault(); return zoom(0); }
  if (mod && k === "w" && current) {
    e.preventDefault();
    if (state.files.some((f) => samePath(f.path, current))) api().CloseFile(current).then(onClosed(current));
    return;
  }
  if (e.key === "F5" || (mod && k === "r")) { e.preventDefault(); return refresh(); }
  if (e.key === "F3" || (mod && k === "g")) { e.preventDefault(); return $("findbar").hidden ? openFind() : stepFind(e.shiftKey ? -1 : 1); }
  if (e.key === "Escape" && !$("findbar").hidden) { e.preventDefault(); return closeFind(); }
  if (!inField && e.target === tree) onTreeKey(e);
}

// ---------- collegamenti agli eventi ----------

$("btn-file").addEventListener("click", openFile);
$("btn-folder").addEventListener("click", openFolder);
$("welcome-file").addEventListener("click", openFile);
$("welcome-folder").addEventListener("click", openFolder);
$("btn-refresh").addEventListener("click", refresh);
$("btn-theme").addEventListener("click", cycleTheme);
$("btn-reveal").addEventListener("click", () => current && api().Reveal(current));
$("btn-copy-path").addEventListener("click", () => {
  if (!current) return;
  copyText(current);
  toast("Percorso copiato");
});

filterInput.addEventListener("input", renderTree);
filterInput.addEventListener("keydown", (e) => {
  if (e.key === "Escape") {
    filterInput.value = "";
    renderTree();
    tree.focus();
  } else if (e.key === "Enter" || e.key === "ArrowDown") {
    const first = tree.querySelector(".row.file");
    if (first) {
      e.preventDefault();
      if (e.key === "Enter") show(first.dataset.path);
      tree.focus();
    }
  }
});

$("find").addEventListener("input", () => runFind());
$("find").addEventListener("keydown", (e) => {
  if (e.key === "Enter") { e.preventDefault(); stepFind(e.shiftKey ? -1 : 1); }
});
$("find-next").addEventListener("click", () => stepFind(1));
$("find-prev").addEventListener("click", () => stepFind(-1));
$("find-close").addEventListener("click", () => closeFind());

content.addEventListener("click", onLinkClick);
content.addEventListener("auxclick", onLinkClick);
document.addEventListener("keydown", onKey);
scroller.tabIndex = -1;

document.querySelector("#main").addEventListener("wheel", (e) => {
  if (!e.ctrlKey) return;
  e.preventDefault();
  zoom(e.deltaY < 0 ? 0.1 : -0.1);
}, { passive: false });

// Il riquadro "Rilascia per aprire" mentre si trascinano file sulla finestra.
let dragDepth = 0;
window.addEventListener("dragenter", (e) => {
  if (!e.dataTransfer || !e.dataTransfer.types.includes("Files")) return;
  dragDepth++;
  document.body.classList.add("dragging");
});
window.addEventListener("dragleave", () => {
  if (--dragDepth <= 0) {
    dragDepth = 0;
    document.body.classList.remove("dragging");
  }
});
window.addEventListener("drop", () => {
  dragDepth = 0;
  document.body.classList.remove("dragging");
});

setupResizer();
start();
