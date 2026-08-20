# Milestone 82: Managed Bundle Provenance Metadata

Date: 2026-06-05

## Summary

Managed bundle manifests now support an optional offline `provenance:` block:

```yaml
provenance:
  source_url: "https://github.com/projectdiscovery/httpx"
  upstream_version: "v1.3.0"
  retrieved_at: "2026-06-05T00:00:00Z"
  verified_by: "operator"
```

The app, CLI, and TUI expose `provenance_status` plus the recorded values for workspace-local bundles, release-managed bundles, release-managed status rows, and managed install plan/result output. Missing metadata reports `provenance_status=missing`; valid metadata reports `recorded`; invalid release metadata becomes a blocked release inventory/status row before any workspace copy/link writes.

The bundled `nuclei-templates` seed manifest now records provenance metadata while keeping the existing payload digest unchanged.

## Safety Boundary

Provenance remains review metadata only. It is not a trust root, signature, expected digest pin, update feed, downloader, sync source, or workspace conflict criterion.

This milestone does not:

- Download or refresh upstream bundles.
- Execute payloads or health probes.
- Create workspace bundle copies or final links during list/status/TUI projection.
- Persist expected digest or provenance policy in the release-managed status-source marker.
- Treat provenance differences as workspace managed-bundle conflicts.

## Validation

Focused RED/GREEN validation passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'Provenance|ManagedToolBundleManifestReportsOptionalProvenanceMetadata|ReleaseManagedToolBundleBlocksInvalidProvenanceMetadataWithoutWorkspaceWrites|ToolchainManagedBundleCommandsReportProvenanceMetadata|NewTUIModelForWorkspaceShowsManagedBundleRowsReadOnly' -count=1 -v
```

Full repository validation, strict doctor, build, and release smoke are still pending after this milestone document write.

## Remaining Work

- Signed provenance and signature verification.
- Release-managed update metadata and current/update-available status.
- Remote sync/download gates with explicit operator policy.
- Full upstream Nuclei template corpus packaging.
- Production third-party binary distribution.
