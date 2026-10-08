const fs = require("fs");
const os = require("os");
const path = require("path");
const { runBackfill } = require("../backfill-guest-locale-keys");

function tmpDir() {
  return fs.mkdtempSync(path.join(os.tmpdir(), "backfill-"));
}

function write(dir, name, data) {
  fs.writeFileSync(path.join(dir, name), JSON.stringify(data, null, 2));
}

function read(dir, name) {
  return JSON.parse(fs.readFileSync(path.join(dir, name), "utf8"));
}

describe("backfill-guest-locale-keys", () => {
  test("inserts missing top-level keys with the English value", () => {
    const dir = tmpDir();
    write(dir, "en.json", { a: "Hello", b: "World" });
    write(dir, "es.json", { a: "Hola" });

    const result = runBackfill({ dir });

    expect(result.inserted.es).toEqual(["b"]);
    expect(read(dir, "es.json")).toEqual({ a: "Hola", b: "World" });
  });

  test("inserts missing nested keys, preserving existing translations", () => {
    const dir = tmpDir();
    write(dir, "en.json", {
      menu: { title: "Menu", price: "Price" },
    });
    write(dir, "es.json", { menu: { title: "Menú" } });

    runBackfill({ dir });

    expect(read(dir, "es.json")).toEqual({
      menu: { title: "Menú", price: "Price" },
    });
  });

  test("does not touch en.json or non-JSON files", () => {
    const dir = tmpDir();
    write(dir, "en.json", { a: "Hello" });
    fs.writeFileSync(path.join(dir, "notes.txt"), "ignore me");

    const result = runBackfill({ dir });

    expect(Object.keys(result.inserted)).toEqual([]);
    expect(fs.readFileSync(path.join(dir, "notes.txt"), "utf8")).toBe(
      "ignore me",
    );
  });

  test("idempotent: second run inserts nothing", () => {
    const dir = tmpDir();
    write(dir, "en.json", { a: "Hello", b: "World" });
    write(dir, "es.json", { a: "Hola" });

    runBackfill({ dir });
    const second = runBackfill({ dir });

    expect(second.inserted.es).toEqual([]);
  });

  test("preserves the key order of en.json", () => {
    const dir = tmpDir();
    write(dir, "en.json", { z: "Z", a: "A", m: "M" });
    write(dir, "es.json", { a: "Aes" });

    runBackfill({ dir });

    const esContent = fs.readFileSync(path.join(dir, "es.json"), "utf8");
    const parsedKeys = Object.keys(JSON.parse(esContent));
    expect(parsedKeys).toEqual(["z", "a", "m"]);
  });
});
