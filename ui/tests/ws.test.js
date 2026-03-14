import test from "node:test";
import assert from "node:assert/strict";

import { buildWSURL } from "../src/lib/ws.ts";

test("buildWSURL maps browser protocol to websocket protocol", () => {
  assert.equal(buildWSURL("http:", "127.0.0.1:5173"), "ws://127.0.0.1:5173/api/ws");
  assert.equal(buildWSURL("https:", "example.com"), "wss://example.com/api/ws");
  assert.equal(buildWSURL("http:", "127.0.0.1:5173", "http://127.0.0.1:4173"), "ws://127.0.0.1:4173/api/ws");
  assert.equal(buildWSURL("https:", "ui.example.com", "api.example.com"), "wss://api.example.com/api/ws");
});
