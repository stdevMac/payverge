package hygiene

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// No compiled binary should be tracked at the backend module root. The
// Makefile output (bin/app) is already gitignored; stray `go build -o payverge`
// artifacts must never be committed.
func TestNoCommittedRootBinaries(t *testing.T) {
	out, err := exec.Command("git", "ls-files", "../../").Output()
	if err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	banned := map[string]bool{"payverge": true, "payverge-backend": true}
	for _, f := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		base := filepath.Base(strings.TrimSpace(f))
		dir := filepath.Dir(strings.TrimSpace(f))
		// Only flag entries that live directly at the backend module root (../../),
		// not deep in subdirectories that happen to share the name.
		if banned[base] && dir == "../.." {
			t.Errorf("committed binary at backend root: %q (run `git rm --cached %s` and add to .gitignore)", base, base)
		}
	}
}
