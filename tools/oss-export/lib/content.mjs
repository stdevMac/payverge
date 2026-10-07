// What the forbidden-strings gate reads out of one file. A plain byte-wise
// read misses text that is stored wide (UTF-16) or compressed, so a file is
// expanded into several views before the rules run over it:
//
//   - the bytes themselves, as UTF-8 text or, for binary files, as latin1;
//   - a UTF-16 decode, when a byte-order mark or the NUL pattern says the
//     file is UTF-16 text;
//   - for any other binary file that holds NUL bytes, the bytes with every
//     NUL removed, which exposes UTF-16 and UTF-32 strings embedded in a
//     binary format (font name tables, Windows resources) at any alignment;
//   - the inflated content of a gzip file (recursively, size-bounded);
//   - the inflated text chunks (zTXt, compressed iTXt) of a PNG;
//   - for a binary file, any gzip member found past offset 0 (appended to an
//     image, or stored compressed inside a tar), inflated and expanded again;
//   - for a text file, its lines decoded from JS/JSON escapes, percent-
//     encoding, HTML character references and base64 (see decodedViews).
//
// Formats that would need a real archive reader (zip and its derivatives,
// bzip2, xz, 7z, zstd, rar, lzip, lz4, brotli) are reported as opaque instead
// of being passed: their content cannot be checked, so they fail the gate
// unless the owner exempts the path. That holds wherever such an archive
// starts: at offset 0, past it (a polyglot, a tar member), or base64-encoded
// inside a text file.

import path from "node:path";
import zlib from "node:zlib";

import { isBinaryBuffer } from "./tree.mjs";

export const MAX_INFLATED_BYTES = 64 * 1024 * 1024;
const MAX_NESTING = 3;
const SNIFF_BYTES = 65536;

const utf8 = new TextDecoder("utf-8", { fatal: true });

function startsWith(buffer, bytes, offset = 0) {
  if (buffer.length < offset + bytes.length) return false;
  for (let i = 0; i < bytes.length; i += 1) if (buffer[offset + i] !== bytes[i]) return false;
  return true;
}

const PI_BLOCK = [0x31, 0x41, 0x59, 0x26, 0x53, 0x59];
const EMPTY_BLOCK = [0x17, 0x72, 0x45, 0x38, 0x50, 0x90];

// Magic numbers of the formats the gate cannot look inside.
const OPAQUE_MAGIC = [
  ["zip", (b) => startsWith(b, [0x50, 0x4b, 0x03, 0x04]) || startsWith(b, [0x50, 0x4b, 0x05, 0x06]) || startsWith(b, [0x50, 0x4b, 0x07, 0x08])],
  [
    "bzip2",
    (b) => startsWith(b, [0x42, 0x5a, 0x68]) && b[3] >= 0x31 && b[3] <= 0x39 && (startsWith(b, PI_BLOCK, 4) || startsWith(b, EMPTY_BLOCK, 4)),
  ],
  ["xz", (b) => startsWith(b, [0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00])],
  ["7z", (b) => startsWith(b, [0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c])],
  ["zstd", (b) => startsWith(b, [0x28, 0xb5, 0x2f, 0xfd])],
  ["rar", (b) => startsWith(b, [0x52, 0x61, 0x72, 0x21, 0x1a, 0x07])],
  ["lzip", (b) => startsWith(b, [0x4c, 0x5a, 0x49, 0x50, 0x01])],
  ["lz4", (b) => startsWith(b, [0x04, 0x22, 0x4d, 0x18])],
];

// Extensions that name an archive or compressed format. They are opaque even
// when the magic number is missing (a truncated or disguised file).
const OPAQUE_EXTENSIONS = new Map(
  Object.entries({
    zip: "zip",
    jar: "zip",
    war: "zip",
    ear: "zip",
    apk: "zip",
    aar: "zip",
    whl: "zip",
    nupkg: "zip",
    xpi: "zip",
    docx: "zip",
    xlsx: "zip",
    pptx: "zip",
    odt: "zip",
    ods: "zip",
    odp: "zip",
    epub: "zip",
    bz2: "bzip2",
    tbz2: "bzip2",
    xz: "xz",
    txz: "xz",
    lzma: "xz",
    "7z": "7z",
    zst: "zstd",
    rar: "rar",
    lz: "lzip",
    lz4: "lz4",
    br: "brotli",
    z: "compress",
  }),
);

const GZIP_MAGIC = [0x1f, 0x8b, 0x08];
const PNG_MAGIC = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];

// Signatures searched past offset 0, where a second file hides: appended to
// another one (image.png + payload.zip) or stored inside a tar. Each check
// past the magic number keeps random bytes in images from matching.
const EMBEDDED_MAGIC = [
  ["zip", [0x50, 0x4b, 0x03, 0x04], (b, i) => b.length >= i + 30 && b.readUInt16LE(i + 8) <= 99],
  ["bzip2", [0x42, 0x5a, 0x68], (b, i) => b[i + 3] >= 0x31 && b[i + 3] <= 0x39 && (startsWith(b, PI_BLOCK, i + 4) || startsWith(b, EMPTY_BLOCK, i + 4))],
  ["xz", [0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00]],
  ["7z", [0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c]],
  ["zstd", [0x28, 0xb5, 0x2f, 0xfd], (b, i) => b.length > i + 4 && (b[i + 4] & 0x08) === 0],
  ["rar", [0x52, 0x61, 0x72, 0x21, 0x1a, 0x07]],
  ["lzip", [0x4c, 0x5a, 0x49, 0x50, 0x01]],
  ["lz4", [0x04, 0x22, 0x4d, 0x18], (b, i) => b.length > i + 4 && (b[i + 4] & 0xc0) === 0x40],
].map(([format, magic, check]) => [format, Buffer.from(magic), check || (() => true)]);
const GZIP_MAGIC_BYTES = Buffer.from(GZIP_MAGIC);
const MAX_EMBEDDED_GZIP = 64;

export function opaqueFormat(buffer, relPath = "") {
  for (const [format, test] of OPAQUE_MAGIC) if (test(buffer)) return format;
  const ext = path.posix.extname(relPath).slice(1).toLowerCase();
  return OPAQUE_EXTENSIONS.get(ext) || null;
}

// utf16Order says whether a buffer is UTF-16 text: "le", "be" or null. A BOM
// decides; otherwise the NUL bytes must sit almost only on one parity, as
// they do when ASCII-range text is stored two bytes per character.
export function utf16Order(buffer) {
  if (buffer.length >= 2 && buffer[0] === 0xff && buffer[1] === 0xfe) return "le";
  if (buffer.length >= 2 && buffer[0] === 0xfe && buffer[1] === 0xff) return "be";
  const end = Math.min(buffer.length - (buffer.length % 2), SNIFF_BYTES);
  if (end < 4) return null;
  let evenNul = 0;
  let oddNul = 0;
  for (let i = 0; i < end; i += 2) {
    if (buffer[i] === 0) evenNul += 1;
    if (buffer[i + 1] === 0) oddNul += 1;
  }
  const pairs = end / 2;
  if (oddNul >= pairs * 0.6 && evenNul <= pairs * 0.05) return "le";
  if (evenNul >= pairs * 0.6 && oddNul <= pairs * 0.05) return "be";
  return null;
}

function decodeUtf16(buffer, order) {
  let body = buffer;
  if (buffer.length >= 2 && ((order === "le" && buffer[0] === 0xff && buffer[1] === 0xfe) || (order === "be" && buffer[0] === 0xfe && buffer[1] === 0xff))) {
    body = buffer.subarray(2);
  }
  const even = Buffer.from(body.subarray(0, body.length - (body.length % 2)));
  if (order === "be") even.swap16();
  return even.toString("utf16le");
}

function withoutNuls(buffer) {
  const out = Buffer.allocUnsafe(buffer.length);
  let n = 0;
  for (let i = 0; i < buffer.length; i += 1) if (buffer[i] !== 0) out[n++] = buffer[i];
  return out.subarray(0, n);
}

function inflate(fn, data) {
  try {
    return { buffer: fn(data, { maxOutputLength: MAX_INFLATED_BYTES }) };
  } catch (error) {
    return { error: error && error.code === "ERR_BUFFER_TOO_LARGE" ? "too-large" : "corrupt" };
  }
}

// pngTextChunks returns the inflated payloads of a PNG's compressed text
// chunks, plus a list of chunks that would not inflate.
function pngTextChunks(buffer) {
  const texts = [];
  const failures = [];
  let offset = PNG_MAGIC.length;
  while (offset + 12 <= buffer.length) {
    const length = buffer.readUInt32BE(offset);
    const type = buffer.toString("latin1", offset + 4, offset + 8);
    const dataStart = offset + 8;
    const dataEnd = dataStart + length;
    if (dataEnd + 4 > buffer.length) break;
    const data = buffer.subarray(dataStart, dataEnd);
    if (type === "zTXt") {
      const keywordEnd = data.indexOf(0);
      if (keywordEnd !== -1) {
        const out = inflate(zlib.inflateSync, data.subarray(keywordEnd + 2));
        if (out.buffer) texts.push({ label: "png-zTXt", buffer: out.buffer });
        else failures.push(`png-zTXt-${out.error}`);
      }
    } else if (type === "iTXt") {
      const keywordEnd = data.indexOf(0);
      if (keywordEnd !== -1 && data[keywordEnd + 1] === 1) {
        const languageEnd = data.indexOf(0, keywordEnd + 3);
        const translatedEnd = languageEnd === -1 ? -1 : data.indexOf(0, languageEnd + 1);
        if (translatedEnd !== -1) {
          const out = inflate(zlib.inflateSync, data.subarray(translatedEnd + 1));
          if (out.buffer) texts.push({ label: "png-iTXt", buffer: out.buffer });
          else failures.push(`png-iTXt-${out.error}`);
        }
      }
    } else if (type === "IEND") {
      break;
    }
    offset = dataEnd + 4;
  }
  return { texts, failures };
}

// gzipMemberData returns where the deflate data of a gzip member starting at
// `offset` begins, or -1 when the RFC 1952 header there is not plausible
// (reserved flag bits set, an unknown XFL). The OS byte is not checked:
// zlib writes 19 on macOS, outside the RFC's list.
function gzipMemberData(buffer, offset) {
  if (buffer.length < offset + 18) return -1;
  const flags = buffer[offset + 3];
  const xfl = buffer[offset + 8];
  if (flags & 0xe0) return -1;
  if (xfl !== 0 && xfl !== 2 && xfl !== 4) return -1;
  let p = offset + 10;
  if (flags & 0x04) {
    if (p + 2 > buffer.length) return -1;
    p += 2 + buffer.readUInt16LE(p);
  }
  for (const bit of [0x08, 0x10]) {
    if (!(flags & bit)) continue;
    const end = buffer.indexOf(0, p);
    if (end === -1) return -1;
    p = end + 1;
  }
  if (flags & 0x02) p += 2;
  return p < buffer.length ? p : -1;
}

// embeddedContent looks past offset 0 of a binary file for archives. A gzip
// member is inflated (trailing bytes after it are ignored) and its content
// expanded like a file of its own; any other archive format is opaque. All
// members of one file share the 64 MB inflation bound.
function embeddedContent(buffer, depth) {
  const views = [];
  const opaque = [];
  for (const [format, magic, check] of EMBEDDED_MAGIC) {
    for (let i = buffer.indexOf(magic, 1); i !== -1; i = buffer.indexOf(magic, i + 1)) {
      if (check(buffer, i)) {
        opaque.push(`embedded-${format}@${i}`);
        break;
      }
    }
  }
  let members = 0;
  let inflatedBytes = 0;
  for (let i = buffer.indexOf(GZIP_MAGIC_BYTES, 1); i !== -1; i = buffer.indexOf(GZIP_MAGIC_BYTES, i + 1)) {
    const dataStart = gzipMemberData(buffer, i);
    if (dataStart === -1) continue;
    if (inflatedBytes >= MAX_INFLATED_BYTES) {
      opaque.push(`embedded-gzip@${i}-too-large`);
      break;
    }
    let inflated;
    try {
      inflated = zlib.inflateRawSync(buffer.subarray(dataStart), {
        maxOutputLength: MAX_INFLATED_BYTES - inflatedBytes,
        finishFlush: zlib.constants.Z_SYNC_FLUSH,
      });
    } catch (error) {
      if (error && error.code === "ERR_BUFFER_TOO_LARGE") {
        opaque.push(`embedded-gzip@${i}-too-large`);
        break;
      }
      continue; // not a deflate stream after all: random bytes
    }
    if (inflated.length === 0) continue;
    inflatedBytes += inflated.length;
    members += 1;
    if (members > MAX_EMBEDDED_GZIP) {
      opaque.push("embedded-gzip-too-many");
      break;
    }
    if (depth >= MAX_NESTING) {
      opaque.push(`embedded-gzip@${i}-nested-too-deep`);
      continue;
    }
    const inner = contentViews(inflated, "", depth + 1);
    const label = `gzip@${i}`;
    for (const view of inner.views) views.push({ ...view, label: view.label ? `${label}/${view.label}` : label });
    for (const reason of inner.opaque) opaque.push(`${label}/${reason}`);
  }
  return { views, opaque };
}

// ---------------------------------------------------------------------------
// Encoded text. A value written as JS or JSON escapes (\u0040, \/, \.),
// percent-encoding (%40), HTML character references (&#64; &commat;) or
// base64 does not match a plain rule. Each decoder rewrites one line at a
// time, one pass, and keeps only the lines it changed; `lineMap` maps every
// kept line back to the source line, so hits still name the file's own line.
// Decoded line breaks and other control characters become spaces; the rest
// of the line is left as it was.

function cleanDecoded(text) {
  return text.replace(/\u0000/g, "").replace(/[\u0001-\u001f\u007f]/g, " ");
}

const JS_ESCAPE = /\\(?:u\{([0-9a-fA-F]{1,6})\}|u([0-9a-fA-F]{4})|x([0-9a-fA-F]{2})|([^\r\n]))/g;
const JS_SINGLE = { n: " ", r: " ", t: " ", b: " ", f: " ", v: " ", 0: " " };

function unescapeJs(line) {
  if (!line.includes("\\")) return null;
  return line.replace(JS_ESCAPE, (match, codePoint, u4, x2, single) => {
    if (codePoint !== undefined) {
      const value = parseInt(codePoint, 16);
      return value <= 0x10ffff ? cleanDecoded(String.fromCodePoint(value)) : match;
    }
    if (u4 !== undefined) return cleanDecoded(String.fromCharCode(parseInt(u4, 16)));
    if (x2 !== undefined) return cleanDecoded(String.fromCharCode(parseInt(x2, 16)));
    return JS_SINGLE[single] ?? single;
  });
}

const PERCENT_RUN = /(?:%[0-9a-fA-F]{2})+/g;

function percentDecode(line) {
  if (!line.includes("%")) return null;
  return line.replace(PERCENT_RUN, (run) => cleanDecoded(Buffer.from(run.replace(/%/g, ""), "hex").toString("utf8")));
}

// A named reference needs its semicolon (a browser decodes only a few
// legacy names without one, none of them useful for hiding text), so code
// such as `&period` in Go is left alone.
const ENTITY = /&(?:#([0-9]{1,7});?|#[xX]([0-9a-fA-F]{1,6});?|([A-Za-z][A-Za-z0-9]{1,31});)/g;
const NAMED_ENTITIES = {
  amp: "&", lt: "<", gt: ">", quot: '"', apos: "'", nbsp: " ", sol: "/", bsol: "\\", period: ".", commat: "@",
  colon: ":", semi: ";", comma: ",", num: "#", percnt: "%", lowbar: "_", hyphen: "-", dash: "-", plus: "+",
  equals: "=", excl: "!", quest: "?", lpar: "(", rpar: ")", ast: "*", Tab: " ", NewLine: " ",
};

function decodeEntities(line) {
  if (!line.includes("&")) return null;
  return line.replace(ENTITY, (match, decimal, hex, name) => {
    if (name !== undefined) return Object.hasOwn(NAMED_ENTITIES, name) ? NAMED_ENTITIES[name] : match;
    const value = parseInt(decimal ?? hex, decimal !== undefined ? 10 : 16);
    return value <= 0x10ffff ? cleanDecoded(String.fromCodePoint(value)) : match;
  });
}

// Standard and URL-safe alphabets together (Node decodes both). A run that
// starts mid-encoding (glued to a word in the same alphabet) decodes to noise.
// The lookbehind makes a match start only where a run starts, so a line full
// of short identifiers is scanned once rather than once per character.
const BASE64_RUN = /(?<![A-Za-z0-9+/_-])[A-Za-z0-9+/_-]{16,}={0,2}/g;
const NON_TEXT = /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f\ufffd]/;

// asText returns decoded bytes as text when they read as text (UTF-8, or
// UTF-16 once NULs are removed), else null. Invalid UTF-8 decodes to U+FFFD,
// which NON_TEXT rejects (cheaper than a fatal TextDecoder per run).
function asText(bytes) {
  const stripped = bytes.includes(0) ? withoutNuls(bytes) : bytes;
  if (stripped.length < 6) return null;
  const text = stripped.toString("utf8");
  return NON_TEXT.test(text) ? null : cleanDecoded(text);
}

function hasKnownMagic(bytes) {
  return startsWith(bytes, GZIP_MAGIC) || startsWith(bytes, PNG_MAGIC) || opaqueFormat(bytes) !== null;
}

// decodeBase64 replaces each base64 run that decodes to text. Runs that decode
// to a file format the gate reads or must refuse (gzip, PNG, an archive) are
// handed to onBlob and expanded like files of their own.
function decodeBase64(line, onBlob) {
  if (line.length < 16) return null;
  let changed = false;
  const out = line.replace(BASE64_RUN, (run) => {
    const bytes = Buffer.from(run, "base64");
    if (bytes.length >= 8 && hasKnownMagic(bytes)) {
      onBlob(bytes);
      return run;
    }
    const text = asText(bytes);
    if (text === null) return run;
    changed = true;
    return text;
  });
  return changed ? out : null;
}

const DECODERS = [
  ["unescaped", unescapeJs],
  ["percent-decoded", percentDecode],
  ["entity-decoded", decodeEntities],
  ["base64-decoded", decodeBase64],
];

export const DECODED_LABELS = DECODERS.map(([label]) => label);

// decodedViews returns the decoded views of a text, plus the opaque reasons
// of any base64 blob in it that holds an archive.
export function decodedViews(text, depth = 0) {
  const views = [];
  const opaque = [];
  const lines = text.split("\n");
  const blobs = [];
  for (const [label, decode] of DECODERS) {
    const kept = [];
    const lineMap = [];
    for (let i = 0; i < lines.length; i += 1) {
      const decoded = decode(lines[i], (bytes) => blobs.push({ line: i + 1, bytes }));
      if (decoded === null || decoded === lines[i]) continue;
      kept.push(decoded);
      lineMap.push(i + 1);
    }
    if (kept.length) views.push({ label, text: kept.join("\n"), lines: true, lineMap });
  }
  for (const blob of blobs) {
    const label = `base64:${blob.line}`;
    if (depth >= MAX_NESTING) {
      opaque.push(`${label}-nested-too-deep`);
      continue;
    }
    const inner = contentViews(blob.bytes, "", depth + 1);
    for (const view of inner.views) {
      // The blob has no lines of its own in the file: report the file line.
      views.push({ label: view.label ? `${label}/${view.label}` : label, text: view.text, lines: false });
    }
    for (const reason of inner.opaque) opaque.push(`${label}/${reason}`);
  }
  return { views, opaque };
}

function textOrBinary(buffer) {
  if (isBinaryBuffer(buffer)) return null;
  try {
    return utf8.decode(buffer);
  } catch {
    return null;
  }
}

// contentViews expands one file into what the gate scans. Each view is
// { label, text, lines, lineMap? }: `lines` says whether line numbers mean
// anything in it, `lineMap` (decoded views) maps its lines to source lines,
// and `label` (null for the plain view) prefixes the reported line. `opaque`
// lists the reasons, if any, that part of the file could not be read.
export function contentViews(buffer, relPath = "", depth = 0) {
  const views = [];
  const opaque = [];
  const text = textOrBinary(buffer);
  if (text !== null) {
    views.push({ label: null, text, lines: true });
    const decoded = decodedViews(text, depth);
    views.push(...decoded.views);
    opaque.push(...decoded.opaque);
    return { views, opaque };
  }
  views.push({ label: null, text: buffer.toString("latin1"), lines: false });

  if (startsWith(buffer, GZIP_MAGIC)) {
    const out = inflate(zlib.gunzipSync, buffer);
    if (!out.buffer) {
      opaque.push(`gzip-${out.error}`);
    } else if (depth >= MAX_NESTING) {
      opaque.push("gzip-nested-too-deep");
    } else {
      // The inner file has no name of its own; only its magic number counts.
      const inner = contentViews(out.buffer, "", depth + 1);
      for (const view of inner.views) views.push({ ...view, label: view.label ? `gzip/${view.label}` : "gzip" });
      for (const reason of inner.opaque) opaque.push(`gzip/${reason}`);
    }
    return { views, opaque };
  }

  const format = opaqueFormat(buffer, relPath);
  if (format) {
    opaque.push(format);
    return { views, opaque };
  }

  if (startsWith(buffer, PNG_MAGIC)) {
    const { texts, failures } = pngTextChunks(buffer);
    for (const chunk of texts) {
      const inner = textOrBinary(chunk.buffer);
      views.push({ label: chunk.label, text: inner ?? chunk.buffer.toString("latin1"), lines: false });
    }
    opaque.push(...failures);
  }

  const order = utf16Order(buffer);
  if (order) {
    views.push({ label: `utf-16${order}`, text: decodeUtf16(buffer, order), lines: true });
  } else if (buffer.includes(0)) {
    views.push({ label: "nul-stripped", text: withoutNuls(buffer).toString("latin1"), lines: false });
  }

  const embedded = embeddedContent(buffer, depth);
  views.push(...embedded.views);
  opaque.push(...embedded.opaque);
  return { views, opaque };
}
