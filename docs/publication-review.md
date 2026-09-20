# Public source and fixture policy

Reviewed: 20 September 2026. Scope: Git-tracked files and nonignored untracked
files in this repository, including tests, frontend, templates and documentation.
Ignored local operational data is not part of the public source set.

## Findings addressed

- Replaced operator-specific frontend branding with neutral application labels.
- Removed exact private trial firmware/ONU-model descriptions and actual lab slot
  lists from public prose. The target platform name remains part of the project.
- Replaced a fixture serial that matched a private capture with an artificial
  value; generalized associated model/position/measurement examples.
- Replaced copied interface-position examples with synthetic ones while keeping
  protocol naming patterns and regression behavior.
- Removed the public database-password default. New configurations must supply
  a unique password. This does not rotate credentials in an existing database.
- Excluded Python bytecode and local TLS/password-file material from Git/image
  contexts, and updated source-export exclusions for frontend build artifacts.
- Removed public instructions that depended on an ignored local Compose helper.

Identifiers removed during a privacy review must not be repeated in the public
report, denylist, commit message or regression-test name. Detailed local evidence
and pre-edit comparisons belong in ignored private storage.

## Additional first-preview review

- Reviewed the blurred UI screenshots. The operator approved retaining model
  names, slot/PON positions, counts and optical readings for the public gallery.
  Subscriber names and serials remain blurred. Screenshot captions distinguish
  the captured development build from current behavior.
- Replaced a real serial reintroduced in API examples with synthetic text.
- Extended UI and root image-context exclusions to nested local secret files.
- Corrected selection of the latest finished inventory, partial/cap warnings,
  unknown discovery counts and missing-discovery semantics.
- Added selected-ONU identity checks, cancellation on navigation and clearing of
  old on-demand results on retry. A collection timestamp is not a sensor timestamp.
- Removed default wildcard CORS. The UI credential proxy now allows GET/HEAD
  only and suppresses cross-origin read permission. These controls do not add
  the planned user login or TLS.
- Updated affected Go/toolchain and frontend dependencies, and added focused
  regression cases, contributor templates and a minimal CI workflow.

Publishing this source as a development preview is separate from approving a
shared installer deployment. Authentication/TLS, per-field freshness, complete
pagination, database restore/retention and firmware acceptance remain pending.
Dependency advisory checks are time-bound; they do not certify container base
images, every library path or the deployment as vulnerability-free.

## What stays public

The intended C300 family, vendor enterprise OIDs, documented MIB enum values,
generic interface-name patterns and synthetic 8-/16-port layout examples are
necessary technical context. They do not identify an installed operator topology.
Upstream MIT attribution and public dependency names remain intact.

Serial-shaped test values are synthetic unless explicitly reviewed otherwise.
Use obvious placeholders (for example `TEST00000001`), documentation IP ranges,
fictional names and deliberately chosen measurements. Do not copy actual CLI
rows into tests and then redact only the customer name.

## Screenshot policy

Screenshots help users understand the application. Sanitized operator captures
are allowed when the operator approves the information that remains visible.
Model names, port numbers, counts and optical levels are not automatically
confidential; assess them in context rather than removing every real value.

Credentials, readable subscriber identifiers, private management addresses and
unapproved operator details must stay out of public images. Review the complete
image and its metadata. Prefer synthetic identities or solid redaction for new
captures; blur strength varies. Approval of the existing gallery does not approve
new captures or copies of its operational data into source-code fixtures.

## Before publishing an export or release

1. Enumerate both tracked and nonignored untracked files:
   `git ls-files --cached --others --exclude-standard`.
2. Inspect source, tests, comments, schemas, screenshots, generated assets and
   archives for operator identity, real addresses/hostnames, contacts, serials,
   MACs, subscriber names, exact deployment inventory and credentials.
3. Compare candidate examples with private captures/credentials locally without
   printing the credential values into shared logs. Pattern scans alone miss
   realistic-looking serials and customer names.
4. Keep `.env`, inventory, captures, logs, database exports, certificates and
   password files under ignored private paths. Verify actual Git tracking:
   adding an ignore rule does not untrack an already tracked secret.
5. Review the actual archive/image contents too; a correct `.gitignore` does not
   protect a hand-built tar file or old container image.
6. If a secret was previously published, rotate it and assess Git/release history.
   Deleting its current source line does not revoke it or clean old artifacts.

This is a bounded review of the current source set, not proof that all historical
copies, private captures, container layers or external artifacts are sanitized.
