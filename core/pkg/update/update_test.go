// Update client tests — bead cross-os-qhp.7.
//
// Pass criteria mapping:
//  1. Manifest + version compare -> TestCheckForUpdate.
//  2. Download + checksum verify, mismatch fails closed -> TestDownloadVerify.
//  3. Install needs explicit Approval, unapproved fails closed -> TestInstallApprovalGate.
//  4. Atomic install (temp + rename) -> TestInstallAtomic.
//  5. Full round trip via fake Fetcher -> TestFullRoundTrip.
package update

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type fakeFetcher struct {
	data []byte
	err  error
}

func (f *fakeFetcher) Fetch(url string) ([]byte, error) { return f.data, f.err }

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestCheckForUpdate(t *testing.T) {
	cases := []struct {
		cur, next string
		want      bool
	}{
		{"v1.0.0", "v1.0.1", true},
		{"v1.0.0", "v1.0.0", false},
		{"v1.2.0", "v1.1.9", false},
		{"1.0", "v1.0.0", false}, // zero-padded equal
		{"v1.0", "v1.0.1", true}, // shorter current, still comparable
		{"v2.0.0", "v1.9.9", false},
	}
	for _, c := range cases {
		got, err := CheckForUpdate(c.cur, Manifest{Version: c.next})
		if err != nil {
			t.Fatalf("cur=%s next=%s: %v", c.cur, c.next, err)
		}
		if got != c.want {
			t.Fatalf("cur=%s next=%s: got=%v want=%v", c.cur, c.next, got, c.want)
		}
	}
	// Malformed versions are typed errors, never a silent false/true.
	if _, err := CheckForUpdate("not-a-version", Manifest{Version: "v1.0.0"}); err == nil {
		t.Fatal("malformed current version must error")
	}
	if _, err := CheckForUpdate("v1.0.0", Manifest{Version: "garbage.x"}); err == nil {
		t.Fatal("malformed manifest version must error")
	}
	if _, err := CheckForUpdate("", Manifest{Version: "v1.0.0"}); err == nil {
		t.Fatal("empty current version must error")
	}
	if _, err := CheckForUpdate("v1.-1.0", Manifest{Version: "v1.0.0"}); err == nil {
		t.Fatal("negative version segment must error")
	}
}

func TestDownloadVerify(t *testing.T) {
	payload := []byte("crossos-signed-binary-bytes")
	good := sha256Hex(payload)

	// Correct checksum: bytes returned, no error.
	got, err := Download(&fakeFetcher{data: payload}, Manifest{URL: "https://example/x", SHA256: good})
	if err != nil {
		t.Fatalf("good checksum: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatal("returned bytes must match fetched bytes")
	}

	// Wrong checksum: fails closed, typed error names both hashes.
	_, err = Download(&fakeFetcher{data: payload}, Manifest{URL: "https://example/x", SHA256: "deadbeef"})
	var mismatch *ChecksumMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("wrong checksum: want *ChecksumMismatchError, got %v", err)
	}
	if mismatch.Want != "deadbeef" || mismatch.Got != good {
		t.Fatalf("mismatch fields: want=%s got=%s (expected want=deadbeef got=%s)", mismatch.Want, mismatch.Got, good)
	}

	// No checksum in manifest: refuse before even fetching (never install
	// unverified bytes).
	if _, err := Download(&fakeFetcher{data: payload}, Manifest{URL: "https://example/x"}); err == nil {
		t.Fatal("manifest with no checksum must be rejected")
	}

	// Fetch failure propagates as a typed wrap, not silently swallowed.
	fetchErr := errors.New("network down")
	if _, err := Download(&fakeFetcher{err: fetchErr}, Manifest{URL: "https://example/x", SHA256: good}); err == nil {
		t.Fatal("fetch failure must propagate")
	}
}

func TestInstallApprovalGate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "crossos-new")
	if err := Install([]byte("payload"), target, Approval{Granted: false}); err == nil {
		t.Fatal("unapproved install must fail closed (no silent updates)")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("unapproved install must not create the target file")
	}
	if err := Install([]byte("payload"), target, Approval{Granted: true, By: "test-user"}); err != nil {
		t.Fatalf("approved install: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "payload" {
		t.Fatalf("installed content=%q err=%v, want %q", got, err, "payload")
	}
}

func TestInstallAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "crossos-current")
	// Executable fixture (review: cross-os-ed): the atomic install must
	// carry the existing mode across, or the first update strips the
	// daemon's executable bit and bricks the launch path.
	if err := os.WriteFile(target, []byte("old-version"), 0o755); err != nil {
		t.Fatal(err)
	}
	approve := Approval{Granted: true, By: "test-user"}
	if err := Install([]byte("new-version"), target, approve); err != nil {
		t.Fatalf("install over existing: %v", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "new-version" {
		t.Fatalf("target=%q, want new-version (atomic replace)", got)
	}
	// NTFS ignores Go chmod exec bits (the 0755 fixture itself Stat()s as
	// 0666 here), so mode preservation is only assertable on Unix. The
	// code path (Stat -> Chmod -> Rename) still runs on Windows — CI's
	// macos runner asserts the real behavior.
	if runtime.GOOS != "windows" {
		if st, err := os.Stat(target); err != nil || st.Mode().Perm() != 0o755 {
			t.Fatalf("target mode=%v err=%v, want preserved 0755", st.Mode(), err)
		}
	}
	// No leftover temp files in the target directory after a successful install.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "crossos-current" {
		t.Fatalf("dir entries=%v, want exactly [crossos-current] (no leaked temp file)", entries)
	}
}

func TestInstallFreshDefaultsExecutable(t *testing.T) {
	// Fresh install (no existing target): mode must be 0755, the daemon
	// default — never CreateTemp's 0600.
	dir := t.TempDir()
	target := filepath.Join(dir, "crossos-new")
	if err := Install([]byte("payload"), target, Approval{Granted: true, By: "test-user"}); err != nil {
		t.Fatalf("fresh install: %v", err)
	}
	// Same NTFS caveat as TestInstallAtomic: mode only assertable on Unix.
	if runtime.GOOS != "windows" {
		if st, err := os.Stat(target); err != nil || st.Mode().Perm() != 0o755 {
			t.Fatalf("fresh target mode=%v err=%v, want 0755", st.Mode(), err)
		}
	}
}

func TestFullRoundTrip(t *testing.T) {
	payload := []byte("crossos-v2-binary")
	m := Manifest{Version: "v2.0.0", Platform: "windows/amd64", URL: "https://example/crossos.exe", SHA256: sha256Hex(payload)}

	needsUpdate, err := CheckForUpdate("v1.9.0", m)
	if err != nil || !needsUpdate {
		t.Fatalf("needsUpdate=%v err=%v, want true/nil", needsUpdate, err)
	}

	data, err := Download(&fakeFetcher{data: payload}, m)
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "crossos.exe")
	if err := Install(data, target, Approval{Granted: true, By: "test-user"}); err != nil {
		t.Fatalf("install: %v", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != string(payload) {
		t.Fatal("round-trip content mismatch")
	}
}
