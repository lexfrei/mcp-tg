package main

import (
	"os"
	"strings"
	"testing"
)

// A pre-release tag (v1.3.0-rc1) must not become "latest" ANYWHERE, and five
// separate places decide that independently — two workflow expressions, two
// container tag conditions, and one GoReleaser option. If any one of them
// drifts, an RC silently becomes the latest image on ghcr and lands on every
// `brew upgrade`, with nothing red to show for it: the failure this whole
// release pipeline is built to prevent, one layer up.
func TestPrereleaseGuards_AllFiveSitesAgree(t *testing.T) {
	release := readRepoFile(t, "../../.github/workflows/release.yml")
	goreleaser := readRepoFile(t, "../../.goreleaser.yaml")

	// The GitHub release itself.
	if !strings.Contains(release, "prerelease: ${{ contains(github.ref_name, '-') }}") {
		t.Error("create-release no longer marks hyphenated tags as pre-releases")
	}

	// GoReleaser updates the same flag when it uploads the archives.
	if !strings.Contains(goreleaser, "prerelease: auto") {
		t.Error(".goreleaser.yaml no longer derives prerelease from the tag")
	}

	// For Homebrew, the formula in the tap IS latest.
	if !strings.Contains(goreleaser, "skip_upload: auto") {
		t.Error("a pre-release would overwrite the formula in the tap")
	}

	// Both metadata-action steps tag the image, and both must gate :latest.
	const latestGuard = "type=raw,value=latest,enable=${{ !contains(github.ref_name, '-') }}"

	if got := strings.Count(release, latestGuard); got != 2 {
		t.Errorf("found %d guarded :latest container tags, want 2 — an RC would become :latest", got)
	}
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(raw)
}
