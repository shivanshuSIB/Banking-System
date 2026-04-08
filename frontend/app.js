// ── Config ──
const API = window.location.origin + "/api";
/** Abort hanging fetches so action buttons always leave the disabled state (see finally blocks). */
const DEFAULT_FETCH_TIMEOUT_MS = 60_000;

// ── Helpers ──
// Account cache stores full objects: id -> { name, currency }
const accountCache = {};

function currencySymbol(currency) {
  return currency === "INR" ? "\u20B9" : "\u20B9"; // Default to INR
}

function formatAmount(value, currency) {
  const sym = currencySymbol(currency || "INR");
  return sym + Number(value);
}

function shortID(id) {
  return id ? id.substring(0, 8) + "..." : "-";
}

function formatDate(iso) {
  const d = new Date(iso);
  return d.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function generateIdempotencyKey() {
  return crypto.randomUUID();
}

function getAccountName(id) {
  return accountCache[id] ? accountCache[id].name : shortID(id);
}

function getAccountCurrency(id) {
  return accountCache[id] ? accountCache[id].currency : "INR";
}

// ── Toast ──
let toastTimer = null;
function showToast(message, type) {
  const toast = document.getElementById("toast");
  toast.textContent = message;
  toast.className = "toast show toast-" + type;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    toast.className = "toast hidden";
  }, 3500);
}

// ── API calls ──
async function fetchWithTimeout(url, init, timeoutMs = DEFAULT_FETCH_TIMEOUT_MS) {
  const controller = new AbortController();
  const tid = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(url, { ...init, signal: controller.signal });
  } catch (err) {
    if (err.name === "AbortError") {
      throw new Error("Request timed out. Check your connection and try again.");
    }
    throw err;
  } finally {
    clearTimeout(tid);
  }
}

async function apiGet(path) {
  const res = await fetchWithTimeout(API + path, {});
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || "Request failed: " + res.status);
  }
  return res.json();
}

async function apiPost(path, data) {
  const res = await fetchWithTimeout(API + path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok && res.status !== 200) {
    throw new Error(body.error || "Request failed: " + res.status);
  }
  return body;
}

/** Clears in-flight guard when opening a page (e.g. user left mid-request). */
function clearActionBusy(buttonId) {
  const btn = document.getElementById(buttonId);
  if (!btn) return;
  btn.dataset.busy = "0";
  btn.classList.remove("btn-busy");
  btn.disabled = false;
}

function beginActionBusy(btn) {
  btn.dataset.busy = "1";
  btn.classList.add("btn-busy");
}

function endActionBusy(btn) {
  btn.dataset.busy = "0";
  btn.classList.remove("btn-busy");
}

function isActionBusy(btn) {
  return btn.dataset.busy === "1";
}

// ── Navigation ──
const navLinks = document.querySelectorAll(".nav-link");
const pages = document.querySelectorAll(".page");

function navigate(pageName) {
  navLinks.forEach((link) => {
    link.classList.toggle("active", link.dataset.page === pageName);
  });
  pages.forEach((page) => {
    page.classList.toggle("active", page.id === "page-" + pageName);
  });

  // Load data for the page
  switch (pageName) {
    case "dashboard":
      loadDashboard();
      break;
    case "accounts":
      loadAccounts();
      clearActionBusy("btn-create-account");
      break;
    case "deposit":
      loadAccountDropdowns();
      clearActionBusy("btn-deposit");
      break;
    case "withdraw":
      loadAccountDropdowns();
      clearActionBusy("btn-withdraw");
      break;
    case "transfer":
      loadAccountDropdowns();
      clearActionBusy("btn-transfer");
      break;
    case "transactions":
      loadTransactions();
      break;
  }
}

navLinks.forEach((link) => {
  link.addEventListener("click", (e) => {
    e.preventDefault();
    navigate(link.dataset.page);
  });
});

// ── Load Accounts into Cache ──
async function cacheAccounts() {
  try {
    const accounts = await apiGet("/accounts");
    accounts.forEach((a) => {
      accountCache[a.id] = { name: a.name, currency: a.currency };
    });
    return accounts;
  } catch (err) {
    console.error("Failed to cache accounts:", err);
    return [];
  }
}

// ── Dashboard ──
async function loadDashboard() {
  try {
    const [accounts, transactions] = await Promise.all([
      cacheAccounts(),
      apiGet("/transactions"),
    ]);

    // Stats
    document.getElementById("stat-accounts").textContent = accounts.length;
    const totalBalance = accounts.reduce((sum, a) => sum + a.balance, 0);
    document.getElementById("stat-balance").textContent =
      formatAmount(totalBalance);
    document.getElementById("stat-transactions").textContent =
      transactions.length;

    // Accounts table
    const accBody = document.getElementById("dashboard-accounts");
    accBody.innerHTML = accounts
      .map(
        (a) => `
      <tr>
        <td>${esc(a.name)}</td>
        <td>${esc(a.currency)}</td>
        <td class="amount">${formatAmount(a.balance, a.currency)}</td>
      </tr>`
      )
      .join("");

    // Recent transactions (top 5)
    const txBody = document.getElementById("dashboard-transactions");
    txBody.innerHTML = transactions
      .slice(0, 5)
      .map(
        (t) => `
      <tr>
        <td><span class="badge badge-${t.type.toLowerCase()}">${t.type}</span></td>
        <td class="amount">${formatAmount(t.amount)}</td>
        <td><span class="badge badge-${t.status.toLowerCase()}">${t.status}</span></td>
        <td>${formatDate(t.created_at)}</td>
      </tr>`
      )
      .join("");
  } catch (err) {
    showToast(err.message, "error");
  }
}

// ── Accounts Page ──
async function loadAccounts() {
  try {
    const accounts = await cacheAccounts();
    const tbody = document.getElementById("accounts-table");
    const empty = document.getElementById("accounts-empty");

    if (accounts.length === 0) {
      tbody.innerHTML = "";
      empty.classList.remove("hidden");
      return;
    }

    empty.classList.add("hidden");
    tbody.innerHTML = accounts
      .map(
        (a) => `
      <tr>
        <td class="id-cell" title="${a.id}">${shortID(a.id)}</td>
        <td>${esc(a.name)}</td>
        <td>${esc(a.currency)}</td>
        <td class="amount">${formatAmount(a.balance, a.currency)}</td>
        <td>${formatDate(a.created_at)}</td>
      </tr>`
      )
      .join("");
  } catch (err) {
    showToast(err.message, "error");
  }
}

// Create account
document.getElementById("btn-create-account").addEventListener("click", async () => {
  const btn = document.getElementById("btn-create-account");
  const name = document.getElementById("acc-name").value.trim();
  const currency = document.getElementById("acc-currency").value;

  if (!name) { showToast("Please enter an account name", "error"); return; }
  if (isActionBusy(btn)) return;

  beginActionBusy(btn);
  try {
    await apiPost("/accounts", { name, currency });
    showToast("Account created successfully", "success");
    document.getElementById("acc-name").value = "";
    loadAccounts();
  } catch (err) {
    showToast(err.message, "error");
  } finally {
    endActionBusy(btn);
  }
});

// ── Account Dropdowns ──
async function loadAccountDropdowns() {
  const accounts = await cacheAccounts();
  const selectors = [
    "dep-account",
    "wd-account",
    "tf-from",
    "tf-to",
  ];

  selectors.forEach((id) => {
    const sel = document.getElementById(id);
    if (!sel) return;
    const currentVal = sel.value;
    // Keep first option (placeholder)
    sel.innerHTML = '<option value="">Select account...</option>';
    accounts.forEach((a) => {
      const opt = document.createElement("option");
      opt.value = a.id;
      opt.textContent = a.name + " (" + formatAmount(a.balance, a.currency) + ")";
      sel.appendChild(opt);
    });
    if (currentVal) sel.value = currentVal;
  });
}

// ── Deposit ──
document.getElementById("btn-deposit").addEventListener("click", async (e) => {
  e.preventDefault();
  const btn = document.getElementById("btn-deposit");
  const account_id = document.getElementById("dep-account").value;
  const rawAmount = document.getElementById("dep-amount").value;
  const note = document.getElementById("dep-note").value.trim();

  if (!account_id) { showToast("Please select an account", "error"); return; }
  const amount = Math.round(parseFloat(rawAmount));
  if (!amount || amount <= 0) { showToast("Amount must be positive", "error"); return; }
  if (isActionBusy(btn)) return;

  beginActionBusy(btn);
  try {
    await apiPost("/deposits", {
      account_id, amount, note,
      idempotency_key: generateIdempotencyKey(),
    });
    showToast("Deposit of " + formatAmount(amount) + " successful", "success");
    document.getElementById("dep-amount").value = "";
    document.getElementById("dep-note").value = "";
    await loadAccountDropdowns();
  } catch (err) {
    showToast(err.message, "error");
  } finally {
    endActionBusy(btn);
  }
});

// ── Withdraw ──
document.getElementById("btn-withdraw").addEventListener("click", async (e) => {
  e.preventDefault();
  const btn = document.getElementById("btn-withdraw");
  const account_id = document.getElementById("wd-account").value;
  const rawAmount = document.getElementById("wd-amount").value;
  const note = document.getElementById("wd-note").value.trim();

  if (!account_id) { showToast("Please select an account", "error"); return; }
  const amount = Math.round(parseFloat(rawAmount));
  if (!amount || amount <= 0) { showToast("Amount must be positive", "error"); return; }
  if (isActionBusy(btn)) return;

  beginActionBusy(btn);
  try {
    await apiPost("/withdrawals", {
      account_id, amount, note,
      idempotency_key: generateIdempotencyKey(),
    });
    showToast("Withdrawal of " + formatAmount(amount) + " successful", "success");
    document.getElementById("wd-amount").value = "";
    document.getElementById("wd-note").value = "";
    await loadAccountDropdowns();
  } catch (err) {
    showToast(err.message, "error");
  } finally {
    endActionBusy(btn);
  }
});

// ── Transfer ──
document.getElementById("btn-transfer").addEventListener("click", async (e) => {
  e.preventDefault();
  const btn = document.getElementById("btn-transfer");
  const from_account_id = document.getElementById("tf-from").value;
  const to_account_id = document.getElementById("tf-to").value;
  const rawAmount = document.getElementById("tf-amount").value;
  const note = document.getElementById("tf-note").value.trim();

  if (!from_account_id || !to_account_id) { showToast("Please select both accounts", "error"); return; }
  if (from_account_id === to_account_id) { showToast("Cannot transfer to the same account", "error"); return; }
  const amount = Math.round(parseFloat(rawAmount));
  if (!amount || amount <= 0) { showToast("Amount must be positive", "error"); return; }
  if (isActionBusy(btn)) return;

  beginActionBusy(btn);
  try {
    await apiPost("/transfers", {
      from_account_id, to_account_id, amount, note,
      idempotency_key: generateIdempotencyKey(),
    });
    showToast("Transfer of " + formatAmount(amount) + " successful", "success");
    document.getElementById("tf-amount").value = "";
    document.getElementById("tf-note").value = "";
    await loadAccountDropdowns();
  } catch (err) {
    showToast(err.message, "error");
  } finally {
    endActionBusy(btn);
  }
});

// ── Transactions ──
let allTransactions = [];

async function loadTransactions() {
  try {
    await cacheAccounts();
    allTransactions = await apiGet("/transactions");
    renderTransactions();
  } catch (err) {
    showToast(err.message, "error");
  }
}

function renderTransactions() {
  const typeFilter = document.getElementById("tx-filter-type").value;
  const statusFilter = document.getElementById("tx-filter-status").value;

  let filtered = allTransactions;
  if (typeFilter) filtered = filtered.filter((t) => t.type === typeFilter);
  if (statusFilter)
    filtered = filtered.filter((t) => t.status === statusFilter);

  const tbody = document.getElementById("transactions-table");
  const empty = document.getElementById("transactions-empty");

  if (filtered.length === 0) {
    tbody.innerHTML = "";
    empty.classList.remove("hidden");
    return;
  }

  empty.classList.add("hidden");
  tbody.innerHTML = filtered
    .map(
      (t) => `
    <tr>
      <td class="id-cell" title="${t.id}">${shortID(t.id)}</td>
      <td><span class="badge badge-${t.type.toLowerCase()}">${t.type}</span></td>
      <td class="amount">${formatAmount(t.amount)}</td>
      <td>${t.from_account_id ? esc(getAccountName(t.from_account_id)) : "-"}</td>
      <td>${t.to_account_id ? esc(getAccountName(t.to_account_id)) : "-"}</td>
      <td><span class="badge badge-${t.status.toLowerCase()}">${t.status}</span></td>
      <td>${esc(t.note || "-")}</td>
      <td>${formatDate(t.created_at)}</td>
      <td>
        <button class="btn-icon" onclick="viewTransaction('${t.id}')" title="View details">&#128269;</button>
        ${
          t.status === "SUCCESS" && t.type !== "REVERSAL"
            ? `<button class="btn-icon" onclick="promptReversal('${t.id}')" title="Reverse">&#8634;</button>`
            : ""
        }
      </td>
    </tr>`
    )
    .join("");
}

document
  .getElementById("tx-filter-type")
  .addEventListener("change", renderTransactions);
document
  .getElementById("tx-filter-status")
  .addEventListener("change", renderTransactions);

// ── View Transaction Detail ──
async function viewTransaction(id) {
  try {
    const t = await apiGet("/transactions/" + id);
    const body = document.getElementById("modal-body");
    body.innerHTML = `
      <ul class="detail-list">
        <li><span class="detail-label">Transaction ID</span><span class="detail-value">${t.id}</span></li>
        <li><span class="detail-label">Type</span><span class="detail-value"><span class="badge badge-${t.type.toLowerCase()}">${t.type}</span></span></li>
        <li><span class="detail-label">Status</span><span class="detail-value"><span class="badge badge-${t.status.toLowerCase()}">${t.status}</span></span></li>
        <li><span class="detail-label">Amount</span><span class="detail-value amount">${formatAmount(t.amount)}</span></li>
        <li><span class="detail-label">From</span><span class="detail-value">${t.from_account_id ? esc(getAccountName(t.from_account_id)) + '<br><small>' + t.from_account_id + '</small>' : "-"}</span></li>
        <li><span class="detail-label">To</span><span class="detail-value">${t.to_account_id ? esc(getAccountName(t.to_account_id)) + '<br><small>' + t.to_account_id + '</small>' : "-"}</span></li>
        ${t.reversal_of ? `<li><span class="detail-label">Reversal Of</span><span class="detail-value">${t.reversal_of}</span></li>` : ""}
        ${t.idempotency_key ? `<li><span class="detail-label">Idempotency Key</span><span class="detail-value">${t.idempotency_key}</span></li>` : ""}
        <li><span class="detail-label">Note</span><span class="detail-value">${esc(t.note || "-")}</span></li>
        ${t.error_message ? `<li><span class="detail-label">Error</span><span class="detail-value" style="color:var(--danger)">${esc(t.error_message)}</span></li>` : ""}
        <li><span class="detail-label">Created</span><span class="detail-value">${formatDate(t.created_at)}</span></li>
      </ul>
    `;

    const footer = document.getElementById("modal-footer");
    if (t.status === "SUCCESS" && t.type !== "REVERSAL") {
      footer.innerHTML = `<button class="btn btn-danger btn-sm" onclick="closeModal(); promptReversal('${t.id}')">Reverse This Transaction</button>`;
    } else {
      footer.innerHTML = "";
    }

    document.getElementById("modal-overlay").classList.remove("hidden");
  } catch (err) {
    showToast(err.message, "error");
  }
}
// Make viewTransaction available globally
window.viewTransaction = viewTransaction;

function closeModal() {
  document.getElementById("modal-overlay").classList.add("hidden");
}

document.getElementById("modal-close").addEventListener("click", closeModal);
document.getElementById("modal-overlay").addEventListener("click", (e) => {
  if (e.target === e.currentTarget) closeModal();
});

// ── Reversal ──
let reversalTargetId = null;

function promptReversal(txId) {
  clearActionBusy("reversal-confirm");
  reversalTargetId = txId;
  const tx = allTransactions.find((t) => t.id === txId);
  const details = document.getElementById("reversal-details");
  if (tx) {
    details.textContent =
      tx.type +
      " of " +
      formatAmount(tx.amount) +
      (tx.note ? ' - "' + tx.note + '"' : "");
  }
  document.getElementById("reversal-note").value = "";
  document.getElementById("reversal-overlay").classList.remove("hidden");
}
window.promptReversal = promptReversal;

document.getElementById("reversal-cancel").addEventListener("click", () => {
  document.getElementById("reversal-overlay").classList.add("hidden");
  reversalTargetId = null;
});

document.getElementById("reversal-close").addEventListener("click", () => {
  document.getElementById("reversal-overlay").classList.add("hidden");
  reversalTargetId = null;
});

document.getElementById("reversal-overlay").addEventListener("click", (e) => {
  if (e.target === e.currentTarget) {
    document.getElementById("reversal-overlay").classList.add("hidden");
    reversalTargetId = null;
  }
});

document
  .getElementById("reversal-confirm")
  .addEventListener("click", async () => {
    if (!reversalTargetId) return;

    const btn = document.getElementById("reversal-confirm");
    if (isActionBusy(btn)) return;

    beginActionBusy(btn);
    try {
      const note = document.getElementById("reversal-note").value.trim();
      await apiPost("/reversals", {
        transaction_id: reversalTargetId,
        note,
        idempotency_key: generateIdempotencyKey(),
      });

      showToast("Transaction reversed successfully", "success");
      document.getElementById("reversal-overlay").classList.add("hidden");
      reversalTargetId = null;
      await loadTransactions();
    } catch (err) {
      showToast(err.message, "error");
    } finally {
      endActionBusy(btn);
    }
  });

// ── XSS escape ──
function esc(str) {
  const div = document.createElement("div");
  div.textContent = str;
  return div.innerHTML;
}

// ── Init ──
navigate("dashboard");
