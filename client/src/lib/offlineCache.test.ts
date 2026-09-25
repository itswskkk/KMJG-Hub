import "fake-indexeddb/auto";
import { IDBFactory } from "fake-indexeddb";
import { beforeEach, describe, expect, it } from "vitest";
import { _resetForTests, cacheDelete, cacheGet, cacheGetAll, cacheKey, cacheSet } from "./offlineCache";

beforeEach(() => {
  globalThis.indexedDB = new IDBFactory();
  _resetForTests();
});

describe("cacheKey", () => {
  it("namespaces keys by Server address so different Servers never collide", () => {
    expect(cacheKey("https://a.test", "p1")).toBe("https://a.test::p1");
    expect(cacheKey("https://a.test", "p1")).not.toBe(cacheKey("https://b.test", "p1"));
  });
});

describe("cacheGet/cacheSet", () => {
  it("returns undefined for a key that was never set", async () => {
    await expect(cacheGet("projects", "missing")).resolves.toBeUndefined();
  });

  it("round-trips a stored value", async () => {
    const value = [{ id: "p1", name: "Alpha" }];
    await cacheSet("projects", "list", value);
    await expect(cacheGet("projects", "list")).resolves.toEqual(value);
  });

  it("overwrites a previous value under the same key", async () => {
    await cacheSet("chat_messages", "p1", [{ id: "m1" }]);
    await cacheSet("chat_messages", "p1", [{ id: "m2" }]);
    await expect(cacheGet("chat_messages", "p1")).resolves.toEqual([{ id: "m2" }]);
  });

  it("keeps different stores independent", async () => {
    await cacheSet("projects", "k", "project-value");
    await expect(cacheGet("tasks", "k")).resolves.toBeUndefined();
  });
});

describe("cacheGetAll", () => {
  it("returns an empty array for an empty store", async () => {
    await expect(cacheGetAll("conversations")).resolves.toEqual([]);
  });

  it("returns every value written to a store", async () => {
    await cacheSet("conversations", "a", { id: "a" });
    await cacheSet("conversations", "b", { id: "b" });
    const all = await cacheGetAll<{ id: string }>("conversations");
    expect(all.map((item) => item.id).sort()).toEqual(["a", "b"]);
  });
});

describe("cacheDelete", () => {
  it("removes a stored value", async () => {
    await cacheSet("tasks", "p1", ["t1"]);
    await cacheDelete("tasks", "p1");
    await expect(cacheGet("tasks", "p1")).resolves.toBeUndefined();
  });
});

describe("resilience when IndexedDB is unavailable", () => {
  it("resolves gracefully instead of throwing when indexedDB is undefined", async () => {
    const original = indexedDB;
    // @ts-expect-error deliberately simulating an environment without IndexedDB
    delete globalThis.indexedDB;
    _resetForTests();
    try {
      await expect(cacheGet("projects", "k")).resolves.toBeUndefined();
      await expect(cacheSet("projects", "k", "v")).resolves.toBeUndefined();
      await expect(cacheGetAll("projects")).resolves.toEqual([]);
    } finally {
      globalThis.indexedDB = original;
      _resetForTests();
    }
  });
});
