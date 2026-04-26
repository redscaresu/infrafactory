import test from "node:test";
import assert from "node:assert/strict";

import { fetchPitfalls, fetchSavePitfalls } from "../src/lib/pitfalls-api.js";

function jsonResponse(body, init = {}) {
  return {
    ok: init.ok !== false,
    status: init.status || 200,
    headers: { get: () => "application/json" },
    async json() {
      return body;
    },
    async text() {
      return JSON.stringify(body);
    }
  };
}

function textResponse(text, init = {}) {
  return {
    ok: init.ok !== false,
    status: init.status || 200,
    headers: { get: () => "text/plain" },
    async json() {
      throw new Error("not json");
    },
    async text() {
      return text;
    }
  };
}

test("fetchPitfalls GETs /api/pitfalls and returns parsed JSON", async () => {
  const calls = [];
  const fakeFetch = async (url, init) => {
    calls.push({ url, init });
    return jsonResponse({ providers: [{ provider: "scaleway", pitfalls: [] }] });
  };

  const payload = await fetchPitfalls(fakeFetch);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, "/api/pitfalls");
  assert.equal(calls[0].init, undefined);
  assert.deepEqual(payload, { providers: [{ provider: "scaleway", pitfalls: [] }] });
});

test("fetchPitfalls throws with the response body when not ok", async () => {
  const fakeFetch = async () => textResponse("disk full", { ok: false, status: 500 });
  await assert.rejects(() => fetchPitfalls(fakeFetch), /disk full/);
});

test("fetchPitfalls surfaces backend JSON error messages", async () => {
  const fakeFetch = async () =>
    jsonResponse({ error: "pitfalls dir missing" }, { ok: false, status: 424 });
  await assert.rejects(() => fetchPitfalls(fakeFetch), /pitfalls dir missing/);
});

test("fetchSavePitfalls PUTs the JSON body to the provider endpoint", async () => {
  const calls = [];
  const fakeFetch = async (url, init) => {
    calls.push({ url, init });
    return jsonResponse({ provider: "scaleway", count: 2 });
  };

  const entries = [
    { resource: "scaleway_lb", rule: "use ip_ids", source: "static", discovered_from: "" },
    { resource: "scaleway_redis", rule: "default 6379", source: "learned", discovered_from: "run-1" }
  ];

  const resp = await fetchSavePitfalls("scaleway", entries, fakeFetch);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, "/api/pitfalls/scaleway");
  assert.equal(calls[0].init.method, "PUT");
  assert.equal(calls[0].init.headers["Content-Type"], "application/json");
  assert.deepEqual(JSON.parse(calls[0].init.body), { pitfalls: entries });
  assert.deepEqual(resp, { provider: "scaleway", count: 2 });
});

test("fetchSavePitfalls URL-encodes the provider segment", async () => {
  const calls = [];
  const fakeFetch = async (url, init) => {
    calls.push({ url, init });
    return jsonResponse({ provider: "name with space", count: 0 });
  };
  await fetchSavePitfalls("name with space", [], fakeFetch);
  assert.equal(calls[0].url, "/api/pitfalls/name%20with%20space");
});

test("fetchSavePitfalls rejects without a provider name", async () => {
  await assert.rejects(() => fetchSavePitfalls("", [], async () => jsonResponse({})), /provider is required/);
});

test("fetchSavePitfalls surfaces error responses", async () => {
  const fakeFetch = async () =>
    jsonResponse({ error: "pitfalls[0].resource is required" }, { ok: false, status: 422 });
  await assert.rejects(
    () => fetchSavePitfalls("scaleway", [{ resource: "", rule: "x" }], fakeFetch),
    /pitfalls\[0\]\.resource is required/
  );
});
