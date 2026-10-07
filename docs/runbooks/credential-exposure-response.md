# Credential exposure response

Use this runbook when the current-tree or full-history scanner reports a
credential-shaped value, or when a credential-bearing file may have left its
approved secret store. The security owner coordinates the response, and new
sign-ups stay closed (`REGISTRATION_MODE=closed`) until every exit criterion is
met.

## Non-negotiable handling rules

- Never record, paste, print, hash, or attach the secret value in issues, logs,
  evidence bundles, chat, shell history, screenshots, or commits.
- Treat the scanner's rule, commit, and path as sufficient location evidence.
- Rotate or revoke the affected credential before any Git history rewrite.
- Do not use `git show`, `git cat-file`, a database viewer, or a text editor to
  recover the historical value.
- A shared-history rewrite is destructive coordination work. It requires the
  repository administrator's explicit approval and a scheduled freeze.

## 1. Contain and classify

1. Close sign-ups (`REGISTRATION_MODE=closed`) and disable the affected
   integration if its active status cannot be established safely.
2. Open a restricted security incident containing only:
   - scanner rule;
   - commit and path;
   - discovery time;
   - incident owner;
   - affected provider/environment, if known without reading the value.
3. Run the scanners from a clean, current checkout:

   ```bash
   node scripts/scan-secrets.mjs
   node scripts/scan-secrets.mjs --history
   ```

4. Determine exposure scope from provider audit logs, repository access logs,
   backups, CI artifacts, forks, and clones. Do not infer safety from deletion
   in the current branch.

## 2. Rotate and prove revocation

Perform these operations through the provider console and the production secret
facility; never pass a credential on a command line.

1. Create a replacement with the minimum required scope.
2. Write it to the canonical secret-store entry and record only the provider
   key identifier and secret-store version identifier.
3. Restart or roll forward the affected service using the exact candidate
   digest.
4. Run the provider's protected synthetic and retain the sanitized request/run
   identifier.
5. Revoke the prior key in the provider console.
6. Prove the replacement succeeds and the superseded credential fails. Valid
   evidence is a provider status/audit record plus a successful protected
   synthetic for the replacement; never retry by extracting the old value from
   Git history.
7. Rotate downstream webhook/signing material too if the provider's exposure
   model makes it necessary.

If any step fails, disable the integration, keep sign-ups closed, and escalate
to the security and release owners.

## 3. Prepare the shared-history rewrite

History removal reduces future accidental disclosure but does not replace
rotation. Start only after revocation proof and explicit repository-admin
approval.

1. Announce a push freeze and enumerate protected branches, tags, forks, open
   pull requests, cached CI artifacts, and deployment clones.
2. Create a restricted, offline recovery bundle of the pre-rewrite refs. Record
   its encrypted storage location and retention deadline, not its contents.
3. In a disposable mirror clone, remove the exact path with `git filter-repo`,
   replacing `<exposed-path>` with the path the scanner reported:

   ```bash
   git filter-repo \
     --path <exposed-path> \
     --invert-paths --force
   ```

4. Before pushing anything, verify every rewritten branch and tag, run the full
   repository gates, and obtain a second-person review of the ref mapping.
5. The repository administrator force-updates the approved refs and invalidates
   cached artifacts. Contributors must discard old clones and clone afresh;
   merging an old branch can reintroduce the object.

Abort before the force-update if ref coverage, recovery-bundle integrity, or
review is incomplete. After the force-update, restoration of old refs would
reintroduce the credential and is an incident decision, not a routine rollback.

## 4. Verify and close

From a brand-new clone of the rewritten remote:

```bash
node scripts/scan-secrets.mjs --history
git log --all -- <exposed-path>
```

The scanner must pass and the path-specific log must be empty. Also verify:

- protected CI/security workflows pass on the rewritten release SHA;
- the replacement provider synthetic passes;
- provider evidence shows the superseded credential is revoked;
- CI artifacts, mirrors, forks, and deployment clones have been remediated;
- the incident has an exposure decision and follow-up owner.

Attach only sanitized evidence: incident ID, commit/path, provider key ID,
secret-store version ID, revocation timestamp, workflow run IDs, exact release
SHA/digests, reviewer identity, and scan conclusions. Never record the secret or
provider payload.

The security owner, repository administrator, and independent release reviewer
must all sign off before sign-ups reopen.
