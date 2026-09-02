"use strict";

const ROOT_KEY = "waymarks.managedRootId";
const STATUS_KEY = "waymarks.nativeStatus";
const INSTANCE_KEY = "waymarks.browserInstanceId";
const SESSION_KEY = "waymarks.reconcileSession";
const nativeStatus = document.querySelector("#native-status");
const folderStatus = document.querySelector("#folder-status");
const candidatesElement = document.querySelector("#candidates");
const createButton = document.querySelector("#create-folder");
const resetButton = document.querySelector("#reset-folder");
const previewButton = document.querySelector("#preview");
const previewReport = document.querySelector("#preview-report");
const applyButton = document.querySelector("#apply");
let selectedSnapshot = null;
let selectedRootId = null;
let latestPreview = null;

async function refresh() {
  candidatesElement.replaceChildren();
  createButton.hidden = true;
  resetButton.hidden = true;
	previewButton.hidden = true;
	previewReport.hidden = true;
	applyButton.hidden = true;
	latestPreview = null;
	selectedSnapshot = null;
	selectedRootId = null;
  const stored = await browser.storage.local.get([ROOT_KEY, STATUS_KEY, SESSION_KEY]);
  renderNativeStatus(stored[STATUS_KEY]);
  const rootId = stored[ROOT_KEY];
  if (typeof rootId === "string" && rootId) {
    resetButton.hidden = false;
    try {
      const subtree = await browser.bookmarks.getSubTree(rootId);
      const snapshot = WaymarksBookmarkTree.snapshot(subtree[0]);
      folderStatus.textContent = `Выбрана папка: ${snapshot.nodes.length - 1} поддерживаемых записей, пропущено: ${snapshot.unsupported}.`;
      folderStatus.className = "ok";
	  selectedSnapshot = snapshot;
	  selectedRootId = rootId;
	  previewButton.hidden = false;
	  const session = stored[SESSION_KEY];
	  if (session && session.snapshot && session.snapshot.root_browser_id === rootId && Date.parse(session.expires_at) > Date.now()) {
		latestPreview = session;
		applyButton.hidden = false;
		previewReport.textContent = "Найден незавершённый reconcile; можно продолжить.";
		previewReport.hidden = false;
	  }
      return;
    } catch (error) {
      folderStatus.textContent = `Выбранная папка недоступна: ${error.message || "ошибка Firefox API"}.`;
      folderStatus.className = "error";
      return;
    }
  }
  const candidates = WaymarksBookmarkTree.discoverSearch(await browser.bookmarks.search({ title: "Waymarks" }));
  if (candidates.length === 0) {
    folderStatus.textContent = "Папка Waymarks не найдена. Создание выполняется только по кнопке.";
    folderStatus.className = "";
    createButton.hidden = false;
    return;
  }
  folderStatus.textContent = candidates.length === 1 ? "Найдена папка. Подтвердите выбор." : "Найдено несколько папок. Выберите управляемую явно.";
  for (const candidate of candidates) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "candidate";
    button.textContent = candidate.path || candidate.title;
    button.addEventListener("click", async () => {
      await browser.storage.local.set({ [ROOT_KEY]: candidate.id });
      await refresh();
    });
    candidatesElement.append(button);
  }
}

function renderNativeStatus(status) {
  if (!status) {
    nativeStatus.textContent = "Native Messaging ещё не проверен.";
    nativeStatus.className = "";
  } else if (status.ok) {
    nativeStatus.textContent = status.conflict
      ? `Native Messaging подключён, но требуется reconcile конфликта; store sequence: ${status.store_sequence}.`
      : `Native Messaging подключён, store sequence: ${status.store_sequence}.`;
    nativeStatus.className = status.conflict ? "error" : "ok";
  } else {
    nativeStatus.textContent = `Native Messaging недоступен (${status.code}).`;
    nativeStatus.className = "error";
  }
}

createButton.addEventListener("click", async () => {
  createButton.disabled = true;
  try {
    const created = await browser.bookmarks.create({ title: "Waymarks" });
    await browser.storage.local.set({ [ROOT_KEY]: created.id });
    await refresh();
  } finally {
    createButton.disabled = false;
  }
});
document.querySelector("#refresh").addEventListener("click", () => void refresh());
resetButton.addEventListener("click", async () => {
  await browser.storage.local.remove([ROOT_KEY, SESSION_KEY, "waymarks.reconcileActive", "waymarks.mapping", "waymarks.cursor", "waymarks.mappingRevision", "waymarks.localOperations", "waymarks.localPending", "waymarks.remotePending", "waymarks.conflict"]);
  await refresh();
});
previewButton.addEventListener("click", async () => {
	if (!selectedSnapshot || !selectedRootId) return;
	previewButton.disabled = true;
	latestPreview = null;
	applyButton.hidden = true;
	try {
	  const stored = await browser.storage.local.get(INSTANCE_KEY);
	  let instanceId = stored[INSTANCE_KEY];
	  if (typeof instanceId !== "string" || !instanceId) {
		instanceId = `firefox-${crypto.randomUUID()}`;
		await browser.storage.local.set({ [INSTANCE_KEY]: instanceId });
	  }
	  const subtree = await browser.bookmarks.getSubTree(selectedRootId);
	  const currentSnapshot = WaymarksBookmarkTree.snapshot(subtree[0]);
	  const fingerprint = await WaymarksBookmarkTree.fingerprint(currentSnapshot, crypto.subtle);
	  const client = WaymarksNative.create(browser.runtime, () => crypto.randomUUID());
	  const preview = await client.preview(instanceId, fingerprint, currentSnapshot);
	  previewReport.textContent = [
		`Создать в Firefox: ${preview.report.create_in_browser}`,
		`Только в Firefox: ${preview.report.browser_only}`,
		`Пропущено неподдерживаемых: ${preview.report.unsupported}`,
		`Store sequence: ${preview.store_sequence}`,
		`Preview действует до: ${preview.expires_at}`,
	  ].join("\n");
	  previewReport.hidden = false;
	  latestPreview = { ...preview, instanceId, snapshot: currentSnapshot, fingerprint };
	  await browser.storage.local.set({ [SESSION_KEY]: latestPreview });
	  applyButton.hidden = false;
	} catch (error) {
	  previewReport.textContent = `Dry-run не выполнен: ${error.message || "ошибка"}`;
	  previewReport.hidden = false;
	} finally {
	  previewButton.disabled = false;
	}
});
applyButton.addEventListener("click", async () => {
	if (!latestPreview || !selectedRootId) return;
	applyButton.disabled = true;
	previewButton.disabled = true;
	try {
	  await browser.storage.local.set({ "waymarks.reconcileActive": { token: latestPreview.preview_token, expires_at: latestPreview.expires_at } });
	  const client = WaymarksNative.create(browser.runtime, () => crypto.randomUUID());
	  const progressKey = `waymarks.reconcile.${latestPreview.preview_token}`;
	  const storedProgress = await browser.storage.local.get(progressKey);
	  const hadProgress = Boolean(storedProgress[progressKey]);
	  const progress = storedProgress[progressKey] || { browser_to_canonical: {}, canonical_to_browser: {}, revisions: {}, pending_canonical: {} };
	  progress.pending_canonical = progress.pending_canonical || {};
	  progress.revisions = progress.revisions || {};
	  for (const mapping of latestPreview.mappings || []) {
		progress.browser_to_canonical[mapping.browser_id] = mapping.canonical_id;
		progress.canonical_to_browser[mapping.canonical_id] = mapping.browser_id;
		if (mapping.revision) progress.revisions[mapping.canonical_id] = mapping.revision;
	  }
	  progress.browser_to_canonical[latestPreview.snapshot.root_browser_id] = latestPreview.canonical_root_id;
	  progress.canonical_to_browser[latestPreview.canonical_root_id] = latestPreview.snapshot.root_browser_id;
	  await browser.storage.local.set({ [progressKey]: progress });
	  if (!hadProgress) {
		const currentTree = await browser.bookmarks.getSubTree(selectedRootId);
		const currentSnapshot = WaymarksBookmarkTree.snapshot(currentTree[0]);
		const currentFingerprint = await WaymarksBookmarkTree.fingerprint(currentSnapshot, crypto.subtle);
		if (currentFingerprint !== latestPreview.fingerprint) throw new Error("Папка изменилась после dry-run; обновите preview");
	  }

	  for (const node of WaymarksBookmarkTree.parentFirst(latestPreview.snapshot)) {
		if (progress.browser_to_canonical[node.browser_id]) continue;
		const parentCanonical = progress.browser_to_canonical[node.parent_browser_id];
		if (!parentCanonical) throw new Error("Не найден canonical parent при импорте");
		const operationId = await WaymarksBookmarkTree.operationID(latestPreview.preview_token, node.browser_id, crypto.subtle);
		const pushed = await client.push(latestPreview.instanceId, latestPreview.preview_token, latestPreview.fingerprint, [{
		  operation_id: operationId,
		  browser_id: node.browser_id,
		  kind: "create",
		  type: node.type,
		  parent_id: parentCanonical,
		  title: node.title,
		  url: node.url,
		  position: node.position,
		}]);
		const result = pushed.results[0];
		if (!result || !result.ok || !result.node) throw new Error(result && result.error ? result.error.message : "Импорт не выполнен");
		progress.browser_to_canonical[node.browser_id] = result.node.id;
		progress.canonical_to_browser[result.node.id] = node.browser_id;
		progress.revisions[result.node.id] = result.node.revision;
		await browser.storage.local.set({ [progressKey]: progress });
	  }

	  let offset = 0;
	  let hasMore = true;
	  while (hasMore) {
		const prepared = await client.apply("prepare", latestPreview.instanceId, latestPreview.preview_token, latestPreview.fingerprint, latestPreview.snapshot, [], offset, 50);
		for (const node of prepared.nodes) {
		  progress.revisions[node.id] = node.revision;
		  if (progress.canonical_to_browser[node.id]) {
			try {
			  await browser.bookmarks.get(progress.canonical_to_browser[node.id]);
			  continue;
			} catch (_) {
			  delete progress.browser_to_canonical[progress.canonical_to_browser[node.id]];
			  delete progress.canonical_to_browser[node.id];
			}
		  }
		const parentBrowser = progress.canonical_to_browser[node.parent_id];
		if (!parentBrowser) throw new Error("Не найден browser parent при применении");
		const marker = `__zer0-waymarks-${node.id}`;
		const markerURL = node.type === "bookmark" ? `https://zer0-waymarks.invalid/pending/${node.id}` : null;
		const pending = progress.pending_canonical[node.id];
		progress.pending_canonical[node.id] = { parent_browser_id: parentBrowser, marker, browser_id: pending && pending.browser_id };
		await browser.storage.local.set({ [progressKey]: progress });
		let created = null;
		if (pending && pending.browser_id) {
		  try { created = (await browser.bookmarks.get(pending.browser_id))[0]; } catch (_) { created = null; }
		}
		if (!created && pending) {
		  const matches = (await browser.bookmarks.getChildren(parentBrowser)).filter((candidate) => candidate.title === marker);
		  if (matches.length > 1) throw new Error("Неоднозначное восстановление созданной закладки");
		  if (matches.length === 1) created = matches[0];
		}
		if (!created) {
		  const create = { parentId: parentBrowser, title: marker, index: node.position };
		  if (markerURL) create.url = markerURL;
		  created = await browser.bookmarks.create(create);
		  progress.pending_canonical[node.id].browser_id = created.id;
		  await browser.storage.local.set({ [progressKey]: progress });
		}
		const changes = { title: node.title };
		if (node.type === "bookmark") changes.url = node.url;
		await browser.bookmarks.update(created.id, changes);
		progress.canonical_to_browser[node.id] = created.id;
		progress.browser_to_canonical[created.id] = node.id;
		delete progress.pending_canonical[node.id];
		await browser.storage.local.set({ [progressKey]: progress });
		}
		offset = prepared.next_offset;
		hasMore = prepared.has_more;
	  }

	  const finalTree = await browser.bookmarks.getSubTree(selectedRootId);
	  const finalSnapshot = WaymarksBookmarkTree.snapshot(finalTree[0]);
	  const finalFingerprint = await WaymarksBookmarkTree.fingerprint(finalSnapshot, crypto.subtle);
	  const mappings = Object.entries(progress.canonical_to_browser).map(([canonical_id, browser_id]) => ({ canonical_id, browser_id }));
	  const committed = await client.apply("commit", latestPreview.instanceId, latestPreview.preview_token, finalFingerprint, finalSnapshot, mappings, 0, 0);
	  await browser.storage.local.set({
		"waymarks.mapping": progress,
		"waymarks.cursor": committed.store_sequence,
		"waymarks.mappingRevision": committed.mapping_revision,
	  });
	  await browser.storage.local.remove(progressKey);
	  await browser.storage.local.remove(SESSION_KEY);
	  await browser.storage.local.remove("waymarks.conflict");
	  previewReport.textContent = `Reconcile завершён. Store sequence: ${committed.store_sequence}.`;
	  latestPreview = null;
	  applyButton.hidden = true;
	} catch (error) {
	  previewReport.textContent = `Reconcile не завершён: ${error.message || "ошибка"}`;
	  previewReport.hidden = false;
	} finally {
	  await browser.storage.local.remove("waymarks.reconcileActive");
	  applyButton.disabled = false;
	  previewButton.disabled = false;
	}
});
void refresh();
