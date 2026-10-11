/**
 * A minimal, deterministic, STORE-only ZIP writer.
 *
 * Why hand-rolled. A campaign kit is up to seven files, and the existing export
 * path (`downloadBlob` in ../postContent.ts) is one `<a download>` click per
 * file — browsers block the second or third in a single user gesture, so a
 * multi-file kit needs one archive. `frontend/package.json` declares no archive
 * library. `fflate` is in the lockfile, but only as a transitive of `jspdf`;
 * promoting an accidental transitive to a direct dependency ties the build to a
 * version another package chose and can drop on any bump.
 *
 * Why store-only. PNG is already DEFLATE-compressed, so a second pass buys
 * essentially nothing on the bulk of a kit; the HTML and caption members are a
 * few kilobytes. Storing removes the entire compressor, the only part of a ZIP
 * writer with interesting failure modes, and keeps the output byte-exact.
 *
 * Why deterministic. Every entry gets the same fixed DOS timestamp rather than
 * `Date.now()`, so the same kit produces the same bytes on every run. That is
 * what lets `zip.test.ts` assert on the archive at all.
 *
 * Scope, stated so nobody assumes more: no ZIP64 (an archive or member over 4 GB
 * is not a thing this feature can produce), no encryption, no directory entries,
 * no data descriptors (sizes are known before the header is written, so none are
 * needed).
 */

/** Local file header. */
const SIG_LOCAL = 0x04034b50;
/** Central directory file header. */
const SIG_CENTRAL = 0x02014b50;
/** End of central directory record. */
const SIG_EOCD = 0x06054b50;

/** PKZIP 2.0 — the floor for the UTF-8 name flag below. */
const VERSION = 20;

/** Bit 11: filenames and comments are UTF-8. */
const FLAG_UTF8 = 0x0800;

/** Compression method 0 — stored, no compression. */
const METHOD_STORE = 0;

/**
 * A fixed DOS date/time: 1980-01-01 00:00:00, the earliest the format can
 * express. Real clock time would make identical input produce different bytes,
 * which defeats both the tests and any downstream content hashing.
 */
const DOS_TIME = 0;
const DOS_DATE = (0 << 9) | (1 << 5) | 1;

export interface ZipEntry {
  /** Archive member name. Forward slashes only; no leading slash. */
  name: string;
  data: Uint8Array;
}

/**
 * CRC-32/ISO-HDLC, the checksum ZIP requires, computed with the standard
 * reflected table. Exported because the tests assert the value written into each
 * header, and a checksum nobody can independently compute is a checksum nobody
 * can verify.
 */
const CRC_TABLE = (() => {
  const table = new Uint32Array(256);
  for (let index = 0; index < 256; index += 1) {
    let value = index;
    for (let bit = 0; bit < 8; bit += 1) {
      value = value & 1 ? 0xedb88320 ^ (value >>> 1) : value >>> 1;
    }
    table[index] = value >>> 0;
  }
  return table;
})();

export function crc32(data: Uint8Array): number {
  let crc = 0xffffffff;
  for (let index = 0; index < data.length; index += 1) {
    crc = CRC_TABLE[(crc ^ data[index]) & 0xff] ^ (crc >>> 8);
  }
  return (crc ^ 0xffffffff) >>> 0;
}

/** Little-endian writer over a fixed-size buffer. */
class Writer {
  private readonly bytes: Uint8Array;
  private offset = 0;

  constructor(size: number) {
    this.bytes = new Uint8Array(size);
  }

  u16(value: number): void {
    this.bytes[this.offset++] = value & 0xff;
    this.bytes[this.offset++] = (value >>> 8) & 0xff;
  }

  u32(value: number): void {
    this.bytes[this.offset++] = value & 0xff;
    this.bytes[this.offset++] = (value >>> 8) & 0xff;
    this.bytes[this.offset++] = (value >>> 16) & 0xff;
    this.bytes[this.offset++] = (value >>> 24) & 0xff;
  }

  raw(data: Uint8Array): void {
    this.bytes.set(data, this.offset);
    this.offset += data.length;
  }

  get position(): number {
    return this.offset;
  }

  finish(): Uint8Array {
    return this.bytes;
  }
}

interface PreparedEntry {
  nameBytes: Uint8Array;
  data: Uint8Array;
  crc: number;
  localOffset: number;
}

export function buildZip(entries: readonly ZipEntry[]): Uint8Array {
  const encoder = new TextEncoder();
  const prepared: PreparedEntry[] = [];

  const LOCAL_HEADER = 30;
  const CENTRAL_HEADER = 46;
  const EOCD = 22;

  let localSize = 0;
  let centralSize = 0;
  entries.forEach((entry) => {
    const nameBytes = encoder.encode(entry.name);
    prepared.push({
      nameBytes,
      data: entry.data,
      crc: crc32(entry.data),
      localOffset: localSize,
    });
    localSize += LOCAL_HEADER + nameBytes.length + entry.data.length;
    centralSize += CENTRAL_HEADER + nameBytes.length;
  });

  const writer = new Writer(localSize + centralSize + EOCD);

  prepared.forEach((entry) => {
    writer.u32(SIG_LOCAL);
    writer.u16(VERSION);
    writer.u16(FLAG_UTF8);
    writer.u16(METHOD_STORE);
    writer.u16(DOS_TIME);
    writer.u16(DOS_DATE);
    writer.u32(entry.crc);
    writer.u32(entry.data.length); // compressed size == uncompressed, stored
    writer.u32(entry.data.length);
    writer.u16(entry.nameBytes.length);
    writer.u16(0); // extra field length
    writer.raw(entry.nameBytes);
    writer.raw(entry.data);
  });

  const centralOffset = writer.position;

  prepared.forEach((entry) => {
    writer.u32(SIG_CENTRAL);
    writer.u16(VERSION); // version made by
    writer.u16(VERSION); // version needed to extract
    writer.u16(FLAG_UTF8);
    writer.u16(METHOD_STORE);
    writer.u16(DOS_TIME);
    writer.u16(DOS_DATE);
    writer.u32(entry.crc);
    writer.u32(entry.data.length);
    writer.u32(entry.data.length);
    writer.u16(entry.nameBytes.length);
    writer.u16(0); // extra field length
    writer.u16(0); // file comment length
    writer.u16(0); // disk number start
    writer.u16(0); // internal file attributes
    writer.u32(0); // external file attributes
    writer.u32(entry.localOffset);
    writer.raw(entry.nameBytes);
  });

  writer.u32(SIG_EOCD);
  writer.u16(0); // this disk
  writer.u16(0); // disk with the central directory
  writer.u16(prepared.length);
  writer.u16(prepared.length);
  writer.u32(centralSize);
  writer.u32(centralOffset);
  writer.u16(0); // archive comment length

  return writer.finish();
}
