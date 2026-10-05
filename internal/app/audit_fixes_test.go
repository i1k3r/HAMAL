package app

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/i1k3r/HAMAL/internal/config"
)

type auditRoom struct {
	RoomID           string `json:"room_id"`
	CreatorToken     string `json:"creator_token"`
	ParticipantToken string `json:"participant_token"`
}

func auditCreateRoom(t *testing.T, a *App, body string) auditRoom {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("create room failed: %d %s", resp.Code, resp.Body.String())
	}
	var rm auditRoom
	if err := json.NewDecoder(resp.Body).Decode(&rm); err != nil {
		t.Fatal(err)
	}
	return rm
}

func auditDo(a *App, method, path, contentType, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)
	return resp
}

func auditCount(t *testing.T, a *App, query string, args ...any) int {
	t.Helper()
	var n int
	if err := a.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// M1: small JSON/form endpoints reject oversized bodies instead of buffering them.
func TestSmallEndpointsRejectOversizedBodies(t *testing.T) {
	a := testApp(t)
	rm := auditCreateRoom(t, a, `{"ttl_seconds": 3600, "pin": "4821"}`)

	hugeJSON := `{"pin":"` + strings.Repeat("1", 2*maxSmallRequestBody) + `"}`
	hugeForm := "pin=" + strings.Repeat("1", 2*maxSmallRequestBody)

	cases := []struct {
		name, path, contentType, body string
	}{
		{"room creation JSON", "/api/v1/rooms", "application/json", hugeJSON},
		{"room creation form", "/api/v1/rooms", "application/x-www-form-urlencoded", hugeForm},
		{"PIN auth JSON", "/api/v1/rooms/" + rm.ParticipantToken + "/auth/pin", "application/json", hugeJSON},
		{"PIN auth form", "/api/v1/rooms/" + rm.ParticipantToken + "/auth/pin", "application/x-www-form-urlencoded", hugeForm},
	}
	roomsBefore := auditCount(t, a, "SELECT COUNT(*) FROM rooms")
	for _, tc := range cases {
		resp := auditDo(a, http.MethodPost, tc.path, tc.contentType, tc.body)
		if resp.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("%s: expected 413, got %d", tc.name, resp.Code)
		}
	}
	if got := auditCount(t, a, "SELECT COUNT(*) FROM rooms"); got != roomsBefore {
		t.Fatalf("oversized requests must not create rooms: before=%d after=%d", roomsBefore, got)
	}
	if attempts := auditCount(t, a, "SELECT pin_attempts FROM rooms WHERE id = ?", rm.RoomID); attempts != 0 {
		t.Fatalf("oversized PIN requests must not be processed as attempts, got %d", attempts)
	}

	// Normal requests still work.
	if resp := auditDo(a, http.MethodPost, "/api/v1/rooms/"+rm.ParticipantToken+"/auth/pin", "application/json", `{"pin":"4821"}`); resp.Code != http.StatusOK {
		t.Fatalf("normal PIN auth must still succeed, got %d", resp.Code)
	}
}

// M1: a huge multipart Content-Type is neither stored nor echoed back.
func TestUploadContentTypeIsBounded(t *testing.T) {
	a := testApp(t)
	rm := auditCreateRoom(t, a, `{"ttl_seconds": 3600}`)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="note.txt"`)
	header.Set("Content-Type", "text/"+strings.Repeat("a", 64<<10))
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("plain text payload"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+rm.ParticipantToken+"/files", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", resp.Code, resp.Body.String())
	}
	var uploaded struct {
		FileID      string `json:"file_id"`
		ContentType string `json:"content_type"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&uploaded)
	if len(uploaded.ContentType) > 255 || !strings.HasPrefix(uploaded.ContentType, "text/plain") {
		t.Fatalf("expected sniffed, bounded content type, got %d bytes", len(uploaded.ContentType))
	}

	dl := auditDo(a, http.MethodGet, "/api/v1/rooms/"+rm.ParticipantToken+"/files/"+uploaded.FileID, "", "")
	if dl.Code != http.StatusOK || len(dl.Header().Get("Content-Type")) > 255 {
		t.Fatalf("download must return a bounded Content-Type, got %d / %d bytes", dl.Code, len(dl.Header().Get("Content-Type")))
	}
}

// L2: participant details are not disclosed, and presence is not recorded, before PIN authentication.
func TestParticipantDetailsHiddenBeforePINAuth(t *testing.T) {
	a := testApp(t)
	rm := auditCreateRoom(t, a, `{"ttl_seconds": 3600, "pin": "4821"}`)

	type status struct {
		PinAuthenticated bool                `json:"pin_authenticated"`
		ParticipantCount int                 `json:"participant_count"`
		Participants     []ParticipantRecord `json:"participants"`
	}
	poll := func(token, remoteAddr string, cookies ...*http.Cookie) status {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+token, nil)
		req.RemoteAddr = remoteAddr
		for _, c := range cookies {
			req.AddCookie(c)
		}
		resp := httptest.NewRecorder()
		a.Handler().ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("status poll failed: %d", resp.Code)
		}
		var st status
		if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
			t.Fatal(err)
		}
		return st
	}

	// An authenticated participant from 192.168.1.20 is present in the room.
	authReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+rm.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin":"4821"}`))
	authReq.Header.Set("Content-Type", "application/json")
	authReq.RemoteAddr = "192.168.1.20:40000"
	authResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(authResp, authReq)
	if authResp.Code != http.StatusOK {
		t.Fatalf("PIN auth failed: %d", authResp.Code)
	}
	cookies := authResp.Result().Cookies()

	// A visitor who only has the link (no PIN) learns nothing about participants...
	unauth := poll(rm.ParticipantToken, "192.168.1.99:40000")
	if unauth.PinAuthenticated || unauth.ParticipantCount != 0 || len(unauth.Participants) != 0 {
		t.Fatalf("unauthenticated poll leaked participant data: %+v", unauth)
	}
	// ...and loading the page does not register them either.
	pageReq := httptest.NewRequest(http.MethodGet, "/r/"+rm.ParticipantToken, nil)
	pageReq.RemoteAddr = "192.168.1.99:40000"
	a.Handler().ServeHTTP(httptest.NewRecorder(), pageReq)

	creatorView := poll(rm.CreatorToken, "192.168.1.10:40000")
	if creatorView.ParticipantCount != 1 || len(creatorView.Participants) != 1 || creatorView.Participants[0].IP != "192.168.1.20" {
		t.Fatalf("creator must see only the authenticated participant, got %+v", creatorView)
	}

	// Presence still works for authenticated participants.
	authed := poll(rm.ParticipantToken, "192.168.1.20:40000", cookies...)
	if !authed.PinAuthenticated || authed.ParticipantCount != 1 || len(authed.Participants) != 1 {
		t.Fatalf("authenticated participant must receive presence, got %+v", authed)
	}
}

// L4: the server only accepts 4-8 digit PINs, matching both frontend forms.
func TestRoomCreationRequiresNumericPIN(t *testing.T) {
	a := testApp(t)
	for _, pin := range []string{"abcd", "12ab", "12 34", "123", "123456789"} {
		resp := auditDo(a, http.MethodPost, "/api/v1/rooms", "application/json", `{"ttl_seconds": 3600, "pin": "`+pin+`"}`)
		if resp.Code != http.StatusBadRequest {
			t.Errorf("PIN %q: expected 400, got %d", pin, resp.Code)
		}
	}
	for _, pin := range []string{"1234", "00000000"} {
		auditCreateRoom(t, a, `{"ttl_seconds": 3600, "pin": "`+pin+`"}`)
	}

	home := auditDo(a, http.MethodGet, "/", "", "").Body.String()
	if !strings.Contains(home, `id="pin-input"`) || !strings.Contains(home, `pattern="[0-9]{4,8}"`) {
		t.Error("room creation form must enforce the numeric PIN pattern")
	}
	rm := auditCreateRoom(t, a, `{"ttl_seconds": 3600, "pin": "1234"}`)
	join := auditDo(a, http.MethodGet, "/r/"+rm.ParticipantToken, "", "").Body.String()
	if !strings.Contains(join, `id="participant-pin-input"`) || !strings.Contains(join, `pattern="[0-9]{4,8}"`) {
		t.Error("participant PIN form must enforce the same numeric PIN pattern")
	}
}

// L9: /auth/pin cannot be used to mint sessions or system messages when no session is needed.
func TestPINAuthDoesNotCreateRedundantSessionsOrMessages(t *testing.T) {
	cfg := config.Default()
	cfg.ShareManagementRateLimit = 10000 // keep the per-IP auth limiter out of this test
	a := testAppWithConfig(t, cfg)

	open := auditCreateRoom(t, a, `{"ttl_seconds": 3600}`)
	for i := 0; i < 5; i++ {
		if resp := auditDo(a, http.MethodPost, "/api/v1/rooms/"+open.ParticipantToken+"/auth/pin", "application/json", `{}`); resp.Code != http.StatusOK {
			t.Fatalf("auth on a room without PIN should stay a harmless 200, got %d", resp.Code)
		}
	}
	if n := auditCount(t, a, "SELECT COUNT(*) FROM room_sessions WHERE room_id = ?", open.RoomID); n != 0 {
		t.Fatalf("no sessions expected for a room without PIN, got %d", n)
	}
	if n := auditCount(t, a, "SELECT COUNT(*) FROM texts WHERE room_id = ?", open.RoomID); n != 1 {
		t.Fatalf("expected only the 'Room created' message, got %d texts", n)
	}

	locked := auditCreateRoom(t, a, `{"ttl_seconds": 3600, "pin": "4821"}`)
	first := auditDo(a, http.MethodPost, "/api/v1/rooms/"+locked.ParticipantToken+"/auth/pin", "application/json", `{"pin":"4821"}`)
	if first.Code != http.StatusOK {
		t.Fatalf("PIN auth failed: %d", first.Code)
	}
	cookies := first.Result().Cookies()
	for i := 0; i < 5; i++ {
		again := auditDo(a, http.MethodPost, "/api/v1/rooms/"+locked.ParticipantToken+"/auth/pin", "application/json", `{"pin":"4821"}`, cookies...)
		if again.Code != http.StatusOK {
			t.Fatalf("re-auth with a valid session should return 200, got %d", again.Code)
		}
	}
	if n := auditCount(t, a, "SELECT COUNT(*) FROM room_sessions WHERE room_id = ?", locked.RoomID); n != 1 {
		t.Fatalf("an already authenticated client must keep one session, got %d", n)
	}
	if n := auditCount(t, a, "SELECT COUNT(*) FROM texts WHERE room_id = ? AND content = 'Client connected'", locked.RoomID); n != 1 {
		t.Fatalf("expected exactly one 'Client connected' message, got %d", n)
	}
}

// L9: text is stored and returned byte-for-byte, and other clients' session identifiers stay private.
func TestTextPreservesWhitespaceAndHidesSessionIDs(t *testing.T) {
	a := testApp(t)
	rm := auditCreateRoom(t, a, `{"ttl_seconds": 3600}`)

	content := "    services:\n      web:\n        image: nginx\n\n"
	payload, _ := json.Marshal(map[string]string{"content": content, "client_session_id": "cs_alice_secret_session"})
	post := auditDo(a, http.MethodPost, "/api/v1/rooms/"+rm.ParticipantToken+"/texts", "application/json", string(payload))
	if post.Code != http.StatusCreated {
		t.Fatalf("text post failed: %d %s", post.Code, post.Body.String())
	}
	if strings.Contains(post.Body.String(), "cs_alice_secret_session") {
		t.Fatal("POST response must not echo the sender session id")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+rm.CreatorToken+"/texts", nil)
	req.Header.Set("X-Client-Session-ID", "cs_bob")
	list := httptest.NewRecorder()
	a.Handler().ServeHTTP(list, req)
	raw := list.Body.String()
	if strings.Contains(raw, "cs_alice_secret_session") || strings.Contains(raw, "sender_session_id") {
		t.Fatalf("text list leaks sender session ids: %s", raw)
	}
	var data struct {
		Texts []struct {
			SenderType string `json:"sender_type"`
			Content    string `json:"content"`
			IsSelf     bool   `json:"is_self"`
		} `json:"texts"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, txt := range data.Texts {
		if txt.SenderType == "client" {
			found = true
			if txt.Content != content {
				t.Fatalf("text content was altered: %q", txt.Content)
			}
			if txt.IsSelf {
				t.Fatal("another client's message must not be marked is_self")
			}
		}
	}
	if !found {
		t.Fatal("client text missing from list")
	}

	// is_self still works for the original sender.
	selfReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+rm.ParticipantToken+"/texts", nil)
	selfReq.Header.Set("X-Client-Session-ID", "cs_alice_secret_session")
	selfList := httptest.NewRecorder()
	a.Handler().ServeHTTP(selfList, selfReq)
	if !strings.Contains(selfList.Body.String(), `"is_self":true`) {
		t.Fatal("sender must still see is_self=true")
	}

	// An oversized body-supplied session id is ignored rather than stored.
	longSess, _ := json.Marshal(map[string]string{"content": "x", "client_session_id": strings.Repeat("s", 4096)})
	if resp := auditDo(a, http.MethodPost, "/api/v1/rooms/"+rm.ParticipantToken+"/texts", "application/json", string(longSess)); resp.Code != http.StatusCreated {
		t.Fatalf("text post with oversized session id should still succeed, got %d", resp.Code)
	}
	if n := auditCount(t, a, "SELECT COALESCE(MAX(LENGTH(sender_session_id)), 0) FROM texts WHERE room_id = ?", rm.RoomID); n > maxClientSessionIDLength {
		t.Fatalf("stored session id exceeds bound: %d", n)
	}
}

// L9: system messages cannot use up the capacity that user text needs.
func TestSystemMessagesDoNotExhaustTextCapacity(t *testing.T) {
	cfg := config.Default()
	cfg.MaxTextsPerRoom = 5
	cfg.ShareManagementRateLimit = 10000
	a := testAppWithConfig(t, cfg)
	rm := auditCreateRoom(t, a, `{"ttl_seconds": 3600}`)

	// Fill the room with system messages (uploads each add "File uploaded: ...").
	for i := 0; i < 8; i++ {
		req := createMultipartRequest(t, "/api/v1/rooms/"+rm.CreatorToken+"/files", "file", "f.txt", []byte("data"))
		resp := httptest.NewRecorder()
		a.Handler().ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("upload %d failed: %d", i, resp.Code)
		}
	}
	if n := auditCount(t, a, "SELECT COUNT(*) FROM texts WHERE room_id = ?", rm.RoomID); n != 5 {
		t.Fatalf("system messages must rotate within the cap, got %d", n)
	}

	// Users still get the full capacity.
	for i := 0; i < 5; i++ {
		if resp := auditDo(a, http.MethodPost, "/api/v1/rooms/"+rm.ParticipantToken+"/texts", "application/json", `{"content":"user text"}`); resp.Code != http.StatusCreated {
			t.Fatalf("user text %d rejected with %d: %s", i, resp.Code, resp.Body.String())
		}
	}
	if resp := auditDo(a, http.MethodPost, "/api/v1/rooms/"+rm.ParticipantToken+"/texts", "application/json", `{"content":"one too many"}`); resp.Code != http.StatusBadRequest {
		t.Fatalf("expected limit error once the room holds only user text, got %d", resp.Code)
	}
	if n := auditCount(t, a, "SELECT COUNT(*) FROM texts WHERE room_id = ? AND sender_type = 'client'", rm.RoomID); n != 5 {
		t.Fatalf("expected 5 client texts, got %d", n)
	}
	if n := auditCount(t, a, "SELECT COUNT(*) FROM texts WHERE room_id = ?", rm.RoomID); n != 5 {
		t.Fatalf("total must stay within the cap, got %d", n)
	}
}

// L13: pages make no third-party requests and no longer claim files are encrypted.
func TestPagesAreSelfContainedAndClaimsAreAccurate(t *testing.T) {
	cfg := config.Default()
	cfg.GlobalShareEnabled = true
	a := testAppWithConfig(t, cfg)
	rm := auditCreateRoom(t, a, `{"ttl_seconds": 3600}`)

	pages := map[string]string{
		"home":        "/",
		"creator":     "/c/" + rm.CreatorToken,
		"participant": "/r/" + rm.ParticipantToken,
		"share":       "/s/gsh_" + strings.Repeat("0", 64),
	}
	for name, path := range pages {
		resp := auditDo(a, http.MethodGet, path, "", "")
		html := resp.Body.String()
		if name != "share" {
			if resp.Code != http.StatusOK {
				t.Fatalf("%s page: unexpected status %d", name, resp.Code)
			}
			if !strings.Contains(html, `href="/static/fonts/fonts.css"`) {
				t.Errorf("%s page must load the self-hosted font stylesheet", name)
			}
		}
		for _, forbidden := range []string{"fonts.googleapis.com", "fonts.gstatic.com", "Files are encrypted", "will not be interrupted", "Cancel Close"} {
			if strings.Contains(html, forbidden) {
				t.Errorf("%s page still contains %q", name, forbidden)
			}
		}
		if csp := resp.Header().Get("Content-Security-Policy"); strings.Contains(csp, "googleapis") || strings.Contains(csp, "gstatic") {
			t.Errorf("%s page CSP still allows Google Fonts: %s", name, csp)
		}
	}

	css := auditDo(a, http.MethodGet, "/static/fonts/fonts.css", "", "")
	if css.Code != http.StatusOK || !strings.Contains(css.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("fonts.css not served as CSS: %d %q", css.Code, css.Header().Get("Content-Type"))
	}
	body, _ := io.ReadAll(css.Body)
	for _, family := range []string{"'Inter'", "'JetBrains Mono'", "'Caveat'"} {
		if !bytes.Contains(body, []byte(family)) {
			t.Errorf("fonts.css is missing %s", family)
		}
	}
	if bytes.Contains(body, []byte("http")) {
		t.Error("fonts.css must only reference local files")
	}
	font := auditDo(a, http.MethodGet, "/static/fonts/inter-latin-wght-normal.woff2", "", "")
	if font.Code != http.StatusOK || !strings.HasPrefix(font.Header().Get("Content-Type"), "font/woff2") {
		t.Fatalf("font file not served correctly: %d %q", font.Code, font.Header().Get("Content-Type"))
	}
}

// L14: the configured text limit reaches the UI, and a bare IP is a valid trusted proxy.
func TestConfiguredLimitsReachUIAndBareIPProxyIsTrusted(t *testing.T) {
	cfg := config.Default()
	cfg.MaxTextSize = 1 << 20
	cfg.ShareManagementRateLimit = 2 // burst 5
	cfg.TrustedProxies = []string{"10.0.0.1"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("a bare IP must be accepted as a trusted proxy: %v", err)
	}
	a := testAppWithConfig(t, cfg)

	createVia := func(clientIP string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.0.0.1:50000"
		req.Header.Set("X-Forwarded-For", clientIP)
		resp := httptest.NewRecorder()
		a.Handler().ServeHTTP(resp, req)
		return resp.Code
	}
	for i := 0; i < 5; i++ {
		if code := createVia("203.0.113.42"); code != http.StatusCreated {
			t.Fatalf("request %d through bare-IP proxy: expected 201, got %d", i+1, code)
		}
	}
	if code := createVia("203.0.113.42"); code != http.StatusTooManyRequests {
		t.Fatalf("expected the forwarded client to be rate limited, got %d", code)
	}
	if code := createVia("203.0.113.43"); code != http.StatusCreated {
		t.Fatalf("a different forwarded client must have its own bucket, got %d", code)
	}

	rm := auditCreateRoom(t, a, `{"ttl_seconds": 3600}`)
	for name, path := range map[string]string{"creator": "/c/" + rm.CreatorToken, "participant": "/r/" + rm.ParticipantToken} {
		html := auditDo(a, http.MethodGet, path, "", "").Body.String()
		if !strings.Contains(html, `data-max-text-size="1048576"`) {
			t.Errorf("%s page must expose the configured max text size", name)
		}
	}
}

// Bundled font files are cacheable long-term; no other static asset changes its caching.
func TestFontFilesHaveLongLivedCacheHeaders(t *testing.T) {
	a := testApp(t)

	font := auditDo(a, http.MethodGet, "/static/fonts/inter-latin-wght-normal.woff2", "", "")
	if font.Code != http.StatusOK {
		t.Fatalf("font not served: %d", font.Code)
	}
	if cc := font.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=31536000") || !strings.Contains(cc, "immutable") {
		t.Fatalf("font file must be cacheable long-term, got Cache-Control %q", cc)
	}

	for _, path := range []string{"/static/fonts/fonts.css", "/static/site.css", "/static/site.js", "/static/fonts/missing.woff2"} {
		resp := auditDo(a, http.MethodGet, path, "", "")
		if cc := resp.Header().Get("Cache-Control"); cc != "" {
			t.Errorf("%s must keep default caching, got Cache-Control %q", path, cc)
		}
	}
	if missing := auditDo(a, http.MethodGet, "/static/fonts/missing.woff2", "", ""); missing.Code != http.StatusNotFound {
		t.Errorf("missing font should be 404, got %d", missing.Code)
	}
}
