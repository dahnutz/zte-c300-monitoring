# Contributing

Thanks for helping build a practical, read-only PON troubleshooting tool.
This is an early development preview maintained in spare time. Small, tested
changes and clear compatibility reports are useful; an OLT is not required to
contribute. There is no promised support response time.

## Where help is most useful

See the [roadmap](docs/roadmap.md) for scope and acceptance criteria. Good first
contributions include improving installation instructions, reproducing an API
example, adding a synthetic regression case or improving mobile accessibility.
Larger work should start with an issue describing the operator problem:

- Named-user HTTPS access without exposing the collector key in the browser.
- Reliable observation age, partial snapshots and pagination.
- Firmware validation for optics, Ethernet UNI and unconfigured ONUs.
- Timescale migration/restore tests and bounded retention.
- Synthetic UI demonstrations, with entirely fictional subscriber/topology data.

MAC history and ONU traffic need OID/counter evidence before implementation.
Provisioning, ONU resets and other OLT writes are outside the project scope.

## Development setup

Fork this repository and clone your fork. The upstream origin and MIT attribution
are recorded in [UPSTREAM.md](UPSTREAM.md). From the repository root:

```sh
# Go 1.26.8, Make and a C compiler are required.
make check
make build

# Node.js 22.18+ and npm are required for frontend work.
cd web
npm ci
npm test
npm run build
cd ..
```

Go tests use synthetic SNMP/Redis fixtures and local sockets. They do not require
an OLT or a production database. Run them without live OLT environment variables.
`npm test` covers inventory/discovery/identity semantics; it is not a complete
browser interaction suite. The GitHub workflow runs these checks on pull requests.

The optional `scripts/smoke.sh` builds isolated collector, Redis and UDP fixture
containers. It needs Docker or Podman and never contacts a real OLT. It does not
test Timescale restore or authenticated mobile access.

For full UI development use the local Compose stack in [deployment](docs/deployment.md).
`npm run dev` starts Vite on loopback and proxies API requests to the collector,
but it does not inject an API key. Use the Compose nginx UI to exercise the
complete server-side proxy. Never put an API/SNMP secret in a `VITE_*` variable,
browser asset or public test fixture.

## Pull requests

1. Make one focused change on a branch; explain the operational problem.
2. Add a meaningful synthetic regression test for changed behavior.
3. Update affected configuration examples, API/OpenAPI documentation and roadmap
   status in the same change. Distinguish implemented, tested and planned.
4. Run the relevant checks above. State anything not tested, especially hardware,
   database migrations, browser behavior or container deployment.
5. Review the complete diff for private data and retain upstream licence notices.

The contributor templates request these details. Please be respectful and keep
feedback focused on reproducible behavior. Maintainers may split or defer broad
changes to keep the project maintainable.

## Hardware reports and privacy

Only test networks you are authorized to access. Start with one known ONU/PON
and read-only access; do not disconnect a customer to create a test case.
[Hardware acceptance](docs/hardware-testing.md) explains what to compare.

Use fictional serials such as `TEST00000001`, documentation addresses and
synthetic measurements in public reports. Remove operator/customer names, host
aliases, management addresses, communities, keys, locations and exact deployed
inventory. Share model/firmware compatibility details only when approved for
publication. Do not attach a full SNMP walk, `.env` or database dump. Screenshots are welcome
when identifiers and secrets are removed and the operator approves the remaining
context. Model names, port numbers and optical readings may be retained when
approved; see the [screenshot policy](docs/publication-review.md#screenshot-policy). Prefer synthetic identities or solid
redaction when preparing new captures.

Keep raw evidence under ignored `private/`. Ignore rules do not remove already
tracked data. See [publication review](docs/publication-review.md). Report
security issues using [SECURITY.md](SECURITY.md), not a public bug attachment.
