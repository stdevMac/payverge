import { isTrustedProxy, trustedProxyIssues } from "./trustedProxies";

const env = (value?: string) => ({ FRONTEND_TRUSTED_PROXIES: value });

describe("isTrustedProxy", () => {
  it("trusts nothing by default", () => {
    expect(isTrustedProxy("127.0.0.1", {})).toBe(false);
    expect(isTrustedProxy("172.18.0.1", env(""))).toBe(false);
    expect(trustedProxyIssues({})).toEqual([]);
  });

  it("matches IPv4/IPv6 CIDRs and single addresses", () => {
    const trust = env("10.0.0.0/8, 127.0.0.1 ,fd00::/8");
    expect(isTrustedProxy("10.200.3.4", trust)).toBe(true);
    expect(isTrustedProxy("::ffff:10.200.3.4", trust)).toBe(true);
    expect(isTrustedProxy("127.0.0.1", trust)).toBe(true);
    expect(isTrustedProxy("127.0.0.2", trust)).toBe(false);
    expect(isTrustedProxy("fd12::1", trust)).toBe(true);
    expect(isTrustedProxy("2001:db8::1", trust)).toBe(false);
    expect(isTrustedProxy("11.0.0.1", trust)).toBe(false);
    expect(isTrustedProxy("not-an-ip", trust)).toBe(false);
  });

  it("ignores invalid and unbounded entries, naming positions not values", () => {
    const trust = env("10.0.0.0/33,proxy.internal,0.0.0.0/0,::/0,1.2.3.4/8/9,192.168.0.0/16");
    expect(isTrustedProxy("8.8.8.8", trust)).toBe(false);
    expect(isTrustedProxy("192.168.4.4", trust)).toBe(true);
    const issues = trustedProxyIssues(trust);
    expect(issues).toEqual([
      "FRONTEND_TRUSTED_PROXIES: entry 1 is not an IP or CIDR (ignored)",
      "FRONTEND_TRUSTED_PROXIES: entry 2 is not an IP or CIDR (ignored)",
      "FRONTEND_TRUSTED_PROXIES: entry 3 trusts every address (ignored)",
      "FRONTEND_TRUSTED_PROXIES: entry 4 trusts every address (ignored)",
      "FRONTEND_TRUSTED_PROXIES: entry 5 is not an IP or CIDR (ignored)",
    ]);
    expect(issues.join(" ")).not.toMatch(/proxy\.internal|10\.0\.0\.0/);
  });
});
