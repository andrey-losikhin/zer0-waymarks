"use strict";

const assert = require("node:assert/strict");
const test = require("node:test");
const { webcrypto } = require("node:crypto");
const tree = require("./bookmark-tree.js");

test("snapshot is deterministic and reports unsupported entries", () => {
  const managed = { id: "m", title: "Waymarks", type: "folder", children: [
    { id: "b", parentId: "m", index: 1, title: "Bad", type: "bookmark", url: "javascript:alert(1)" },
    { id: "a", parentId: "m", index: 0, title: "Example", type: "bookmark", url: "https://example.test/docs" },
    { id: "s", parentId: "m", index: 2, title: "", type: "separator" },
  ] };
  assert.deepEqual(tree.snapshot(managed), {
    root_browser_id: "m",
    unsupported: 2,
    nodes: [
      { browser_id: "a", parent_browser_id: "m", type: "bookmark", title: "Example", url: "https://example.test/docs", position: 0 },
      { browser_id: "m", parent_browser_id: null, type: "folder", title: "Waymarks", url: null, position: 0 },
    ],
  });
});

test("search discovery keeps folders only and sorts IDs", () => {
  assert.deepEqual(tree.discoverSearch([
    { id: "z", title: "Waymarks", type: "folder" },
    { id: "bookmark", title: "Waymarks", type: "bookmark", url: "https://example.test" },
    { id: "a", title: "Waymarks", type: "folder" },
  ]), [
    { id: "a", title: "Waymarks", path: "Waymarks (a)" },
    { id: "z", title: "Waymarks", path: "Waymarks (z)" },
  ]);
});

test("snapshot rejects raw unsafe values before URL normalization", () => {
  const managed = { id: "m", title: "Waymarks", type: "folder", children: [
    { id: "control", parentId: "m", title: "Control", type: "bookmark", url: "https://example.test/\n" },
    { id: "userinfo", parentId: "m", title: "User", type: "bookmark", url: "https://user@example.test/" },
    { id: "title", parentId: "m", title: "я".repeat(513), type: "bookmark", url: "https://example.test/" },
    { id: "http", parentId: "m", title: "HTTP", type: "bookmark", url: "http://example.test/" },
  ] };
  const result = tree.snapshot(managed);
  assert.equal(result.unsupported, 3);
  assert.equal(result.nodes.some((node) => node.browser_id === "http"), true);
});

test("snapshot enforces total node and depth limits", () => {
  const wide = { id: "root", title: "Waymarks", type: "folder", children: [] };
  for (let index = 0; index < tree.MAX_NODES; index += 1) {
    wide.children.push({ id: `b-${index}`, parentId: "root", title: "Item", type: "bookmark", url: "https://example.test/" });
  }
  assert.throws(() => tree.snapshot(wide), /node limit/);

  const deep = { id: "root", title: "Waymarks", type: "folder", children: [] };
  let current = deep;
  for (let depth = 0; depth <= tree.MAX_DEPTH; depth += 1) {
    const child = { id: `f-${depth}`, parentId: current.id, title: "Folder", type: "folder", children: [] };
    current.children.push(child);
    current = child;
  }
  assert.throws(() => tree.snapshot(deep), /depth limit/);
});

test("fingerprint is stable across node order", async () => {
	const left = { root_browser_id: "root", nodes: [
	  { browser_id: "b", parent_browser_id: "root", type: "folder", title: "B", url: null, position: 1 },
	  { browser_id: "a", parent_browser_id: null, type: "folder", title: "Waymarks", url: null, position: 0 },
	], unsupported: 0 };
	const right = { ...left, nodes: left.nodes.slice().reverse() };
	assert.equal(await tree.fingerprint(left, webcrypto.subtle), await tree.fingerprint(right, webcrypto.subtle));
});

test("fingerprint matches the Go bridge fixture", async () => {
	const fixture = { root_browser_id: "root", nodes: [
	  { browser_id: "bookmark", parent_browser_id: "root", type: "bookmark", title: "Example", url: "https://example.test/", position: 1 },
	  { browser_id: "root", parent_browser_id: null, type: "folder", title: "Waymarks", url: null, position: 0 },
	], unsupported: 0 };
	assert.equal(await tree.fingerprint(fixture, webcrypto.subtle), "60830cd81ceabb8327e91fe07937506cc4c98894ddcf05628adbe0af99d304f3");
});

test("fingerprint matches Go escaping for HTML-sensitive Unicode", async () => {
	const fixture = { root_browser_id: "root", nodes: [
	  { browser_id: "bookmark", parent_browser_id: "root", type: "bookmark", title: "<A>\u2028", url: "https://example.test/?a=1&b=2", position: 1 },
	  { browser_id: "root", parent_browser_id: null, type: "folder", title: "Waymarks", url: null, position: 0 },
	], unsupported: 0 };
	assert.equal(await tree.fingerprint(fixture, webcrypto.subtle), "27c9aa1e69f0f149ce1a69bc146c7228ca6e13dd08e15d8d4254bce275bb9b89");
});

test("parentFirst orders folders before descendants", () => {
	const snapshot = { root_browser_id: "root", nodes: [
	  { browser_id: "child", parent_browser_id: "folder", type: "bookmark", title: "Child", url: "https://example.test", position: 0 },
	  { browser_id: "root", parent_browser_id: null, type: "folder", title: "Waymarks", url: null, position: 0 },
	  { browser_id: "folder", parent_browser_id: "root", type: "folder", title: "Folder", url: null, position: 1 },
	], unsupported: 0 };
	assert.deepEqual(tree.parentFirst(snapshot).map((node) => node.browser_id), ["folder", "child"]);
});

test("operationID is deterministic and bounded", async () => {
	const first = await tree.operationID("a".repeat(64), "browser-id", webcrypto.subtle);
	const second = await tree.operationID("a".repeat(64), "browser-id", webcrypto.subtle);
	assert.equal(first, second);
	assert.equal(first.length <= 128, true);
});
