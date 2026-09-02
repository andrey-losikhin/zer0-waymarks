"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const test = require("node:test");
const vm = require("node:vm");

function event() {
  let listener = null;
  return { addListener(value) { listener = value; }, fire(...args) { listener(...args); } };
}

async function settle() {
  for (let index = 0; index < 8; index += 1) await new Promise((resolve) => setImmediate(resolve));
}

test("managed Firefox create is pushed and added to mapping", async () => {
  const rootCanonical = "canonical-root";
  const storage = {
    "waymarks.browserInstanceId": "firefox-test",
    "waymarks.managedRootId": "browser-root",
    "waymarks.cursor": 7,
    "waymarks.mapping": {
      browser_to_canonical: { "browser-root": rootCanonical },
      canonical_to_browser: { [rootCanonical]: "browser-root" },
      revisions: { [rootCanonical]: 1 },
    },
  };
  const createdEvent = event();
  const pushed = [];
  const browser = {
    storage: { local: {
      async get(keys) {
        const names = Array.isArray(keys) ? keys : [keys];
        return Object.fromEntries(names.filter((key) => Object.hasOwn(storage, key)).map((key) => [key, storage[key]]));
      },
      async set(values) { Object.assign(storage, values); },
    } },
    runtime: { onInstalled: event(), onStartup: event() },
    alarms: { create() {}, onAlarm: event() },
    bookmarks: {
      onCreated: createdEvent, onChanged: event(), onMoved: event(), onRemoved: event(),
      async get() { throw new Error("unexpected get"); },
      async getSubTree() { return [{ id: "browser-root", title: "Waymarks", children: [] }]; },
    },
  };
  const client = {
    async pull(_instance, after) { return { events: [], through_sequence: after, has_more: false }; },
    async push(instance, token, fingerprint, operations) {
      pushed.push({ instance, token, fingerprint, operations });
      return { results: [{ ok: true, node: { id: "canonical-created", revision: 1 } }], through_sequence: 8, mapping_revision: 2 };
    },
  };
  let uuid = 0;
  const context = {
    browser,
    WaymarksNative: { create: () => client },
    crypto: { randomUUID: () => `uuid-${++uuid}` },
    URL,
    Promise,
    Date,
    Error,
    Object,
    Set,
    String,
  };
  vm.runInNewContext(fs.readFileSync(__dirname + "/background.js", "utf8"), context, { filename: "background.js" });
  await settle();
  createdEvent.fire("browser-created", { id: "browser-created", parentId: "browser-root", title: "Example", url: "https://example.test", index: 2 });
  await settle();
  assert.equal(pushed.length, 1);
  assert.deepEqual(JSON.parse(JSON.stringify(pushed[0].operations[0])), {
    operation_id: "firefox-uuid-1",
    browser_id: "browser-created",
    kind: "create",
    type: "bookmark",
    parent_id: rootCanonical,
    title: "Example",
    url: "https://example.test",
    position: 2,
  });
  assert.equal(storage["waymarks.mapping"].browser_to_canonical["browser-created"], "canonical-created");
  assert.equal(storage["waymarks.mapping"].revisions["canonical-created"], 1);
  assert.equal(storage["waymarks.cursor"], 7, "push must not skip unpulled helper events");
});

test("pulled canonical create is applied once and advances cursor", async () => {
  const rootCanonical = "canonical-root";
  const storage = {
    "waymarks.browserInstanceId": "firefox-test",
    "waymarks.managedRootId": "browser-root",
    "waymarks.cursor": 1,
    "waymarks.mapping": {
      browser_to_canonical: { "browser-root": rootCanonical },
      canonical_to_browser: { [rootCanonical]: "browser-root" },
      revisions: { [rootCanonical]: 1 },
    },
  };
  const createdEvent = event();
  const browserCreates = [];
  const browserUpdates = [];
  let pushCount = 0;
  const browser = {
    storage: { local: {
      async get(keys) {
        const names = Array.isArray(keys) ? keys : [keys];
        return Object.fromEntries(names.filter((key) => Object.hasOwn(storage, key)).map((key) => [key, storage[key]]));
      },
      async set(values) { Object.assign(storage, values); },
    } },
    runtime: { onInstalled: event(), onStartup: event() },
    alarms: { create() {}, onAlarm: event() },
    bookmarks: {
      onCreated: createdEvent, onChanged: event(), onMoved: event(), onRemoved: event(),
      async getChildren() { return []; },
      async getSubTree() { return [{ id: "browser-root", title: "Waymarks", children: [{ id: "browser-remote", parentId: "browser-root", title: "Remote", url: "https://remote.test" }] }]; },
      async create(details) {
        browserCreates.push(details);
        const created = { id: "browser-remote", parentId: details.parentId, title: details.title, url: details.url, index: details.index };
        createdEvent.fire(created.id, created);
        return created;
      },
      async update(id, changes) { browserUpdates.push({ id, changes }); return { id, ...changes }; },
    },
  };
  const remoteNode = { id: "canonical-remote", type: "bookmark", parent_id: rootCanonical, title: "Remote", url: "https://remote.test", position: 3, revision: 1, deleted_at: null };
  const client = {
    async pull(_instance, after) {
      if (after >= 2) return { events: [], through_sequence: after, has_more: false };
      return { events: [{ sequence: 2, operation_id: "helper-create", kind: "node.created", payload: { node: remoteNode } }], through_sequence: 2, has_more: false };
    },
    async push() { pushCount += 1; throw new Error("remote echo was pushed"); },
  };
  const context = {
    browser,
    WaymarksNative: { create: () => client },
    crypto: { randomUUID: () => "unused" },
    URL,
    Promise,
    Date,
    Error,
    Object,
    Set,
    String,
  };
  vm.runInNewContext(fs.readFileSync(__dirname + "/background.js", "utf8"), context, { filename: "background.js" });
  await settle();
  assert.equal(browserCreates.length, 1);
  assert.equal(browserCreates[0].parentId, "browser-root");
  assert.equal(browserCreates[0].index, 3);
  assert.equal(browserCreates[0].title, `__zer0-waymarks-${remoteNode.id}`);
  assert.equal(browserCreates[0].url, `https://zer0-waymarks.invalid/pending/${remoteNode.id}`);
  assert.deepEqual(JSON.parse(JSON.stringify(browserUpdates)), [{ id: "browser-remote", changes: { title: "Remote", url: "https://remote.test" } }]);
  assert.equal(pushCount, 0);
  assert.equal(storage["waymarks.cursor"], 2);
  assert.equal(storage["waymarks.mapping"].canonical_to_browser[remoteNode.id], "browser-remote");
  assert.equal(storage["waymarks.remotePending"], null);
});

test("restart retries durable local create before pull and suppresses its echo", async () => {
  const rootCanonical = "canonical-root";
  const operation = { operation_id: "firefox-stable", browser_id: "browser-local", kind: "create", type: "bookmark", parent_id: rootCanonical, title: "Local", url: "https://local.test", position: 1 };
  const storage = {
    "waymarks.browserInstanceId": "firefox-test",
    "waymarks.managedRootId": "browser-root",
    "waymarks.cursor": 4,
    "waymarks.localPending": { operation },
    "waymarks.mapping": {
      browser_to_canonical: { "browser-root": rootCanonical },
      canonical_to_browser: { [rootCanonical]: "browser-root" },
      revisions: { [rootCanonical]: 1 },
    },
  };
  let browserCreateCount = 0;
  let pushCount = 0;
  const browser = {
    storage: { local: {
      async get(keys) {
        const names = Array.isArray(keys) ? keys : [keys];
        return Object.fromEntries(names.filter((key) => Object.hasOwn(storage, key)).map((key) => [key, storage[key]]));
      },
      async set(values) { Object.assign(storage, values); },
    } },
    runtime: { onInstalled: event(), onStartup: event() },
    alarms: { create() {}, onAlarm: event() },
    bookmarks: {
      onCreated: event(), onChanged: event(), onMoved: event(), onRemoved: event(),
      async getSubTree() { return [{ id: "browser-root", title: "Waymarks", children: [{ id: "browser-local", parentId: "browser-root", title: "Local", url: "https://local.test" }] }]; },
      async create() { browserCreateCount += 1; throw new Error("unexpected duplicate"); },
    },
  };
  const node = { id: "canonical-local", revision: 1 };
  const client = {
    async push(_instance, _token, _fingerprint, operations) {
      pushCount += 1;
      assert.equal(operations[0].operation_id, operation.operation_id);
      return { results: [{ ok: true, node }], mapping_revision: 2 };
    },
    async pull(_instance, after) {
      if (after >= 5) return { events: [], through_sequence: after, has_more: false };
      return { events: [{ sequence: 5, operation_id: operation.operation_id, kind: "node.created", payload: { node: { ...node, type: "bookmark", parent_id: rootCanonical, title: "Local", url: "https://local.test", position: 1 } } }], through_sequence: 5, has_more: false };
    },
  };
  vm.runInNewContext(fs.readFileSync(__dirname + "/background.js", "utf8"), {
    browser, WaymarksNative: { create: () => client }, crypto: { randomUUID: () => "unused" }, URL, Promise, Date, Error, Object, Set, String,
  }, { filename: "background.js" });
  await settle();
  assert.equal(pushCount, 1);
  assert.equal(browserCreateCount, 0);
  assert.equal(storage["waymarks.localPending"], null);
  assert.equal(storage["waymarks.cursor"], 5);
  assert.equal(storage["waymarks.mapping"].browser_to_canonical[operation.browser_id], node.id);
});

test("managed update is pushed and move outside becomes remove", async () => {
  const rootCanonical = "canonical-root";
  const canonicalId = "canonical-bookmark";
  const storage = {
    "waymarks.browserInstanceId": "firefox-test",
    "waymarks.managedRootId": "browser-root",
    "waymarks.cursor": 10,
    "waymarks.mapping": {
      browser_to_canonical: { "browser-root": rootCanonical, "browser-bookmark": canonicalId },
      canonical_to_browser: { [rootCanonical]: "browser-root", [canonicalId]: "browser-bookmark" },
      revisions: { [rootCanonical]: 1, [canonicalId]: 1 },
    },
  };
  const changedEvent = event();
  const movedEvent = event();
  const pushed = [];
  let current = { id: "browser-bookmark", parentId: "browser-root", title: "Updated", url: "https://updated.test", index: 2 };
  const browser = {
    storage: { local: {
      async get(keys) {
        const names = Array.isArray(keys) ? keys : [keys];
        return Object.fromEntries(names.filter((key) => Object.hasOwn(storage, key)).map((key) => [key, storage[key]]));
      },
      async set(values) { Object.assign(storage, values); },
      async remove(keys) { for (const key of Array.isArray(keys) ? keys : [keys]) delete storage[key]; },
    } },
    runtime: { onInstalled: event(), onStartup: event() },
    alarms: { create() {}, onAlarm: event() },
    bookmarks: {
      onCreated: event(), onChanged: changedEvent, onMoved: movedEvent, onRemoved: event(),
      async get() { return [current]; },
      async getSubTree(id) {
        if (id === "browser-root") return [{ id: "browser-root", title: "Waymarks", children: [current] }];
        return [current];
      },
    },
  };
  const client = {
    async pull(_instance, after) { return { events: [], through_sequence: after, has_more: false }; },
    async push(_instance, _token, _fingerprint, operations) {
      const operation = operations[0];
      pushed.push(operation);
      if (operation.kind === "update") return { results: [{ ok: true, node: { id: canonicalId, revision: 2 } }], mapping_revision: 2 };
      return { results: [{ ok: true, node: { id: canonicalId, revision: 3, deleted_at: "now" } }], mapping_revision: 3 };
    },
  };
  let uuid = 0;
  vm.runInNewContext(fs.readFileSync(__dirname + "/background.js", "utf8"), {
    browser, WaymarksNative: { create: () => client }, crypto: { randomUUID: () => `uuid-${++uuid}` }, URL, Promise, Date, Error, Object, Set, String,
  }, { filename: "background.js" });
  await settle();
  changedEvent.fire("browser-bookmark", { title: "Updated" });
  await settle();
  assert.equal(pushed[0].kind, "update");
  assert.equal(pushed[0].base_revision, 1);
  current = { ...current, parentId: "outside", url: "javascript:unsupported" };
  movedEvent.fire("browser-bookmark", { oldParentId: "browser-root", parentId: "outside", index: 0 });
  await settle();
  assert.equal(pushed[1].kind, "remove");
  assert.equal(pushed[1].base_revision, 2);
  assert.equal(storage["waymarks.mapping"].browser_to_canonical["browser-bookmark"], undefined);
});
