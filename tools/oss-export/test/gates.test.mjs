// Unit tests for the local gates and the drop/scrub steps, run directly
// against small trees (no git repository, no scanners).

import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { after, describe, it } from "node:test";
import zlib from "node:zlib";

import { parseDropList, parseForbidden, parseScrubRules } from "../lib/config.mjs";
import { MAX_INFLATED_BYTES, decodedViews, utf16Order } from "../lib/content.mjs";
import {
  docLinksGate,
  envGate,
  forbiddenGate,
  gitignoreGate,
  isEnvFile,
  licenseGate,
  markdownLinkTargets,
  privatePathsGate,
  resolveLink,
  sizeGate,
  symlinkGate,
} from "../lib/gates.mjs";
import { applyDrop, applyScrub } from "../lib/steps.mjs";
import { walk } from "../lib/tree.mjs";
import { APACHE_LICENSE, makeTempRoot } from "./helpers.mjs";

const tmpRoot = makeTempRoot("gates");
after(() => fs.rmSync(tmpRoot, { recursive: true, force: true }));

let counter = 0;
function tree(files = {}, symlinks = {}) {
  counter += 1;
  const root = path.join(tmpRoot, `t${counter}`);
  fs.mkdirSync(root, { recursive: true });
  for (const [rel, content] of Object.entries(files)) {
    const abs = path.join(root, rel);
    fs.mkdirSync(path.dirname(abs), { recursive: true });
    fs.writeFileSync(abs, content);
  }
  for (const [rel, target] of Object.entries(symlinks)) {
    const abs = path.join(root, rel);
    fs.mkdirSync(path.dirname(abs), { recursive: true });
    fs.symlinkSync(target, abs);
  }
  return root;
}

describe("(c) forbidden strings", () => {
  const rules = parseForbidden(
    [
      "acme-private-corp ;; name=org",
      "lit:CaseOnly ;; name=case",
      "re:tok_[0-9]{4} ;; name=token allow=fixtures/**",
      "rei:internal\\.example ;; name=host only=config/**",
    ].join("\n"),
  );

  it("reports file:line and rule name for content, paths, binaries, symlinks and the author", () => {
    const root = tree(
      {
        "src/a.txt": "one\nACME-Private-Corp here\nthree tok_1234\n",
        "acme-private-corp/readme.md": "clean\n",
        "img.bin": Buffer.concat([Buffer.from([0, 1, 2]), Buffer.from("acme-private-corp")]),
      },
      { "link": "../acme-private-corp/x" },
    );
    const result = forbiddenGate({
      root,
      entries: walk(root),
      rules,
      author: "Someone <x@acme-private-corp.test>",
      message: "Release\n\nwith ACME-private-corp",
    });
    assert.equal(result.status, "fail");
    const hits = result.hits.map((h) => `${h.file}:${h.line}:${h.rule}`).sort();
    assert.deepEqual(hits, [
      "<commit author>:path:org",
      "<commit message>:3:org",
      "acme-private-corp/readme.md:path:org",
      "img.bin:binary:org",
      "link:path:org",
      "src/a.txt:2:org",
      "src/a.txt:3:token",
    ]);
    assert.deepEqual(result.perRule.org, { lines: 6, files: 6 });
    // No hit ever carries the matched text.
    assert.ok(!JSON.stringify(result.hits).includes("tok_1234"));
  });

  it("honours lit: case, allow= and only=", () => {
    const root = tree({
      "a.txt": "caseonly\n",
      "fixtures/f.txt": "tok_9999\n",
      "config/c.yml": "INTERNAL.example\n",
      "other/c.yml": "internal.example\n",
    });
    const result = forbiddenGate({ root, entries: walk(root), rules, author: null });
    assert.deepEqual(
      result.hits.map((h) => `${h.file}:${h.rule}`),
      ["config/c.yml:host"],
    );
  });

  it("sha256: rules catch a known blob at any path and stop once its bytes change", () => {
    const pixels = Buffer.concat([Buffer.from("RIFF\0\0\0\0WEBPVP8 "), Buffer.from([0, 7, 1, 2, 3, 250, 0, 9])]);
    const digest = createHash("sha256").update(pixels).digest("hex");
    const blobRules = parseForbidden(
      [
        `sha256:${digest} ;; name=wallet-screenshot-blob`,
        `sha256:${"0".repeat(64)} ;; name=wallet-screenshot-blob`,
        "re:^public/images/features/(?:menu|staff)\\.webp$ ;; name=wallet-screenshot",
      ].join("\n"),
    );
    const root = tree({
      "public/images/features/menu.webp": pixels,
      "docs/renamed-copy.webp": pixels,
      "public/images/features/staff.webp": Buffer.concat([pixels, Buffer.from([1])]),
      "src/page.tsx": 'src="public/images/features/menu.webp"\n',
      "public/images/features/clean.webp": Buffer.from([0, 1, 2]),
    });
    const result = forbiddenGate({ root, entries: walk(root), rules: blobRules, author: null });
    assert.equal(result.status, "fail");
    assert.deepEqual(
      result.hits.map((h) => `${h.file}:${h.line}:${h.rule}`).sort(),
      [
        "docs/renamed-copy.webp:blob:wallet-screenshot-blob",
        "public/images/features/menu.webp:blob:wallet-screenshot-blob",
        "public/images/features/menu.webp:path:wallet-screenshot",
        // Re-encoded bytes slip past the hash; the anchored path rule still
        // holds it. An anchored rule never matches a reference in code.
        "public/images/features/staff.webp:path:wallet-screenshot",
      ],
    );
    assert.equal(result.rules, 2);
  });

  it("reads UTF-16 text, wide strings inside binaries, gzip and PNG text chunks", () => {
    const utf16be = (text) => Buffer.from(text, "utf16le").swap16();
    const pngChunk = (type, data) => {
      const length = Buffer.alloc(4);
      length.writeUInt32BE(data.length);
      return Buffer.concat([length, Buffer.from(type, "latin1"), data, Buffer.alloc(4)]);
    };
    const png = (...chunks) =>
      Buffer.concat([
        Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
        pngChunk("IHDR", Buffer.alloc(13)),
        ...chunks,
        pngChunk("IEND", Buffer.alloc(0)),
      ]);
    const secret = "acme-private-corp";
    const root = tree({
      "le-bom.txt": Buffer.concat([Buffer.from([0xff, 0xfe]), Buffer.from(`one\r\n${secret}\r\n`, "utf16le")]),
      "be-nobom.txt": utf16be(`line one\n${secret.toUpperCase()}\n`),
      "font.bin": Buffer.concat([Buffer.from([0, 1, 0xff, 0x10, 7, 0, 0, 0x80, 0x81]), utf16be(secret), Buffer.from([0xff, 0, 3])]),
      "notes.txt.gz": zlib.gzipSync(`x\n${secret}\n`),
      "nested.gz": zlib.gzipSync(zlib.gzipSync(`${secret}\n`)),
      "ztxt.png": png(pngChunk("zTXt", Buffer.concat([Buffer.from("Author\0\0"), zlib.deflateSync(secret)]))),
      "itxt.png": png(pngChunk("iTXt", Buffer.concat([Buffer.from("Comment\0\x01\0en\0\0"), zlib.deflateSync(`by ${secret}`)]))),
      "plain.png": png(pngChunk("tEXt", Buffer.from("Software\0paint"))),
      "clean.gz": zlib.gzipSync("nothing\n"),
    });
    assert.equal(utf16Order(fs.readFileSync(path.join(root, "be-nobom.txt"))), "be");
    const result = forbiddenGate({ root, entries: walk(root), rules, author: null });
    assert.deepEqual(
      result.hits.map((h) => `${h.file}:${h.line}:${h.rule}`).sort(),
      [
        "be-nobom.txt:utf-16be:2:org",
        "font.bin:nul-stripped:org",
        "itxt.png:png-iTXt:org",
        "le-bom.txt:utf-16le:2:org",
        "nested.gz:gzip/gzip:1:org",
        "notes.txt.gz:gzip:2:org",
        "ztxt.png:png-zTXt:org",
      ],
    );
  });

  it("fails archives it cannot read unless --allow-opaque names them", () => {
    const root = tree({
      "bundle.zip": Buffer.concat([Buffer.from("PK\x03\x04", "latin1"), Buffer.alloc(26)]),
      "deck.pptx": Buffer.from([0, 1, 2, 3]),
      "dump.bz2": Buffer.concat([Buffer.from("BZh9", "latin1"), Buffer.from([0x31, 0x41, 0x59, 0x26, 0x53, 0x59, 0])]),
      "broken.gz": Buffer.from([0x1f, 0x8b, 0x08, 0, 0, 0, 0, 0, 0, 0xff, 1, 2, 3]),
      "page.html.br": Buffer.from([0x1b, 0x02, 0x00, 0xf8]),
      "inner.tgz": zlib.gzipSync(Buffer.concat([Buffer.from("PK\x05\x06", "latin1"), Buffer.alloc(18)])),
      // Text that merely starts with a magic number is still text.
      "README.md": "PK\x03\x04 and BZh91AY&SY are magic numbers\n",
    });
    const result = forbiddenGate({ root, entries: walk(root), rules, author: null });
    assert.equal(result.status, "fail");
    assert.deepEqual(
      result.hits.map((h) => `${h.file}:${h.line}:${h.rule}`).sort(),
      [
        "broken.gz:gzip-corrupt:opaque-archive",
        "bundle.zip:zip:opaque-archive",
        "deck.pptx:zip:opaque-archive",
        "dump.bz2:bzip2:opaque-archive",
        "inner.tgz:gzip/zip:opaque-archive",
        "page.html.br:brotli:opaque-archive",
      ],
    );
    const allowed = forbiddenGate({
      root,
      entries: walk(root),
      rules,
      author: null,
      opaqueAllow: ["*.zip", "deck.pptx", "dump.bz2", "broken.gz", "*.br", "inner.tgz"],
    });
    assert.equal(allowed.status, "pass");
    assert.equal(allowed.notes.length, 6);
  });

  // A tar has no magic number at offset 0 and an image ends where its own
  // format says, so whatever follows (a tar member, an appended payload) was
  // read only as raw bytes. A compressed member hid its content and an archive
  // member was not flagged at all.
  const tarMember = (name, data) => {
    const header = Buffer.alloc(512);
    header.write(name, 0, "latin1");
    header.write("0000644\0", 100, "latin1");
    header.write(`${data.length.toString(8).padStart(11, "0")}\0`, 124, "latin1");
    header.write("0", 156, "latin1");
    header.write("ustar\0" + "00", 257, "latin1");
    header.fill(0x20, 148, 156);
    let sum = 0;
    for (const byte of header) sum += byte;
    header.write(`${sum.toString(8).padStart(6, "0")}\0 `, 148, "latin1");
    const padded = Buffer.alloc(Math.ceil(data.length / 512) * 512);
    data.copy(padded);
    return Buffer.concat([header, padded]);
  };
  const tar = (...members) => Buffer.concat([...members, Buffer.alloc(1024)]);
  const zipOf = (name, text) => {
    const data = zlib.deflateRawSync(text);
    const header = Buffer.alloc(30);
    header.writeUInt32LE(0x04034b50, 0);
    header.writeUInt16LE(20, 4);
    header.writeUInt16LE(8, 8);
    header.writeUInt32LE(data.length, 18);
    header.writeUInt32LE(Buffer.byteLength(text), 22);
    header.writeUInt16LE(name.length, 26);
    return Buffer.concat([header, Buffer.from(name, "latin1"), data]);
  };
  const tinyPng = Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    Buffer.from([0, 0, 0, 13]),
    Buffer.from("IHDR", "latin1"),
    Buffer.alloc(17),
    Buffer.from([0, 0, 0, 0]),
    Buffer.from("IEND", "latin1"),
    Buffer.alloc(4),
  ]);

  it("reads gzip members and flags archives past offset 0 (tar members, polyglots)", () => {
    const secret = "acme-private-corp";
    const zip = zipOf("notes.txt", `owner ${secret}\n`);
    assert.ok(!zip.includes(secret), "the fixture hides the value");
    const root = tree({
      "outer.tar": tar(tarMember("readme.txt", Buffer.from("hello\n")), tarMember("inner.zip", zip)),
      "outer.tar.gz": zlib.gzipSync(tar(tarMember("inner.zip", zip))),
      "member.tar": tar(tarMember("notes.txt.gz", zlib.gzipSync(`x\n${secret}\n`))),
      "zip-polyglot.png": Buffer.concat([tinyPng, zip]),
      "gzip-polyglot.png": Buffer.concat([tinyPng, zlib.gzipSync(`${secret}\n`), Buffer.from("trailing")]),
      "xz-polyglot.webp": Buffer.concat([Buffer.from("RIFF\0\0\0\0WEBP", "latin1"), Buffer.from([0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00, 0, 0])]),
      // A clean image, and a tar of clean members, still pass.
      "clean.png": tinyPng,
      "clean.tar": tar(tarMember("a.txt.gz", zlib.gzipSync("nothing\n"))),
    });
    const png = tinyPng.length;
    const result = forbiddenGate({ root, entries: walk(root), rules, author: null });
    assert.deepEqual(
      result.hits.map((h) => `${h.file}:${h.line}:${h.rule}`).sort(),
      [
        `gzip-polyglot.png:gzip@${png}:1:org`,
        "member.tar:gzip@512:2:org",
        "outer.tar.gz:gzip/embedded-zip@512:opaque-archive",
        "outer.tar:embedded-zip@1536:opaque-archive",
        "xz-polyglot.webp:embedded-xz@12:opaque-archive",
        `zip-polyglot.png:embedded-zip@${png}:opaque-archive`,
      ],
    );
    const allowed = forbiddenGate({ root, entries: walk(root), rules, author: null, opaqueAllow: ["*.tar", "*.tar.gz", "*.png", "*.webp"] });
    assert.deepEqual(
      allowed.hits.map((h) => `${h.file}:${h.line}:${h.rule}`).sort(),
      [`gzip-polyglot.png:gzip@${png}:1:org`, "member.tar:gzip@512:2:org"],
      "--allow-opaque never hides content the gate did read",
    );
  });

  it("bounds the embedded gzip scan", () => {
    const members = [];
    for (let i = 0; i < 70; i += 1) members.push(Buffer.from([0, 1]), zlib.gzipSync(`member ${i}\n`));
    const root = tree({
      "many.bin": Buffer.concat(members),
      "bomb.tar": tar(tarMember("bomb.gz", zlib.gzipSync(Buffer.alloc(MAX_INFLATED_BYTES + 1024)))),
    });
    const result = forbiddenGate({ root, entries: walk(root), rules, author: null });
    assert.deepEqual(
      result.hits.map((h) => `${h.file}:${h.line}:${h.rule}`).sort(),
      ["bomb.tar:embedded-gzip@512-too-large:opaque-archive", "many.bin:embedded-gzip-too-many:opaque-archive"],
    );
  });

  it("decodes JS/JSON escapes, percent-encoding, character references and base64 line by line", () => {
    const secret = "acme-private-corp";
    const b64 = (text) => Buffer.from(text).toString("base64");
    const root = tree({
      "config.json": `{\n  "a": 1,\n  "org": "acme\\u002dprivate\\x2dcorp"\n}\n`,
      "escaped.js": `const s = "acme\\u{2d}private\\-corp";\n`,
      "url.txt": "one\ntwo\nhttps://x.test/?q=acme%2Dprivate%2dcorp&r=1\n",
      "page.html": "<p>acme&#45;private&#x2D;corp</p>\n<p>acme&hyphen;private&dash;corp</p>\n",
      "blob.txt": `line one\nkey: ${b64(`owner: ${secret}`)}\n`,
      "blob-url-safe.txt": `${Buffer.from(`${secret}??>>`).toString("base64url")}\n`,
      "blob-wide.txt": `${Buffer.from(`${secret}`, "utf16le").toString("base64")}\n`,
      "gzip-blob.txt": `a\nb\ndata: ${zlib.gzipSync(`x\n${secret}\n`).toString("base64")}\n`,
      "zip-blob.txt": `${zipOf("n.txt", secret).toString("base64")}\n`,
      // Plain on its own line plus an escape: one hit, on the plain line.
      "both.txt": `${secret} \\u0041\n`,
      // Clean controls: identifiers, an integrity hash, a data URI.
      "clean.js": `import { useTranslationProvider } from "./i18n";\nconst integrity = "sha512-${createHash("sha512").update("x").digest("base64")}";\nconst img = "data:image/png;base64,${tinyPng.toString("base64")}";\nconst re = /\\d+\\.\\w+/;\n`,
    });
    const result = forbiddenGate({
      root,
      entries: walk(root),
      rules,
      author: null,
      message: `Release\n\nsee ${b64(secret + " ok")}`,
    });
    assert.deepEqual(
      result.hits.map((h) => `${h.file}:${h.line}:${h.rule}`).sort(),
      [
        "<commit message>:base64-decoded:3:org",
        "blob-url-safe.txt:base64-decoded:1:org",
        "blob-wide.txt:base64-decoded:1:org",
        "blob.txt:base64-decoded:2:org",
        "both.txt:1:org",
        "config.json:unescaped:3:org",
        "escaped.js:unescaped:1:org",
        "gzip-blob.txt:base64:3/gzip:org",
        "page.html:entity-decoded:1:org",
        "page.html:entity-decoded:2:org",
        "url.txt:percent-decoded:3:org",
        "zip-blob.txt:base64:1/zip:opaque-archive",
      ],
    );
    assert.ok(!JSON.stringify(result.hits).includes(secret));
  });

  it("keeps only the lines a decoder changed, mapped to their source lines", () => {
    const text = "\tfmt.Sprintf(\"%s %d\", a, &period)\nx := \"a\\tb\"\n\tok\nq=%41%42\n";
    const { views, opaque } = decodedViews(text);
    assert.deepEqual(opaque, []);
    assert.deepEqual(
      views.map((v) => [v.label, v.text, v.lineMap]),
      [
        ["unescaped", 'x := "a b"', [2]],
        ["percent-decoded", "q=AB", [4]],
      ],
    );
  });

  it("stops inflating a gzip bomb at the size bound", () => {
    const bomb = zlib.gzipSync(Buffer.alloc(MAX_INFLATED_BYTES + 1024));
    const root = tree({ "bomb.gz": bomb });
    const result = forbiddenGate({ root, entries: walk(root), rules, author: null });
    assert.deepEqual(result.hits, [{ file: "bomb.gz", line: "gzip-too-large", rule: "opaque-archive" }]);
  });

  it("passes a clean tree", () => {
    const root = tree({ "a.txt": "nothing to see\n" });
    assert.equal(forbiddenGate({ root, entries: walk(root), rules, author: "Fixture <f@example.com>" }).status, "pass");
  });
});

describe("(d) file size", () => {
  it("fails over the limit unless allowlisted", () => {
    const root = tree({
      "big.bin": Buffer.alloc(2048),
      "fonts/print/noto-sans-cjk-sc/a.woff2": Buffer.alloc(4096),
      "small.txt": "x",
    });
    const entries = walk(root);
    const result = sizeGate({ entries, maxBytes: 1024, sizeAllow: ["fonts/print/noto-*-cjk-*"] });
    assert.equal(result.status, "fail");
    assert.deepEqual(
      result.hits.map((h) => h.file),
      ["big.bin"],
    );
    assert.equal(result.notes.length, 1);
    assert.equal(sizeGate({ entries, maxBytes: 8192, sizeAllow: [] }).status, "pass");
  });
});

describe("(e) symlinks", () => {
  it("fails on absolute and escaping targets, notes dangling in-tree links", () => {
    const root = tree(
      { "a/real.txt": "x" },
      {
        "a/ok": "real.txt",
        "a/up-ok": "../a/real.txt",
        "a/dangling": "missing.txt",
        "a/escape": "../../outside",
        "abs": "/etc/hosts",
      },
    );
    const result = symlinkGate({ root, entries: walk(root) });
    assert.equal(result.status, "fail");
    assert.deepEqual(
      result.hits.map((h) => `${h.file}:${h.rule}`).sort(),
      ["a/escape:escapes-tree", "abs:absolute-target"],
    );
    assert.equal(result.notes.length, 1);
  });

  it("passes in-tree links", () => {
    const root = tree({ "a.txt": "x" }, { "b": "a.txt" });
    assert.equal(symlinkGate({ root, entries: walk(root) }).status, "pass");
  });

  it("follows chains of links: judges where a link lands, not its text", () => {
    const root = tree(
      { "a/real.txt": "x" },
      {
        // sub/up lands on the root itself: fine on its own.
        "sub/up": "..",
        // Lexically sub/up/../.. is the root, but up is a link to the root,
        // so this really lands two levels above it.
        "sub/esc": "up/../..",
        "sub/esc-deeper": "up/../../outside",
        "chain/abs": "/etc/hosts",
        "chain/via-abs": "abs",
        "chain/ok": "../sub/up/a/real.txt",
        "loop/a": "b",
        "loop/b": "a",
      },
    );
    const result = symlinkGate({ root, entries: walk(root) });
    assert.equal(result.status, "fail");
    assert.deepEqual(
      result.hits.map((h) => `${h.file}:${h.rule}`).sort(),
      [
        "chain/abs:absolute-target",
        "chain/via-abs:escapes-tree",
        "loop/a:symlink-loop",
        "loop/b:symlink-loop",
        "sub/esc-deeper:escapes-tree",
        "sub/esc:escapes-tree",
      ],
    );
    assert.equal(resolveLink(fs.realpathSync(root), "sub/up").verdict, "ok");
    assert.equal(resolveLink(fs.realpathSync(root), "chain/ok").verdict, "ok");
    // The kernel agrees that sub/esc leaves the tree.
    assert.equal(fs.realpathSync.native(path.join(root, "sub/esc")), path.dirname(path.dirname(fs.realpathSync(root))));
  });
});

describe("(f) env files", () => {
  it("fails on env files and honours --allow-env-file", () => {
    const root = tree({ ".env": "", "ci/.env.ci": "", "a/.env.example": "", "b/prod.env": "" });
    const entries = walk(root);
    const result = envGate({ entries, envAllow: [] });
    assert.deepEqual(
      result.hits.map((h) => h.file).sort(),
      [".env", "b/prod.env", "ci/.env.ci"],
    );
    const allowed = envGate({ entries, envAllow: [".env", "ci/.env.ci", "b/prod.env"] });
    assert.equal(allowed.status, "pass");
    assert.equal(allowed.notes.length, 3);
  });

  it("catches the other common env-file names and the .envs/ layout", () => {
    const flagged = [
      ".env-production",
      ".env_local",
      ".ENV.Local",
      ".envrc",
      ".envrc.local",
      "x/Prod.ENV",
      ".envs/.production/.django",
      "deploy/.envs/.local/postgres",
      ".env.example.bak",
    ];
    const clean = [
      ".env.example",
      ".envs/.production/.django.example",
      "src/env.ts",
      "frontend/next-env.d.ts",
      "src/environment.js",
      "envs/notes.md",
      "src/.environment",
    ];
    for (const rel of flagged) assert.equal(isEnvFile(rel), true, rel);
    for (const rel of clean) assert.equal(isEnvFile(rel), false, rel);
    const root = tree({ ".env-production": "", ".envs/.production/.django": "", "a/.env.example": "" });
    assert.deepEqual(
      envGate({ entries: walk(root), envAllow: [] }).hits.map((h) => h.file).sort(),
      [".env-production", ".envs/.production/.django"],
    );
  });
});

describe("(g) gitignore", () => {
  it("reports ignored files and skips negated patterns", () => {
    const root = tree({
      ".gitignore": "*.log\n!keep.log\nbuild/\n",
      "debug.log": "",
      "keep.log": "",
      "build/out.js": "",
      "sub/.gitignore": "secret.txt\n",
      "sub/secret.txt": "",
      "src/app.js": "",
    });
    const result = gitignoreGate({ root, entries: walk(root), tmpDir: tmpRoot });
    assert.equal(result.status, "fail");
    assert.deepEqual(
      result.hits.map((h) => h.file).sort(),
      ["build/out.js", "debug.log", "sub/secret.txt"],
    );
  });

  it("ignores the user's global excludes", () => {
    const root = tree({ ".gitignore": "*.log\n", "src/app.js": "" });
    // A global excludes file that ignores *.js, reachable both through a
    // global config and through the XDG default location.
    const home = tree({ "xdg/git/ignore": "*.js\n", "excludes": "*.js\n" });
    fs.writeFileSync(path.join(home, "gitconfig"), `[core]\n\texcludesFile = ${path.join(home, "excludes")}\n`);
    const saved = { GIT_CONFIG_GLOBAL: process.env.GIT_CONFIG_GLOBAL, XDG_CONFIG_HOME: process.env.XDG_CONFIG_HOME };
    process.env.GIT_CONFIG_GLOBAL = path.join(home, "gitconfig");
    process.env.XDG_CONFIG_HOME = path.join(home, "xdg");
    try {
      const result = gitignoreGate({ root, entries: walk(root), tmpDir: tmpRoot });
      assert.equal(result.status, "pass");
    } finally {
      for (const [key, value] of Object.entries(saved)) {
        if (value === undefined) delete process.env[key];
        else process.env[key] = value;
      }
    }
  });
});

describe("(h) license", () => {
  it("requires an Apache-2.0 LICENSE", () => {
    assert.equal(licenseGate({ root: tree({}) }).hits[0].rule, "missing");
    assert.equal(licenseGate({ root: tree({ LICENSE: "MIT License\n" }) }).hits[0].rule, "not-apache-2.0");
    const ok = licenseGate({ root: tree({ LICENSE: APACHE_LICENSE }) });
    assert.equal(ok.status, "pass");
    assert.equal(ok.notes.length, 1, "missing NOTICE is a note");
    assert.equal(licenseGate({ root: tree({ LICENSE: APACHE_LICENSE, NOTICE: "n" }) }).notes.length, 0);
  });

  it("compares the whole text, not marker phrases", () => {
    assert.equal(
      createHash("sha256").update(APACHE_LICENSE).digest("hex"),
      "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
      "lib/apache-2.0.txt must stay the canonical apache.org text",
    );
    const holder = "Marcos Maceo";
    const check = (text, copyrightHolder = holder) => licenseGate({ root: tree({ LICENSE: text, NOTICE: "n" }), copyrightHolder });
    const terms = APACHE_LICENSE.slice(0, APACHE_LICENSE.indexOf("END OF TERMS AND CONDITIONS") + 27);
    const fill = (line) => APACHE_LICENSE.replace("[yyyy] [name of copyright owner]", line);
    const filled = fill("2026 Marcos Maceo");

    // Accepted: reflowed whitespace, CRLF, https links, no appendix, the
    // placeholder with or without a holder, and the copyright line filled in
    // with exactly the expected holder.
    assert.equal(check(APACHE_LICENSE.replace(/\n/g, "\r\n").replace(/ {3}/g, "  ")).status, "pass");
    assert.equal(check(APACHE_LICENSE.replaceAll("http://www.apache.org/", "https://www.apache.org/")).status, "pass");
    assert.equal(check(`${terms}\n`).status, "pass");
    assert.equal(check(APACHE_LICENSE, null).status, "pass");
    const filledResult = check(filled);
    assert.equal(filledResult.status, "pass");
    assert.match(filledResult.notes.join("\n"), /expected --copyright-holder/);
    for (const line of ["2024-2026 Marcos Maceo", "(c) 2026 Marcos  Maceo", "© 2024, 2026 Marcos Maceo."]) {
      assert.equal(check(fill(line)).status, "pass", line);
    }
    assert.equal(check(fill("2024-2026 Acme, Inc."), "Acme, Inc.").status, "pass");

    // Rejected: added restrictions, edited terms, the markers alone.
    const commonsClause = `${APACHE_LICENSE}\n"Commons Clause" License Condition v1.0\n\nThe Software is provided to you by the Licensor under the License, subject to the following condition.\n`;
    assert.deepEqual(check(commonsClause).hits, [{ file: "LICENSE", rule: "text-after-apache-terms" }]);
    assert.deepEqual(check(`${terms}\n\nAdditional restriction: no commercial use.\n`).hits, [
      { file: "LICENSE", rule: "text-after-apache-terms" },
    ]);
    // A restriction riding on the copyright line, a different holder, or any
    // filled-in line when no holder is configured.
    const mismatch = [{ file: "LICENSE", rule: "copyright-holder-mismatch" }];
    assert.deepEqual(check(fill("2026 Marcos Maceo; commercial use requires a separate licence")).hits, mismatch);
    assert.deepEqual(
      check(fill("2026 Marcos Maceo. Commercial use is not permitted without written consent (Commons Clause)")).hits,
      mismatch,
    );
    assert.deepEqual(check(fill("2026 Someone Else")).hits, mismatch);
    assert.deepEqual(check(fill("2026 Acme, Inc."), "Acme").hits, mismatch);
    assert.deepEqual(check(filled, null).hits, mismatch);
    assert.deepEqual(check(APACHE_LICENSE.replace("royalty-free, irrevocable", "royalty-free, revocable")).hits, [
      { file: "LICENSE", rule: "not-apache-2.0" },
    ]);
    const markersOnly = [
      "Apache License",
      "Version 2.0, January 2004",
      "TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION",
      "END OF TERMS AND CONDITIONS",
    ].join("\n");
    assert.deepEqual(check(markersOnly).hits, [{ file: "LICENSE", rule: "not-apache-2.0" }]);
  });
});

describe("(i) private paths", () => {
  it("fails when docs/superpowers or a drop entry survives", () => {
    const root = tree({ "docs/superpowers/plan.md": "", "reviews/r.md": "", "src/a.js": "" });
    const result = privatePathsGate({ root, entries: walk(root), dropEntries: parseDropList("reviews\n") });
    assert.equal(result.status, "fail");
    assert.deepEqual(
      result.hits.map((h) => h.rule).sort(),
      ["drop-entry-survived:**/docs/superpowers", "drop-entry-survived:reviews", "private-tree-present"],
    );
  });

  it("finds docs/superpowers at any depth and in any case", () => {
    const root = tree({
      "frontend/docs/superpowers/plans/p.md": "",
      "a/b/Docs/SuperPowers/x.md": "",
      "docs/superpowers-public/ok.md": "",
      "src/docs/readme.md": "",
    });
    const result = privatePathsGate({ root, entries: walk(root), dropEntries: [] });
    assert.equal(result.status, "fail");
    assert.deepEqual(
      result.hits.filter((h) => h.rule === "private-tree-present").map((h) => h.file),
      ["a/b/Docs/SuperPowers", "frontend/docs/superpowers"],
    );
    assert.deepEqual(
      result.hits.filter((h) => h.rule.startsWith("drop-entry-survived")).map((h) => h.file),
      ["frontend/docs/superpowers/plans/p.md"],
    );
  });

  it("the built-in drop removes nested docs/superpowers trees", () => {
    const root = tree({
      "docs/superpowers/plan.md": "",
      "frontend/docs/superpowers/plans/p.md": "",
      "frontend/docs/guide.md": "",
      "docs/superpowers-public/ok.md": "",
    });
    const result = applyDrop(root, []);
    assert.equal(result.entries[0].pattern, "**/docs/superpowers");
    assert.equal(result.entries[0].files, 2);
    assert.deepEqual(
      walk(root).map((e) => e.rel),
      ["docs/superpowers-public/ok.md", "frontend/docs/guide.md"],
    );
    assert.equal(privatePathsGate({ root, entries: walk(root), dropEntries: [] }).status, "pass");
  });

  it("passes once the drop step ran", () => {
    const root = tree({ "docs/superpowers/plan.md": "", "reviews/r.md": "", "src/a.js": "" });
    const dropEntries = parseDropList("reviews\n");
    applyDrop(root, dropEntries);
    assert.ok(!fs.existsSync(path.join(root, "docs/superpowers")));
    assert.ok(fs.existsSync(path.join(root, "docs")) === false, "emptied parent directories are removed");
    assert.equal(privatePathsGate({ root, entries: walk(root), dropEntries }).status, "pass");
  });
});

describe("(j) doc links", () => {
  const readme = [
    "# Project",
    "See [skills](.claude/skills/rebrand/SKILL.md) and [agents](docs/agents/README.md#setup).",
    "![logo](/assets/logo.png \"Logo\")",
    "[web](https://example.com/x) [mail](mailto:a@example.com) [top](#project)",
    "Inline `[not](a/link.md)` code is ignored.",
    "```md",
    "[fenced](nowhere.md)",
    "```",
    "[ref]: docs/missing.md",
    "",
  ].join("\n");

  it("extracts inline, image and reference links outside code", () => {
    assert.deepEqual(
      markdownLinkTargets(readme).map((l) => l.target),
      [".claude/skills/rebrand/SKILL.md", "docs/agents/README.md#setup", "/assets/logo.png", "https://example.com/x", "mailto:a@example.com", "#project", "docs/missing.md"],
    );
  });

  it("fails when an entry-point doc links to a path the drop removed", () => {
    const root = tree({ "README.md": readme, "docs/agents/README.md": "[back](../../README.md) [up](../../../x.md)\n", "assets/logo.png": "" });
    const result = docLinksGate({ root });
    assert.equal(result.status, "fail");
    assert.deepEqual(
      result.hits.map((h) => `${h.file}:${h.line} ${h.rule} ${h.detail}`),
      [
        "README.md:2 link-target-missing .claude/skills/rebrand/SKILL.md",
        "README.md:9 link-target-missing docs/missing.md",
        "docs/agents/README.md:1 link-leaves-tree ../../../x.md",
      ],
    );
  });

  it("passes when every relative link resolves, and skips absent docs", () => {
    const root = tree({
      "README.md": readme.replace("[ref]: docs/missing.md", "[ref]: docs/agents/README.md"),
      ".claude/skills/rebrand/SKILL.md": "",
      "docs/agents/README.md": "[back](../../README.md) [space](my%20file.md)\n",
      "docs/agents/my file.md": "",
      "assets/logo.png": "",
    });
    assert.deepEqual(docLinksGate({ root }).hits, []);
    assert.equal(docLinksGate({ root: tree({ "src/a.js": "" }) }).status, "pass");
  });
});

describe("drop and scrub steps", () => {
  it("drops globs, counts bytes and reports unmatched entries", () => {
    const root = tree({ "a/x.sql": "12345", "a/y.sql": "1", "a/keep.md": "k", "b/c.txt": "c" });
    const result = applyDrop(root, parseDropList("a/*.sql\nnot-there\n"));
    assert.equal(result.droppedFiles, 2);
    assert.equal(result.droppedBytes, 6);
    assert.deepEqual(result.unmatched, ["**/docs/superpowers", "not-there"]);
    assert.deepEqual(
      walk(root).map((e) => e.rel),
      ["a/keep.md", "b/c.txt"],
    );
  });

  it("scrubs text files only, honouring only=/exclude=", () => {
    const root = tree({
      "a.txt": "path /home/alice/x and /home/alice/y\n",
      "keep/a.txt": "/home/alice/z\n",
      "bin.dat": Buffer.concat([Buffer.from([0]), Buffer.from("/home/alice/")]),
      "latin1.txt": Buffer.from([0x2f, 0x68, 0x6f, 0x6d, 0x65, 0x2f, 0x61, 0x6c, 0x69, 0x63, 0x65, 0x2f, 0xe9]),
    });
    const rules = parseScrubRules("s#/home/alice/#/path/to/#g ;; name=home exclude=keep\n");
    const result = applyScrub(root, rules);
    assert.equal(fs.readFileSync(path.join(root, "a.txt"), "utf8"), "path /path/to/x and /path/to/y\n");
    assert.equal(fs.readFileSync(path.join(root, "keep/a.txt"), "utf8"), "/home/alice/z\n");
    assert.ok(fs.readFileSync(path.join(root, "bin.dat")).includes("/home/alice/"));
    assert.deepEqual(result.rules[0], { name: "home", lineNo: 1, replacements: 2, files: ["a.txt"] });
    assert.deepEqual(result.skippedNonUtf8, ["latin1.txt"]);
  });
});

describe("envGate on a clean tree", () => {
  it("passes", () => {
    const root = tree({ ".env.example": "", "src/a.js": "" });
    assert.equal(envGate({ entries: walk(root), envAllow: [] }).status, "pass");
  });
});
