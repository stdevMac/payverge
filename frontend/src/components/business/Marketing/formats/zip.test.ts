import { buildZip, crc32, type ZipEntry } from "./zip";

const encoder = new TextEncoder();

function u32(bytes: Uint8Array, offset: number): number {
  return (
    (bytes[offset] |
      (bytes[offset + 1] << 8) |
      (bytes[offset + 2] << 16) |
      (bytes[offset + 3] << 24)) >>>
    0
  );
}

function u16(bytes: Uint8Array, offset: number): number {
  return bytes[offset] | (bytes[offset + 1] << 8);
}

const ENTRIES: ZipEntry[] = [
  { name: "post-4x5.png", data: encoder.encode("PNGDATA") },
  { name: "tent-5x7.html", data: encoder.encode("<!doctype html><p>hi</p>") },
  { name: "caption.txt", data: encoder.encode("Milanesa night") },
];

describe("crc32", () => {
  // The three standard CRC-32/ISO-HDLC check values.
  it("matches known vectors", () => {
    expect(crc32(encoder.encode(""))).toBe(0x00000000);
    expect(crc32(encoder.encode("a"))).toBe(0xe8b7be43);
    expect(crc32(encoder.encode("123456789"))).toBe(0xcbf43926);
  });
});

describe("buildZip", () => {
  it("starts with a local file header", () => {
    const bytes = buildZip(ENTRIES);
    expect(u32(bytes, 0)).toBe(0x04034b50);
  });

  it("stores rather than deflates, so sizes match exactly", () => {
    const bytes = buildZip([ENTRIES[0]]);
    expect(u16(bytes, 8)).toBe(0); // compression method 0 = store
    expect(u32(bytes, 18)).toBe(ENTRIES[0].data.length); // compressed size
    expect(u32(bytes, 22)).toBe(ENTRIES[0].data.length); // uncompressed size
    expect(u32(bytes, 14)).toBe(crc32(ENTRIES[0].data));
  });

  it("flags names as UTF-8", () => {
    const bytes = buildZip(ENTRIES);
    expect(u16(bytes, 6) & 0x0800).toBe(0x0800);
  });

  it("ends with an end-of-central-directory record naming every entry", () => {
    const bytes = buildZip(ENTRIES);
    const eocd = bytes.length - 22;
    expect(u32(bytes, eocd)).toBe(0x06054b50);
    expect(u16(bytes, eocd + 8)).toBe(ENTRIES.length);
    expect(u16(bytes, eocd + 10)).toBe(ENTRIES.length);
    const cdSize = u32(bytes, eocd + 12);
    const cdOffset = u32(bytes, eocd + 16);
    expect(cdOffset + cdSize).toBe(eocd);
    expect(u32(bytes, cdOffset)).toBe(0x02014b50);
  });

  it("writes every file's bytes verbatim", () => {
    const bytes = buildZip(ENTRIES);
    const text = new TextDecoder().decode(bytes);
    expect(text).toContain("PNGDATA");
    expect(text).toContain("<!doctype html><p>hi</p>");
    expect(text).toContain("Milanesa night");
    expect(text).toContain("post-4x5.png");
  });

  // Determinism is what makes the archive testable at all — a timestamp would
  // make every run's bytes differ.
  it("produces identical bytes for identical input", () => {
    expect(Array.from(buildZip(ENTRIES))).toEqual(Array.from(buildZip(ENTRIES)));
  });

  it("handles an empty archive", () => {
    const bytes = buildZip([]);
    expect(bytes).toHaveLength(22);
    expect(u32(bytes, 0)).toBe(0x06054b50);
  });

  it("handles non-ASCII names", () => {
    const bytes = buildZip([
      { name: "milanesa-ñoquis.png", data: encoder.encode("x") },
    ]);
    expect(new TextDecoder().decode(bytes)).toContain("milanesa-ñoquis.png");
  });
});
