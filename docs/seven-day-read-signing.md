# Dedicated long-lived private GET signing

Accepted 2026-10-24: maintainer requested a dedicated read-only key, meal-show-only prefix, real seven-day links, existing CLI/config reuse, and fail-closed behavior. Implementation authorization: 去做吧. Baseline: 8a647bf1b1554dcca84d71f5c4a6b7611296e002.

Implementation Gate: READY. The existing STS signing algorithm is retained for ordinary profiles. An explicit direct-read profile uses the existing OSS SDK with V4 signing and a canonical directory-prefix guard. It cannot assume roles for upload/delete/list/stat. Unknown mode or incomplete scope fails closed. Its RAM policy must independently grant only GetObject on that same prefix. No persistence other than the existing private TOML profile; no proxy, database, cache or timer.

```yaml
kind: VerifiedDesignReceipt
version: 1
work_id: dedicated-read-signing
baseline_sha: 8a647bf1b1554dcca84d71f5c4a6b7611296e002
scope: explicit scoped read-only signing profile, maximum seven days
architecture:
  artifacts: [docs/seven-day-read-signing.md, internal/profile/profile.go, internal/oss/client.go]
  acceptance_source: accepted dedicated-key minimal design in Mamamate release conversation
  acceptance_evidence: 去做吧 on 2026-10-24
coverage:
  algorithms: validate mode/prefix/key/TTL, locally sign GET using V4, reject other operations and STS TTL over one hour
  data_structures: existing TOML plus signing-mode/read-prefix; unchanged resolve JSON
  cross_trace: profile and object key choose signer; TTL determines SDK signature; cloud IAM enforces readable scope
  invariants: no implicit credential fallback, no secret output, direct-read cannot perform other operations
  failures_recovery: fail before signing on invalid scope; preserve credentials while issued links remain valid; revoke only for explicit emergency
  migration: existing profiles unchanged; dedicated RAM/profile creation and live verification belong to Mamamate release task and block production cutover
open_material_items: []
verified_by: Codex
verified_at: 2026-10-24
verdict: READY
```

Use a **new dedicated RAM user**, never a root or broadly privileged key. Keep its profile in the existing private config directory (directory 0700, file 0600). Example (placeholders only):

```toml
bucket = "mamamate"
region = "cn-huhehaote"
endpoint = "oss-cn-huhehaote.aliyuncs.com"
signing-mode = "direct-read"
read-prefix = "xiaohe-meal-show/"
access-key-id = "<dedicated-read-only-key>"
access-key-secret = "<secret>"
```

No role-arn in this profile. RAM must enforce:

```json
{"Version":"1","Statement":[{"Effect":"Allow","Action":["oss:GetObject"],"Resource":["acs:oss:*:*:mamamate/xiaohe-meal-show/*"]}]}
```

`resolve --profile mamamate-meal-show-read --key xiaohe-meal-show/<object>.png --ttl 168h` returns a private V4 GET URL without a security token. Keys are canonical, within the configured directory, and TTL is an integer number of seconds from 1 to 604800. Existing STS URLs must not exceed their one-hour credential lifetime. Cloud IAM is the security boundary; the local prefix check is an additional guard, not a replacement.

Rollback: retain this credential until all issued URLs have expired. Removing/rotating it invalidates outstanding links. Do not shorten object retention during rollback. Signing does not assert that an object exists, nor that cloud permissions or retention have been verified: production must test those separately.

Validation: new tests failed before implementation (profile fields absent), then passed using real SDK signatures and real command/profile round trips, without cloud credentials or mocked signers. All Go package tests and vet pass; changed production statement coverage 34/36 (94.44%), whole repository 81.4%. SDK error returns for an already validated bucket are not forced through mocks. Credential-gated live tests remain skipped without explicit integration configuration. Production IAM and >1-hour access validation are pending; current principal lacks RAM ListUsers permission (403 NoPermission).
