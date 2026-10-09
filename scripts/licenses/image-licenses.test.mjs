// Contract for the copyleft licence texts the release ships:
// node --test scripts/licenses/image-licenses.test.mjs
//
// The backend links go-ethereum library packages and the frontend image ships
// the sharp-libvips shared libraries, both LGPL-3.0-or-later. LGPL-3.0 section 4
// requires conveying a copy of the LGPL and of the GPL it incorporates, so the
// repository and both container images must carry the verbatim texts.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const read = (...p) => readFileSync(path.join(ROOT, ...p));

// SHA-256 of the unmodified FSF texts (gnu.org/licenses/{gpl,lgpl}-3.0.txt).
const TEXTS = {
  "GPL-3.0.txt": {
    sha256: "3972dc9744f6499f0f9b2dbf76696f2ae7ad8af9b23dde66d6af86c9dfb36986",
    title: "GNU GENERAL PUBLIC LICENSE",
  },
  "LGPL-3.0.txt": {
    sha256: "da7eabb7bafdf7d3ae5e9f223aa5bdc1eece45ac569dc21b3b037520b4464768",
    title: "GNU LESSER GENERAL PUBLIC LICENSE",
  },
};

test("LICENSES/ holds the verbatim GPL-3.0 and LGPL-3.0 texts", () => {
  for (const [name, { sha256, title }] of Object.entries(TEXTS)) {
    const body = read("LICENSES", name);
    assert.equal(body.toString("utf8").split("\n")[0].trim(), title, `LICENSES/${name} title`);
    assert.equal(createHash("sha256").update(body).digest("hex"), sha256, `LICENSES/${name} must be the unmodified FSF text`);
  }
});

test("both images copy LICENSES/ and declare the LGPL in their licence label", () => {
  for (const image of ["backend", "frontend"]) {
    for (const name of Object.keys(TEXTS)) {
      assert.ok(read(image, "LICENSES", name).equals(read("LICENSES", name)), `${image}/LICENSES/${name} must be a byte-identical copy of LICENSES/${name}`);
    }
    const dockerfile = read(image, "Dockerfile").toString("utf8");
    assert.match(dockerfile, /^COPY (--chown=\S+ )?LICENSES\/ \/usr\/share\/doc\/payverge\/LICENSES\/$/m, `${image}/Dockerfile must copy LICENSES/`);
    assert.match(dockerfile, /org\.opencontainers\.image\.licenses="Apache-2\.0 AND LGPL-3\.0-or-later"/, `${image}/Dockerfile licence label`);
  }
});

test("NOTICE points at the shipped licence texts", () => {
  assert.match(read("NOTICE").toString("utf8"), /LICENSES\/\s*\(LGPL-3\.0\.txt, GPL-3\.0\.txt\)/);
});
