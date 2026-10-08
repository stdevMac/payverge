import { shots } from "./shots.js";

// Unpacked-extension gotcha: Chrome serves popup files fresh from disk but
// caches the manifest + service worker until an explicit reload (a browser
// restart is NOT enough). If this popup finds itself running under a stale
// manifest, reload the whole extension once — same effect as the Reload
// button on chrome://extensions. The version equality check makes a loop
// impossible: after the reload getManifest() reports the current version.
const EXPECTED_VERSION = "0.3.0";
// The title doubles as an externally observable beacon (readable by tab
// title) of which manifest version this page is actually running under.
document.title = `pv-shots boot v${chrome.runtime.getManifest().version}`;
if (chrome.runtime.getManifest().version !== EXPECTED_VERSION) {
  document.title = `pv-shots STALE v${chrome.runtime.getManifest().version} — reloading`;
  chrome.runtime.reload();
  throw new Error("stale extension version — reloading from disk");
}

const shotsEl = document.getElementById("shots");
const runBtn = document.getElementById("run");
const logEl = document.getElementById("log");

for (const shot of shots) {
  const label = document.createElement("label");
  const cb = document.createElement("input");
  cb.type = "checkbox";
  cb.checked = true;
  cb.value = shot.name;
  label.appendChild(cb);
  label.appendChild(document.createTextNode(" " + shot.name));
  shotsEl.appendChild(label);
}

function selectedShotIds() {
  return [...shotsEl.querySelectorAll("input:checked")].map((cb) => cb.value);
}

function startRun() {
  const shotIds = selectedShotIds();
  if (shotIds.length === 0) return;
  logEl.textContent = "";
  runBtn.disabled = true;
  chrome.runtime
    .sendMessage({ type: "RUN", shotIds })
    .then(() => {
      document.title = `pv-shots RUN sent (${shotIds.length})`;
    })
    .catch((e) => {
      document.title = `pv-shots RUN send FAILED: ${e.message}`;
    });
}

runBtn.addEventListener("click", startRun);

chrome.runtime.onMessage.addListener((msg) => {
  if (msg.type === "PROGRESS") {
    const detail = msg.message ? ` — ${msg.message}` : "";
    logEl.textContent += `${msg.name}: ${msg.state}${detail}\n`;
  } else if (msg.type === "DONE") {
    logEl.textContent += "Done.\n";
    runBtn.disabled = false;
  }
});

// The popup closes when the run focuses the target window, so live progress
// is usually lost. Show the persisted log from the last run on open — that is
// where skip/error reasons (missing table code, expired session…) surface.
chrome.storage.local.get(["lastRunLog", "lastRunAt"]).then(({ lastRunLog, lastRunAt }) => {
  if (logEl.textContent || !lastRunLog) return;
  const when = lastRunAt ? new Date(lastRunAt).toLocaleString() : "";
  logEl.textContent = `— last run${when ? ` (${when})` : ""} —\n${lastRunLog}\n`;
});

// Scripted/hands-free entry point: opening
//   chrome-extension://<id>/popup.html?autorun=1[&shots=a,b,c]
// as a regular tab runs the (optionally filtered) shot list without
// clicking. Target-tab discovery in the background finds the dashboard tab
// on its own, so the popup does not need to be anchored to it.
const params = new URLSearchParams(location.search);
if (params.get("autorun") === "1") {
  const only = (params.get("shots") || "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  if (only.length > 0) {
    for (const cb of shotsEl.querySelectorAll("input")) {
      cb.checked = only.includes(cb.value);
    }
  }
  startRun();
}
