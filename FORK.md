# openzro/openzro-operator fork point

Forked from `netbirdio/kubernetes-operator` at upstream tag `v0.3.1`
(commit `dd054d50ecebc19ea0060247f22e76394cbbd60b`, released 2026-04-24).
The upstream is BSD-3-Clause (Copyright 2025, netbirdio); the LICENSE
file is preserved verbatim under the BSD-3 attribution clause.

## Rename scope

Done at the fork point and finalized in 2026-04-28:

- Go module: `github.com/netbirdio/kubernetes-operator` → `github.com/openzro/openzro-operator`
- CRD API group: `netbird.io` → `openzro.io`
- CRD Kind prefixes: `NB*` → `OZ*` (e.g. `NBGroup` → `OZGroup`,
  `NBPolicy` → `OZPolicy`, `NBSetupKey` → `OZSetupKey`,
  `NBResource` → `OZResource`, `NBRoutingPeer` → `OZRoutingPeer`,
  `NBCondition*` → `OZCondition*`).
- Chart paths: `helm/kubernetes-operator/` → `helm/openzro-operator/`
- Internal package paths: `internal/netbirdmock/` → `internal/openzromock/`,
  `internal/netbirdutil/` → `internal/openzroutil/`
- File naming: `nb*_types.go` / `nb*_controller.go` /
  `nb*_controller_test.go` / `crds/openzro.io_nb*.yaml` /
  `helm/openzro-operator/templates/nb*.yaml` → `oz*` equivalents.
- Field rename: the lowercase (unexported) `openZro *openzro.Client`
  field on every reconciler became uppercase (exported) `OpenZro`,
  so `cmd/main.go` (a different package) can populate it via struct
  literals.
- All prose / strings / comments rebranded.

## API surface backports (clean-room, BSD-3)

Upstream's operator at `v0.3.1` was developed against a NetBird core
post our fork point (`netbirdio/netbird` v0.53.0 BSD-3). Five surfaces
were re-introduced into `openzro/openzro` in 2026-04-28 so this
operator builds and tests pass:

| Symbol                                         | Where (in openzro/openzro)                                |
|------------------------------------------------|-----------------------------------------------------------|
| `api.Zone`, `api.ZoneRequest`                  | `management/server/http/api/dns_zones.go`                 |
| `api.DNSRecord`, `api.DNSRecordRequest`, `api.DNSRecordType*` | same |
| `Client.DNSZones` (8 methods)                  | `management/client/rest/dns_zones.go`                     |
| `api.Service`, `api.ServiceRequest`, `api.ServiceTarget`, `api.ServiceTarget*`, `api.ServiceRequestMode*` | `management/server/http/api/reverse_proxy.go` |
| `Client.ReverseProxyServices` (5 methods)      | `management/client/rest/reverse_proxy.go`                 |
| `Groups.GetByName(ctx, name)`                  | added to `management/client/rest/groups.go`               |
| `WithUserAgent(string) option`                 | `management/client/rest/options.go`                       |
| `*APIError` typed error + `IsNotFound`/etc.    | `management/client/rest/errors.go`                        |

The types live but the **server-side handlers + storage migrations
are deferred** (tracked in
`/home/kleber/.claude/projects/-home-kleber-Dados-openzro-openzro/memory/project_enterprise_gaps.md`
under "DNS Zones" and "Reverse-proxy Services"). The operator
builds and unit-tests against the openzromock; against a real
openZro management server, calls into those features will return
404 until the server-side ships.

## Module pin

The operator `replace`s `github.com/openzro/openzro` to the
sibling working copy at `../openzro` for development. CI must
substitute a pseudo-version pin (`go get github.com/openzro/openzro@<commit-sha>`)
before building.

## CRD compatibility

CRDs went from `netbird.io/v1` to `openzro.io/v1` and Kinds from
`NB*` to `OZ*`. **Clusters running the upstream operator cannot
be in-place upgraded.** Migration must recreate resources under
the new group + Kind; this is a clean fork, not a drop-in
replacement.

## Upstream

- https://github.com/netbirdio/kubernetes-operator
- Tag: v0.3.1
- License: BSD-3-Clause (preserved verbatim)
