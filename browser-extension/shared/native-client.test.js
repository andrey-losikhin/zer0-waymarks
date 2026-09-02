"use strict";

const assert = require("node:assert/strict");
const test = require("node:test");
const native = require("./native-client.js");

test("pull sends the versioned Native Messaging envelope", async () => {
  let sent;
  const runtime = {
    async sendNativeMessage(host, message) {
      sent = { host, message };
      return { protocol_version: 1, request_id: message.request_id, ok: true, data: { events: [] } };
    },
  };
  const client = native.create(runtime, () => "request-1");
  const result = await client.pull("firefox-test", 7, 100);
  assert.deepEqual(result, { events: [] });
  assert.equal(sent.host, "zer0.waymarks");
  assert.deepEqual(sent.message, {
    protocol_version: 1,
    request_id: "request-1",
    method: "changes.pull",
    params: { browser_instance_id: "firefox-test", after_sequence: 7, limit: 100 },
  });
});

test("pull rejects a mismatched response", async () => {
  const runtime = { async sendNativeMessage() { return { protocol_version: 1, request_id: "other", ok: true }; } };
  const client = native.create(runtime, () => "request-1");
  await assert.rejects(client.pull("firefox-test", 0, 100), /invalid native host response/);
});

test("preview sends the complete snapshot", async () => {
	let sent;
	const runtime = { async sendNativeMessage(host, message) {
	  sent = { host, message };
	  return { protocol_version: 1, request_id: message.request_id, ok: true, data: { preview_token: "token" } };
	} };
	const client = native.create(runtime, () => "preview-1");
	const snapshot = { root_browser_id: "root", nodes: [], unsupported: 0 };
	await client.preview("firefox-test", "fingerprint", snapshot);
	assert.deepEqual(sent.message.params, { browser_instance_id: "firefox-test", browser_fingerprint: "fingerprint", snapshot });
});

test("push sends preview authorization and operations", async () => {
	let sent;
	const runtime = { async sendNativeMessage(host, message) {
	  sent = { host, message };
	  return { protocol_version: 1, request_id: message.request_id, ok: true, data: { results: [] } };
	} };
	const client = native.create(runtime, () => "push-1");
	const operations = [{ operation_id: "op-1", kind: "remove", node_id: "node", base_revision: 1 }];
	await client.push("firefox-test", "token", "fingerprint", operations);
	assert.deepEqual(sent.message.params, { browser_instance_id: "firefox-test", preview_token: "token", browser_fingerprint: "fingerprint", operations });
});

test("apply sends prepare and commit fields", async () => {
	let sent;
	const runtime = { async sendNativeMessage(host, message) {
	  sent = { host, message };
	  return { protocol_version: 1, request_id: message.request_id, ok: true, data: { phase: "prepare" } };
	} };
	const client = native.create(runtime, () => "apply-1");
	const snapshot = { root_browser_id: "root", nodes: [], unsupported: 0 };
	await client.apply("prepare", "firefox-test", "token", "fingerprint", snapshot, [], 0, 50);
	assert.deepEqual(sent.message.params, { phase: "prepare", browser_instance_id: "firefox-test", preview_token: "token", browser_fingerprint: "fingerprint", snapshot, mappings: [], offset: 0, limit: 50 });
});
