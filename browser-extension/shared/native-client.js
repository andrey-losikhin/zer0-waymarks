(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  root.WaymarksNative = api;
})(typeof globalThis === "object" ? globalThis : this, function () {
  "use strict";

  const HOST_NAME = "zer0.waymarks";

  function create(runtime, randomUUID) {
    if (!runtime || typeof runtime.sendNativeMessage !== "function") {
      throw new TypeError("runtime.sendNativeMessage is required");
    }
    if (typeof randomUUID !== "function") {
      throw new TypeError("randomUUID is required");
    }

    async function request(method, params) {
      const requestId = randomUUID();
      const response = await runtime.sendNativeMessage(HOST_NAME, {
        protocol_version: 1,
        request_id: requestId,
        method,
        params,
      });
      if (!response || response.protocol_version !== 1 || response.request_id !== requestId || typeof response.ok !== "boolean") {
        throw new Error("invalid native host response");
      }
      if (!response.ok) {
        const error = new Error(response.error && response.error.message ? response.error.message : "native host request failed");
        error.code = response.error && response.error.code ? response.error.code : "storage_error";
        throw error;
      }
      return response.data;
    }

    return {
      pull(browserInstanceId, afterSequence, limit) {
        return request("changes.pull", {
          browser_instance_id: browserInstanceId,
          after_sequence: afterSequence,
          limit,
        });
      },
	  preview(browserInstanceId, browserFingerprint, snapshot) {
		return request("reconcile.preview", {
		  browser_instance_id: browserInstanceId,
		  browser_fingerprint: browserFingerprint,
		  snapshot,
		});
	  },
	  push(browserInstanceId, previewToken, browserFingerprint, operations) {
		return request("changes.push", {
		  browser_instance_id: browserInstanceId,
		  preview_token: previewToken,
		  browser_fingerprint: browserFingerprint,
		  operations,
		});
	  },
	  apply(phase, browserInstanceId, previewToken, browserFingerprint, snapshot, mappings, offset, limit) {
		return request("reconcile.apply", {
		  phase,
		  browser_instance_id: browserInstanceId,
		  preview_token: previewToken,
		  browser_fingerprint: browserFingerprint,
		  snapshot,
		  mappings: mappings || [],
		  offset: offset || 0,
		  limit: limit || 0,
		});
	  },
    };
  }

  return { HOST_NAME, create };
});
