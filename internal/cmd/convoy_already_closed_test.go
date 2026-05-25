package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCloseConvoyIfComplete_AlreadyClosedSkipsNotify verifies (gst-9fn) that
// when a fresh-read shows the convoy is already closed, closeConvoyIfComplete
// does NOT re-issue bd close or fire notifyConvoyCompletion. Stale list reads
// previously caused "Convoy landed" mail to re-fire on every patrol cycle.
func TestCloseConvoyIfComplete_AlreadyClosedSkipsNotify(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows — shell stubs")
	}

	binDir := t.TempDir()
	townBeads := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townBeads, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	closeLog := filepath.Join(binDir, "bd-close.log")
	mailLog := filepath.Join(binDir, "gt-mail.log")

	// bd stub: `show` reports the convoy as already closed; `close` would log
	// invocation; `sql` returns empty (no extra deps).
	bdScript := `#!/bin/sh
CMD=""
for arg in "$@"; do
  case "$arg" in
    --*) ;;
    *) CMD="$arg"; break ;;
  esac
done

case "$CMD" in
  show)
    echo '[{"id":"hq-cv-already","title":"Already-closed convoy","status":"closed","issue_type":"convoy","description":""}]'
    exit 0
    ;;
  close)
    echo "$@" >> "` + closeLog + `"
    exit 0
    ;;
  sql)
    echo '[]'
    exit 0
    ;;
  dep)
    echo '[]'
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	// gt stub: log any mail invocation so we can assert no notification fires.
	gtScript := `#!/bin/sh
echo "$@" >> "` + mailLog + `"
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "gt"), []byte(gtScript), 0755); err != nil {
		t.Fatalf("write gt stub: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	tracked := []trackedIssueInfo{
		{ID: "gt-a", Status: "closed"},
		{ID: "gt-b", Status: "closed"},
	}

	out, err := captureConvoyStdoutErr(t, func() error {
		ready, err := closeConvoyIfComplete(townBeads, "hq-cv-already", "Already-closed convoy", tracked, false)
		if ready {
			t.Errorf("ready=true, want false (no transition observed)")
		}
		return err
	})
	if err != nil {
		t.Fatalf("closeConvoyIfComplete: %v", err)
	}

	if !strings.Contains(out, "already closed") {
		t.Errorf("output should mention 'already closed', got: %q", out)
	}

	// bd close must NOT have been called for an already-closed convoy.
	if data, err := os.ReadFile(closeLog); err == nil && len(data) > 0 {
		t.Errorf("bd close was invoked for already-closed convoy: %s", data)
	}

	// No mail/nudge should have been sent.
	if data, err := os.ReadFile(mailLog); err == nil && len(data) > 0 {
		t.Errorf("gt mail/nudge invoked for already-closed convoy: %s", data)
	}
}
