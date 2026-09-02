(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  root.WaymarksBookmarkTree = api;
})(typeof globalThis === "object" ? globalThis : this, function () {
  "use strict";

  const MAX_NODES = 5000;
  const MAX_DEPTH = 32;

  function nodeType(node) {
    if (node.type === "folder" || Array.isArray(node.children)) return "folder";
    if (node.type === "separator") return "separator";
    return "bookmark";
  }

  function discoverSearch(results) {
    if (!Array.isArray(results)) throw new Error("invalid Firefox bookmark search result");
    return results
      .filter((node) => nodeType(node) === "folder" && node.title === "Waymarks")
      .map((node) => ({ id: String(node.id), title: node.title, path: `Waymarks (${node.id})` }))
      .sort((left, right) => left.id.localeCompare(right.id));
  }

  function snapshot(rootNode) {
    if (!rootNode || nodeType(rootNode) !== "folder") throw new Error("managed root is not a folder");
    const nodes = [];
    let unsupported = 0;
    walk(rootNode, 0, { count: 0 }, (node) => {
      const type = nodeType(node);
      const title = typeof node.title === "string" ? node.title : "";
      if (!isAllowedTitle(title)) {
        if (type === "folder") throw new Error("folder title is invalid");
        unsupported += 1;
        return;
      }
      if (type === "separator" || (type === "bookmark" && !isAllowedURL(node.url))) {
        unsupported += 1;
        return;
      }
      nodes.push({
        browser_id: String(node.id),
        parent_browser_id: node.id === rootNode.id ? null : String(node.parentId),
        type,
        title,
        url: type === "bookmark" ? node.url : null,
        position: Number.isInteger(node.index) ? node.index : 0,
      });
    });
    nodes.sort((left, right) => left.browser_id.localeCompare(right.browser_id));
    return { root_browser_id: String(rootNode.id), nodes, unsupported };
  }

  function walk(node, depth, state, visit) {
    if (depth > MAX_DEPTH) throw new Error("bookmark tree depth limit exceeded");
    state.count += 1;
    if (state.count > MAX_NODES) throw new Error("bookmark tree node limit exceeded");
    visit(node);
    const children = Array.isArray(node.children) ? node.children : [];
    for (const child of children) {
      walk(child, depth + 1, state, visit);
    }
  }

  function isAllowedURL(value) {
    if (typeof value !== "string" || utf8Length(value) > 8192 || /[\u0000-\u001f\u007f]/u.test(value)) return false;
    try {
      const parsed = new URL(value);
      return (parsed.protocol === "https:" || parsed.protocol === "http:") && Boolean(parsed.hostname) && !parsed.username && !parsed.password;
    } catch (_) {
      return false;
    }
  }

  function isAllowedTitle(value) {
    return typeof value === "string" && utf8Length(value) <= 1024 && !value.includes("\0");
  }

  function utf8Length(value) {
    return new TextEncoder().encode(value).length;
  }

  async function fingerprint(snapshotValue, subtle) {
	if (!subtle || typeof subtle.digest !== "function") throw new TypeError("SubtleCrypto is required");
	const normalized = {
	  root_browser_id: snapshotValue.root_browser_id,
	  nodes: snapshotValue.nodes.slice().sort((left, right) => compareUTF8(left.browser_id, right.browser_id)),
	  unsupported: snapshotValue.unsupported,
	};
	const canonical = JSON.stringify(normalized).replace(/[<>&\u2028\u2029]/gu, (character) => `\\u${character.charCodeAt(0).toString(16).padStart(4, "0")}`);
	const digest = await subtle.digest("SHA-256", new TextEncoder().encode(canonical));
	return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("");
  }

  function parentFirst(snapshotValue) {
	const byID = new Map(snapshotValue.nodes.map((node) => [node.browser_id, node]));
	const depths = new Map([[snapshotValue.root_browser_id, 0]]);
	function depth(node, seen) {
	  if (depths.has(node.browser_id)) return depths.get(node.browser_id);
	  if (!node.parent_browser_id || seen.has(node.browser_id)) throw new Error("invalid bookmark parent graph");
	  const parent = byID.get(node.parent_browser_id);
	  if (!parent) throw new Error("bookmark parent is missing");
	  const nextSeen = new Set(seen);
	  nextSeen.add(node.browser_id);
	  const value = depth(parent, nextSeen) + 1;
	  depths.set(node.browser_id, value);
	  return value;
	}
	return snapshotValue.nodes
	  .filter((node) => node.browser_id !== snapshotValue.root_browser_id)
	  .slice()
	  .sort((left, right) => depth(left, new Set()) - depth(right, new Set()) || left.position - right.position || compareUTF8(left.browser_id, right.browser_id));
  }

  async function operationID(previewToken, browserID, subtle) {
	const digest = await subtle.digest("SHA-256", new TextEncoder().encode(browserID));
	const suffix = Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("").slice(0, 32);
	return `reconcile-${previewToken.slice(0, 16)}-${suffix}`;
  }

  function compareUTF8(left, right) {
	const leftBytes = new TextEncoder().encode(left);
	const rightBytes = new TextEncoder().encode(right);
	const length = Math.min(leftBytes.length, rightBytes.length);
	for (let index = 0; index < length; index += 1) {
	  if (leftBytes[index] !== rightBytes[index]) return leftBytes[index] - rightBytes[index];
	}
	return leftBytes.length - rightBytes.length;
  }

  return { MAX_NODES, MAX_DEPTH, discoverSearch, snapshot, fingerprint, parentFirst, operationID };
});
