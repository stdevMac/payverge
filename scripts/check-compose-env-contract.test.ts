import assert from "node:assert/strict";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import test from "node:test";

import {
  checkComposeContract,
  forwardsEnvFile,
  overlayBase,
  tryLoadYamlParser,
} from "./check-compose-env-contract";

const REQUIRED = [
  "JWT_SECRET_KEY",
  "PLUGIN_SECRET_KEY",
  "RPC_URL",
  "EMAIL_PROVIDER",
  "EMAIL_API_KEY",
  "FROM_EMAIL",
  "FROM_EMAIL_UPDATES",
  "S3_BUCKET",
  "AWS_ACCESS_KEY",
  "AWS_SECRET_KEY",
  "S3_PROTECTED_BUCKET",
  "AWS_PROTECTED_ACCESS_KEY",
  "AWS_PROTECTED_SECRET_KEY",
  "ALLOWED_ORIGINS",
  "COOKIE_DOMAIN",
  "TRUSTED_PLATFORM",
  "TRUSTED_PROXIES",
  "AI_DAILY_BUDGET_USD",
];

function compose(vars: readonly string[]): string {
  // List form, which the minimal fallback parser (used when no YAML library
  // is passed) understands.
  const env = vars.map((v) => `      - ${v}=\${${v}:-}`).join("\n");
  return `services:\n  backend:\n    image: example/backend\n    environment:\n${env}\n`;
}

function repo(files: Record<string, string>): string {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "compose-env-contract-"));
  for (const [rel, content] of Object.entries(files)) {
    fs.mkdirSync(path.dirname(path.join(root, rel)), { recursive: true });
    fs.writeFileSync(path.join(root, rel), content);
  }
  return root;
}

test("checks only the dev stack while deploy/ does not exist", () => {
  const root = repo({ "docker-compose.yml": compose(REQUIRED) });
  const { results, deployError } = checkComposeContract(root, null);
  assert.equal(deployError, null);
  assert.deepEqual(
    results.filter((r) => !r.skipped).map((r) => r.file),
    ["docker-compose.yml"],
  );
});

test("checks a self-host compose file one directory below deploy/", () => {
  const root = repo({
    "docker-compose.yml": compose(REQUIRED),
    "deploy/Caddyfile": ":80\n",
    "deploy/compose/docker-compose.yml": compose(REQUIRED.filter((v) => v !== "RPC_URL")),
  });
  const { results, deployError } = checkComposeContract(root, null);
  assert.equal(deployError, null);
  const deploy = results.find((r) => r.file === "deploy/compose/docker-compose.yml");
  assert.ok(deploy && !deploy.skipped);
  assert.deepEqual(deploy.missing, ["RPC_URL"]);
});

test("fails when deploy/ exists but no compose file there declares a backend", () => {
  const root = repo({
    "docker-compose.yml": compose(REQUIRED),
    "deploy/Caddyfile": ":80\n",
    "deploy/a/b/docker-compose.yml": compose(REQUIRED),
  });
  const { deployError } = checkComposeContract(root, null);
  assert.match(deployError ?? "", /deploy\/ exists but no compose file/);
});

// A self-host compose that hands the operator's .env to the backend through
// env_file forwards every variable in it, so the gate must not demand that each
// production-required name is also listed under environment.
const envFileCompose = (envFile: string): string =>
  `services:\n  backend:\n    image: example/backend\n${envFile}    environment:\n      - PORT=8080\n  frontend:\n    image: example/frontend\n`;

test("a mandatory env_file forwards every variable (fallback parser)", () => {
  for (const envFile of ["    env_file: .env\n", "    env_file:\n      - .env\n"]) {
    const root = repo({
      "docker-compose.yml": compose(REQUIRED),
      "deploy/docker-compose.yml": envFileCompose(envFile),
    });
    const { results, deployError } = checkComposeContract(root, null);
    assert.equal(deployError, null);
    const deploy = results.find((r) => r.file === "deploy/docker-compose.yml");
    assert.deepEqual(deploy, { file: "deploy/docker-compose.yml", missing: [], skipped: false });
  }
});

test("an optional env_file does not count as forwarding (YAML parser)", (t) => {
  const yamlLoad = tryLoadYamlParser();
  if (!yamlLoad) {
    t.skip("neither yaml nor js-yaml resolves from frontend/");
    return;
  }
  const optional = repo({
    "docker-compose.yml": compose(REQUIRED),
    "deploy/docker-compose.yml": envFileCompose(
      "    env_file:\n      - path: .env\n        required: false\n",
    ),
  });
  const deploy = checkComposeContract(optional, yamlLoad).results.find(
    (r) => r.file === "deploy/docker-compose.yml",
  );
  assert.ok(deploy && !deploy.skipped);
  assert.deepEqual(deploy.missing, REQUIRED);

  const mandatory = repo({
    "docker-compose.yml": compose(REQUIRED),
    "deploy/docker-compose.yml": envFileCompose("    env_file:\n      - path: .env\n"),
  });
  const ok = checkComposeContract(mandatory, yamlLoad).results.find(
    (r) => r.file === "deploy/docker-compose.yml",
  );
  assert.deepEqual(ok?.missing, []);
});

test("forwardsEnvFile accepts compose's short and long env_file forms", () => {
  assert.equal(forwardsEnvFile(".env"), true);
  assert.equal(forwardsEnvFile([".env", "extra.env"]), true);
  assert.equal(forwardsEnvFile([{ path: ".env" }]), true);
  assert.equal(forwardsEnvFile([{ path: ".env", required: true }]), true);
  assert.equal(forwardsEnvFile([{ path: ".env", required: false }]), false);
  assert.equal(forwardsEnvFile(undefined), false);
  assert.equal(forwardsEnvFile(""), false);
  assert.equal(forwardsEnvFile([]), false);
});

// deploy/demo/docker-compose.demo.yml only adds DEMO_* settings to the backend
// and is always layered after deploy/docker-compose.yml, so it is checked as
// compose runs it: merged onto that base, not as a standalone stack.
const overlay = (vars: readonly string[]): string =>
  `services:\n  backend:\n    environment:\n${vars.map((v) => `      - ${v}=x`).join("\n")}\n`;

test("an overlay is checked merged onto the nearest base compose file", () => {
  for (const yamlLoad of [null, tryLoadYamlParser()]) {
    const root = repo({
      "docker-compose.yml": compose(REQUIRED),
      "deploy/docker-compose.yml": compose(REQUIRED),
      // Overriding a forwarded variable with an empty value still forwards it.
      "deploy/demo/docker-compose.demo.yml": overlay(["DEMO_MODE", "EMAIL_API_KEY"]),
    });
    const { results, deployError } = checkComposeContract(root, yamlLoad);
    assert.equal(deployError, null);
    assert.deepEqual(
      results.find((r) => r.file === "deploy/demo/docker-compose.demo.yml"),
      {
        file: "deploy/demo/docker-compose.demo.yml",
        missing: [],
        skipped: false,
        mergedWith: "deploy/docker-compose.yml",
      },
    );
  }
});

test("an overlay still fails when neither it nor its base forwards a variable", () => {
  const root = repo({
    "docker-compose.yml": compose(REQUIRED),
    "deploy/docker-compose.yml": compose(REQUIRED.filter((v) => v !== "RPC_URL")),
    "deploy/demo/docker-compose.demo.yml": overlay(["DEMO_MODE"]),
  });
  const demo = checkComposeContract(root, null).results.find(
    (r) => r.file === "deploy/demo/docker-compose.demo.yml",
  );
  assert.deepEqual(demo?.missing, ["RPC_URL"]);

  // The overlay can supply what the base lacks.
  const fixed = repo({
    "docker-compose.yml": compose(REQUIRED),
    "deploy/docker-compose.yml": compose(REQUIRED.filter((v) => v !== "RPC_URL")),
    "deploy/demo/docker-compose.demo.yml": overlay(["RPC_URL"]),
  });
  const ok = checkComposeContract(fixed, null).results.find(
    (r) => r.file === "deploy/demo/docker-compose.demo.yml",
  );
  assert.deepEqual(ok?.missing, []);
});

test("an overlay-named file with no base is checked on its own", () => {
  const root = repo({
    "docker-compose.yml": compose(REQUIRED),
    "deploy/demo/docker-compose.demo.yml": overlay(["DEMO_MODE"]),
  });
  const demo = checkComposeContract(root, null).results.find(
    (r) => r.file === "deploy/demo/docker-compose.demo.yml",
  );
  assert.ok(demo && !demo.skipped && demo.mergedWith === undefined);
  assert.deepEqual(demo.missing, REQUIRED);
});

test("overlayBase resolves only docker-compose.<variant>.yml under deploy/", () => {
  const root = repo({
    "deploy/docker-compose.yml": compose(REQUIRED),
    "deploy/platforms/coolify/docker-compose.yml": compose(REQUIRED),
  });
  assert.equal(overlayBase(root, "deploy/docker-compose.build.yml"), "deploy/docker-compose.yml");
  assert.equal(overlayBase(root, "deploy/demo/docker-compose.demo.yml"), "deploy/docker-compose.yml");
  assert.equal(
    overlayBase(root, "deploy/platforms/coolify/docker-compose.x.yml"),
    "deploy/platforms/coolify/docker-compose.yml",
  );
  assert.equal(overlayBase(root, "deploy/docker-compose.yml"), null);
  assert.equal(overlayBase(root, "docker-compose.prod-local.yml"), null);
});
