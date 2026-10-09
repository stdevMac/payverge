// Parsers for the export pipeline's private configuration files.
//
// Every list shares one line grammar:
//   - blank lines and lines whose first non-space character is `#` are ignored;
//   - `<body>` optionally followed by ` ;; key=value key=value # comment`.
// Paths and globs are always relative to the exported tree's root, use `/`,
// and are anchored at the root. A pattern that names a directory also covers
// everything below it.

import fs from "node:fs";

export class ConfigError extends Error {
  constructor(file, line, message) {
    super(`${file}${line ? `:${line}` : ""}: ${message}`);
    this.name = "ConfigError";
  }
}

const OPTION_SEPARATOR = /\s;;(?:\s|$)/;

function splitOptions(raw) {
  const match = OPTION_SEPARATOR.exec(raw);
  if (!match) return { body: raw.trim(), optionText: "" };
  return {
    body: raw.slice(0, match.index).trim(),
    optionText: raw.slice(match.index + match[0].length).trim(),
  };
}

function parseOptionTokens(optionText, file, lineNo, allowedKeys) {
  const options = {};
  if (!optionText) return options;
  const tokens = optionText.split(/\s+/);
  for (const token of tokens) {
    if (token.startsWith("#")) break; // trailing comment
    const eq = token.indexOf("=");
    if (eq <= 0) throw new ConfigError(file, lineNo, `malformed option "${token}" (expected key=value)`);
    const key = token.slice(0, eq);
    const value = token.slice(eq + 1);
    if (!allowedKeys.includes(key)) {
      throw new ConfigError(file, lineNo, `unknown option "${key}" (allowed: ${allowedKeys.join(", ")})`);
    }
    if (!value) throw new ConfigError(file, lineNo, `option "${key}" has an empty value`);
    if (key === "name") {
      options.name = value;
    } else {
      options[key] = (options[key] || []).concat(value.split(",").filter(Boolean));
    }
  }
  return options;
}

function contentLines(text) {
  return text.split(/\r?\n/).map((line, index) => ({ line, lineNo: index + 1 }));
}

function isComment(line) {
  const trimmed = line.trim();
  return trimmed === "" || trimmed.startsWith("#");
}

// ---------------------------------------------------------------------------
// Globs

function expandBraces(pattern) {
  const open = pattern.indexOf("{");
  if (open === -1) return [pattern];
  const close = pattern.indexOf("}", open);
  if (close === -1) return [pattern];
  const head = pattern.slice(0, open);
  const tail = pattern.slice(close + 1);
  return pattern
    .slice(open + 1, close)
    .split(",")
    .flatMap((choice) => expandBraces(head + choice + tail));
}

function globBodyToRegExpSource(glob) {
  let out = "";
  for (let i = 0; i < glob.length; i += 1) {
    const ch = glob[i];
    if (ch === "*") {
      if (glob[i + 1] === "*") {
        const followedBySlash = glob[i + 2] === "/";
        out += followedBySlash ? "(?:.*/)?" : ".*";
        i += followedBySlash ? 2 : 1;
      } else {
        out += "[^/]*";
      }
    } else if (ch === "?") {
      out += "[^/]";
    } else if ("\\^$.|+()[]{}".includes(ch)) {
      out += `\\${ch}`;
    } else {
      out += ch;
    }
  }
  return out;
}

export function normalizeGlob(pattern) {
  let p = pattern.trim().replace(/\\/g, "/");
  while (p.startsWith("./")) p = p.slice(2);
  p = p.replace(/^\/+/, "").replace(/\/+$/, "");
  return p;
}

// compileGlob returns a predicate over repo-relative paths. A match on a
// directory prefix counts, so "docs/qa" covers "docs/qa/x/y.md".
export function compileGlob(pattern) {
  const normalized = normalizeGlob(pattern);
  if (!normalized) throw new Error("empty glob");
  const sources = expandBraces(normalized).map(globBodyToRegExpSource);
  const re = new RegExp(`^(?:${sources.join("|")})(?:/.*)?$`);
  const fn = (relPath) => re.test(relPath);
  fn.pattern = normalized;
  return fn;
}

export function compileGlobs(patterns = []) {
  const compiled = patterns.map(compileGlob);
  return (relPath) => compiled.some((fn) => fn(relPath));
}

// ---------------------------------------------------------------------------
// drop.txt: one path or glob per line.

export function parseDropList(text, file = "drop.txt") {
  const entries = [];
  for (const { line, lineNo } of contentLines(text)) {
    if (isComment(line)) continue;
    const { body, optionText } = splitOptions(line);
    parseOptionTokens(optionText, file, lineNo, []);
    if (!body) continue;
    if (/\s/.test(body)) throw new ConfigError(file, lineNo, "drop entries must not contain whitespace");
    if (body.split("/").includes("..")) throw new ConfigError(file, lineNo, "drop entries must not contain '..'");
    const pattern = normalizeGlob(body);
    if (!pattern) throw new ConfigError(file, lineNo, "drop entry resolves to the tree root");
    entries.push({ pattern, lineNo, match: compileGlob(pattern) });
  }
  return entries;
}

// ---------------------------------------------------------------------------
// forbidden.txt
//   literal (case-insensitive)        acme-private-corp
//   lit:<literal> (case-sensitive)    lit:Foo
//   re:<JS regex> (case-sensitive)    re:AKIA[0-9A-Z]{16}
//   rei:<JS regex> (case-insensitive) rei:foo\s+bar
//   sha256:<64 hex>                   a file whose bytes hash to this, at any path
// Options: name=<label> allow=<glob,...> only=<glob,...>
// Text rules are matched against the whole path and the whole file content
// (never with the m flag), so a re: rule anchored with ^...$ is path-only.
// Several sha256: lines may share one name; they form a single rule.

const EMPTY_SHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855";

export function escapeRegExp(text) {
  return text.replace(/[\\^$.*+?()[\]{}|]/g, "\\$&");
}

export function parseForbidden(text, file = "forbidden.txt") {
  const rules = [];
  const names = new Set();
  for (const { line, lineNo } of contentLines(text)) {
    if (isComment(line)) continue;
    const { body, optionText } = splitOptions(line);
    const options = parseOptionTokens(optionText, file, lineNo, ["name", "allow", "only"]);
    if (!body) throw new ConfigError(file, lineNo, "empty pattern");
    if (body.startsWith("sha256:")) {
      const digest = body.slice(7);
      if (!/^[0-9a-f]{64}$/.test(digest)) {
        throw new ConfigError(file, lineNo, "sha256: rules need 64 lowercase hex digits");
      }
      if (digest === EMPTY_SHA256) throw new ConfigError(file, lineNo, "sha256: rule matches every empty file");
      if (options.allow || options.only) {
        throw new ConfigError(file, lineNo, "sha256: rules match file content at any path; allow= and only= do not apply");
      }
      const name = options.name || `L${lineNo}`;
      const existing = rules.find((rule) => rule.name === name);
      if (existing && !existing.digests) throw new ConfigError(file, lineNo, `duplicate rule name "${name}"`);
      if (existing) {
        existing.digests.add(digest);
      } else {
        names.add(name);
        rules.push({ name, lineNo, regex: null, digests: new Set([digest]), allow: null, only: null });
      }
      continue;
    }
    let source;
    let flags;
    if (body.startsWith("re:") || body.startsWith("rei:")) {
      source = body.slice(body.indexOf(":") + 1);
      flags = body.startsWith("rei:") ? "gi" : "g";
    } else if (body.startsWith("lit:")) {
      source = escapeRegExp(body.slice(4));
      flags = "g";
    } else {
      source = escapeRegExp(body);
      flags = "gi";
    }
    if (!source) throw new ConfigError(file, lineNo, "empty pattern");
    let regex;
    try {
      regex = new RegExp(source, flags);
    } catch (error) {
      throw new ConfigError(file, lineNo, `invalid regular expression: ${error.message}`);
    }
    if (regex.test("")) throw new ConfigError(file, lineNo, "pattern matches the empty string");
    regex.lastIndex = 0;
    const name = options.name || `L${lineNo}`;
    if (names.has(name)) throw new ConfigError(file, lineNo, `duplicate rule name "${name}"`);
    names.add(name);
    rules.push({
      name,
      lineNo,
      regex,
      allow: options.allow ? compileGlobs(options.allow) : null,
      only: options.only ? compileGlobs(options.only) : null,
    });
  }
  if (rules.length === 0) throw new ConfigError(file, 0, "forbidden list has no rules");
  return rules;
}

// ---------------------------------------------------------------------------
// scrub.rules: s<d>PATTERN<d>REPLACEMENT<d>FLAGS [;; name=.. only=.. exclude=..]
//   PATTERN is a JavaScript regular expression.
//   REPLACEMENT: `\x` emits x literally; $1..$99, ${N}, $& and $<name> expand.
//   FLAGS: g (replace all), i, m, s, u.

const DELIMITER_FORBIDDEN = /[A-Za-z0-9\s\\]/;

const REGEX_SYNTAX = "^$.*+?()[]{}|/";

// readDelimited reads up to the next unescaped delimiter. An escaped delimiter
// always means that character literally: inside a pattern a regex syntax
// character such as | stays escaped, anything else (e.g. #) is emitted bare so
// the pattern still compiles under the u flag.
function readDelimited(text, start, delimiter, file, lineNo, what) {
  let out = "";
  for (let i = start; i < text.length; i += 1) {
    const ch = text[i];
    if (ch === "\\" && i + 1 < text.length) {
      const next = text[i + 1];
      if (next === delimiter) {
        out += what === "pattern" && REGEX_SYNTAX.includes(delimiter) ? `\\${delimiter}` : delimiter;
      } else {
        out += `\\${next}`;
      }
      i += 1;
    } else if (ch === delimiter) {
      return { value: out, end: i };
    } else {
      out += ch;
    }
  }
  throw new ConfigError(file, lineNo, `unterminated ${what}`);
}

export function parseReplacement(template) {
  // Returns a list of parts: {lit} | {group: number|string} | {whole: true}
  const parts = [];
  let literal = "";
  const flush = () => {
    if (literal) parts.push({ lit: literal });
    literal = "";
  };
  for (let i = 0; i < template.length; i += 1) {
    const ch = template[i];
    if (ch === "\\" && i + 1 < template.length) {
      literal += template[i + 1];
      i += 1;
    } else if (ch === "$") {
      const rest = template.slice(i + 1);
      let m;
      if ((m = /^(\d{1,2})/.exec(rest))) {
        flush();
        parts.push({ group: Number(m[1]) });
        i += m[0].length;
      } else if ((m = /^\{(\d{1,2})\}/.exec(rest))) {
        flush();
        parts.push({ group: Number(m[1]) });
        i += m[0].length;
      } else if ((m = /^<([A-Za-z_][A-Za-z0-9_]*)>/.exec(rest))) {
        flush();
        parts.push({ group: m[1] });
        i += m[0].length;
      } else if (rest.startsWith("&")) {
        flush();
        parts.push({ whole: true });
        i += 1;
      } else {
        literal += "$";
      }
    } else {
      literal += ch;
    }
  }
  flush();
  return parts;
}

function buildReplacer(parts) {
  return (...args) => {
    const hasNamed = typeof args[args.length - 1] === "object" && args[args.length - 1] !== null;
    const groups = hasNamed ? args[args.length - 1] : {};
    const captureCount = args.length - (hasNamed ? 4 : 3);
    const match = args[0];
    let out = "";
    for (const part of parts) {
      if (part.lit !== undefined) out += part.lit;
      else if (part.whole) out += match;
      else if (typeof part.group === "number") {
        out += part.group === 0 ? match : part.group <= captureCount ? (args[part.group] ?? "") : "";
      } else out += groups?.[part.group] ?? "";
    }
    return out;
  };
}

export function parseScrubRules(text, file = "scrub.rules") {
  const rules = [];
  for (const { line, lineNo } of contentLines(text)) {
    if (isComment(line)) continue;
    const trimmed = line.trim();
    if (!trimmed.startsWith("s") || trimmed.length < 4) {
      throw new ConfigError(file, lineNo, "rules must look like s#PATTERN#REPLACEMENT#FLAGS");
    }
    const delimiter = trimmed[1];
    if (DELIMITER_FORBIDDEN.test(delimiter)) {
      throw new ConfigError(file, lineNo, `invalid delimiter "${delimiter}"`);
    }
    const pattern = readDelimited(trimmed, 2, delimiter, file, lineNo, "pattern");
    const replacement = readDelimited(trimmed, pattern.end + 1, delimiter, file, lineNo, "replacement");
    const rest = trimmed.slice(replacement.end + 1);
    const flagMatch = /^[a-z]*/.exec(rest);
    const flagText = flagMatch[0];
    const after = rest.slice(flagText.length);
    if (after && !/^\s/.test(after)) throw new ConfigError(file, lineNo, `unexpected text after flags: "${after}"`);
    const afterTrim = after.trim();
    let optionText = "";
    if (afterTrim.startsWith(";;")) optionText = afterTrim.slice(2).trim();
    else if (afterTrim && !afterTrim.startsWith("#")) {
      throw new ConfigError(file, lineNo, "options must follow ';;' and comments must start with '#'");
    }
    const options = parseOptionTokens(optionText, file, lineNo, ["name", "only", "exclude"]);
    for (const flag of flagText) {
      if (!"gimsu".includes(flag)) throw new ConfigError(file, lineNo, `unsupported flag "${flag}"`);
    }
    if (!pattern.value) throw new ConfigError(file, lineNo, "empty pattern");
    let regex;
    try {
      regex = new RegExp(pattern.value, Array.from(new Set(flagText + "")).join(""));
    } catch (error) {
      throw new ConfigError(file, lineNo, `invalid regular expression: ${error.message}`);
    }
    if (regex.test("")) throw new ConfigError(file, lineNo, "pattern matches the empty string");
    regex.lastIndex = 0;
    // Replacement text may itself contain an escaped delimiter; un-escape the
    // remaining backslashes in parseReplacement.
    const parts = parseReplacement(replacement.value);
    rules.push({
      name: options.name || `L${lineNo}`,
      lineNo,
      regex,
      global: flagText.includes("g"),
      replacer: buildReplacer(parts),
      only: options.only ? compileGlobs(options.only) : null,
      exclude: options.exclude ? compileGlobs(options.exclude) : null,
    });
  }
  return rules;
}

// ---------------------------------------------------------------------------
// secrets-allow.txt: reviewed synthetic fixtures, pinned by value hash.
//   <scanner> <rule-id> <path> sha256:<64 hex>   [# note]

export function parseSecretsAllow(text, file = "secrets-allow.txt") {
  const entries = new Set();
  for (const { line, lineNo } of contentLines(text)) {
    if (isComment(line)) continue;
    const withoutComment = line.replace(/\s#.*$/, "").trim();
    const fields = withoutComment.split(/\s+/);
    if (fields.length !== 4) {
      throw new ConfigError(file, lineNo, "expected: <scanner> <rule-id> <path> sha256:<hex>");
    }
    const [scanner, rule, filePath, digest] = fields;
    if (!["gitleaks", "trufflehog"].includes(scanner)) {
      throw new ConfigError(file, lineNo, `unknown scanner "${scanner}"`);
    }
    if (!/^sha256:[0-9a-f]{64}$/.test(digest)) {
      throw new ConfigError(file, lineNo, "fingerprint must be sha256:<64 lowercase hex>");
    }
    entries.add(secretKey(scanner, rule, normalizeGlob(filePath), digest.slice(7)));
  }
  return entries;
}

export function secretKey(scanner, rule, filePath, digestHex) {
  return `${scanner}\t${rule}\t${filePath}\t${digestHex}`;
}

export function readConfigFile(filePath, parser) {
  if (!filePath) return null;
  const text = fs.readFileSync(filePath, "utf8");
  return parser(text, filePath);
}
