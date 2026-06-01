# ADR-0014: Provider-Endpoint Flag Discipline for v5 GCP Provider

## Status
Accepted

## Context

terraform-provider-google v5.x routes API calls through per-service
endpoint flags (`compute_custom_endpoint`, `sql_custom_endpoint`,
`kms_custom_endpoint`, etc.). infrafactory's generator template
(`internal/cli/generate_command.go::buildGoogleProviderBlock`) injects
those flags pointing at fakegcp so apply doesn't escape to the real
cloud. The pattern is straightforward — except when it isn't.

During the 2026-06-01 sweep we hit three repeated failure shapes that
all looked superficially different but had the same root cause —
mismatches between the URL a provider client constructs and the URL
fakegcp serves:

1. **Trailing-path doubling** (T2, T-E, T11 partial). Some lib clients
   prepend `v1/projects/...` to BasePath themselves. With
   `kms_custom_endpoint = "%s/v1/"` the URL became `/v1/v1/projects/...`
   which fakegcp 501'd. The v5 provider's strip-version regex
   (`(?P<base>http[s]://.*)(?P<version>/[^/]+?/$)`) requires `https://`
   literally, so it never fires on our `http://` fakegcp endpoints —
   the strip is a no-op and the trailing path doubles.

2. **Distinct v1 vs v3 endpoint flags** (T-D-2). The provider has
   *separate* endpoint overrides for cloudresourcemanager v1 and v3
   (`cloud_resource_manager_custom_endpoint` vs
   `resource_manager_v3_custom_endpoint`). Newer code paths
   (`google_service_networking_connection`'s getProject preflight,
   among others) use v3 only. Setting only v1 leaves v3 calls
   escaping to the real cloud, surfacing as a misleading
   `Error 401 ACCESS_TOKEN_TYPE_UNSUPPORTED` that looks like an
   auth problem but is actually a missing endpoint flag.

3. **Dual URL prefixes against the same endpoint** (T-E-2). KMS in
   particular uses TWO URL shapes: the lib client prepends
   `v1/projects/...` (so READ paths land at `/v1/projects/...`), but
   template-based code uses raw `{{KMSBasePath}}projects/...` with no
   v1 prefix (so CREATE paths land at `/projects/...`). fakegcp must
   register handlers under BOTH prefixes.

The misdiagnosis cost across these three shapes was significant: each
one's symptom looked unrelated to the others, and the v5 SDK's error
messages often misattribute the cause (auth-style errors for what is
actually a missing route).

## Decision

Three rules for endpoint-flag work going forward:

1. **Host-only by default.** New `*_custom_endpoint` entries in
   `buildGoogleProviderBlock` should default to `%s/` (host-only) and
   document with inline comments WHY when a trailing version-path is
   needed. The default-host-only-form sidesteps the strip-regex
   `https://`-requirement bug entirely.

2. **Use the v5 binary as ground truth.** When a new GCP scenario
   surfaces an "escape to real cloud" symptom, run
   `strings <provider-binary> | grep -E "GOOGLE_.*_CUSTOM_ENDPOINT"`
   to enumerate every endpoint flag the provider actually reads. If
   there are version-suffixed variants (`_V3_`, `_V2_`, etc.), set
   them all. The session that diagnosed T-D-2 discovered
   `GOOGLE_RESOURCE_MANAGER_V3_CUSTOM_ENDPOINT` this way.

3. **Dual-prefix mock routes when the symptom is split apply/read.**
   When CREATE 501s but READ 200s (or vice versa) against the same
   resource, the lib is using two URL shapes. Register the same
   handler set under both prefixes via a shared closure (see
   `fakegcp/handlers/handlers.go::registerKMSRoutes` for the canonical
   pattern landed in fakegcp `a3b1ea8`).

## Consequences

**Benefits**:
- Future endpoint-flag misses will be diagnosable in minutes
  (binary-strings hunt) rather than the multi-hour debugging sessions
  T-D-2 took.
- The pattern of "register the same handler set under multiple URL
  prefixes" generalises to other services likely to exhibit the same
  split (Compute, Container, etc., if they ever surface similar
  symptoms).
- Inline comments in `buildGoogleProviderBlock` now record the
  *why* of each non-host-only entry, so future contributors don't
  "tidy up" trailing-path values back to a form that re-triggers the
  doubling bug.

**Tradeoffs**:
- Cross-repo discipline: every endpoint-flag addition in
  `infrafactory/internal/cli/generate_command.go` may need a matching
  route in `fakegcp/handlers/handlers.go`. There's no automated
  enforcement linking the two.
- The "host-only by default" rule occasionally needs to be relaxed
  per-service. Each such relaxation should carry an inline comment
  citing the specific provider lib behaviour that requires it
  (see `cloud_resource_manager_custom_endpoint` and
  `service_usage_custom_endpoint` for the exception pattern).

**Follow-up work**:
- The remaining `*_custom_endpoint` entries that still ship with
  trailing version paths (`cloud_resource_manager_custom_endpoint`,
  `pubsub_custom_endpoint`, `storage_custom_endpoint`,
  `secret_manager_custom_endpoint`, `redis_custom_endpoint`,
  `cloud_run_v2_custom_endpoint`) should be audited per rule 2 — for
  each, confirm whether a matching `_V3_` flag exists and whether
  the trailing path is load-bearing or a no-op. Tracked as T11
  in `docs/NEXT_SESSION.md` (partial).
