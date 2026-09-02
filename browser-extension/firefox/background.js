"use strict";

const INSTANCE_KEY = "waymarks.browserInstanceId";
const ROOT_KEY = "waymarks.managedRootId";
const STATUS_KEY = "waymarks.nativeStatus";
const MAPPING_KEY = "waymarks.mapping";
const CURSOR_KEY = "waymarks.cursor";
const MAPPING_REVISION_KEY = "waymarks.mappingRevision";
const LOCAL_OPERATIONS_KEY = "waymarks.localOperations";
const LOCAL_PENDING_KEY = "waymarks.localPending";
const REMOTE_PENDING_KEY = "waymarks.remotePending";
const RECONCILE_ACTIVE_KEY = "waymarks.reconcileActive";
const CONFLICT_KEY = "waymarks.conflict";
const CHECK_ALARM = "waymarks.nativeCheck";
let queue = Promise.resolve();
let suppressed = null;

function enqueue(action) {
  queue = queue.then(action, action).catch((error) => setStatusError(error));
  return queue;
}

async function browserInstanceId() {
  const stored = await browser.storage.local.get(INSTANCE_KEY);
  if (typeof stored[INSTANCE_KEY] === "string" && stored[INSTANCE_KEY]) return stored[INSTANCE_KEY];
  const value = `firefox-${crypto.randomUUID()}`;
  await browser.storage.local.set({ [INSTANCE_KEY]: value });
  return value;
}

async function setStatusError(error) {
  await browser.storage.local.set({ [STATUS_KEY]: { ok: false, checked_at: new Date().toISOString(), code: typeof error.code === "string" ? error.code : "native_unavailable" } });
}

async function disableBridge() {
  await browser.storage.local.remove([ROOT_KEY, MAPPING_KEY, CURSOR_KEY, MAPPING_REVISION_KEY, LOCAL_OPERATIONS_KEY, LOCAL_PENDING_KEY, REMOTE_PENDING_KEY, RECONCILE_ACTIVE_KEY]);
}

function normalizedMapping(value) {
  if (!value || typeof value !== "object") return null;
  value.browser_to_canonical = value.browser_to_canonical || {};
  value.canonical_to_browser = value.canonical_to_browser || {};
  value.revisions = value.revisions || {};
  return value;
}

async function loadState() {
  const stored = await browser.storage.local.get([ROOT_KEY, MAPPING_KEY, CURSOR_KEY, LOCAL_OPERATIONS_KEY, LOCAL_PENDING_KEY, REMOTE_PENDING_KEY, RECONCILE_ACTIVE_KEY, CONFLICT_KEY]);
  return {
    rootId: stored[ROOT_KEY],
    mapping: normalizedMapping(stored[MAPPING_KEY]),
    cursor: Number.isInteger(stored[CURSOR_KEY]) ? stored[CURSOR_KEY] : 0,
    localOperations: stored[LOCAL_OPERATIONS_KEY] || {},
    localPending: stored[LOCAL_PENDING_KEY] || null,
    remotePending: stored[REMOTE_PENDING_KEY] || null,
    reconcileActive: stored[RECONCILE_ACTIVE_KEY] || null,
    conflict: stored[CONFLICT_KEY] || null,
  };
}

function supportedNode(node) {
  if (!node || node.type === "separator" || typeof node.title !== "string") return false;
  if (!node.url) return true;
  try {
    const parsed = new URL(node.url);
    return (parsed.protocol === "https:" || parsed.protocol === "http:") && !parsed.username && !parsed.password;
  } catch (_) {
    return false;
  }
}

async function pushBrowserNode(kind, browserId, suppliedNode) {
  const state = await loadState();
  const mapping = state.mapping;
  if (!mapping || !state.rootId || state.reconcileActive) return;
  const canonicalId = mapping.browser_to_canonical[browserId];
  if (kind === "create" && canonicalId) return;
  if (kind !== "create" && !canonicalId) return;
  const node = suppliedNode || (await browser.bookmarks.get(browserId))[0];
  if (kind !== "remove" && !supportedNode(node)) return;
  const parentCanonical = node.parentId ? mapping.browser_to_canonical[String(node.parentId)] : null;
  if (kind !== "remove" && !parentCanonical) return;
  const operationId = `firefox-${crypto.randomUUID()}`;
  const operation = { operation_id: operationId, browser_id: String(browserId), kind };
  if (kind === "remove") {
    operation.node_id = canonicalId;
    operation.base_revision = mapping.revisions[canonicalId];
  } else {
    operation.type = node.url ? "bookmark" : "folder";
    operation.parent_id = parentCanonical;
    operation.title = node.title;
    operation.url = node.url || null;
    operation.position = Number.isInteger(node.index) ? node.index : 0;
    if (kind === "update") {
      operation.node_id = canonicalId;
      operation.base_revision = mapping.revisions[canonicalId];
    }
  }
  if ((kind === "update" || kind === "remove") && !operation.base_revision) throw new Error("Неизвестна canonical revision; выполните reconcile");
  const pending = { operation };
  await browser.storage.local.set({ [LOCAL_PENDING_KEY]: pending });
  await finishLocalPush(state, pending);
}

async function finishLocalPush(state, pending) {
  const mapping = state.mapping;
  const operation = pending.operation;
  const client = WaymarksNative.create(browser.runtime, () => crypto.randomUUID());
  const output = await client.push(await browserInstanceId(), "", "", [operation]);
  const result = output.results && output.results[0];
  if (!result || !result.ok || !result.node) {
    const error = new Error(result && result.error ? result.error.message : "Изменение закладки не сохранено");
    error.code = result && result.error ? result.error.code : "storage_error";
    if (error.code === "conflict") {
      state.localPending = null;
      await browser.storage.local.set({ [CONFLICT_KEY]: { operation, error: result.error }, [LOCAL_PENDING_KEY]: null });
    }
    throw error;
  }
  if (operation.kind === "create") {
    mapping.browser_to_canonical[operation.browser_id] = result.node.id;
    mapping.canonical_to_browser[result.node.id] = operation.browser_id;
  } else if (operation.kind === "remove") {
    delete mapping.browser_to_canonical[operation.browser_id];
    delete mapping.canonical_to_browser[operation.node_id];
  }
  mapping.revisions[result.node.id] = result.node.revision;
  state.localOperations[operation.operation_id] = true;
  state.localPending = null;
  await browser.storage.local.set({ [MAPPING_KEY]: mapping, [LOCAL_OPERATIONS_KEY]: state.localOperations, [LOCAL_PENDING_KEY]: null, [MAPPING_REVISION_KEY]: output.mapping_revision });
}

async function createRemoteNode(node, mapping, pending) {
  const parentBrowserId = mapping.canonical_to_browser[node.parent_id];
  if (!parentBrowserId) throw new Error("Не найден browser parent для remote node");
  let created = null;
  const marker = `__zer0-waymarks-${node.id}`;
  const markerURL = node.type === "bookmark" ? `https://zer0-waymarks.invalid/pending/${node.id}` : null;
  if (pending && pending.node_id === node.id && pending.browser_id) {
    try { created = (await browser.bookmarks.get(pending.browser_id))[0]; } catch (_) { created = null; }
  }
  if (pending && pending.node_id === node.id && !created) {
    const matches = (await browser.bookmarks.getChildren(parentBrowserId)).filter((item) => item.title === marker);
    if (matches.length > 1) throw new Error("Неоднозначное восстановление remote create");
    if (matches.length === 1) created = matches[0];
  }
  if (!created) {
    await browser.storage.local.set({ [REMOTE_PENDING_KEY]: { kind: "create", node_id: node.id } });
    suppressed = { kind: "create", parentId: String(parentBrowserId), title: marker, url: markerURL };
    try {
      const details = { parentId: parentBrowserId, title: marker, index: node.position };
      if (markerURL) details.url = markerURL;
      created = await browser.bookmarks.create(details);
    } finally {
      suppressed = null;
    }
    await browser.storage.local.set({ [REMOTE_PENDING_KEY]: { kind: "create", node_id: node.id, browser_id: String(created.id) } });
  }
  suppressed = { kind: "update", browserId: String(created.id) };
  try {
    const changes = { title: node.title };
    if (node.type === "bookmark") changes.url = node.url;
    await browser.bookmarks.update(created.id, changes);
  } finally {
    suppressed = null;
  }
  mapping.canonical_to_browser[node.id] = String(created.id);
  mapping.browser_to_canonical[String(created.id)] = node.id;
}

async function applyRemoteEvent(event, state) {
  const node = event.payload && event.payload.node;
  if (event.kind && event.kind.startsWith("conflict.")) {
    if (node) state.mapping.revisions[node.id] = node.revision;
    state.conflict = { event };
    await browser.storage.local.set({ [CONFLICT_KEY]: state.conflict, [MAPPING_KEY]: state.mapping });
    return;
  }
  if (!node || event.kind === "root.created") return;
  const mapping = state.mapping;
  if (state.localOperations[event.operation_id]) {
    mapping.revisions[node.id] = node.revision;
    delete state.localOperations[event.operation_id];
    return;
  }
  let browserId = mapping.canonical_to_browser[node.id];
  if (event.kind === "node.created" && !browserId) {
    await createRemoteNode(node, mapping, state.remotePending);
    browserId = mapping.canonical_to_browser[node.id];
  } else if (event.kind === "node.updated" && browserId) {
    const current = (await browser.bookmarks.get(browserId))[0];
    const parentId = mapping.canonical_to_browser[node.parent_id];
    suppressed = { kind: "update", browserId: String(browserId) };
    try {
      if (current.title !== node.title || (current.url || null) !== (node.url || null)) {
        const changes = { title: node.title };
        if (node.type === "bookmark") changes.url = node.url;
        await browser.bookmarks.update(browserId, changes);
      }
      if (parentId && (String(current.parentId) !== String(parentId) || current.index !== node.position)) await browser.bookmarks.move(browserId, { parentId, index: node.position });
    } finally {
      suppressed = null;
    }
  } else if (event.kind === "node.removed" && browserId) {
    suppressed = { kind: "remove", browserId: String(browserId) };
    try {
      await browser.bookmarks.remove(browserId);
    } catch (error) {
      if (!String(error.message || "").toLowerCase().includes("not found")) throw error;
    } finally {
      suppressed = null;
    }
    delete mapping.browser_to_canonical[browserId];
    delete mapping.canonical_to_browser[node.id];
  }
  mapping.revisions[node.id] = node.revision;
  state.remotePending = null;
  await browser.storage.local.set({ [MAPPING_KEY]: mapping, [LOCAL_OPERATIONS_KEY]: state.localOperations, [REMOTE_PENDING_KEY]: null });
}

async function sync() {
  const instanceId = await browserInstanceId();
  const client = WaymarksNative.create(browser.runtime, () => crypto.randomUUID());
  const state = await loadState();
  if (!state.mapping || !state.rootId) {
    const page = await client.pull(instanceId, 0, 1);
    await browser.storage.local.set({ [STATUS_KEY]: { ok: true, checked_at: new Date().toISOString(), store_sequence: page.through_sequence } });
    return;
  }
  if (state.reconcileActive) {
    const expiresAt = typeof state.reconcileActive === "object" ? Date.parse(state.reconcileActive.expires_at) : 0;
    if (expiresAt > Date.now()) return;
    await browser.storage.local.remove(RECONCILE_ACTIVE_KEY);
    state.reconcileActive = null;
  }
  if (state.localPending) await finishLocalPush(state, state.localPending);
  let hasMore = true;
  while (hasMore) {
    const page = await client.pull(instanceId, state.cursor, 100);
    for (const event of page.events || []) await applyRemoteEvent(event, state);
    state.cursor = page.through_sequence;
    hasMore = page.has_more;
    await browser.storage.local.set({ [CURSOR_KEY]: state.cursor, [MAPPING_KEY]: state.mapping, [LOCAL_OPERATIONS_KEY]: state.localOperations });
  }
  await scanUnmapped(state);
  await browser.storage.local.set({ [STATUS_KEY]: { ok: true, checked_at: new Date().toISOString(), store_sequence: state.cursor, conflict: Boolean(state.conflict) } });
}

async function scanUnmapped(state) {
  let subtree;
  try { subtree = (await browser.bookmarks.getSubTree(state.rootId))[0]; } catch (_) { await disableBridge(); return; }
  const nodes = [];
  function visit(node) {
    if (String(node.id) !== String(state.rootId)) nodes.push(node);
    for (const child of Array.isArray(node.children) ? node.children : []) visit(child);
  }
  visit(subtree);
  for (const node of nodes) {
    if (!state.mapping.browser_to_canonical[String(node.id)] && state.mapping.browser_to_canonical[String(node.parentId)] && supportedNode(node)) {
      await pushBrowserNode("create", String(node.id), node);
      state.mapping = (await loadState()).mapping;
    }
  }
}

browser.bookmarks.onCreated.addListener((id, node) => {
  if (suppressed && suppressed.kind === "create" && String(node.parentId) === suppressed.parentId && node.title === suppressed.title && (node.url || null) === suppressed.url) return;
  void enqueue(() => pushBrowserNode("create", String(id), node));
});
browser.bookmarks.onChanged.addListener((id) => {
  if (suppressed && suppressed.kind === "update" && String(id) === suppressed.browserId) return;
  void enqueue(async () => {
    const state = await loadState();
    if (String(id) === String(state.rootId)) return disableBridge();
    return pushBrowserNode("update", String(id));
  });
});
browser.bookmarks.onMoved.addListener((id, moveInfo) => {
  if (suppressed && suppressed.kind === "update" && String(id) === suppressed.browserId) return;
  void enqueue(async () => {
    const state = await loadState();
    if (String(id) === String(state.rootId)) return disableBridge();
    if (!state.mapping) return;
    const mapped = Boolean(state.mapping.browser_to_canonical[String(id)]);
    const parentMapped = Boolean(state.mapping.browser_to_canonical[String(moveInfo.parentId)]);
    const node = (await browser.bookmarks.get(String(id)))[0];
    if (mapped && !parentMapped) {
      let root = node;
      try { root = (await browser.bookmarks.getSubTree(String(id)))[0]; } catch (_) { /* leaf or already unavailable */ }
      const removed = [];
      function postorder(item) {
        for (const child of item && Array.isArray(item.children) ? item.children : []) postorder(child);
        if (item) removed.push(item);
      }
      postorder(root);
      for (const item of removed) await pushBrowserNode("remove", String(item.id), item);
      return;
    }
    if (!mapped && parentMapped) return pushBrowserNode("create", String(id), node);
    if (mapped && parentMapped) return pushBrowserNode("update", String(id), node);
  });
});
browser.bookmarks.onRemoved.addListener((id, info) => {
  if (suppressed && suppressed.kind === "remove" && String(id) === suppressed.browserId) return;
  void enqueue(async () => {
    const state = await loadState();
    if (String(id) === String(state.rootId)) {
      await disableBridge();
      return;
    }
    const nodes = [];
    function postorder(node) {
      for (const child of node && Array.isArray(node.children) ? node.children : []) postorder(child);
      if (node) nodes.push(node);
    }
    postorder(info.node);
    for (const node of nodes) await pushBrowserNode("remove", String(node.id), node);
  });
});

browser.runtime.onInstalled.addListener(() => {
  browser.alarms.create(CHECK_ALARM, { periodInMinutes: 1 });
  void enqueue(sync);
});
browser.runtime.onStartup.addListener(() => void enqueue(sync));
browser.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === CHECK_ALARM) void enqueue(sync);
});
void enqueue(sync);
