package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// knownPublicSecretDigests holds SHA-256 digests (hex) of secret values that
// have been committed to this repository: compose defaults, .env examples,
// CI fixtures, and the retired PLUGIN_SECRET_KEY fallback. The source tree is
// public, so every one of them is known to attackers and must never
// authenticate a production deploy.
//
// Digests rather than literals keep secret scanners quiet and avoid
// re-publishing the values a second time. TestKnownUnsafeSecretsCoverRepoLiterals
// parses the tracked env, compose, workflow, script and Makefile sources and
// fails (printing the digest to add, never the value) when a new literal
// appears without being denylisted.
var knownPublicSecretDigests = map[string]string{
	// deploy/.env.example commented AWS_ACCESS_KEY example (Garage/MinIO key id).
	"fe25b490262670f6a4e25ef903d475cc1d1667241685fa1c24f7e775b36d49f9": "deploy env example AWS_ACCESS_KEY",
	// docker-compose.yml JWT_SECRET_KEY default (local dev).
	"5682f06db169a9704b0b650f2cc10f02b082ee84129cf0dd3e27e864aaa87e1e": "compose dev JWT_SECRET_KEY",
	// docker-compose.prod-local.yml JWT_SECRET_KEY default.
	"b07d99ac713a14cc734fb10343997550bcb231d7b08a3b51cdd96dd733408778": "prod-local JWT_SECRET_KEY",
	// docker-compose*.yml / perf staging DB_PASSWORD default.
	"8c63f49197c63ef88f9140993e2f2ae2c73f9185c2821c002566f9202c69b63b": "compose DB_PASSWORD",
	// .env.example LOCAL_ADMIN_PASSWORD (prod-local seeded admin).
	"843d44abd138c932bc24dafc24215eb627ed60de9241048738d7cf35fed70b48": "prod-local admin password",

	// .github/e2e/ci.env.fixture fixtures.
	"9402302ab4e89dab8125ade1d1ae3150d7b252a82b222ed276b3b2473793a9af": "CI DB_PASSWORD",
	"d799da0a8a53b78967d111fde3c70a155b5f828b4ee8d821f742ca0a0d5673a3": "CI JWT_SECRET_KEY",
	"28e8e388984cd8154dc0e84f919e1a5fa2eafe74051161bb45d40781d058ab97": "CI PLUGIN_SECRET_KEY",
	// Regenerated CI-only keys (the originals above stay public in history).
	"bd3dafbee7ce33a6ff3cfaf0119f338561cef3d1bcc0a5d954bddc3b641d34ec": "CI JWT_SECRET_KEY (regenerated)",
	"fac976b94b1be0368df0047d43b2bb53dcf2969b2d5c78427c9c2bd27544dcb6": "CI PLUGIN_SECRET_KEY (regenerated)",
	"a7ab210a4884a3549acae0ab9046ec78ed420ac2d577a48a5d2d50225f2ae15a": "CI AWS_ACCESS_KEY",
	"1979821c786f94f92bc574b08ab25aa331fe271e4831f1869912aea27b09c664": "CI AWS_SECRET_KEY",
	"d84d5531cc1fa80e0be6de506c989fdf5fc1a16d0f85012240d0f547e0bc0eac": "CI AWS_PROTECTED_ACCESS_KEY",
	"3ed60c61faa4557fa289dbcffaebfa385143bd65457fe90de5d1e85ee2bb7274": "CI AWS_PROTECTED_SECRET_KEY",
	"3918aac381ab690a6234ac1edcdf751497fb728da1bca2ee47df756c1c262fed": "CI EMAIL_API_KEY",
	"e0c8c825927ae7118861981df607febc4d6c4d5368aac1cca206cf26c713d1f0": "CI METRICS_TOKEN",

	// scripts/verify-backend.sh hermetic test PLUGIN_SECRET_KEY (valid 32-byte raw key).
	"adb4d2f0c72c1f71458bad67b5b1b6b8a664737e3647545ef83d5369cfe5bd39": "hermetic test PLUGIN_SECRET_KEY",

	// scripts/validate-production-config.test.mjs release fixture.
	"128edc14c328adb8904f8d4aa111bd6a05200e400b44024f101515f73a8dece2": "release-config test DB_PASSWORD",
	"2692f68ac2ad35074dcf7bf514afc98ee9a91d9ace9edb46adbf1838f91d6358": "release-config test JWT_SECRET_KEY",
	// The sequential 32-character hex string: a valid raw 32-byte key, and the
	// key material of the base64 fixture most tests use.
	"3eb1bd439947eb762998e566ccc2e099c791118b2f40579cc4f7da2b5061b7f9": "release-config test PLUGIN_SECRET_KEY (sequential hex)",
	"a8de72a81b027f8fe7611f517b8a1a357294ff463a3df3b17b91e5b1c08e4774": "release-config test AWS_ACCESS_KEY",
	"42800fc1f52e653b7c709dd068f4c31ec0ff246c276d318645eb600398bb14c4": "release-config test AWS_SECRET_KEY",
	"c070d8fc3a7ec2268aeffde1967f961e5de72bed4a70836eee8964c61bce2b2f": "release-config test AWS_PROTECTED_ACCESS_KEY",
	"8d372025a2512cc955e365b39b911d2a31f09ac25a7e5857449bb8a4b2076857": "release-config test AWS_PROTECTED_SECRET_KEY",
	"055efce4288c472a5c7f2ba8261de6d838b5faaa4133732bd811216b49199041": "release-config test EMAIL_API_KEY",

	// Workflow, drill and script fixtures.
	"9efdd258fc5064bd7d1db1f2e07db1178e2ab14664c28f82f9363c55e7eb9253": "perf-bench workflow JWT_SECRET_KEY",
	"6ce2cf97b95063e096d06d9683da8437c124a6673796a14c9844143d7f47ce0c": "restore drill JWT_SECRET_KEY",
	"a37ef283bc169c0ad4242e196cdcca9d54b94a2fee2ea35f65e2b690dff30c04": "restore drill PG_PASSWORD",
	"aeebad4a796fcc2e15dc4c6061b45ed9b373f26adfc798ca7d2d8cc58182718e": "genesis-schema scratch POSTGRES_PASSWORD",
	"9caf06bb4436cdbfa20af9121a626bc1093c4f54b31c0fa937957856135345b6": "prod-local test BACKUP_AWS_SECRET_ACCESS_KEY",
	"cfefc3cb8f22b1d0dcbe7c6e6bda2b6f12d452e9bd8d8c4fdbc7feba8be4ae28": "prod-local test JWT_SECRET_KEY",
	"4c5dc9b7708905f77f5e5d16316b5dfb425e68cb326dcd55a860e90a7707031e": "script test GH_TOKEN / TELEGRAM_ESCALATION_BOT_TOKEN",
	"442cbf3fdf0f24777036e660c32d6e4b5f7621231bff2754673c5a188a2f614a": "QA automation env example QA_PASSWORD",

	// Markdown examples (READMEs, runbooks, plans, the benchmark log), pinned
	// by TestKnownUnsafeSecretsCoverDocExamples.
	"eb26d4a9a0ef04727f52fb5acc8936196ae90e23003fc5117ce8e3e28c49363f": "benchmark docs JWT_SECRET_KEY",
	"2ceac6f36363c6246a64cca805cd43ca7a01b14eb2fcc532ceec3f60f2f7df1c": "plan test-command JWT_SECRET_KEY",
	"17e71769c39df052b2eb1105defa820818db4575d793033234866836c08c8bd5": "security-hardening docs JWT_SECRET_KEY (36 chars)",

	// Retired PLUGIN_SECRET_KEY dev fallback: the 32-byte seed string (a valid
	// raw key on its own) and sha256(seed) as raw bytes, base64, and unpadded
	// base64.
	"14072cbdf504ee86de90a5af0d1c8fb71eeb7d1b78ead61f3eca70a32cd9ad6f": "legacy plugin key seed",
	"a720375ea53d8d5f89093e3cbadba5c714ed8aa03a4789e7cbd662e71444faf0": "legacy plugin key (raw)",
	"cb0d8c607d7fbfbe53ffbf9d746091e95f3183b0879dae088180c79343e1ce7a": "legacy plugin key (base64)",
	"68142f87127e2dbabc2e5ab967518b2058c9cae821c713ec9f1ef6e606ffad2c": "legacy plugin key (base64, unpadded)",
}

// knownPublicSecretPrefixes covers whole families of committed dev defaults
// (e.g. payverge_local_dev_jwt_secret_2026 and its _min32 variant) so a
// suffix tweak does not slip past the digest list.
var knownPublicSecretPrefixes = []string{
	"payverge_local_dev_",
	"payverge_ci_",
	"ci_minio_",
	"ci_metrics_token_",
	"ci-stub-",
}

// trivialSecrets are generic values that are unsafe for any credential.
var trivialSecrets = map[string]struct{}{
	"admin":             {},
	"password":          {},
	"payverge":          {},
	"payverge_password": {},
	"postgres":          {},
	"pw":                {},
	"root":              {},
	"secret":            {},
	"test":              {},
}

// IsKnownUnsafeSecret reports whether a credential value is publicly known:
// a placeholder from an example file, a default committed to this repository
// (compose, prod-local, CI fixtures), the retired PLUGIN_SECRET_KEY fallback,
// or a trivial word such as "password". Empty input returns false —
// requiredness is the caller's concern.
//
// Use it for every operator-supplied secret that guards production data
// (JWT_SECRET_KEY, PLUGIN_SECRET_KEY, DB_PASSWORD, ADMIN_PASSWORD, ...).
func IsKnownUnsafeSecret(raw string) bool {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false
	}
	if isPlaceholderValue(value) || isTemplatePlaceholder(value) {
		return true
	}
	lower := strings.ToLower(value)
	if _, ok := trivialSecrets[lower]; ok {
		return true
	}
	for _, prefix := range knownPublicSecretPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	sum := sha256.Sum256([]byte(value))
	_, ok := knownPublicSecretDigests[hex.EncodeToString(sum[:])]
	return ok
}

// isTemplatePlaceholder reports a documentation placeholder such as
// <password> or <at-least-32-random-bytes>: a value copied verbatim from a
// README instead of being generated. Scoped to secrets only; isPlaceholderValue
// stays free of it because an SMTP_FROM of <noreply@example.com> is a valid
// address.
func isTemplatePlaceholder(value string) bool {
	return len(value) >= 2 && strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")
}

// IsKnownUnsafePluginKey reports whether a PLUGIN_SECRET_KEY is publicly
// known either as written or by the 32-byte key it decodes to. The same AES
// key can be spelled raw, base64, unpadded base64 or hex; every spelling of a
// published key is refused.
func IsKnownUnsafePluginKey(raw string) bool {
	if IsKnownUnsafeSecret(raw) {
		return true
	}
	key, ok := pluginKeyMaterial(raw)
	if !ok {
		return false
	}
	sum := sha256.Sum256(key)
	_, known := knownPublicSecretDigests[hex.EncodeToString(sum[:])]
	return known
}

// pluginKeyMaterial decodes a PLUGIN_SECRET_KEY the way the cipher does:
// standard or unpadded base64 of 32 bytes, else exactly 32 raw bytes, else
// 64 hex characters (security.decodePluginKey checks the same order).
func pluginKeyMaterial(raw string) ([]byte, bool) {
	raw = strings.TrimSpace(raw)
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		if decoded, err := encoding.DecodeString(raw); err == nil && len(decoded) == 32 {
			return decoded, true
		}
	}
	if len(raw) == 32 {
		return []byte(raw), true
	}
	if len(raw) == 64 {
		if decoded, err := hex.DecodeString(raw); err == nil {
			return decoded, true
		}
	}
	return nil, false
}
