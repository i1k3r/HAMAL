package app

// Template contract tests (Phase 0 of the Light Table plan; test-only).
//
// site.js finds its elements with getElementById and reads configuration from data-* attributes
// on <body>. Because site.js silently skips missing elements (`if (!el) return`), renaming or
// removing one of them in a template would make a feature disappear without any error. These
// tests freeze that contract:
//
//   - TestTemplateContractIDs: every id site.js reads must still be rendered in each page state
//     where it was rendered at the baseline (6765e5e).
//   - TestTemplateContractBodyData: the data-* attributes on <body> of each page state.
//   - TestTemplateContractCoversSiteJS: every id literal passed to getElementById in site.js is
//     listed below, so a new lookup cannot bypass the contract.
//
// Changing any table here is a contract change: the PR must say so (label `contract-change`).
// To print the current rendering as Go literals (for a reviewed update), run:
//
//	HAMAL_CONTRACT_PRINT=1 go test ./internal/app -run TestTemplateContractIDs -v

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/i1k3r/HAMAL/internal/config"
)

// Page states rendered by the contract tests.
const (
	stateHome              = "home"
	stateCreator           = "creator"
	stateCreatorSharing    = "creator-sharing"
	stateParticipant       = "participant"
	stateParticipantLocked = "participant-pin-locked"
	stateParticipantAuthed = "participant-pin-authenticated"
	stateShare             = "share"
)

// siteJSIDContract lists every id that site.js reads with getElementById and the page states in
// which the baseline templates render it.
var siteJSIDContract = []struct {
	ID     string
	States []string
}{
	{"activity-list", []string{"creator", "creator-sharing"}},
	{"btn-clear-activity", []string{"creator", "creator-sharing"}},
	{"btn-copy-all-participants", []string{"creator", "creator-sharing"}},
	{"close-modal-btn", []string{"creator-sharing"}},
	{"close-room-btn", []string{"creator", "creator-sharing"}},
	{"closing-countdown", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"closing-ring-fill", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"confirm-close-btn", []string{"creator", "creator-sharing"}},
	{"copy-link-btn", []string{"creator", "creator-sharing"}},
	{"copy-share-btn", []string{"creator-sharing"}},
	{"copy-share-toast", []string{"creator-sharing"}},
	{"copy-toast", []string{"creator", "creator-sharing"}},
	{"countdown", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked", "share"}},
	{"create-room-form", []string{"home"}},
	{"create-share-form", []string{"creator-sharing"}},
	{"create-submit-btn", []string{"home"}},
	{"creator-files-section", []string{"creator", "creator-sharing"}},
	{"creator-text-compose", []string{"creator", "creator-sharing"}},
	{"creator-texts-section", []string{"creator", "creator-sharing"}},
	{"dropzone", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"dropzone-title", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"file-count", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"file-input", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"file-list", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"file-preview-overlay", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"files-mode-container", []string{"participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"form-error", []string{"home"}},
	{"generate-share-btn", []string{"creator-sharing"}},
	{"generated-share-input", []string{"creator-sharing"}},
	{"inactive-message", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"inactive-title", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"metric-connected-count", []string{"creator", "creator-sharing"}},
	{"metric-transferred-size", []string{"creator", "creator-sharing"}},
	{"modal-about-close", []string{"creator", "creator-sharing", "home"}},
	{"modal-about-dismiss", []string{"creator", "creator-sharing", "home"}},
	{"modal-backdrop", []string{"creator", "creator-sharing", "home"}},
	{"modal-close-cancel", []string{"creator", "creator-sharing"}},
	{"modal-file-id", []string{"creator-sharing"}},
	{"modal-file-subtitle", []string{"creator-sharing"}},
	{"modal-qr-close", []string{"creator", "creator-sharing"}},
	{"nav-btn-about", []string{"creator", "creator-sharing", "home"}},
	{"nav-btn-files", []string{"creator", "creator-sharing", "home"}},
	{"nav-btn-room", []string{"creator", "creator-sharing", "home"}},
	{"nav-btn-texts", []string{"creator", "creator-sharing"}},
	{"nav-file-count", []string{"creator", "creator-sharing", "home"}},
	{"nav-text-count", []string{"creator", "creator-sharing"}},
	{"participant-count", []string{"creator", "creator-sharing"}},
	{"participant-count-val", []string{"participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"participant-link-input", []string{"creator", "creator-sharing"}},
	{"participant-list", []string{"creator", "creator-sharing"}},
	{"participant-pin-input", []string{"participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"pin-barrier-card", []string{"participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"pin-cooldown", []string{"participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"pin-cooldown-text", []string{"participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"pin-error", []string{"participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"pin-error-text", []string{"participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"pin-eye-icon", []string{"home"}},
	{"pin-form", []string{"participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"pin-input", []string{"home"}},
	{"pin-slots-wrapper", []string{"participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"pin-toggle-btn", []string{"home"}},
	{"preview-body", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"preview-close-btn", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"preview-filename", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"progress-bar-fill", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"qr-box", []string{"creator", "creator-sharing"}},
	{"qr-image", []string{"creator", "creator-sharing"}},
	{"qr-lightbox-box", []string{"creator", "creator-sharing"}},
	{"room-active-card", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"room-closing-card", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"room-inactive-card", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"send-text-btn", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"share-error-box", []string{"creator-sharing"}},
	{"share-result-box", []string{"creator-sharing"}},
	{"share-ttl-select", []string{"creator-sharing"}},
	{"status-badge", []string{"participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"tab-files", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"tab-text", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"tab-text-badge", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"text-compose-error", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"text-count", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"text-input", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"text-list", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"text-mode-container", []string{"participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"text-size-counter", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"theme-btn-dark", []string{"creator", "creator-sharing", "home"}},
	{"theme-btn-light", []string{"creator", "creator-sharing", "home"}},
	{"theme-toggle-btn", []string{"share"}},
	{"ttl-select", []string{"home"}},
	{"unlock-btn", []string{"participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"unlock-btn-text", []string{"participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"upload-error", []string{"creator", "creator-sharing"}},
	{"upload-filename", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"upload-percent", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
	{"upload-progress-container", []string{"creator", "creator-sharing", "participant", "participant-pin-authenticated", "participant-pin-locked"}},
}

// siteJSIDsNotInTemplates are ids site.js looks up that no baseline template renders. site.js
// treats them as optional (null-guarded fallbacks or elements created at runtime).
var siteJSIDsNotInTemplates = map[string]string{
	"card-inactive":     "fallback for room-inactive-card; null-guarded",
	"create-btn":        "fallback for create-submit-btn; null-guarded",
	"create-room-error": "fallback for form-error; null-guarded",
	"room-pin-card":     "fallback for pin-barrier-card; null-guarded",
	"room-pin-input":    "fallback for pin-input; null-guarded",
	"room-ttl-select":   "fallback for ttl-select; null-guarded",
}

// bodyDataContract lists the data-* attributes on <body> for each page state.
var bodyDataContract = map[string][]string{
	stateHome:              {"page"},
	stateCreator:           {"closing-seconds", "expires", "global-share-enabled", "max-text-size", "page", "participant-token", "pin-required", "role", "status", "token"},
	stateCreatorSharing:    {"closing-seconds", "expires", "global-share-enabled", "max-text-size", "page", "participant-token", "pin-required", "role", "status", "token"},
	stateParticipant:       {"close-deadline", "closing-seconds", "expires", "is-locked", "max-text-size", "page", "pin-authenticated", "pin-required", "retry-after", "role", "status", "token"},
	stateParticipantLocked: {"close-deadline", "closing-seconds", "expires", "is-locked", "max-text-size", "page", "pin-authenticated", "pin-required", "retry-after", "role", "status", "token"},
	stateParticipantAuthed: {"close-deadline", "closing-seconds", "expires", "is-locked", "max-text-size", "page", "pin-authenticated", "pin-required", "retry-after", "role", "status", "token"},
	stateShare:             {"expires", "page", "token"},
}

var (
	idAttrRe     = regexp.MustCompile(`\bid="([^"]+)"`)
	bodyTagRe    = regexp.MustCompile(`(?s)<body\b[^>]*>`)
	dataAttrRe   = regexp.MustCompile(`\bdata-([a-z0-9-]+)=`)
	getByIDLitRe = regexp.MustCompile(`getElementById\(\s*['"]([^'"]+)['"]\s*\)`)
)

func contractGet(t *testing.T, a *App, path string, cookies ...*http.Cookie) string {
	t.Helper()
	resp := auditDo(a, http.MethodGet, path, "", "", cookies...)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET %s: unexpected status %d", path, resp.Code)
	}
	return resp.Body.String()
}

// renderContractStates renders every page state the contract covers.
func renderContractStates(t *testing.T) map[string]string {
	t.Helper()
	pages := map[string]string{}

	plain := testApp(t)
	pages[stateHome] = contractGet(t, plain, "/")
	open := auditCreateRoom(t, plain, `{"ttl_seconds": 3600}`)
	pages[stateCreator] = contractGet(t, plain, "/c/"+open.CreatorToken)
	pages[stateParticipant] = contractGet(t, plain, "/r/"+open.ParticipantToken)

	locked := auditCreateRoom(t, plain, `{"ttl_seconds": 3600, "pin": "2468"}`)
	pages[stateParticipantLocked] = contractGet(t, plain, "/r/"+locked.ParticipantToken)
	auth := auditDo(plain, http.MethodPost, "/api/v1/rooms/"+locked.ParticipantToken+"/auth/pin", "application/json", `{"pin": "2468"}`)
	if auth.Code != http.StatusOK {
		t.Fatalf("PIN auth failed: %d %s", auth.Code, auth.Body.String())
	}
	pages[stateParticipantAuthed] = contractGet(t, plain, "/r/"+locked.ParticipantToken, auth.Result().Cookies()...)

	cfg := config.Default()
	cfg.GlobalShareEnabled = true
	sharing := testAppWithConfig(t, cfg)
	shared := auditCreateRoom(t, sharing, `{"ttl_seconds": 3600}`)
	pages[stateCreatorSharing] = contractGet(t, sharing, "/c/"+shared.CreatorToken)

	upload := httptest.NewRecorder()
	sharing.Handler().ServeHTTP(upload, createMultipartRequest(t, "/api/v1/rooms/"+shared.CreatorToken+"/files", "file", "contract.txt", []byte("contract")))
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", upload.Code, upload.Body.String())
	}
	var file struct {
		ID string `json:"file_id"`
	}
	if err := json.Unmarshal(upload.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	shareResp := auditDo(sharing, http.MethodPost, "/api/v1/rooms/"+shared.CreatorToken+"/files/"+file.ID+"/share", "application/json", `{"ttl_seconds": 3600}`)
	if shareResp.Code != http.StatusCreated {
		t.Fatalf("share failed: %d %s", shareResp.Code, shareResp.Body.String())
	}
	var share struct {
		URL string `json:"share_url"`
	}
	if err := json.Unmarshal(shareResp.Body.Bytes(), &share); err != nil {
		t.Fatal(err)
	}
	i := strings.Index(share.URL, "/s/")
	if i < 0 {
		t.Fatalf("unexpected share_url %q", share.URL)
	}
	pages[stateShare] = contractGet(t, sharing, share.URL[i:])
	return pages
}

func idsIn(html string) map[string]bool {
	out := map[string]bool{}
	for _, m := range idAttrRe.FindAllStringSubmatch(html, -1) {
		out[m[1]] = true
	}
	return out
}

func bodyDataIn(t *testing.T, html string) []string {
	t.Helper()
	tag := bodyTagRe.FindString(html)
	if tag == "" {
		t.Fatal("no <body> tag")
	}
	var out []string
	for _, m := range dataAttrRe.FindAllStringSubmatch(tag, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

func siteJSIDs(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile("static/site.js")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, m := range getByIDLitRe.FindAllStringSubmatch(string(src), -1) {
		seen[m[1]] = true
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func TestTemplateContractIDs(t *testing.T) {
	pages := renderContractStates(t)
	present := map[string]map[string]bool{}
	for state, html := range pages {
		present[state] = idsIn(html)
	}

	if os.Getenv("HAMAL_CONTRACT_PRINT") == "1" {
		var b strings.Builder
		states := []string{stateCreator, stateCreatorSharing, stateHome, stateParticipant, stateParticipantAuthed, stateParticipantLocked, stateShare}
		sort.Strings(states)
		for _, id := range siteJSIDs(t) {
			var in []string
			for _, s := range states {
				if present[s][id] {
					in = append(in, fmt.Sprintf("%q", s))
				}
			}
			if len(in) == 0 {
				fmt.Fprintf(&b, "// %s: not rendered by any template\n", id)
				continue
			}
			fmt.Fprintf(&b, "\t{%q, []string{%s}},\n", id, strings.Join(in, ", "))
		}
		t.Log("\n" + b.String())
		return
	}

	for _, c := range siteJSIDContract {
		for _, state := range c.States {
			if !present[state][c.ID] {
				t.Errorf("id %q is read by site.js but is no longer rendered in the %s page", c.ID, state)
			}
		}
	}
	for id := range siteJSIDsNotInTemplates {
		for state, ids := range present {
			if ids[id] {
				t.Errorf("id %q is listed as not rendered by any template, but the %s page renders it; move it into siteJSIDContract", id, state)
			}
		}
	}
}

func TestTemplateContractBodyData(t *testing.T) {
	pages := renderContractStates(t)
	for state, want := range bodyDataContract {
		got := bodyDataIn(t, pages[state])
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s page <body> data attributes changed:\n got  %v\n want %v", state, got, want)
		}
	}
}

func TestTemplateContractCoversSiteJS(t *testing.T) {
	listed := map[string]bool{}
	for _, c := range siteJSIDContract {
		if listed[c.ID] {
			t.Errorf("id %q is listed twice", c.ID)
		}
		listed[c.ID] = true
		if len(c.States) == 0 {
			t.Errorf("id %q has no page states", c.ID)
		}
	}
	for id := range siteJSIDsNotInTemplates {
		listed[id] = true
	}
	inSiteJS := map[string]bool{}
	for _, id := range siteJSIDs(t) {
		inSiteJS[id] = true
		if !listed[id] {
			t.Errorf("site.js reads id %q, which is not in the template contract; add it (contract change)", id)
		}
	}
	for id := range listed {
		if !inSiteJS[id] {
			t.Errorf("contract lists id %q, but site.js no longer reads it; remove it (contract change)", id)
		}
	}
}
