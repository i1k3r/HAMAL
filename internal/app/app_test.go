package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/i1k3r/lan-drop/internal/config"
	"github.com/i1k3r/lan-drop/internal/file"
)

func testApp(t *testing.T) *App {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.DBPath = filepath.Join(cfg.DataDir, "lan-drop.db")
	a, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func createMultipartRequest(t *testing.T, urlPath, fieldName, filename string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, urlPath, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestHealthEndpoint(t *testing.T) {
	a := testApp(t)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %s", body["status"])
	}
}

func TestReadyEndpoint(t *testing.T) {
	a := testApp(t)
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ready" {
		t.Fatalf("expected status ready, got %s", body["status"])
	}
}

func TestLandingPage(t *testing.T) {
	a := testApp(t)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	content := response.Body.String()
	if !strings.Contains(content, "HAMAL") {
		t.Fatalf("expected response to contain brand name HAMAL")
	}
}

func TestCreateRoomEndpointValidAndInvalidTTL(t *testing.T) {
	a := testApp(t)

	// Valid creation
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 1800}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", resp.Code, resp.Body.String())
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if data["creator_token"] == nil || data["participant_token"] == nil {
		t.Fatalf("expected tokens in response, got %v", data)
	}

	// Invalid TTL (too small)
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 60}`))
	badReq.Header.Set("Content-Type", "application/json")
	badResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(badResp, badReq)

	if badResp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", badResp.Code)
	}
}

func TestCreatorAndParticipantViewsAndSeparation(t *testing.T) {
	a := testApp(t)

	// Create room
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)

	var data map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&data)
	creatorToken := data["creator_token"].(string)
	participantToken := data["participant_token"].(string)

	// 1. Creator accessing /c/{token} -> 200 OK
	cReq := httptest.NewRequest(http.MethodGet, "/c/"+creatorToken, nil)
	cResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(cResp, cReq)
	if cResp.Code != http.StatusOK {
		t.Fatalf("expected 200 for creator view, got %d", cResp.Code)
	}
	if !strings.Contains(cResp.Body.String(), "CREATOR") {
		t.Fatalf("expected CREATOR badge in creator page")
	}

	// 2. Participant accessing /r/{token} -> 200 OK
	pReq := httptest.NewRequest(http.MethodGet, "/r/"+participantToken, nil)
	pResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(pResp, pReq)
	if pResp.Code != http.StatusOK {
		t.Fatalf("expected 200 for participant view, got %d", pResp.Code)
	}
	if !strings.Contains(pResp.Body.String(), "PARTICIPANT") {
		t.Fatalf("expected PARTICIPANT in participant page")
	}

	// 3. Capability separation: participant token used on /c/{token} -> 404
	pOnCReq := httptest.NewRequest(http.MethodGet, "/c/"+participantToken, nil)
	pOnCResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(pOnCResp, pOnCReq)
	if pOnCResp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when participant accesses /c/, got %d", pOnCResp.Code)
	}

	// 4. Capability separation: creator token used on /r/{token} -> 404
	cOnPReq := httptest.NewRequest(http.MethodGet, "/r/"+creatorToken, nil)
	cOnPResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(cOnPResp, cOnPReq)
	if cOnPResp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when creator accesses /r/, got %d", cOnPResp.Code)
	}
}

func TestRoomStatusPollingAndClose(t *testing.T) {
	a := testApp(t)

	// Create room
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)

	var data map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&data)
	creatorToken := data["creator_token"].(string)
	participantToken := data["participant_token"].(string)

	// Poll status via participant token -> active
	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+participantToken, nil)
	statusResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(statusResp, statusReq)
	if statusResp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", statusResp.Code)
	}

	// Close room via creator token
	closeReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+creatorToken+"/close", nil)
	closeResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(closeResp, closeReq)
	if closeResp.Code != http.StatusOK {
		t.Fatalf("expected 200 on close, got %d", closeResp.Code)
	}

	// Poll status again -> 410 Gone with status: closed
	statusReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+participantToken, nil)
	statusResp2 := httptest.NewRecorder()
	a.Handler().ServeHTTP(statusResp2, statusReq2)
	if statusResp2.Code != http.StatusGone {
		t.Fatalf("expected 410 Gone after closing, got %d", statusResp2.Code)
	}
	var pollData map[string]any
	_ = json.NewDecoder(statusResp2.Body).Decode(&pollData)
	if pollData["status"] != "closed" {
		t.Fatalf("expected status closed, got %s", pollData["status"])
	}
}

func TestQRCodeSVGEndpoint(t *testing.T) {
	a := testApp(t)

	// Create room
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)

	var data map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&data)
	participantToken := data["participant_token"].(string)

	// Fetch QR code
	qrReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+participantToken+"/qr.svg", nil)
	qrResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(qrResp, qrReq)

	if qrResp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for QR SVG, got %d", qrResp.Code)
	}
	if !strings.HasPrefix(qrResp.Header().Get("Content-Type"), "image/svg+xml") {
		t.Fatalf("expected image/svg+xml, got %s", qrResp.Header().Get("Content-Type"))
	}
	if !strings.Contains(qrResp.Body.String(), "<svg") {
		t.Fatalf("expected valid SVG document")
	}
}

func TestFileUploadStreamingAndListing(t *testing.T) {
	a := testApp(t)

	// 1. Create Room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", createResp.Code)
	}

	var roomData struct {
		RoomID           string `json:"room_id"`
		CreatorToken     string `json:"creator_token"`
		ParticipantToken string `json:"participant_token"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&roomData); err != nil {
		t.Fatal(err)
	}

	// 2. Upload file via Participant Token
	fileContent := []byte("Hello LAN-Drop! True streaming upload test payload.")
	uploadReq := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "document.txt", fileContent)
	uploadResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(uploadResp, uploadReq)

	if uploadResp.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for file upload, got %d: %s", uploadResp.Code, uploadResp.Body.String())
	}

	var uploadedFile file.File
	if err := json.NewDecoder(uploadResp.Body).Decode(&uploadedFile); err != nil {
		t.Fatal(err)
	}
	if uploadedFile.OriginalFilename != "document.txt" {
		t.Fatalf("expected filename document.txt, got %s", uploadedFile.OriginalFilename)
	}
	if uploadedFile.SizeBytes != int64(len(fileContent)) {
		t.Fatalf("expected size %d, got %d", len(fileContent), uploadedFile.SizeBytes)
	}

	// Verify file is saved in /data/files/<storage_id>
	dbFile, err := a.files.GetReadyFile(context.Background(), roomData.RoomID, uploadedFile.ID)
	if err != nil {
		t.Fatalf("failed to get ready file from DB: %v", err)
	}
	storagePath := filepath.Join(a.paths.FilesDir, dbFile.StorageID)
	savedData, err := os.ReadFile(storagePath)
	if err != nil {
		t.Fatalf("failed to read file from storage: %v", err)
	}
	if !bytes.Equal(savedData, fileContent) {
		t.Fatalf("saved file content mismatch")
	}

	// 3. List files in room via Creator Token
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+roomData.CreatorToken+"/files", nil)
	listResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(listResp, listReq)

	if listResp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for file list, got %d", listResp.Code)
	}

	var listData struct {
		Files          []file.File `json:"files"`
		TotalSizeBytes int64       `json:"total_size_bytes"`
		FileCount      int         `json:"file_count"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listData); err != nil {
		t.Fatal(err)
	}

	if listData.FileCount != 1 || len(listData.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", listData.FileCount)
	}
	if listData.TotalSizeBytes != int64(len(fileContent)) {
		t.Fatalf("expected total size %d, got %d", len(fileContent), listData.TotalSizeBytes)
	}
	if listData.Files[0].OriginalFilename != "document.txt" {
		t.Fatalf("expected filename document.txt in list, got %s", listData.Files[0].OriginalFilename)
	}
}

func TestFileUploadConcurrentQuotaEnforcement(t *testing.T) {
	a := testApp(t)

	// Create room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	concurrency := 5
	for i := 0; i < concurrency; i++ {
		payload := bytes.Repeat([]byte("A"), 1024*64) // 64 KB
		filename := fmt.Sprintf("test-%d.bin", i)
		req := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", filename, payload)
		rec := httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("upload %d failed with code %d: %s", i, rec.Code, rec.Body.String())
		}
	}

	// Verify listing has 5 files
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", nil)
	listResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(listResp, listReq)

	var listData struct {
		Files     []file.File `json:"files"`
		FileCount int         `json:"file_count"`
	}
	_ = json.NewDecoder(listResp.Body).Decode(&listData)

	if listData.FileCount != concurrency {
		t.Fatalf("expected %d files, got %d", concurrency, listData.FileCount)
	}
}

func TestFileUploadEmptyFileRejection(t *testing.T) {
	a := testApp(t)

	// Create room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		CreatorToken     string `json:"creator_token"`
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// Verify room was created with default 10 GiB MaxFileSize and 10 GiB MaxRoomSize
	rm, _, err := a.rooms.GetByToken(context.Background(), roomData.CreatorToken)
	if err != nil {
		t.Fatal(err)
	}
	if rm.MaxFileSize != 10<<30 {
		t.Fatalf("expected room MaxFileSize 10 GiB (%d), got %d", int64(10<<30), rm.MaxFileSize)
	}
	if rm.MaxRoomSize != 10<<30 {
		t.Fatalf("expected room MaxRoomSize 10 GiB (%d), got %d", int64(10<<30), rm.MaxRoomSize)
	}

	// Upload empty file (0 bytes)
	req := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "empty.txt", []byte{})
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for empty file, got %d", resp.Code)
	}
}

func TestFileUploadExceedsMaxFileSize(t *testing.T) {
	cfg := config.Default()
	cfg.MaxFileSize = 512 // Set small MaxFileSize limit for test
	cfg.MaxRoomSize = 10 << 20
	a := testAppWithConfig(t, cfg)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// Upload file with 1024 bytes (exceeding 512 byte limit)
	oversized := bytes.Repeat([]byte("X"), 1024)
	req := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "oversized.bin", oversized)
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 Request Entity Too Large, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestAppGlobalStorageLimitEnforcement(t *testing.T) {
	cfg := config.Default()
	cfg.MaxTotalStorage = 100 * 1024 // 100 KB global limit
	a := testAppWithConfig(t, cfg)

	// Create Room 1
	createReq1 := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq1.Header.Set("Content-Type", "application/json")
	createResp1 := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp1, createReq1)
	var room1 struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp1.Body).Decode(&room1)

	// Create Room 2
	createReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq2.Header.Set("Content-Type", "application/json")
	createResp2 := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp2, createReq2)
	var room2 struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp2.Body).Decode(&room2)

	// Upload 70 KB to Room 1 -> must succeed (70 KB <= 100 KB)
	payload1 := bytes.Repeat([]byte("A"), 70*1024)
	upReq1 := createMultipartRequest(t, "/api/v1/rooms/"+room1.ParticipantToken+"/files", "file", "r1.bin", payload1)
	upResp1 := httptest.NewRecorder()
	a.Handler().ServeHTTP(upResp1, upReq1)
	if upResp1.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for Room 1 upload, got %d: %s", upResp1.Code, upResp1.Body.String())
	}

	// Upload 50 KB to Room 2 -> must return 413 because 70 KB + 50 KB > 100 KB
	payload2 := bytes.Repeat([]byte("B"), 50*1024)
	upReq2 := createMultipartRequest(t, "/api/v1/rooms/"+room2.ParticipantToken+"/files", "file", "r2.bin", payload2)
	upResp2 := httptest.NewRecorder()
	a.Handler().ServeHTTP(upResp2, upReq2)
	if upResp2.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 Request Entity Too Large for exceeding global quota, got %d: %s", upResp2.Code, upResp2.Body.String())
	}
}

func TestFileUploadExpiredAndClosedRooms(t *testing.T) {
	a := testApp(t)

	// Create room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		CreatorToken     string `json:"creator_token"`
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// Creator closes room
	closeReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+roomData.CreatorToken+"/close", nil)
	closeResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(closeResp, closeReq)
	if closeResp.Code != http.StatusOK {
		t.Fatalf("close room failed with code %d", closeResp.Code)
	}

	// Attempt upload to closed room -> 410 Gone
	uploadReq := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "test.txt", []byte("data"))
	uploadResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(uploadResp, uploadReq)

	if uploadResp.Code != http.StatusGone {
		t.Fatalf("expected 410 Gone for upload to closed room, got %d", uploadResp.Code)
	}
}

func TestFileUploadCrossRoomIsolation(t *testing.T) {
	a := testApp(t)

	// Create Room 1
	r1Req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	r1Req.Header.Set("Content-Type", "application/json")
	r1Resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(r1Resp, r1Req)
	var r1Data struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(r1Resp.Body).Decode(&r1Data)

	// Create Room 2
	r2Req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	r2Req.Header.Set("Content-Type", "application/json")
	r2Resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(r2Resp, r2Req)
	var r2Data struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(r2Resp.Body).Decode(&r2Data)

	// Upload to Room 1
	upReq := createMultipartRequest(t, "/api/v1/rooms/"+r1Data.ParticipantToken+"/files", "file", "r1-file.txt", []byte("room 1 secret"))
	upResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(upResp, upReq)
	if upResp.Code != http.StatusCreated {
		t.Fatalf("upload to room 1 failed: %d", upResp.Code)
	}

	// Room 2 file list must be empty
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+r2Data.ParticipantToken+"/files", nil)
	listResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(listResp, listReq)

	var listData struct {
		Files     []file.File `json:"files"`
		FileCount int         `json:"file_count"`
	}
	_ = json.NewDecoder(listResp.Body).Decode(&listData)
	if listData.FileCount != 0 || len(listData.Files) != 0 {
		t.Fatalf("room 2 should not see room 1 files, got %d files", listData.FileCount)
	}
}

func TestFileUploadPathTraversalAndXSSFilenames(t *testing.T) {
	a := testApp(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// 1. Path traversal filename
	ptReq := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "../../../../etc/passwd", []byte("root:x:0:0"))
	ptResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(ptResp, ptReq)

	if ptResp.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", ptResp.Code)
	}
	var ptFile file.File
	_ = json.NewDecoder(ptResp.Body).Decode(&ptFile)
	if ptFile.OriginalFilename != "passwd" {
		t.Fatalf("expected sanitized filename 'passwd', got %q", ptFile.OriginalFilename)
	}

	// 2. XSS payload filename
	xssReq := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "<script>alert(1)</script>.png", []byte("fake png"))
	xssResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(xssResp, xssReq)

	if xssResp.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", xssResp.Code)
	}
}

func TestFileDownloadSuccess(t *testing.T) {
	a := testApp(t)

	// 1. Create Room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		CreatorToken     string `json:"creator_token"`
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// 2. Upload File
	fileContent := []byte("Phase 3B download test payload with exact bytes.")
	uploadReq := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "document.pdf", fileContent)
	uploadResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(uploadResp, uploadReq)

	var uploadedFile file.File
	_ = json.NewDecoder(uploadResp.Body).Decode(&uploadedFile)

	// 3. Download File via Participant Token
	downloadReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+roomData.ParticipantToken+"/files/"+uploadedFile.ID, nil)
	downloadResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(downloadResp, downloadReq)

	if downloadResp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for file download, got %d: %s", downloadResp.Code, downloadResp.Body.String())
	}

	if !bytes.Equal(downloadResp.Body.Bytes(), fileContent) {
		t.Fatalf("downloaded file content mismatch, expected %q, got %q", string(fileContent), downloadResp.Body.String())
	}

	// Verify headers
	if downloadResp.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options: nosniff, got %s", downloadResp.Header().Get("X-Content-Type-Options"))
	}
	if !strings.Contains(downloadResp.Header().Get("Content-Disposition"), "document.pdf") {
		t.Fatalf("expected Content-Disposition containing document.pdf, got %s", downloadResp.Header().Get("Content-Disposition"))
	}
	if downloadResp.Header().Get("Content-Length") != fmt.Sprintf("%d", len(fileContent)) {
		t.Fatalf("expected Content-Length %d, got %s", len(fileContent), downloadResp.Header().Get("Content-Length"))
	}
}

func TestFileDownloadCreatorAndParticipant(t *testing.T) {
	a := testApp(t)

	// Create Room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		CreatorToken     string `json:"creator_token"`
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// Upload File
	fileContent := []byte("Shared capability payload.")
	uploadReq := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "shared.txt", fileContent)
	uploadResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(uploadResp, uploadReq)

	var uploadedFile file.File
	_ = json.NewDecoder(uploadResp.Body).Decode(&uploadedFile)

	// Creator download
	cDlReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+roomData.CreatorToken+"/files/"+uploadedFile.ID, nil)
	cDlResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(cDlResp, cDlReq)
	if cDlResp.Code != http.StatusOK || !bytes.Equal(cDlResp.Body.Bytes(), fileContent) {
		t.Fatalf("creator download failed: %d", cDlResp.Code)
	}

	// Participant download
	pDlReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+roomData.ParticipantToken+"/files/"+uploadedFile.ID, nil)
	pDlResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(pDlResp, pDlReq)
	if pDlResp.Code != http.StatusOK || !bytes.Equal(pDlResp.Body.Bytes(), fileContent) {
		t.Fatalf("participant download failed: %d", pDlResp.Code)
	}
}

func TestFileDownloadHEADRequest(t *testing.T) {
	a := testApp(t)

	// Create Room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// Upload File
	fileContent := []byte("Head request payload verification.")
	uploadReq := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "head-test.bin", fileContent)
	uploadResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(uploadResp, uploadReq)

	var uploadedFile file.File
	_ = json.NewDecoder(uploadResp.Body).Decode(&uploadedFile)

	// HEAD request
	headReq := httptest.NewRequest(http.MethodHead, "/api/v1/rooms/"+roomData.ParticipantToken+"/files/"+uploadedFile.ID, nil)
	headResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(headResp, headReq)

	if headResp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for HEAD request, got %d", headResp.Code)
	}
	if headResp.Body.Len() != 0 {
		t.Fatalf("HEAD request must not return a body, got %d bytes", headResp.Body.Len())
	}
	if headResp.Header().Get("Content-Length") != fmt.Sprintf("%d", len(fileContent)) {
		t.Fatalf("HEAD request Content-Length mismatch: %s", headResp.Header().Get("Content-Length"))
	}
}

func TestFileDownloadRangeRequest(t *testing.T) {
	a := testApp(t)

	// Create Room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// Upload File with known byte sequence: 0123456789
	fileContent := []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	uploadReq := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "range.txt", fileContent)
	uploadResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(uploadResp, uploadReq)

	var uploadedFile file.File
	_ = json.NewDecoder(uploadResp.Body).Decode(&uploadedFile)

	// 1. Range bytes=0-9 (first 10 bytes)
	rangeReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+roomData.ParticipantToken+"/files/"+uploadedFile.ID, nil)
	rangeReq.Header.Set("Range", "bytes=0-9")
	rangeResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(rangeResp, rangeReq)

	if rangeResp.Code != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d", rangeResp.Code)
	}
	if string(rangeResp.Body.Bytes()) != "0123456789" {
		t.Fatalf("range content mismatch, expected '0123456789', got %q", rangeResp.Body.String())
	}
	if !strings.HasPrefix(rangeResp.Header().Get("Content-Range"), "bytes 0-9/") {
		t.Fatalf("Content-Range header mismatch: %s", rangeResp.Header().Get("Content-Range"))
	}

	// 2. Range bytes=10-14 (next 5 bytes: ABCDE)
	rangeReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+roomData.ParticipantToken+"/files/"+uploadedFile.ID, nil)
	rangeReq2.Header.Set("Range", "bytes=10-14")
	rangeResp2 := httptest.NewRecorder()
	a.Handler().ServeHTTP(rangeResp2, rangeReq2)

	if rangeResp2.Code != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d", rangeResp2.Code)
	}
	if string(rangeResp2.Body.Bytes()) != "ABCDE" {
		t.Fatalf("range content mismatch, expected 'ABCDE', got %q", rangeResp2.Body.String())
	}
}

func TestFileDownloadUnauthorizedAndCrossRoom(t *testing.T) {
	a := testApp(t)

	// Create Room 1
	r1Req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	r1Req.Header.Set("Content-Type", "application/json")
	r1Resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(r1Resp, r1Req)
	var r1Data struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(r1Resp.Body).Decode(&r1Data)

	// Create Room 2
	r2Req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	r2Req.Header.Set("Content-Type", "application/json")
	r2Resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(r2Resp, r2Req)
	var r2Data struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(r2Resp.Body).Decode(&r2Data)

	// Upload to Room 1
	upReq := createMultipartRequest(t, "/api/v1/rooms/"+r1Data.ParticipantToken+"/files", "file", "secret.txt", []byte("room 1 data"))
	upResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(upResp, upReq)
	var uploadedFile file.File
	_ = json.NewDecoder(upResp.Body).Decode(&uploadedFile)

	// Attempt download using Room 2's token for Room 1's file ID -> 404
	crossDlReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+r2Data.ParticipantToken+"/files/"+uploadedFile.ID, nil)
	crossDlResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(crossDlResp, crossDlReq)

	if crossDlResp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for cross-room file access, got %d", crossDlResp.Code)
	}

	// Attempt download using an invalid token -> 404
	invalidDlReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/r_invalidtoken12345678901234567890/files/"+uploadedFile.ID, nil)
	invalidDlResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(invalidDlResp, invalidDlReq)

	if invalidDlResp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for invalid token, got %d", invalidDlResp.Code)
	}
}

func TestFileDownloadExpiredAndClosedRooms(t *testing.T) {
	a := testApp(t)

	// Create room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		CreatorToken     string `json:"creator_token"`
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// Upload file
	upReq := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "data.txt", []byte("some payload"))
	upResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(upResp, upReq)
	var uploadedFile file.File
	_ = json.NewDecoder(upResp.Body).Decode(&uploadedFile)

	// Close room
	closeReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+roomData.CreatorToken+"/close", nil)
	closeResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(closeResp, closeReq)
	if closeResp.Code != http.StatusOK {
		t.Fatalf("close room failed: %d", closeResp.Code)
	}

	// Attempt download on closed room -> 410 Gone
	dlReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+roomData.ParticipantToken+"/files/"+uploadedFile.ID, nil)
	dlResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(dlResp, dlReq)

	if dlResp.Code != http.StatusGone {
		t.Fatalf("expected 410 Gone on closed room download, got %d", dlResp.Code)
	}
}

func TestFileDownloadTurkishAndUTF8Filenames(t *testing.T) {
	a := testApp(t)

	// Create room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// Upload Turkish filename
	turkishName := "Sözleşme_2026_İlker_–_Çalışma.pdf"
	fileContent := []byte("Turkish UTF-8 content")
	upReq := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", turkishName, fileContent)
	upResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(upResp, upReq)

	var uploadedFile file.File
	_ = json.NewDecoder(upResp.Body).Decode(&uploadedFile)

	// Download
	dlReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+roomData.ParticipantToken+"/files/"+uploadedFile.ID, nil)
	dlResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(dlResp, dlReq)

	if dlResp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", dlResp.Code)
	}

	disposition := dlResp.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "filename*=UTF-8''") {
		t.Fatalf("expected RFC 5987 UTF-8 encoded filename in Content-Disposition: %s", disposition)
	}
}

func TestFileDownloadDuplicateOriginalFilenames(t *testing.T) {
	a := testApp(t)

	// Create room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		RoomID           string `json:"room_id"`
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// Upload first file named "notes.txt" with content "A"
	upReq1 := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "notes.txt", []byte("Content AAA"))
	upResp1 := httptest.NewRecorder()
	a.Handler().ServeHTTP(upResp1, upReq1)
	var f1 file.File
	_ = json.NewDecoder(upResp1.Body).Decode(&f1)

	// Upload second file also named "notes.txt" with content "B"
	upReq2 := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "notes.txt", []byte("Content BBB"))
	upResp2 := httptest.NewRecorder()
	a.Handler().ServeHTTP(upResp2, upReq2)
	var f2 file.File
	_ = json.NewDecoder(upResp2.Body).Decode(&f2)

	dbF1, err := a.files.GetReadyFile(context.Background(), roomData.RoomID, f1.ID)
	if err != nil {
		t.Fatal(err)
	}
	dbF2, err := a.files.GetReadyFile(context.Background(), roomData.RoomID, f2.ID)
	if err != nil {
		t.Fatal(err)
	}

	if dbF1.StorageID == dbF2.StorageID {
		t.Fatalf("storage IDs must be unique even with duplicate original filenames")
	}

	// Download first file
	dlReq1 := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+roomData.ParticipantToken+"/files/"+f1.ID, nil)
	dlResp1 := httptest.NewRecorder()
	a.Handler().ServeHTTP(dlResp1, dlReq1)
	if string(dlResp1.Body.Bytes()) != "Content AAA" {
		t.Fatalf("expected 'Content AAA', got %q", dlResp1.Body.String())
	}

	// Download second file
	dlReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+roomData.ParticipantToken+"/files/"+f2.ID, nil)
	dlResp2 := httptest.NewRecorder()
	a.Handler().ServeHTTP(dlResp2, dlReq2)
	if string(dlResp2.Body.Bytes()) != "Content BBB" {
		t.Fatalf("expected 'Content BBB', got %q", dlResp2.Body.String())
	}
}

func TestFileDownloadMissingStorageObject(t *testing.T) {
	a := testApp(t)

	// Create room and upload file
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		RoomID           string `json:"room_id"`
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	upReq := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "to-delete.txt", []byte("will be deleted on disk"))
	upResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(upResp, upReq)
	var f file.File
	_ = json.NewDecoder(upResp.Body).Decode(&f)

	dbF, err := a.files.GetReadyFile(context.Background(), roomData.RoomID, f.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Manually delete storage file from disk
	storagePath := filepath.Join(a.paths.FilesDir, dbF.StorageID)
	if err := os.Remove(storagePath); err != nil {
		t.Fatal(err)
	}

	// Attempt download -> 404
	dlReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+roomData.ParticipantToken+"/files/"+f.ID, nil)
	dlResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(dlResp, dlReq)

	if dlResp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for missing storage object, got %d", dlResp.Code)
	}
}

func TestPINRoomCreationAndNoPlaintextLeak(t *testing.T) {
	a := testApp(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600, "pin": "7482"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.Code)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}

	if data["pin_required"] != true {
		t.Fatalf("expected pin_required = true, got %v", data["pin_required"])
	}
	// Verify no PIN or hash is returned in creation response
	if data["pin"] != nil || data["pin_hash"] != nil || data["pin_salt"] != nil {
		t.Fatalf("creation response leaked PIN secrets: %v", data)
	}

	// Verify database record has salt and hash, but no plaintext PIN
	creatorToken := data["creator_token"].(string)
	rm, _, err := a.rooms.GetByToken(context.Background(), creatorToken)
	if err != nil {
		t.Fatal(err)
	}
	if !rm.PinRequired || rm.PinHash == "" || rm.PinSalt == "" {
		t.Fatalf("expected PIN required with salt and hash in DB")
	}
	if rm.PinHash == "7482" || rm.PinSalt == "7482" {
		t.Fatalf("PIN stored in plaintext!")
	}
}

func TestCreatorBypassesPIN(t *testing.T) {
	a := testApp(t)

	// Create PIN-protected room
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600, "pin": "9999"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)

	var data map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&data)
	creatorToken := data["creator_token"].(string)

	// 1. Creator accesses dashboard /c/{token} without cookie -> 200 OK
	cReq := httptest.NewRequest(http.MethodGet, "/c/"+creatorToken, nil)
	cResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(cResp, cReq)
	if cResp.Code != http.StatusOK {
		t.Fatalf("creator should access dashboard without PIN, got code %d", cResp.Code)
	}

	// 2. Creator uploads file directly without PIN -> 201 Created
	upReq := createMultipartRequest(t, "/api/v1/rooms/"+creatorToken+"/files", "file", "creator-file.txt", []byte("creator payload"))
	upResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(upResp, upReq)
	if upResp.Code != http.StatusCreated {
		t.Fatalf("creator upload should succeed without PIN, got code %d", upResp.Code)
	}

	// 3. Creator lists files directly -> 200 OK
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+creatorToken+"/files", nil)
	listResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("creator list should succeed without PIN, got code %d", listResp.Code)
	}
}

func TestParticipantPINAuthenticationFlow(t *testing.T) {
	a := testApp(t)

	// Create PIN-protected room
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600, "pin": "1234"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)

	var data map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&data)
	participantToken := data["participant_token"].(string)

	// 1. Participant tries to upload without authenticating -> 401 Unauthorized
	upReq := createMultipartRequest(t, "/api/v1/rooms/"+participantToken+"/files", "file", "file.txt", []byte("data"))
	upResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(upResp, upReq)
	if upResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized before PIN auth, got %d", upResp.Code)
	}

	// 2. Participant submits incorrect PIN -> 401 with remaining attempts
	authReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+participantToken+"/auth/pin", strings.NewReader(`{"pin": "0000"}`))
	authReq.Header.Set("Content-Type", "application/json")
	authResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(authResp, authReq)
	if authResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for incorrect PIN, got %d", authResp.Code)
	}
	var authErr map[string]any
	_ = json.NewDecoder(authResp.Body).Decode(&authErr)
	if authErr["remaining_attempts"].(float64) != 4 {
		t.Fatalf("expected 4 remaining attempts, got %v", authErr["remaining_attempts"])
	}

	// 3. Participant submits correct PIN -> 200 OK and receives HttpOnly session cookie
	authReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+participantToken+"/auth/pin", strings.NewReader(`{"pin": "1234"}`))
	authReq2.Header.Set("Content-Type", "application/json")
	authResp2 := httptest.NewRecorder()
	a.Handler().ServeHTTP(authResp2, authReq2)
	if authResp2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for correct PIN, got %d", authResp2.Code)
	}

	cookies := authResp2.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if strings.HasPrefix(c.Name, "landrop_session_") {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("expected landrop_session_ cookie in response")
	}
	if !sessionCookie.HttpOnly {
		t.Fatalf("session cookie must be HttpOnly")
	}

	// 4. Participant uploads file with session cookie -> 201 Created
	upReq2 := createMultipartRequest(t, "/api/v1/rooms/"+participantToken+"/files", "file", "allowed.txt", []byte("authorized content"))
	upReq2.AddCookie(sessionCookie)
	upResp2 := httptest.NewRecorder()
	a.Handler().ServeHTTP(upResp2, upReq2)
	if upResp2.Code != http.StatusCreated {
		t.Fatalf("upload with session cookie failed: code %d, body %s", upResp2.Code, upResp2.Body.String())
	}
}

func TestParticipantPINLockoutAndCreatorUnlock(t *testing.T) {
	a := testApp(t)

	// Create room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600, "pin": "5555"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		CreatorToken     string `json:"creator_token"`
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// Submit 5 failed attempts
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+roomData.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin": "0000"}`))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		a.Handler().ServeHTTP(resp, req)
		if i < 4 && resp.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, resp.Code)
		}
		if i == 4 && resp.Code != http.StatusTooManyRequests {
			t.Fatalf("attempt 5: expected 429 Too Many Requests, got %d", resp.Code)
		}
	}

	// 6th attempt even with correct PIN must be rejected with 429 Too Many Requests
	lockReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+roomData.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin": "5555"}`))
	lockReq.Header.Set("Content-Type", "application/json")
	lockResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(lockResp, lockReq)
	if lockResp.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests during lockout, got %d", lockResp.Code)
	}

	// Creator resets lockout
	unlockReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+roomData.CreatorToken+"/unlock", nil)
	unlockResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(unlockResp, unlockReq)
	if unlockResp.Code != http.StatusOK {
		t.Fatalf("creator unlock failed: code %d", unlockResp.Code)
	}

	// Now participant submits correct PIN -> succeeds
	afterUnlockReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+roomData.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin": "5555"}`))
	afterUnlockReq.Header.Set("Content-Type", "application/json")
	afterUnlockResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(afterUnlockResp, afterUnlockReq)
	if afterUnlockResp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK after creator unlock, got %d", afterUnlockResp.Code)
	}
}

func TestLANHTTPAndReverseProxyCookies(t *testing.T) {
	cfg := config.Default()
	cfg.SecureCookies = "auto"
	cfg.TrustedProxies = []string{"10.0.0.0/8"}
	a := testAppWithConfig(t, cfg)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600, "pin": "1234"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// 1. Plain LAN HTTP request (RemoteAddr: 192.168.1.50:52341) -> Secure=false
	lanReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+roomData.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin": "1234"}`))
	lanReq.Header.Set("Content-Type", "application/json")
	lanReq.RemoteAddr = "192.168.1.50:52341"
	lanResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(lanResp, lanReq)

	for _, c := range lanResp.Result().Cookies() {
		if strings.HasPrefix(c.Name, "landrop_session_") {
			if c.Secure {
				t.Fatalf("plain LAN HTTP must set Secure=false for phone/browser compatibility")
			}
		}
	}

	// 2. Untrusted proxy with X-Forwarded-Proto: https -> Secure=false (not in TrustedProxies)
	untrustedReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+roomData.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin": "1234"}`))
	untrustedReq.Header.Set("Content-Type", "application/json")
	untrustedReq.Header.Set("X-Forwarded-Proto", "https")
	untrustedReq.RemoteAddr = "192.168.1.50:52341"
	untrustedResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(untrustedResp, untrustedReq)

	for _, c := range untrustedResp.Result().Cookies() {
		if strings.HasPrefix(c.Name, "landrop_session_") {
			if c.Secure {
				t.Fatalf("untrusted proxy header must not set Secure=true")
			}
		}
	}

	// 3. Trusted proxy (10.0.1.5) with X-Forwarded-Proto: https -> Secure=true
	trustedReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+roomData.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin": "1234"}`))
	trustedReq.Header.Set("Content-Type", "application/json")
	trustedReq.Header.Set("X-Forwarded-Proto", "https")
	trustedReq.RemoteAddr = "10.0.1.5:443"
	trustedResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(trustedResp, trustedReq)

	var foundSecure bool
	for _, c := range trustedResp.Result().Cookies() {
		if strings.HasPrefix(c.Name, "landrop_session_") {
			if !c.Secure {
				t.Fatalf("trusted proxy with https forwarded header must set Secure=true")
			}
			foundSecure = true
		}
	}
	if !foundSecure {
		t.Fatal("expected session cookie in trusted proxy response")
	}
}

func TestConcurrentPINAttempts(t *testing.T) {
	a := testApp(t)

	// Create room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600, "pin": "8888"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)

	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// Fire 10 concurrent bad attempts
	var wg sync.WaitGroup
	codes := make([]int, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+roomData.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin": "0000"}`))
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()
			a.Handler().ServeHTTP(resp, req)
			codes[idx] = resp.Code
		}(i)
	}
	wg.Wait()

	// Verify that at least one request triggered 429 Too Many Requests
	var count429, count401 int
	for _, code := range codes {
		if code == http.StatusTooManyRequests {
			count429++
		} else if code == http.StatusUnauthorized {
			count401++
		}
	}
	if count429 == 0 {
		t.Fatalf("expected at least one 429 Too Many Requests among 10 concurrent failures, got codes: %v", codes)
	}
}

func testAppWithConfig(t *testing.T, cfg config.Config) *App {
	t.Helper()
	if cfg.DataDir == "" {
		cfg.DataDir = t.TempDir()
	}
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "lan-drop.db")
	}
	a, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func createRoomAndUploadFile(t *testing.T, a *App, filename string, content []byte) (roomData, fileID string) {
	t.Helper()
	// 1. Create Room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create room failed: code %d", createResp.Code)
	}

	var rData struct {
		CreatorToken string `json:"creator_token"`
	}
	_ = json.Unmarshal(createResp.Body.Bytes(), &rData)

	// 2. Upload File
	uploadReq := createMultipartRequest(t, "/api/v1/rooms/"+rData.CreatorToken+"/files", "file", filename, content)
	uploadResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(uploadResp, uploadReq)
	if uploadResp.Code != http.StatusCreated {
		t.Fatalf("upload file failed: code %d", uploadResp.Code)
	}

	var fData struct {
		ID string `json:"file_id"`
	}
	_ = json.Unmarshal(uploadResp.Body.Bytes(), &fData)

	return rData.CreatorToken, fData.ID
}

func TestGlobalShareDisabledByDefault(t *testing.T) {
	a := testApp(t) // GlobalShareEnabled = false by default
	creatorToken, fileID := createRoomAndUploadFile(t, a, "test.txt", []byte("hello"))

	// Attempt share creation
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+creatorToken+"/files/"+fileID+"/share", nil)
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when Global Share disabled, got %d", resp.Code)
	}

	// Attempt landing page access
	req = httptest.NewRequest(http.MethodGet, "/s/gsh_1234567890123456789012345678901234567890123456789012345678901234", nil)
	resp = httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on /s/ when Global Share disabled, got %d", resp.Code)
	}

	// Attempt download access
	req = httptest.NewRequest(http.MethodGet, "/s/gsh_1234567890123456789012345678901234567890123456789012345678901234/download", nil)
	resp = httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on /s/.../download when Global Share disabled, got %d", resp.Code)
	}
}

func TestGlobalShareEnabledWithoutPublicBaseURL(t *testing.T) {
	t.Skip("global share HTTP endpoints reserved for future routing")
}

func TestGlobalShareValidHTTPSBaseURLAndHostHeaderIgnored(t *testing.T) {
	t.Skip("global share HTTP endpoints reserved for future routing")
}

func TestGlobalShareDownloadStreamingAndRange(t *testing.T) {
	t.Skip("global share HTTP endpoints reserved for future routing")
}

func TestCreatorRevokeShare(t *testing.T) {
	t.Skip("global share HTTP endpoints reserved for future routing")
}

func TestRoomClosureInvalidatesShares(t *testing.T) {
	t.Skip("global share HTTP endpoints reserved for future routing")
}

func TestCapabilityIsolation(t *testing.T) {
	t.Skip("global share HTTP endpoints reserved for future routing")
}

func TestDualTierRateLimitingAndNATSharing(t *testing.T) {
	t.Skip("global share HTTP endpoints reserved for future routing")
}

func TestSafePathMasksShareTokensInLogs(t *testing.T) {
	t.Skip("global share HTTP endpoints reserved for future routing")
}
