# Security policy

## Supported scope

This is an early development preview. Security fixes target the current main
development line; there is no stable-release support or response-time commitment.
Read-only SNMP reduces configuration risk but does not eliminate disclosure or
management-plane load risk.

The supplied UI has **no browser authentication or TLS**. Keep API/UI ports on
loopback and use an SSH tunnel for controlled trials. A shared or mobile deployment
needs an authenticated HTTPS boundary; see [edge access](docs/edge-access.md).
Do not expose the default Compose deployment directly to the Internet.

API routes require a key; the nginx UI injects a shared key for browser requests.
The proxy is limited to GET/HEAD and does not grant cross-origin read access.
That proxy is not a user login or a per-installer authorization boundary. The
health, readiness, version and collector metrics endpoints are unauthenticated.
SNMPv2c communities require a trusted management network/VPN and read-only ACLs.

## Reporting a vulnerability

If this GitHub repository offers **Security → Advisories → Report a vulnerability**,
use that private channel. Availability depends on the repository owner enabling
private vulnerability reporting. Include affected revision, impact, minimal
synthetic reproduction and any proposed fix; never include production credentials
or subscriber data.

If the private reporting button is unavailable, open only a neutral issue asking
the maintainer to enable private reporting. Keep exploit details and sensitive
attachments out of that public issue. No private reporting address is currently
published; do not assume a normal issue or pull request is confidential.

Do not test a public installation or a third party's OLT without authorization.
If your own secret was exposed, revoke/rotate it and review Git/release history;
deleting the current file alone does not remove earlier copies.
