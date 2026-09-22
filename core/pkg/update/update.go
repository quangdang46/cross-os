package update

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Manifest is one release entry (mirrors the release.yml manifest line:
// version, platform, download URL, SHA256 checksum).
type Manifest struct {
	Version  string // e.g. "v1.2.0"
	Platform string // "darwin/arm64", "darwin/amd64", "windows/amd64"
	URL      string
	SHA256   string // hex-encoded, lowercase
}

// Approval is the explicit user-approve token (§8.4). Same shape as
// plugin.Approval — kept local so this package has no marketplace
// dependency; installs without a granted approval fail closed, never
// silently.
type Approval struct {
	Granted bool
	By      string
}

// Fetcher retrieves raw bytes for a URL. Injectable so tests never touch
// the network; the production caller wires an http.Client-backed Fetcher.
type Fetcher interface {
	Fetch(url string) ([]byte, error)
}

// CheckForUpdate reports whether m is newer than currentVersion. Versions
// are dotted-integer strings with an optional leading "v" ("v1.2.0",
// "1.2.0"); a malformed version on either side is a typed error, never a
// silent false.
func CheckForUpdate(currentVersion string, m Manifest) (bool, error) {
	cur, err := parseVersion(currentVersion)
	if err != nil {
		return false, fmt.Errorf("update: current version %q: %w", currentVersion, err)
	}
	next, err := parseVersion(m.Version)
	if err != nil {
		return false, fmt.Errorf("update: manifest version %q: %w", m.Version, err)
	}
	return compareVersions(next, cur) > 0, nil
}

// parseVersion splits a dotted version into integer components, stripping
// a leading "v". Empty components (e.g. "v" alone) or non-numeric parts
// are rejected — a version we can't compare must never silently look
// "not newer" (which would suppress a real update) or "always newer"
// (which would install an unparseable release).
func parseVersion(v string) ([]int, error) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return nil, fmt.Errorf("empty version")
	}
	parts := strings.Split(v, ".")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("non-numeric segment %q", p)
		}
		if n < 0 {
			return nil, fmt.Errorf("negative segment %q", p)
		}
		out[i] = n
	}
	return out, nil
}

// compareVersions returns 1 if a>b, -1 if a<b, 0 if equal. Shorter
// versions are zero-padded ("1.2" == "1.2.0"). Callers only test the sign,
// so the result is clamped (never a raw av-bv magnitude).
func compareVersions(a, b []int) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		switch {
		case av > bv:
			return 1
		case av < bv:
			return -1
		}
	}
	return 0
}

// ChecksumMismatchError reports a download whose SHA256 didn't match the
// manifest — the bytes are NEVER installed on this error.
type ChecksumMismatchError struct {
	Want, Got string
}

func (e *ChecksumMismatchError) Error() string {
	return fmt.Sprintf("update: checksum mismatch: manifest says %s, downloaded bytes hash to %s", e.Want, e.Got)
}

// Download fetches m.URL via f and verifies the SHA256 checksum against
// m.SHA256. A mismatch fails closed: the caller never sees unverified
// bytes returned as if they were good (they're returned alongside the
// error only for forensic logging — callers must check err before using
// the bytes).
func Download(f Fetcher, m Manifest) ([]byte, error) {
	if strings.TrimSpace(m.SHA256) == "" {
		return nil, fmt.Errorf("update: manifest %s has no checksum (never install unverified)", m.Version)
	}
	data, err := f.Fetch(m.URL)
	if err != nil {
		return nil, fmt.Errorf("update: fetch %s: %w", m.URL, err)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	want := strings.ToLower(strings.TrimSpace(m.SHA256))
	if got != want {
		return data, &ChecksumMismatchError{Want: want, Got: got}
	}
	return data, nil
}

// Install writes data to targetPath, gated on an explicit Approval (§8.4:
// updates never silently enable new integrations — this mirrors the
// marketplace Install/InstallManifest approval gate). Writes atomically:
// temp file in the same directory, then rename, so a crash mid-write
// never leaves a partially-written target in place.
//
// Mode preservation (review: cross-os-ed caught that os.CreateTemp's 0600
// would strip the executable bit off the daemon binary on first update,
// bricking the launch path): the temp file inherits the existing target's
// permission bits when the target exists; a fresh install defaults to
// 0755 (the target is a daemon binary, not a config file).
func Install(data []byte, targetPath string, ap Approval) error {
	if !ap.Granted {
		return fmt.Errorf("update: install %q needs user approval (no silent updates)", targetPath)
	}
	mode := os.FileMode(0o755)
	if st, err := os.Stat(targetPath); err == nil {
		mode = st.Mode().Perm()
	}
	dir := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(dir, ".update-*")
	if err != nil {
		return fmt.Errorf("update: create temp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	// Clean up the temp file on any failure path; a successful rename
	// removes the source, so this Remove becomes a no-op then.
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("update: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("update: close temp: %w", err)
	}
	// Chmod before rename: rename preserves the temp file's mode, and the
	// temp arrives 0600 from CreateTemp regardless of the target.
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("update: chmod temp to %o: %w", mode, err)
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		return fmt.Errorf("update: rename into place: %w", err)
	}
	return nil
}
