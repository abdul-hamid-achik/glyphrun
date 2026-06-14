# Distribution & Releasing

Glyphrun ships cross-platform binaries via [GoReleaser](https://goreleaser.com).

## Installing a release

Once a version is published:

```bash
# Homebrew (macOS / Linux), after the tap is set up:
brew install abdul-hamid-achik/tap/glyph

# Or download a prebuilt archive for your platform from the Releases page and
# put the binary on your PATH:
#   https://github.com/abdul-hamid-achik/glyphrun/releases
```

Build from source instead:

```bash
go install github.com/abdul-hamid-achik/glyphrun/cmd/glyph@latest
```

## Cutting a release

Releases are automated: pushing a `v*` tag runs `.github/workflows/release.yml`,
which invokes GoReleaser to build the matrix (darwin/linux/windows × amd64/arm64
— Windows as `.zip`, the rest as `.tar.gz`), publish a GitHub Release with
archives + `checksums.txt`, and update the Homebrew cask (macOS).

```bash
git tag v0.2.0
git push origin v0.2.0
```

Validate and dry-run locally before tagging:

```bash
goreleaser check                       # validate .goreleaser.yaml
goreleaser build --snapshot --clean    # build all targets without releasing
```

The build injects `internal/version` metadata via ldflags, so `glyph --version`
reports the tag, short commit, and commit date.

## Homebrew tap prerequisites

The cask is pushed to a separate tap repository. To enable `brew install`:

1. Create the tap repo `abdul-hamid-achik/homebrew-tap`.
2. Add a repository secret `HOMEBREW_TAP_TOKEN` — a PAT with write access to that
   tap — to the `glyphrun` repo.

Without these, a tagged release still publishes binaries and checksums; only the
cask update step fails. Remove the `homebrew_casks` block from `.goreleaser.yaml`
to skip it entirely.
