/**
 * @jest-environment node
 *
 * FeatureUnavailable links the self-hosting docs in the public repository.
 * Every target must exist in this tree (and must not be a path the OSS export
 * drops), otherwise the empty state links a 404.
 */
import fs from "fs";
import path from "path";
import { FEATURE_DOCS_URL } from "./featureGates";

const REPO_ROOT = path.resolve(__dirname, "../../../..");
const PREFIX = /^https:\/\/github\.com\/stdevMac\/payverge\/(?:blob|tree)\/main\//;

describe("FEATURE_DOCS_URL targets", () => {
  it.each(Object.entries(FEATURE_DOCS_URL))("%s docs exist in the public tree", (_f, url) => {
    expect(url).toMatch(PREFIX);
    const rel = String(url).replace(PREFIX, "");
    expect(rel.startsWith("docs/superpowers")).toBe(false);
    expect(fs.existsSync(path.join(REPO_ROOT, rel))).toBe(true);
  });
});
