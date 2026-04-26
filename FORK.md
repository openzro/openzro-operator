# openzro/openzro-operator fork point

Forked from `netbirdio/kubernetes-operator` at upstream tag `v0.3.1`
(commit `dd054d50ecebc19ea0060247f22e76394cbbd60b`, released 2026-04-24).
The upstream is BSD-3-Clause (Copyright 2025, netbirdio); the LICENSE
file is preserved verbatim under the BSD-3 attribution clause.

## Rename scope

- Go module: `github.com/netbirdio/kubernetes-operator` → `github.com/openzro/openzro-operator`
- CRD API group: `netbird.io` → `openzro.io`
- CRD Kind prefixes: `Nb*` → `Oz*` (`NbGroup` → `OzGroup`, etc.)
- Chart paths: `helm/kubernetes-operator/` → `helm/openzro-operator/`
- Internal package paths: `internal/netbirdmock/` → `internal/openzromock/`,
  `internal/netbirdutil/` → `internal/openzroutil/`
- All prose / strings / comments rebranded.

## Known issues at fork time

The upstream operator pins `github.com/openzro/openzro` (mechanical
rename of `github.com/netbirdio/netbird`) at version `v0.69.0`, which
does not exist in our `openzro/openzro` core (we forked from upstream
`v0.52.2`). `go build ./...` therefore fails out of the box with a
"reading go.mod at revision v0.69.0" error.

Resolution paths (pick one, follow-up commit):

  1. Re-pin to a pseudo-version of openzro/openzro main:
       go get github.com/openzro/openzro@<commit-sha>
  2. Cut a `v0.69.0` tag in openzro/openzro that points at our latest
     release (consistent with the operator's expected version, but
     conflates upstream version numbers with ours).
  3. Use a local `replace` directive while developing.

Track this in an issue once the repo has issues enabled.

## CRD compatibility

CRDs went from `netbird.io/v1` to `openzro.io/v1`. **Clusters running
the upstream operator cannot be in-place upgraded.** Migration must
recreate resources under the new group; this is a clean fork, not a
drop-in replacement.

## Upstream

- https://github.com/netbirdio/kubernetes-operator
- Tag: v0.3.1
- License: BSD-3-Clause (preserved verbatim)
