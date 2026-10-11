import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  ConfigError,
  compileGlob,
  compileGlobs,
  parseDropList,
  parseForbidden,
  parseScrubRules,
  parseSecretsAllow,
  secretKey,
} from "../lib/config.mjs";
import { isEnvFile } from "../lib/gates.mjs";

function scrub(rulesText, input, file = "a.txt") {
  let text = input;
  for (const rule of parseScrubRules(rulesText)) {
    if (rule.only && !rule.only(file)) continue;
    if (rule.exclude && rule.exclude(file)) continue;
    text = text.replace(rule.regex, rule.replacer);
  }
  return text;
}

describe("globs", () => {
  it("anchors at the root and covers directories", () => {
    const g = compileGlob("docs/qa");
    assert.ok(g("docs/qa"));
    assert.ok(g("docs/qa/a/b.md"));
    assert.ok(!g("x/docs/qa/a.md"));
    assert.ok(!g("docs/qa-old/a.md"));
  });

  it("supports *, **, ? and braces", () => {
    assert.ok(compileGlob("a/*.sql")("a/x.sql"));
    assert.ok(!compileGlob("a/*.sql")("a/b/x.sql"));
    assert.ok(compileGlob("**/.env.example")(".env.example"));
    assert.ok(compileGlob("**/.env.example")("backend/.env.example"));
    assert.ok(compileGlob("f?o")("foo"));
    assert.ok(!compileGlob("f?o")("f/o"));
    const braces = compileGlob("docker-compose.{a,b}.yml");
    assert.ok(braces("docker-compose.a.yml"));
    assert.ok(braces("docker-compose.b.yml"));
    assert.ok(!braces("docker-compose.c.yml"));
  });

  it("normalises leading ./ and slashes and escapes regex characters", () => {
    assert.ok(compileGlob("./docs/")("docs/x"));
    assert.ok(compileGlob("/docs")("docs/x"));
    assert.ok(compileGlob("a+b(1).txt")("a+b(1).txt"));
    assert.ok(!compileGlob("a.txt")("abtxt"));
    assert.ok(compileGlobs(["x", "y/*"])("y/z"));
    assert.ok(!compileGlobs([])("anything"));
  });
});

describe("drop.txt", () => {
  it("parses entries, comments and blank lines", () => {
    const entries = parseDropList("# c\n\nreviews\n  docs/qa/  \n.github/workflows/x.yml\n");
    assert.deepEqual(
      entries.map((e) => e.pattern),
      ["reviews", "docs/qa", ".github/workflows/x.yml"],
    );
    assert.equal(entries[1].lineNo, 4);
  });

  it("rejects .., whitespace, the root and options", () => {
    assert.throws(() => parseDropList("../x\n"), ConfigError);
    assert.throws(() => parseDropList("a b\n"), ConfigError);
    assert.throws(() => parseDropList("/\n"), ConfigError);
    assert.throws(() => parseDropList("a ;; name=x\n"), ConfigError);
  });
});

describe("forbidden.txt", () => {
  it("treats plain lines as case-insensitive literals", () => {
    const [rule] = parseForbidden("a.b+c ;; name=x\n");
    assert.equal(rule.name, "x");
    assert.equal([..."A.B+C axb+c".matchAll(rule.regex)].length, 1);
  });

  it("supports lit:, re: and rei:", () => {
    const rules = parseForbidden("lit:Foo\nre:ba[rz]\nrei:QU+X\n");
    const [lit, re, rei] = rules;
    assert.equal([..."Foo foo".matchAll(lit.regex)].length, 1);
    assert.equal([..."bar baz BAR".matchAll(re.regex)].length, 2);
    assert.equal([..."quux QUX".matchAll(rei.regex)].length, 2);
    assert.deepEqual(
      rules.map((r) => r.name),
      ["L1", "L2", "L3"],
    );
  });

  it("parses allow= and only= globs", () => {
    const [rule] = parseForbidden("secret ;; name=s allow=a/**,b.txt only=src # trailing comment\n");
    assert.ok(rule.allow("a/x/y"));
    assert.ok(rule.allow("b.txt"));
    assert.ok(rule.only("src/z.js"));
    assert.ok(!rule.only("lib/z.js"));
  });

  it("keeps ;; inside a pattern when it is not surrounded by spaces", () => {
    const [rule] = parseForbidden("re:a;;b\n");
    assert.ok(rule.regex.test("a;;b"));
  });

  it("parses sha256: content rules and merges digests that share a name", () => {
    const a = "a".repeat(64);
    const b = "b".repeat(64);
    const rules = parseForbidden(`sha256:${a} ;; name=blob # one\nsha256:${b} ;; name=blob\nword ;; name=text\n`);
    assert.equal(rules.length, 2);
    assert.equal(rules[0].name, "blob");
    assert.equal(rules[0].regex, null);
    assert.deepEqual([...rules[0].digests], [a, b]);
    assert.equal(rules[1].name, "text");
    assert.throws(() => parseForbidden(`sha256:${"A".repeat(64)}\n`), /lowercase hex/);
    assert.throws(() => parseForbidden("sha256:abc\n"), /lowercase hex/);
    assert.throws(
      () => parseForbidden("sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\n"),
      /every empty file/,
    );
    assert.throws(() => parseForbidden(`sha256:${a} ;; name=x only=src\n`), /any path/);
    assert.throws(() => parseForbidden(`word ;; name=x\nsha256:${a} ;; name=x\n`), /duplicate/);
    assert.throws(() => parseForbidden(`sha256:${a} ;; name=x\nword ;; name=x\n`), /duplicate/);
  });

  it("rejects bad lists", () => {
    assert.throws(() => parseForbidden("# only comments\n"), /no rules/);
    assert.throws(() => parseForbidden("re:a*\n"), /empty string/);
    assert.throws(() => parseForbidden("re:(\n"), /invalid regular expression/);
    assert.throws(() => parseForbidden("a ;; name=x\nb ;; name=x\n"), /duplicate/);
    assert.throws(() => parseForbidden("a ;; bogus=1\n"), /unknown option/);
    assert.throws(() => parseForbidden("a ;; name\n"), /malformed option/);
    assert.throws(() => parseForbidden("lit:\n"), /empty pattern/);
  });
});

describe("scrub.rules", () => {
  it("replaces with groups, named groups and $&", () => {
    assert.equal(scrub("s#(\\w+)@corp\\.test#$1@example.com#g\n", "a@corp.test b@corp.test"), "a@example.com b@example.com");
    assert.equal(scrub("s#(?<u>\\w+)@x#${1}+$<u>#\n", "me@x"), "me+me");
    assert.equal(scrub("s#abc#[$&]#g\n", "abc"), "[abc]");
    assert.equal(scrub("s#a#\\$1#g\n", "a"), "$1");
    assert.equal(scrub("s#a#$9#g\n", "a"), "");
  });

  it("replaces only the first match without g", () => {
    assert.equal(scrub("s#a#b#\n", "aaa"), "baa");
  });

  it("honours flags, other delimiters and escaped delimiters", () => {
    assert.equal(scrub("s|FOO|bar|gi\n", "foo FOO"), "bar bar");
    assert.equal(scrub("s|a\\|b|c|g\n", "a|b ab"), "c ab");
    assert.equal(scrub("s#a\\#b#c\\#d#gu\n", "a#b"), "c#d");
    assert.equal(scrub("s#^x#y#gm\n", "x\nx"), "y\ny");
    assert.equal(scrub("s#/home/u/#/path/to/#g   # trailing comment\n", "/home/u/a"), "/path/to/a");
  });

  it("honours only= and exclude=", () => {
    const rules = "s#a#b#g ;; name=r only=src exclude=src/keep.txt\n";
    assert.equal(scrub(rules, "a", "src/x.txt"), "b");
    assert.equal(scrub(rules, "a", "src/keep.txt"), "a");
    assert.equal(scrub(rules, "a", "lib/x.txt"), "a");
  });

  it("rejects malformed rules", () => {
    assert.throws(() => parseScrubRules("x#a#b#g\n"), ConfigError);
    assert.throws(() => parseScrubRules("sxaxbxg\n"), /invalid delimiter/);
    assert.throws(() => parseScrubRules("s#a#b\n"), /unterminated/);
    assert.throws(() => parseScrubRules("s#a#b#q\n"), /unsupported flag/);
    assert.throws(() => parseScrubRules("s#a#b#gjunk\n"), ConfigError);
    assert.throws(() => parseScrubRules("s#a#b#g name=x\n"), /';;'/);
    assert.throws(() => parseScrubRules("s#a*#b#g\n"), /empty string/);
    assert.throws(() => parseScrubRules("s#(#b#g\n"), /invalid regular expression/);
    assert.throws(() => parseScrubRules("s#a#b#g ;; allow=x\n"), /unknown option/);
  });
});

describe("secrets-allow.txt", () => {
  const digest = "a".repeat(64);

  it("parses fingerprints and trailing comments", () => {
    const set = parseSecretsAllow(`# header\ngitleaks rule-x ./a/b.go sha256:${digest}  # fixture\n`);
    assert.ok(set.has(secretKey("gitleaks", "rule-x", "a/b.go", digest)));
  });

  it("rejects malformed lines", () => {
    assert.throws(() => parseSecretsAllow("gitleaks r a.go\n"), ConfigError);
    assert.throws(() => parseSecretsAllow(`other r a.go sha256:${digest}\n`), /unknown scanner/);
    assert.throws(() => parseSecretsAllow("gitleaks r a.go sha256:XYZ\n"), /fingerprint/);
  });
});

describe("env file names", () => {
  it("flags env files and spares examples", () => {
    for (const name of [".env", ".env.local", "a/.env.ci", "x/prod.env", ".envrc"]) {
      assert.ok(isEnvFile(name), name);
    }
    for (const name of [".env.example", "b/.env.ci.example", ".github/e2e/ci.env.fixture", "envoy.yaml", "a/env.ts", "x.environment"]) {
      assert.ok(!isEnvFile(name), name);
    }
  });
});
