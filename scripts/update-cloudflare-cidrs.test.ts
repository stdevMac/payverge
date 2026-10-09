import assert from "node:assert/strict";
import test from "node:test";

import {
  formatCloudflareCidrsCaddy,
  parseCloudflareIpList,
  validateCloudflareSnapshot,
} from "./update-cloudflare-cidrs";

const ipv4 = ["192.0.2.0/24"];
const ipv6 = ["2001:db8::/32"];
const generatedAt = "2026-08-01T00:00:00Z";

test("accepts a current snapshot with the published CIDR hash", () => {
  const snapshot = formatCloudflareCidrsCaddy({ ipv4, ipv6, generatedAt });
  assert.doesNotThrow(() =>
    validateCloudflareSnapshot(
      snapshot,
      ipv4,
      ipv6,
      new Date("2026-08-15T00:00:00Z"),
      45,
    ),
  );
});

test("rejects changed Cloudflare CIDRs", () => {
  const snapshot = formatCloudflareCidrsCaddy({ ipv4, ipv6, generatedAt });
  assert.throws(
    () =>
      validateCloudflareSnapshot(
        snapshot,
        ["198.51.100.0/24"],
        ipv6,
        new Date("2026-08-15T00:00:00Z"),
        45,
      ),
    /snapshot hash does not match Cloudflare/,
  );
});

test("rejects a tampered trusted-proxy body even when the header hash remains", () => {
  const snapshot = formatCloudflareCidrsCaddy({ ipv4, ipv6, generatedAt }).replace(
    "192.0.2.0/24",
    "203.0.113.0/24",
  );
  assert.throws(
    () =>
      validateCloudflareSnapshot(
        snapshot,
        ipv4,
        ipv6,
        new Date("2026-08-15T00:00:00Z"),
        45,
      ),
    /snapshot body does not match its content hash/,
  );
});

test("rejects a snapshot past the review window", () => {
  const snapshot = formatCloudflareCidrsCaddy({ ipv4, ipv6, generatedAt });
  assert.throws(
    () =>
      validateCloudflareSnapshot(
        snapshot,
        ipv4,
        ipv6,
        new Date("2026-09-16T00:00:01Z"),
        45,
      ),
    /snapshot is older than 45 days/,
  );
});

test("rejects malformed provider input before it reaches the trust snapshot", () => {
  assert.throws(() => parseCloudflareIpList("not-a-cidr\n"), /invalid Cloudflare CIDR/);
});
