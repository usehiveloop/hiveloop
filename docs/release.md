# Release Process

All Hiveloop release artifacts use one GitHub release tag as the source of
truth.

Accepted tag formats:

- `vX.Y.Z`
- `vX.Y.Z-rc.1`

Publishing a GitHub release starts `.github/workflows/release.yml`, which builds
and publishes:

- `ghcr.io/usehiveloop/hiveloop:<tag>` and `:<semver>`
- `ghcr.io/usehiveloop/sandbox-bridge:<tag>` and `:<semver>`
- `ghcr.io/usehiveloop/employee-sandbox:<tag>` and `:<semver>`
- bridge release tarballs for Linux and macOS targets
- `release-manifest.json` attached to the GitHub release

Stable releases also update `latest`. Prereleases do not.

Sandbox snapshot promotion is intentionally separate from release publishing.
After a release succeeds, run the manual `promote-sandbox-release` workflow with
the release tag to register Daytona snapshots.

The release manifest contains the environment values needed for runtime
promotion, including:

- `BRIDGE_BINARY_VERSION`
- `BRIDGE_BASE_IMAGE_PREFIX`
- `BRIDGE_BASE_DEDICATED_IMAGE_PREFIX`
- `EMPLOYEE_SANDBOX_BASE_IMAGE_PREFIX`
