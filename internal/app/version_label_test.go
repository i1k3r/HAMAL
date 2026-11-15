package app

import (
	"strings"
	"testing"
)

// The bottom-left sidebar version label was removed in v0.8.14. The version itself must remain
// visible elsewhere (Settings > About on the home and creator pages, the PIN screen), so this test
// checks both that the label is gone and that those other places still render it.
func TestSidebarVersionLabelRemoved(t *testing.T) {
	pages := renderContractStates(t)
	for state, html := range pages {
		if strings.Contains(html, "sidebar-version-tag") {
			t.Errorf("%s: still renders the sidebar version label", state)
		}
	}
	for _, state := range []string{stateHome, stateCreator} {
		if !strings.Contains(pages[state], "v0.8.14") {
			t.Errorf("%s: the version is no longer shown in Settings > About", state)
		}
		if !strings.Contains(pages[state], `class="sidebar-footer-row"`) {
			t.Errorf("%s: the sidebar footer row is missing", state)
		}
	}
	if !strings.Contains(pages[stateParticipantLocked], `class="pin-version font-mono">v0.8.14<`) {
		t.Errorf("%s: the PIN screen no longer shows the version", stateParticipantLocked)
	}
}
