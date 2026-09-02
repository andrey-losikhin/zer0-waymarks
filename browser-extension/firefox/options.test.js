"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const test = require("node:test");
const vm = require("node:vm");

function element() {
  const listeners = {};
  return {
    hidden: false, disabled: false, textContent: "", className: "", children: [],
    addEventListener(name, listener) { listeners[name] = listener; },
    fire(name) { return listeners[name](); },
    replaceChildren() { this.children = []; },
    append(value) { this.children.push(value); },
  };
}

async function settle() {
  for (let index = 0; index < 12; index += 1) await new Promise((resolve) => setImmediate(resolve));
}

test("unfinished root-only reconcile resumes and commits mapping", async () => {
  const elements = Object.fromEntries(["native-status", "folder-status", "candidates", "create-folder", "reset-folder", "preview", "preview-report", "apply", "refresh"].map((id) => [`#${id}`, element()]));
  const snapshot = { root_browser_id: "browser-root", nodes: [{ browser_id: "browser-root", parent_browser_id: null, type: "folder", title: "Waymarks", url: null, position: 0 }], unsupported: 0 };
  const expiresAt = new Date(Date.now() + 60_000).toISOString();
  const storage = {
    "waymarks.browserInstanceId": "firefox-test",
    "waymarks.managedRootId": "browser-root",
    "waymarks.reconcileSession": {
      preview_token: "preview-token",
      expires_at: expiresAt,
      instanceId: "firefox-test",
      fingerprint: "fingerprint",
      snapshot,
      canonical_root_id: "canonical-root",
      mappings: [{ browser_id: "browser-root", canonical_id: "canonical-root", revision: 1 }],
    },
  };
  const applyCalls = [];
  const browser = {
    storage: { local: {
      async get(keys) {
        const names = Array.isArray(keys) ? keys : [keys];
        return Object.fromEntries(names.filter((key) => Object.hasOwn(storage, key)).map((key) => [key, storage[key]]));
      },
      async set(values) { Object.assign(storage, values); },
      async remove(keys) { for (const key of Array.isArray(keys) ? keys : [keys]) delete storage[key]; },
    } },
    bookmarks: {
      async getSubTree() { return [{ id: "browser-root", title: "Waymarks", children: [] }]; },
      async search() { return []; },
      async get() { throw new Error("unexpected get"); },
    },
    runtime: {},
  };
  const client = {
    async apply(...args) {
      applyCalls.push(args);
      if (args[0] === "prepare") return { nodes: [], next_offset: 0, has_more: false };
      return { store_sequence: 9, mapping_revision: 2 };
    },
    async push() { throw new Error("unexpected push"); },
  };
  const context = {
    browser,
    document: {
      querySelector(selector) { return elements[selector]; },
      createElement() { return element(); },
    },
    WaymarksBookmarkTree: {
      snapshot() { return snapshot; },
      async fingerprint() { return "fingerprint"; },
      parentFirst() { return []; },
    },
    WaymarksNative: { create: () => client },
    crypto: { randomUUID: () => "uuid", subtle: {} },
    Date,
    Promise,
    Error,
    Object,
  };
  vm.runInNewContext(fs.readFileSync(__dirname + "/options.js", "utf8"), context, { filename: "options.js" });
  await settle();
  assert.equal(elements["#apply"].hidden, false);
  await elements["#apply"].fire("click");
  await settle();
  assert.deepEqual(applyCalls.map((call) => call[0]), ["prepare", "commit"]);
  assert.equal(storage["waymarks.cursor"], 9);
  assert.equal(storage["waymarks.mappingRevision"], 2);
  assert.equal(storage["waymarks.mapping"].canonical_to_browser["canonical-root"], "browser-root");
  assert.equal(storage["waymarks.reconcileSession"], undefined);
  assert.equal(storage["waymarks.reconcileActive"], undefined);
  assert.match(elements["#preview-report"].textContent, /Reconcile завершён/);
});
