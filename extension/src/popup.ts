/**
 * The toolbar popup. For this milestone it reports the daemon and the last
 * thing the extension tried in this tab; the clipper task grows it further.
 */

import { loadSettings } from "./settings";
import { getTabStatus } from "./status";

function el<T extends HTMLElement>(id: string): T {
  const node = document.getElementById(id);
  if (node === null) throw new Error(`missing element #${id}`);
  return node as T;
}

async function init() {
  const settings = await loadSettings();
  el("daemon").textContent = `Daemon: ${settings.daemonUrl}`;

  const status = el("status");
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  const record = tab?.id === undefined ? null : await getTabStatus(tab.id);
  if (record === null) {
    status.textContent = "Nothing to report for this tab.";
    status.className = "status";
  } else {
    status.textContent = record.message;
    status.className = `status ${record.kind === "opened" ? "ok" : "error"}`;
  }

  el<HTMLButtonElement>("options").addEventListener("click", () => {
    void chrome.runtime.openOptionsPage();
  });
}

void init();
