"use strict";

const $q = document.getElementById("q");
const $go = document.getElementById("go");
const $list = document.getElementById("suggestions");
const $status = document.getElementById("status");
const $message = document.getElementById("message");
const $trendingSection = document.getElementById("trending-section");
const $trending = document.getElementById("trending");
const $segs = [...document.querySelectorAll(".seg")];

const DEBOUNCE_MS = 150;
let timer = null;
let inflight = null; // AbortController for the outstanding /suggest, if any
let items = [];
let active = -1;     // highlighted suggestion, -1 = none
let mode = "count";  // count | recency
let lastPrefix = ""; // prefix the current results matched, for bolding

$q.addEventListener("input", () => {
  clearTimeout(timer);
  timer = setTimeout(fetchSuggestions, DEBOUNCE_MS);
});

$q.addEventListener("keydown", (e) => {
  if (e.key === "ArrowDown") { e.preventDefault(); move(1); }
  else if (e.key === "ArrowUp") { e.preventDefault(); move(-1); }
  else if (e.key === "Enter") {
    if (active >= 0 && items[active]) $q.value = items[active].query;
    submitSearch();
  } else if (e.key === "Escape") { hideList(); }
});

$go.addEventListener("click", submitSearch);

$segs.forEach((b) =>
  b.addEventListener("click", () => {
    mode = b.dataset.mode;
    $segs.forEach((s) => s.classList.toggle("active", s === b));
    fetchSuggestions();
  })
);

document.addEventListener("click", (e) => {
  if (!e.target.closest(".search")) hideList();
});

async function fetchSuggestions() {
  const q = $q.value.trim();
  if (!q) { hideList(); $status.textContent = ""; return; }

  if (inflight) inflight.abort(); // drop the older request, we only want the latest
  inflight = new AbortController();
  $status.textContent = "…";

  try {
    const res = await fetch(`/suggest?q=${encodeURIComponent(q)}&mode=${mode}`, { signal: inflight.signal });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    items = await res.json();
    lastPrefix = q.toLowerCase();
    render();
    $status.textContent = items.length ? `${items.length} result${items.length > 1 ? "s" : ""}` : "no matches";
  } catch (err) {
    if (err.name === "AbortError") return;
    $status.textContent = "error fetching suggestions";
    hideList();
  }
}

function render() {
  $list.innerHTML = "";
  active = -1;
  if (!items.length) { hideList(); return; }
  const n = lastPrefix.length;
  items.forEach((it) => {
    const li = document.createElement("li");
    li.setAttribute("role", "option");

    // bold the part of the query that matched what was typed
    const q = document.createElement("span");
    q.className = "q";
    const head = it.query.slice(0, n);
    const tail = it.query.slice(n);
    if (head) {
      const b = document.createElement("strong");
      b.textContent = head;
      q.appendChild(b);
    }
    if (tail) q.appendChild(document.createTextNode(tail));

    // count in count mode, combined recency score otherwise
    const sc = document.createElement("span");
    sc.className = "score";
    sc.textContent = fmtScore(it.score);

    li.append(q, sc);
    li.addEventListener("mousedown", (e) => { // mousedown fires before input blur
      e.preventDefault();
      $q.value = it.query;
      submitSearch();
    });
    $list.appendChild(li);
  });
  $list.hidden = false;
}

function fmtScore(s) {
  return Number.isInteger(s) ? s.toLocaleString() : s.toFixed(2);
}

function move(delta) {
  if (!items.length) return;
  active = (active + delta + items.length) % items.length;
  [...$list.children].forEach((li, i) => li.classList.toggle("active", i === active));
}

function hideList() { $list.hidden = true; active = -1; }

async function submitSearch() {
  const q = $q.value.trim();
  if (!q) return;
  hideList();
  try {
    const res = await fetch("/search", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ q }),
    });
    const data = await res.json();
    showMessage(res.ok ? (data.message || "Searched") : (data.error || "Error"));
  } catch {
    showMessage("Network error");
  }
  loadTrending();
}

function showMessage(text) {
  $message.textContent = text;
  $message.hidden = false;
}

async function loadTrending() {
  try {
    const res = await fetch("/trending");
    if (!res.ok) return; // nothing trending yet, leave the panel hidden
    const data = await res.json();
    if (!Array.isArray(data) || !data.length) return;
    $trending.innerHTML = "";
    data.forEach((it) => {
      const li = document.createElement("li");
      li.textContent = it.query;
      $trending.appendChild(li);
    });
    $trendingSection.hidden = false;
  } catch { /* trending is optional */ }
}

loadTrending();
