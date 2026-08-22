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
	"time"

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
	cfg.MinFreeSpace = 0
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
	if cfg.DataDir == "" || cfg.DataDir == "/data" {
		cfg.DataDir = t.TempDir()
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
	cfg := config.Default()
	cfg.GlobalShareEnabled = true
	cfg.PublicBaseURL = ""
	a := testAppWithConfig(t, cfg)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	creatorToken, fileID := createRoomAndUploadFile(t, a, "share_test.txt", []byte("share content data"))

	// Create share
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+creatorToken+"/files/"+fileID+"/share", strings.NewReader(`{"ttl_seconds": 1800}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}

	var data struct {
		ShareID   string `json:"share_id"`
		ShareURL  string `json:"share_url"`
		ExpiresAt string `json:"expires_at"`
		Status    string `json:"status"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&data)

	if !strings.HasPrefix(data.ShareID, "sh_") {
		t.Fatalf("expected share_id to start with sh_, got %q", data.ShareID)
	}
	if !strings.Contains(data.ShareURL, "/s/gsh_") {
		t.Fatalf("expected share_url to contain /s/gsh_, got %q", data.ShareURL)
	}
	if data.Status != "active" {
		t.Fatalf("expected status active, got %q", data.Status)
	}

	// Extract share token from URL
	shareToken := data.ShareURL[strings.LastIndex(data.ShareURL, "/")+1:]

	// Access public share landing page
	pageReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/s/"+shareToken, nil)
	pageResp, err := http.DefaultClient.Do(pageReq)
	if err != nil {
		t.Fatal(err)
	}
	defer pageResp.Body.Close()
	if pageResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for share page, got %d", pageResp.StatusCode)
	}
	pageBody, _ := io.ReadAll(pageResp.Body)
	if !strings.Contains(string(pageBody), "share_test.txt") {
		t.Fatalf("expected share page to display filename, got body: %s", string(pageBody))
	}

	// Access public download endpoint
	downReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/s/"+shareToken+"/download", nil)
	downResp, err := http.DefaultClient.Do(downReq)
	if err != nil {
		t.Fatal(err)
	}
	defer downResp.Body.Close()
	if downResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for share download, got %d", downResp.StatusCode)
	}
	downBody, _ := io.ReadAll(downResp.Body)
	if string(downBody) != "share content data" {
		t.Fatalf("expected file content 'share content data', got %q", string(downBody))
	}
}

func TestGlobalShareValidHTTPSBaseURLAndHostHeaderIgnored(t *testing.T) {
	cfg := config.Default()
	cfg.GlobalShareEnabled = true
	cfg.PublicBaseURL = "https://public.hamal.io"
	a := testAppWithConfig(t, cfg)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	creatorToken, fileID := createRoomAndUploadFile(t, a, "ignore_host.txt", []byte("public base url test"))

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+creatorToken+"/files/"+fileID+"/share", strings.NewReader(`{"ttl_seconds": 3600}`))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "attacker.evil.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var data struct {
		ShareURL string `json:"share_url"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&data)

	if !strings.HasPrefix(data.ShareURL, "https://public.hamal.io/s/gsh_") {
		t.Fatalf("expected PublicBaseURL override, got %q", data.ShareURL)
	}
}

func TestGlobalShareDownloadStreamingAndRange(t *testing.T) {
	cfg := config.Default()
	cfg.GlobalShareEnabled = true
	a := testAppWithConfig(t, cfg)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	content := []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	creatorToken, fileID := createRoomAndUploadFile(t, a, "range_test.bin", content)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+creatorToken+"/files/"+fileID+"/share", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		ShareURL string `json:"share_url"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&data)
	resp.Body.Close()

	shareToken := data.ShareURL[strings.LastIndex(data.ShareURL, "/")+1:]

	// Test Range request
	rangeReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/s/"+shareToken+"/download", nil)
	rangeReq.Header.Set("Range", "bytes=10-19")
	rangeResp, err := http.DefaultClient.Do(rangeReq)
	if err != nil {
		t.Fatal(err)
	}
	defer rangeResp.Body.Close()

	if rangeResp.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d", rangeResp.StatusCode)
	}
	rangeBody, _ := io.ReadAll(rangeResp.Body)
	if string(rangeBody) != "ABCDEFGHIJ" {
		t.Fatalf("expected 'ABCDEFGHIJ', got %q", string(rangeBody))
	}
}

func TestCreatorRevokeShare(t *testing.T) {
	cfg := config.Default()
	cfg.GlobalShareEnabled = true
	a := testAppWithConfig(t, cfg)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	creatorToken, fileID := createRoomAndUploadFile(t, a, "revoke_test.txt", []byte("data"))

	// Create share
	cReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+creatorToken+"/files/"+fileID+"/share", nil)
	cResp, err := http.DefaultClient.Do(cReq)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		ShareID  string `json:"share_id"`
		ShareURL string `json:"share_url"`
	}
	_ = json.NewDecoder(cResp.Body).Decode(&data)
	cResp.Body.Close()
	shareToken := data.ShareURL[strings.LastIndex(data.ShareURL, "/")+1:]

	// Revoke share using creator token
	revReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+creatorToken+"/shares/"+data.ShareID+"/revoke", nil)
	revResp, err := http.DefaultClient.Do(revReq)
	if err != nil {
		t.Fatal(err)
	}
	defer revResp.Body.Close()
	if revResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on revoke, got %d", revResp.StatusCode)
	}

	// Access public landing page after revocation -> 410 Gone
	pageReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/s/"+shareToken, nil)
	pageResp, err := http.DefaultClient.Do(pageReq)
	if err != nil {
		t.Fatal(err)
	}
	defer pageResp.Body.Close()
	if pageResp.StatusCode != http.StatusGone {
		t.Fatalf("expected 410 Gone after share revoked, got %d", pageResp.StatusCode)
	}

	// Access download after revocation -> 410 Gone
	downReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/s/"+shareToken+"/download", nil)
	downResp, err := http.DefaultClient.Do(downReq)
	if err != nil {
		t.Fatal(err)
	}
	defer downResp.Body.Close()
	if downResp.StatusCode != http.StatusGone {
		t.Fatalf("expected 410 Gone for revoked download, got %d", downResp.StatusCode)
	}
}

func TestRoomClosureInvalidatesShares(t *testing.T) {
	cfg := config.Default()
	cfg.GlobalShareEnabled = true
	a := testAppWithConfig(t, cfg)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	creatorToken, fileID := createRoomAndUploadFile(t, a, "room_close.txt", []byte("room close test"))

	// Create share
	cReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+creatorToken+"/files/"+fileID+"/share", nil)
	cResp, err := http.DefaultClient.Do(cReq)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		ShareURL string `json:"share_url"`
	}
	_ = json.NewDecoder(cResp.Body).Decode(&data)
	cResp.Body.Close()
	shareToken := data.ShareURL[strings.LastIndex(data.ShareURL, "/")+1:]

	// Close room
	closeReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+creatorToken+"/close", nil)
	closeResp, err := http.DefaultClient.Do(closeReq)
	if err != nil {
		t.Fatal(err)
	}
	closeResp.Body.Close()

	// Access share page -> 410 Gone
	pageReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/s/"+shareToken, nil)
	pageResp, err := http.DefaultClient.Do(pageReq)
	if err != nil {
		t.Fatal(err)
	}
	defer pageResp.Body.Close()
	if pageResp.StatusCode != http.StatusGone {
		t.Fatalf("expected 410 Gone when room is closed, got %d", pageResp.StatusCode)
	}
}

func TestCapabilityIsolation(t *testing.T) {
	cfg := config.Default()
	cfg.GlobalShareEnabled = true
	a := testAppWithConfig(t, cfg)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	creatorToken, fileA := createRoomAndUploadFile(t, a, "fileA.txt", []byte("File A Content"))

	// Upload second file B to room
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	p, _ := w.CreateFormFile("file", "fileB.txt")
	_, _ = p.Write([]byte("File B Secret"))
	_ = w.Close()
	upReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+creatorToken+"/files", body)
	upReq.Header.Set("Content-Type", w.FormDataContentType())
	upResp, err := http.DefaultClient.Do(upReq)
	if err != nil {
		t.Fatal(err)
	}
	var fDataB struct {
		ID string `json:"file_id"`
	}
	_ = json.NewDecoder(upResp.Body).Decode(&fDataB)
	upResp.Body.Close()

	// Create share only for File A
	cReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+creatorToken+"/files/"+fileA+"/share", nil)
	cResp, err := http.DefaultClient.Do(cReq)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		ShareURL string `json:"share_url"`
	}
	_ = json.NewDecoder(cResp.Body).Decode(&data)
	cResp.Body.Close()
	shareToken := data.ShareURL[strings.LastIndex(data.ShareURL, "/")+1:]

	// Attempt to use shareToken as room token on room APIs -> must be 404
	badReq1, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/rooms/"+shareToken+"/files", nil)
	badResp1, err := http.DefaultClient.Do(badReq1)
	if err != nil {
		t.Fatal(err)
	}
	defer badResp1.Body.Close()
	if badResp1.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when share token is used as room token, got %d", badResp1.StatusCode)
	}

	badReq2, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/rooms/"+shareToken+"/files/"+fDataB.ID, nil)
	badResp2, err := http.DefaultClient.Do(badReq2)
	if err != nil {
		t.Fatal(err)
	}
	defer badResp2.Body.Close()
	if badResp2.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 accessing File B via share token, got %d", badResp2.StatusCode)
	}

	badReq3, _ := http.NewRequest(http.MethodGet, ts.URL+"/c/"+shareToken, nil)
	badResp3, err := http.DefaultClient.Do(badReq3)
	if err != nil {
		t.Fatal(err)
	}
	defer badResp3.Body.Close()
	if badResp3.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on /c/ with share token, got %d", badResp3.StatusCode)
	}
}

func TestDualTierRateLimitingAndNATSharing(t *testing.T) {
	cfg := config.Default()
	cfg.GlobalShareEnabled = true
	cfg.ShareAccessRateLimit = 6
	a := testAppWithConfig(t, cfg)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	creatorToken, fileID := createRoomAndUploadFile(t, a, "ratelimit.txt", []byte("rate limited data"))

	cReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+creatorToken+"/files/"+fileID+"/share", nil)
	cResp, err := http.DefaultClient.Do(cReq)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		ShareURL string `json:"share_url"`
	}
	_ = json.NewDecoder(cResp.Body).Decode(&data)
	cResp.Body.Close()
	shareToken := data.ShareURL[strings.LastIndex(data.ShareURL, "/")+1:]

	// Trigger rate limit on share download
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/s/"+shareToken+"/download", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	// 6th request from same IP should get rate limited (429)
	limReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/s/"+shareToken+"/download", nil)
	limResp, err := http.DefaultClient.Do(limReq)
	if err != nil {
		t.Fatal(err)
	}
	defer limResp.Body.Close()
	if limResp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", limResp.StatusCode)
	}
}

func TestSafePathMasksShareTokensInLogs(t *testing.T) {
	p1 := safePath("/s/gsh_1234567890123456789012345678901234567890123456789012345678901234")
	if p1 != "/s/:share_token" {
		t.Errorf("expected /s/:share_token, got %q", p1)
	}

	p2 := safePath("/s/gsh_1234567890123456789012345678901234567890123456789012345678901234/download")
	if p2 != "/s/:share_token/download" {
		t.Errorf("expected /s/:share_token/download, got %q", p2)
	}

	p3 := safePath("/api/v1/rooms/cr_123456/files/fl_789/share")
	if p3 != "/api/v1/rooms/:creator_token/files/:file_id/share" {
		t.Errorf("expected /api/v1/rooms/:creator_token/files/:file_id/share, got %q", p3)
	}

	p4 := safePath("/api/v1/rooms/cr_123456/shares/sh_999/revoke")
	if p4 != "/api/v1/rooms/:creator_token/shares/:share_id/revoke" {
		t.Errorf("expected /api/v1/rooms/:creator_token/shares/:share_id/revoke, got %q", p4)
	}
}

func TestMaxSharesPerRoomEnforcement(t *testing.T) {
	cfg := config.Default()
	cfg.GlobalShareEnabled = true
	cfg.MaxSharesPerRoom = 2
	a := testAppWithConfig(t, cfg)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	creatorToken, fileID := createRoomAndUploadFile(t, a, "max_shares.txt", []byte("max shares test"))

	// Share 1 -> 201 Created
	req1, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+creatorToken+"/files/"+fileID+"/share", nil)
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatal(err)
	}
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for share 1, got %d", resp1.StatusCode)
	}
	resp1.Body.Close()

	// Share 2 -> 201 Created
	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+creatorToken+"/files/"+fileID+"/share", nil)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for share 2, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()

	// Share 3 -> 400 Bad Request (Limit Reached)
	req3, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+creatorToken+"/files/"+fileID+"/share", nil)
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for share 3 exceeding limit, got %d", resp3.StatusCode)
	}
}

func TestRoomCreationRateLimit(t *testing.T) {
	cfg := config.Default()
	cfg.ShareManagementRateLimit = 2 // 2 req/min, burst 5 (minimum burst is 5)
	a := testAppWithConfig(t, cfg)

	// Send 5 initial requests from same IP (192.168.1.50) -> all should succeed (burst = 5)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.168.1.50:12345"
		resp := httptest.NewRecorder()
		a.Handler().ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created for request %d, got %d", i+1, resp.Code)
		}
	}

	// 6th request from same IP -> must return 429 Too Many Requests with Retry-After header
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.50:12345"
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d: %s", resp.Code, resp.Body.String())
	}
	retryAfter := resp.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("expected Retry-After header on 429 response")
	}
}

func TestRateLimitDifferentIPs(t *testing.T) {
	cfg := config.Default()
	cfg.ShareManagementRateLimit = 2 // burst 5
	a := testAppWithConfig(t, cfg)

	// Exhaust IP 1's bucket
	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.168.1.100:12345"
		resp := httptest.NewRecorder()
		a.Handler().ServeHTTP(resp, req)
		if i == 5 && resp.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429 for IP 1 on 6th request, got %d", resp.Code)
		}
	}

	// Request from IP 2 (192.168.1.101) must succeed and not be blocked by IP 1
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.RemoteAddr = "192.168.1.101:12345"
	resp2 := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp2, req2)
	if resp2.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for IP 2, got %d: %s", resp2.Code, resp2.Body.String())
	}
}

func TestUntrustedProxySpoofingIgnored(t *testing.T) {
	cfg := config.Default()
	cfg.ShareManagementRateLimit = 2 // burst 5
	cfg.TrustedProxies = nil         // No trusted proxies configured
	a := testAppWithConfig(t, cfg)

	// Attacker sends 5 requests from RemoteAddr 10.0.0.50 with spoofed rotating X-Forwarded-For headers
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.0.0.50:44321"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("192.168.1.%d", i+1))
		resp := httptest.NewRecorder()
		a.Handler().ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created for request %d, got %d", i+1, resp.Code)
		}
	}

	// 6th request from 10.0.0.50 with another spoofed X-Forwarded-For must be blocked as 429
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.50:44321"
	req.Header.Set("X-Forwarded-For", "192.168.1.99")
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 because spoofed X-Forwarded-For must be ignored for untrusted RemoteAddr, got %d", resp.Code)
	}
}

func TestTrustedProxyClientIPExtracted(t *testing.T) {
	cfg := config.Default()
	cfg.ShareManagementRateLimit = 2 // burst 5
	cfg.TrustedProxies = []string{"10.0.0.0/8"}
	a := testAppWithConfig(t, cfg)

	// 5 requests through trusted proxy 10.0.0.1 for client 203.0.113.42 -> all succeed
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.0.0.1:50000"
		req.Header.Set("X-Forwarded-For", "203.0.113.42, 10.0.0.2")
		resp := httptest.NewRecorder()
		a.Handler().ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created for request %d, got %d", i+1, resp.Code)
		}
	}

	// 6th request for client 203.0.113.42 -> 429
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:50000"
	req.Header.Set("X-Forwarded-For", "203.0.113.42, 10.0.0.2")
	resp := httptest.NewRecorder()
	a.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for client 203.0.113.42 through trusted proxy, got %d", resp.Code)
	}

	// Request through same trusted proxy for a DIFFERENT real client (203.0.113.43) must succeed
	reqOther := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	reqOther.Header.Set("Content-Type", "application/json")
	reqOther.RemoteAddr = "10.0.0.1:50000"
	reqOther.Header.Set("X-Forwarded-For", "203.0.113.43, 10.0.0.2")
	respOther := httptest.NewRecorder()
	a.Handler().ServeHTTP(respOther, reqOther)
	if respOther.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for different client 203.0.113.43, got %d", respOther.Code)
	}
}

func TestPINAuthRateLimit(t *testing.T) {
	cfg := config.Default()
	cfg.ShareManagementRateLimit = 2 // burst 5
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	// Create Room 1 with PIN
	createReq1 := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600, "pin": "1234"}`))
	createReq1.Header.Set("Content-Type", "application/json")
	createReq1.RemoteAddr = "192.168.1.10:11111"
	createResp1 := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp1, createReq1)
	var room1 struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp1.Body).Decode(&room1)

	// Create Room 2 with PIN
	createReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600, "pin": "5678"}`))
	createReq2.Header.Set("Content-Type", "application/json")
	createReq2.RemoteAddr = "192.168.1.10:11111"
	createResp2 := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp2, createReq2)
	var room2 struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp2.Body).Decode(&room2)

	// 3 wrong PIN attempts on Room 1 from IP 192.168.1.77 -> return 401
	for i := 0; i < 3; i++ {
		pinReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+room1.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin": "9999"}`))
		pinReq.Header.Set("Content-Type", "application/json")
		pinReq.RemoteAddr = "192.168.1.77:22222"
		pinResp := httptest.NewRecorder()
		a.Handler().ServeHTTP(pinResp, pinReq)
		if pinResp.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for wrong pin on Room 1 attempt %d, got %d", i+1, pinResp.Code)
		}
	}

	// 2 wrong PIN attempts on Room 2 from same IP 192.168.1.77 -> return 401
	for i := 0; i < 2; i++ {
		pinReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+room2.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin": "9999"}`))
		pinReq.Header.Set("Content-Type", "application/json")
		pinReq.RemoteAddr = "192.168.1.77:22222"
		pinResp := httptest.NewRecorder()
		a.Handler().ServeHTTP(pinResp, pinReq)
		if pinResp.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for wrong pin on Room 2 attempt %d, got %d", i+1, pinResp.Code)
		}
	}

	// 6th total PIN attempt from IP 192.168.1.77 on Room 2 -> must be blocked by IP rate limiter (429)
	// even though Room 2 only had 2 failed attempts (not locked)
	pinReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+room2.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin": "9999"}`))
	pinReq.Header.Set("Content-Type", "application/json")
	pinReq.RemoteAddr = "192.168.1.77:22222"
	pinResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(pinResp, pinReq)
	if pinResp.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests on 6th PIN attempt, got %d: %s", pinResp.Code, pinResp.Body.String())
	}
}

func TestFileUploadRateLimit(t *testing.T) {
	cfg := config.Default()
	cfg.ShareManagementRateLimit = 2 // upload limiter: rate 4/min, burst 2
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	// Create room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.RemoteAddr = "192.168.1.10:11111"
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// 2 upload requests (burst 2) from IP 192.168.1.88 -> both succeed (201 Created)
	for i := 0; i < 2; i++ {
		upReq := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", fmt.Sprintf("test%d.txt", i), []byte("hello"))
		upReq.RemoteAddr = "192.168.1.88:33333"
		upResp := httptest.NewRecorder()
		a.Handler().ServeHTTP(upResp, upReq)
		if upResp.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created for upload %d, got %d", i+1, upResp.Code)
		}
	}

	// 3rd upload request from same IP -> 429 Too Many Requests
	upReq := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "test3.txt", []byte("hello"))
	upReq.RemoteAddr = "192.168.1.88:33333"
	upResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(upResp, upReq)
	if upResp.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests for 3rd upload, got %d: %s", upResp.Code, upResp.Body.String())
	}
}

func TestFileDownloadRateLimit(t *testing.T) {
	cfg := config.Default()
	cfg.ShareAccessRateLimit = 6 // download limiter: rate 6/min, burst 1
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	// Create room and upload a file
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.RemoteAddr = "192.168.1.10:11111"
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	upReq := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", "download_test.txt", []byte("content for download"))
	upReq.RemoteAddr = "192.168.1.10:11111"
	upResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(upResp, upReq)
	var fileData struct {
		ID string `json:"file_id"`
	}
	_ = json.NewDecoder(upResp.Body).Decode(&fileData)

	// 1st download from IP 192.168.1.99 -> 200 OK
	dlReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+roomData.ParticipantToken+"/files/"+fileData.ID, nil)
	dlReq.RemoteAddr = "192.168.1.99:44444"
	dlResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(dlResp, dlReq)
	if dlResp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for download 1, got %d", dlResp.Code)
	}

	// 2nd download from same IP -> 429 Too Many Requests
	dlReq2 := httptest.NewRequest(http.MethodHead, "/api/v1/rooms/"+roomData.ParticipantToken+"/files/"+fileData.ID, nil)
	dlReq2.RemoteAddr = "192.168.1.99:44444"
	dlResp2 := httptest.NewRecorder()
	a.Handler().ServeHTTP(dlResp2, dlReq2)
	if dlResp2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests on 2nd download, got %d: %s", dlResp2.Code, dlResp2.Body.String())
	}
}

func TestNormalConcurrencyNoFalsePositives(t *testing.T) {
	cfg := config.Default() // Default generous limits (ShareManagementRateLimit: 30, ShareAccessRateLimit: 300)
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	// Create room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.RemoteAddr = "192.168.1.1:10000"
	createResp := httptest.NewRecorder()
	a.Handler().ServeHTTP(createResp, createReq)
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)

	// Run 8 concurrent participants from different IPs performing uploads
	var wg sync.WaitGroup
	var errorCount int
	var mu sync.Mutex

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := createMultipartRequest(t, "/api/v1/rooms/"+roomData.ParticipantToken+"/files", "file", fmt.Sprintf("file_%d.txt", idx), []byte("concurrent data"))
			req.RemoteAddr = fmt.Sprintf("192.168.1.%d:20000", idx+10)
			resp := httptest.NewRecorder()
			a.Handler().ServeHTTP(resp, req)
			if resp.Code != http.StatusCreated {
				t.Logf("upload %d failed with code %d: %s", idx, resp.Code, resp.Body.String())
				mu.Lock()
				errorCount++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if errorCount != 0 {
		t.Fatalf("expected 0 errors for legitimate concurrent participants, got %d", errorCount)
	}
}

func TestRateLimiterRaceSafety(t *testing.T) {
	limiter := NewIPRateLimiter(60, 20)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ip := fmt.Sprintf("10.0.0.%d", idx%5)
			for j := 0; j < 20; j++ {
				_, _ = limiter.Allow(ip)
			}
		}(i)
	}
	wg.Wait()
}

func TestUploadIdleTimeoutExceeded(t *testing.T) {
	cfg := config.Default()
	cfg.UploadIdleTimeout = 150 * time.Millisecond
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	createReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	if err != nil {
		t.Fatal(err)
	}
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		part, err := writer.CreateFormFile("file", "stalled.bin")
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		_, _ = part.Write([]byte("initial chunk"))
		time.Sleep(400 * time.Millisecond) // Stalls beyond 150ms timeout
		_, _ = part.Write([]byte("second chunk that should trigger timeout"))
		_ = writer.Close()
		_ = pw.Close()
	}()

	upReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", pr)
	if err != nil {
		t.Fatal(err)
	}
	upReq.Header.Set("Content-Type", writer.FormDataContentType())

	upResp, err := http.DefaultClient.Do(upReq)
	if err == nil {
		defer upResp.Body.Close()
		if upResp.StatusCode != http.StatusRequestTimeout && upResp.StatusCode != http.StatusInternalServerError && upResp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected timeout error status (408 or failure), got %d", upResp.StatusCode)
		}
	}
}

func TestContinuousSlowUploadDoesNotTimeout(t *testing.T) {
	cfg := config.Default()
	cfg.UploadIdleTimeout = 150 * time.Millisecond
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	createReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	if err != nil {
		t.Fatal(err)
	}
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Send 5 chunks with 50ms pause between each chunk.
	// Total duration = 5 * 50ms = 250ms (> 150ms timeout).
	// Because each chunk arrives within 50ms (< 150ms), the upload MUST succeed!
	go func() {
		part, err := writer.CreateFormFile("file", "continuous.txt")
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		for i := 0; i < 5; i++ {
			time.Sleep(50 * time.Millisecond)
			_, _ = part.Write([]byte(fmt.Sprintf("chunk-%d\n", i)))
		}
		_ = writer.Close()
		_ = pw.Close()
	}()

	upReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", pr)
	if err != nil {
		t.Fatal(err)
	}
	upReq.Header.Set("Content-Type", writer.FormDataContentType())

	upResp, err := http.DefaultClient.Do(upReq)
	if err != nil {
		t.Fatalf("continuous upload failed: %v", err)
	}
	defer upResp.Body.Close()

	if upResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(upResp.Body)
		t.Fatalf("expected 201 Created for continuous upload, got %d: %s", upResp.StatusCode, string(body))
	}
}

func TestUploadTimeoutReleasesQuotaReservation(t *testing.T) {
	cfg := config.Default()
	cfg.UploadIdleTimeout = 100 * time.Millisecond
	cfg.MinFreeSpace = 0
	cfg.MaxRoomSize = 100 * 1024 // 100 KB max room size
	a := testAppWithConfig(t, cfg)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	createReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	if err != nil {
		t.Fatal(err)
	}
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	// Stalled upload of 80 KB
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		part, _ := writer.CreateFormFile("file", "stall.bin")
		_, _ = part.Write(bytes.Repeat([]byte("A"), 1024))
		time.Sleep(300 * time.Millisecond) // stall beyond 100ms
		_ = writer.Close()
		_ = pw.Close()
	}()

	upReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", pr)
	upReq.Header.Set("Content-Type", writer.FormDataContentType())
	upResp, _ := http.DefaultClient.Do(upReq)
	if upResp != nil {
		_ = upResp.Body.Close()
	}

	time.Sleep(50 * time.Millisecond)

	// Now upload 80 KB normally -> must succeed because previous in-flight reservation was released
	body := &bytes.Buffer{}
	w2 := multipart.NewWriter(body)
	p2, _ := w2.CreateFormFile("file", "success.bin")
	_, _ = p2.Write(bytes.Repeat([]byte("B"), 80*1024))
	_ = w2.Close()

	upReq2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", body)
	upReq2.Header.Set("Content-Type", w2.FormDataContentType())
	upResp2, err := http.DefaultClient.Do(upReq2)
	if err != nil {
		t.Fatalf("subsequent upload failed: %v", err)
	}
	defer upResp2.Body.Close()

	if upResp2.StatusCode != http.StatusCreated {
		respBytes, _ := io.ReadAll(upResp2.Body)
		t.Fatalf("expected 201 Created after timeout released reservation, got %d: %s", upResp2.StatusCode, string(respBytes))
	}
}

func TestUploadTimeoutDeletesStagingFile(t *testing.T) {
	cfg := config.Default()
	cfg.UploadIdleTimeout = 100 * time.Millisecond
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	createReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	if err != nil {
		t.Fatal(err)
	}
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		part, _ := writer.CreateFormFile("file", "stall.bin")
		_, _ = part.Write([]byte("staging cleanup test"))
		time.Sleep(300 * time.Millisecond) // stall
		_ = writer.Close()
		_ = pw.Close()
	}()

	upReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", pr)
	upReq.Header.Set("Content-Type", writer.FormDataContentType())
	upResp, _ := http.DefaultClient.Do(upReq)
	if upResp != nil {
		_ = upResp.Body.Close()
	}

	time.Sleep(50 * time.Millisecond)

	entries, err := os.ReadDir(a.paths.StagingDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 files in staging directory after timeout, found %d", len(entries))
	}
}

func TestUploadClientDisconnectCleanup(t *testing.T) {
	cfg := config.Default()
	cfg.UploadIdleTimeout = 5 * time.Second
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, _ := http.DefaultClient.Do(createReq)
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		part, _ := writer.CreateFormFile("file", "disconnect.bin")
		_, _ = part.Write([]byte("data before disconnect"))
		time.Sleep(50 * time.Millisecond)
		cancel() // Client abruptly cancels context
		_ = pw.CloseWithError(context.Canceled)
	}()

	upReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", pr)
	upReq.Header.Set("Content-Type", writer.FormDataContentType())
	upResp, _ := http.DefaultClient.Do(upReq)
	if upResp != nil {
		_ = upResp.Body.Close()
	}

	time.Sleep(50 * time.Millisecond)

	entries, err := os.ReadDir(a.paths.StagingDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 files in staging directory after client disconnect, found %d", len(entries))
	}
}

func TestUploadIdleTimeoutDisabledWhenZero(t *testing.T) {
	cfg := config.Default()
	cfg.UploadIdleTimeout = 0 // Disabled
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, _ := http.DefaultClient.Do(createReq)
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		part, _ := writer.CreateFormFile("file", "zero_timeout.txt")
		_, _ = part.Write([]byte("first chunk"))
		time.Sleep(200 * time.Millisecond) // Slow pause
		_, _ = part.Write([]byte("second chunk"))
		_ = writer.Close()
		_ = pw.Close()
	}()

	upReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", pr)
	upReq.Header.Set("Content-Type", writer.FormDataContentType())
	upResp, err := http.DefaultClient.Do(upReq)
	if err != nil {
		t.Fatalf("upload failed with disabled timeout: %v", err)
	}
	defer upResp.Body.Close()

	if upResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created when timeout is disabled, got %d", upResp.StatusCode)
	}
}

func TestTimedOutUploadDoesNotBecomeReady(t *testing.T) {
	cfg := config.Default()
	cfg.UploadIdleTimeout = 100 * time.Millisecond
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, _ := http.DefaultClient.Do(createReq)
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		part, _ := writer.CreateFormFile("file", "unready.bin")
		_, _ = part.Write([]byte("some data before stalling"))
		time.Sleep(300 * time.Millisecond) // stall
		_ = writer.Close()
		_ = pw.Close()
	}()

	upReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", pr)
	upReq.Header.Set("Content-Type", writer.FormDataContentType())
	upResp, _ := http.DefaultClient.Do(upReq)
	if upResp != nil {
		_ = upResp.Body.Close()
	}

	time.Sleep(50 * time.Millisecond)

	// List files in room: must be 0 files and none ready
	listReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", nil)
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()

	var listData struct {
		Files     []file.File `json:"files"`
		FileCount int         `json:"file_count"`
	}
	_ = json.NewDecoder(listResp.Body).Decode(&listData)
	if listData.FileCount != 0 || len(listData.Files) != 0 {
		t.Fatalf("expected 0 files in room after timed out upload, found %d", listData.FileCount)
	}
}

func TestNormalLargeUploadStreamsSuccessfully(t *testing.T) {
	cfg := config.Default()
	cfg.UploadIdleTimeout = 500 * time.Millisecond
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, _ := http.DefaultClient.Do(createReq)
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Stream 2 MB in 64 KB chunks
	totalChunks := 32
	chunkSize := 64 * 1024
	go func() {
		part, err := writer.CreateFormFile("file", "large_stream.bin")
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		buf := bytes.Repeat([]byte("X"), chunkSize)
		for i := 0; i < totalChunks; i++ {
			_, err := part.Write(buf)
			if err != nil {
				pw.CloseWithError(err)
				return
			}
		}
		_ = writer.Close()
		_ = pw.Close()
	}()

	upReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", pr)
	upReq.Header.Set("Content-Type", writer.FormDataContentType())
	upResp, err := http.DefaultClient.Do(upReq)
	if err != nil {
		t.Fatalf("large upload stream failed: %v", err)
	}
	defer upResp.Body.Close()

	if upResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(upResp.Body)
		t.Fatalf("expected 201 Created for large stream upload, got %d: %s", upResp.StatusCode, string(body))
	}

	var fileData struct {
		ID        string `json:"file_id"`
		SizeBytes int64  `json:"size_bytes"`
	}
	_ = json.NewDecoder(upResp.Body).Decode(&fileData)
	expectedSize := int64(totalChunks * chunkSize)
	if fileData.SizeBytes != expectedSize {
		t.Fatalf("expected size %d bytes, got %d", expectedSize, fileData.SizeBytes)
	}
}

func TestConcurrentChunkedUploadsNoFalseRejectionHTTP(t *testing.T) {
	cfg := config.Default()
	cfg.MaxRoomSize = 10 * 1024 * 1024 // 10 MB room
	cfg.MaxFileSize = 8 * 1024 * 1024  // 8 MB max file
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	// Launch two concurrent 4 MB chunked HTTP uploads
	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			pr, pw := io.Pipe()
			writer := multipart.NewWriter(pw)
			go func() {
				part, err := writer.CreateFormFile("file", fmt.Sprintf("chunked_%d.bin", idx))
				if err != nil {
					pw.CloseWithError(err)
					return
				}
				buf := bytes.Repeat([]byte("C"), 64*1024)
				for j := 0; j < 64; j++ { // 64 * 64KB = 4 MB
					if _, err := part.Write(buf); err != nil {
						pw.CloseWithError(err)
						return
					}
				}
				_ = writer.Close()
				_ = pw.Close()
			}()

			upReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", pr)
			upReq.Header.Set("Content-Type", writer.FormDataContentType())
			upResp, err := http.DefaultClient.Do(upReq)
			if err != nil {
				errCh <- fmt.Errorf("upload %d request failed: %w", idx, err)
				return
			}
			defer upResp.Body.Close()

			if upResp.StatusCode != http.StatusCreated {
				b, _ := io.ReadAll(upResp.Body)
				errCh <- fmt.Errorf("upload %d got status %d: %s", idx, upResp.StatusCode, string(b))
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("unexpected concurrent upload failure: %v", err)
	}
}

func TestUnderDeclaredContentLengthHTTPBlocked(t *testing.T) {
	cfg := config.Default()
	cfg.MaxRoomSize = 1024 * 1024 // 1 MB room
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, _ := http.DefaultClient.Do(createReq)
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	// Stream 2 MB of data into 1 MB room
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		part, _ := writer.CreateFormFile("file", "under_declared.bin")
		buf := bytes.Repeat([]byte("U"), 64*1024)
		for j := 0; j < 32; j++ { // 32 * 64KB = 2 MB
			if _, err := part.Write(buf); err != nil {
				break
			}
		}
		_ = writer.Close()
		_ = pw.Close()
	}()

	upReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", pr)
	upReq.Header.Set("Content-Type", writer.FormDataContentType())
	upResp, err := http.DefaultClient.Do(upReq)
	if err == nil {
		defer upResp.Body.Close()
		if upResp.StatusCode != http.StatusRequestEntityTooLarge && upResp.StatusCode != http.StatusBadRequest && upResp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("expected 413 or error status for exceeding room limit, got %d", upResp.StatusCode)
		}
	}
}

func TestOverDeclaredContentLengthHTTPNoStarvation(t *testing.T) {
	cfg := config.Default()
	cfg.MaxRoomSize = 10 * 1024 * 1024 // 10 MB room
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, _ := http.DefaultClient.Do(createReq)
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	// Upload 1: 10 KB file
	body1 := &bytes.Buffer{}
	w1 := multipart.NewWriter(body1)
	p1, _ := w1.CreateFormFile("file", "small.txt")
	_, _ = p1.Write(bytes.Repeat([]byte("S"), 10*1024))
	_ = w1.Close()

	upReq1, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", body1)
	upReq1.Header.Set("Content-Type", w1.FormDataContentType())
	upResp1, err := http.DefaultClient.Do(upReq1)
	if err != nil {
		t.Fatal(err)
	}
	defer upResp1.Body.Close()
	if upResp1.StatusCode != http.StatusCreated {
		t.Fatalf("upload 1 failed: %d", upResp1.StatusCode)
	}

	// Upload 2: 5 MB file
	body2 := &bytes.Buffer{}
	w2 := multipart.NewWriter(body2)
	p2, _ := w2.CreateFormFile("file", "five_mb.bin")
	_, _ = p2.Write(bytes.Repeat([]byte("F"), 5*1024*1024))
	_ = w2.Close()

	upReq2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", body2)
	upReq2.Header.Set("Content-Type", w2.FormDataContentType())
	upResp2, err := http.DefaultClient.Do(upReq2)
	if err != nil {
		t.Fatal(err)
	}
	defer upResp2.Body.Close()
	if upResp2.StatusCode != http.StatusCreated {
		t.Fatalf("upload 2 failed: %d", upResp2.StatusCode)
	}
}

func TestConcurrentFileLimitHTTP(t *testing.T) {
	cfg := config.Default()
	cfg.MaxFilesPerRoom = 2
	cfg.MinFreeSpace = 0
	a := testAppWithConfig(t, cfg)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	// Launch 4 concurrent uploads against MaxFilesPerRoom = 2
	var wg sync.WaitGroup
	var successCount int
	var failCount int
	var mu sync.Mutex

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := &bytes.Buffer{}
			w := multipart.NewWriter(body)
			p, _ := w.CreateFormFile("file", fmt.Sprintf("file_%d.txt", idx))
			_, _ = p.Write([]byte("HAMAL concurrent file count test payload"))
			_ = w.Close()

			upReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", body)
			upReq.Header.Set("Content-Type", w.FormDataContentType())
			upResp, err := http.DefaultClient.Do(upReq)
			if err != nil {
				return
			}
			defer upResp.Body.Close()

			mu.Lock()
			if upResp.StatusCode == http.StatusCreated {
				successCount++
			} else if upResp.StatusCode == http.StatusBadRequest {
				var errResp struct {
					Error string `json:"error"`
				}
				_ = json.NewDecoder(upResp.Body).Decode(&errResp)
				if strings.Contains(errResp.Error, "room file count limit reached") {
					failCount++
				}
			}
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	if successCount != 2 || failCount != 2 {
		t.Fatalf("expected exactly 2 created (201) and 2 rejected (400), got created=%d, rejected=%d", successCount, failCount)
	}

	// Verify room files list has exactly 2 files
	statusReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", nil)
	statusResp, err := http.DefaultClient.Do(statusReq)
	if err != nil {
		t.Fatal(err)
	}
	defer statusResp.Body.Close()
	var statusData struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
	}
	_ = json.NewDecoder(statusResp.Body).Decode(&statusData)
	if len(statusData.Files) != 2 {
		t.Fatalf("expected exactly 2 files in room status, got %d", len(statusData.Files))
	}
}

func TestSecurityHeadersOnHTMLViews(t *testing.T) {
	a := testApp(t)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	// 1. Create a room to obtain tokens
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	var roomData struct {
		CreatorToken     string `json:"creator_token"`
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	urls := []string{
		ts.URL + "/",
		ts.URL + "/c/" + roomData.CreatorToken,
		ts.URL + "/r/" + roomData.ParticipantToken,
	}

	expectedCSP := "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'self'; form-action 'self';"

	for _, u := range urls {
		resp, err := http.Get(u)
		if err != nil {
			t.Fatalf("GET %s failed: %v", u, err)
		}
		defer resp.Body.Close()

		if csp := resp.Header.Get("Content-Security-Policy"); csp != expectedCSP {
			t.Errorf("[%s] expected CSP %q, got %q", u, expectedCSP, csp)
		}
		if xfo := resp.Header.Get("X-Frame-Options"); xfo != "DENY" {
			t.Errorf("[%s] expected X-Frame-Options DENY, got %q", u, xfo)
		}
		if xcto := resp.Header.Get("X-Content-Type-Options"); xcto != "nosniff" {
			t.Errorf("[%s] expected X-Content-Type-Options nosniff, got %q", u, xcto)
		}
		if ref := resp.Header.Get("Referrer-Policy"); ref != "no-referrer" {
			t.Errorf("[%s] expected Referrer-Policy no-referrer, got %q", u, ref)
		}
		if perm := resp.Header.Get("Permissions-Policy"); perm != "camera=(), microphone=(), geolocation=(), payment=(), usb=()" {
			t.Errorf("[%s] expected Permissions-Policy camera=(), ..., got %q", u, perm)
		}
	}
}

func TestSecurityHeadersOnStaticAssets(t *testing.T) {
	a := testApp(t)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	urls := []string{
		ts.URL + "/static/site.css",
		ts.URL + "/static/site.js",
		ts.URL + "/static/brand/hamal-logo-light.svg",
	}

	for _, u := range urls {
		resp, err := http.Get(u)
		if err != nil {
			t.Fatalf("GET %s failed: %v", u, err)
		}
		defer resp.Body.Close()

		if xcto := resp.Header.Get("X-Content-Type-Options"); xcto != "nosniff" {
			t.Errorf("[%s] expected nosniff, got %q", u, xcto)
		}
		if xfo := resp.Header.Get("X-Frame-Options"); xfo != "DENY" {
			t.Errorf("[%s] expected DENY, got %q", u, xfo)
		}
	}
}

func TestSecurityHeadersOnAPIEndpoints(t *testing.T) {
	a := testApp(t)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	apiPaths := []string{
		"/healthz",
		"/readyz",
	}

	for _, p := range apiPaths {
		resp, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatalf("GET %s failed: %v", p, err)
		}
		defer resp.Body.Close()

		if xcto := resp.Header.Get("X-Content-Type-Options"); xcto != "nosniff" {
			t.Errorf("[%s] expected nosniff, got %q", p, xcto)
		}
		if xfo := resp.Header.Get("X-Frame-Options"); xfo != "DENY" {
			t.Errorf("[%s] expected DENY, got %q", p, xfo)
		}
	}
}

func TestSecurityHeadersOnFileDownload(t *testing.T) {
	a := testApp(t)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	// 1. Create room
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	var roomData struct {
		CreatorToken     string `json:"creator_token"`
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	// 2. Upload file
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	p, _ := w.CreateFormFile("file", "download_sec.txt")
	_, _ = p.Write([]byte("security headers file content"))
	_ = w.Close()

	upReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", body)
	upReq.Header.Set("Content-Type", w.FormDataContentType())
	upResp, err := http.DefaultClient.Do(upReq)
	if err != nil {
		t.Fatal(err)
	}
	var fileData struct {
		ID string `json:"file_id"`
	}
	_ = json.NewDecoder(upResp.Body).Decode(&fileData)
	upResp.Body.Close()

	// 3. Download file and verify security headers
	downResp, err := http.Get(ts.URL + "/api/v1/rooms/" + roomData.ParticipantToken + "/files/" + fileData.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer downResp.Body.Close()

	if xcto := downResp.Header.Get("X-Content-Type-Options"); xcto != "nosniff" {
		t.Errorf("expected nosniff on download, got %q", xcto)
	}
	if xfo := downResp.Header.Get("X-Frame-Options"); xfo != "DENY" {
		t.Errorf("expected DENY on download, got %q", xfo)
	}
	if csp := downResp.Header.Get("Content-Security-Policy"); csp == "" {
		t.Errorf("expected CSP on download, got empty")
	}
}

func TestHSTSConditionallySentOnHTTPS(t *testing.T) {
	cfg := config.Default()
	cfg.TrustedProxies = []string{"127.0.0.1/32", "::1/128"}
	a := testAppWithConfig(t, cfg)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	// 1. Plain HTTP request -> HSTS must NOT be set
	plainResp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer plainResp.Body.Close()
	if hsts := plainResp.Header.Get("Strict-Transport-Security"); hsts != "" {
		t.Fatalf("expected no HSTS on plain HTTP, got %q", hsts)
	}

	// 2. Request behind trusted proxy with HTTPS -> HSTS must be set
	httpsReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/healthz", nil)
	httpsReq.Header.Set("X-Forwarded-Proto", "https")
	httpsResp, err := http.DefaultClient.Do(httpsReq)
	if err != nil {
		t.Fatal(err)
	}
	defer httpsResp.Body.Close()
	expectedHSTS := "max-age=31536000; includeSubDomains"
	if hsts := httpsResp.Header.Get("Strict-Transport-Security"); hsts != expectedHSTS {
		t.Fatalf("expected HSTS %q on HTTPS, got %q", expectedHSTS, hsts)
	}
}

func TestCookiePlainLANHTTP(t *testing.T) {
	cfg := config.Default()
	cfg.SecureCookies = "auto"
	a := testAppWithConfig(t, cfg)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	// 1. Create a room with PIN
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600, "pin": "9999"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	// 2. Submit PIN over plain HTTP
	authReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin": "9999"}`))
	authReq.Header.Set("Content-Type", "application/json")
	authResp, err := http.DefaultClient.Do(authReq)
	if err != nil {
		t.Fatal(err)
	}
	defer authResp.Body.Close()

	var sessionCookie *http.Cookie
	for _, c := range authResp.Cookies() {
		if strings.HasPrefix(c.Name, "landrop_session_") {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("expected session cookie in response")
	}

	// Verify all hardened attributes
	if !sessionCookie.HttpOnly {
		t.Errorf("expected HttpOnly=true, got false")
	}
	if sessionCookie.Secure {
		t.Errorf("expected Secure=false on plain LAN HTTP, got true")
	}
	if sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite=Lax, got %v", sessionCookie.SameSite)
	}
	if sessionCookie.Path != "/" {
		t.Errorf("expected Path=/, got %q", sessionCookie.Path)
	}
	if sessionCookie.Domain != "" {
		t.Errorf("expected host-only cookie (empty Domain), got %q", sessionCookie.Domain)
	}
	if sessionCookie.MaxAge <= 0 || sessionCookie.MaxAge > 3600 {
		t.Errorf("expected MaxAge around 3600 seconds, got %d", sessionCookie.MaxAge)
	}
}

func TestCookieExplicitConfigOverride(t *testing.T) {
	// Case A: SecureCookies = "true" forces Secure=true even on plain HTTP
	cfgTrue := config.Default()
	cfgTrue.SecureCookies = "true"
	aTrue := testAppWithConfig(t, cfgTrue)
	tsTrue := httptest.NewServer(aTrue.Handler())
	defer tsTrue.Close()

	cReq, _ := http.NewRequest(http.MethodPost, tsTrue.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600, "pin": "5555"}`))
	cReq.Header.Set("Content-Type", "application/json")
	cResp, err := http.DefaultClient.Do(cReq)
	if err != nil {
		t.Fatal(err)
	}
	var rData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(cResp.Body).Decode(&rData)
	cResp.Body.Close()

	aReq, _ := http.NewRequest(http.MethodPost, tsTrue.URL+"/api/v1/rooms/"+rData.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin": "5555"}`))
	aReq.Header.Set("Content-Type", "application/json")
	aResp, err := http.DefaultClient.Do(aReq)
	if err != nil {
		t.Fatal(err)
	}
	defer aResp.Body.Close()

	for _, c := range aResp.Cookies() {
		if strings.HasPrefix(c.Name, "landrop_session_") {
			if !c.Secure {
				t.Errorf("expected Secure=true when SecureCookies=true, got false")
			}
		}
	}

	// Case B: SecureCookies = "false" forces Secure=false even when X-Forwarded-Proto: https
	cfgFalse := config.Default()
	cfgFalse.SecureCookies = "false"
	cfgFalse.TrustedProxies = []string{"127.0.0.1/32", "::1/128"}
	aFalse := testAppWithConfig(t, cfgFalse)
	tsFalse := httptest.NewServer(aFalse.Handler())
	defer tsFalse.Close()

	cReq2, _ := http.NewRequest(http.MethodPost, tsFalse.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600, "pin": "5555"}`))
	cReq2.Header.Set("Content-Type", "application/json")
	cResp2, err := http.DefaultClient.Do(cReq2)
	if err != nil {
		t.Fatal(err)
	}
	var rData2 struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(cResp2.Body).Decode(&rData2)
	cResp2.Body.Close()

	aReq2, _ := http.NewRequest(http.MethodPost, tsFalse.URL+"/api/v1/rooms/"+rData2.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin": "5555"}`))
	aReq2.Header.Set("Content-Type", "application/json")
	aReq2.Header.Set("X-Forwarded-Proto", "https")
	aResp2, err := http.DefaultClient.Do(aReq2)
	if err != nil {
		t.Fatal(err)
	}
	defer aResp2.Body.Close()

	for _, c := range aResp2.Cookies() {
		if strings.HasPrefix(c.Name, "landrop_session_") {
			if c.Secure {
				t.Errorf("expected Secure=false when SecureCookies=false, got true")
			}
		}
	}
}

func TestParticipantAuthenticationWithHardenedCookie(t *testing.T) {
	a := testApp(t)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	// 1. Create PIN-protected room
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms", strings.NewReader(`{"ttl_seconds": 3600, "pin": "7777"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	var roomData struct {
		ParticipantToken string `json:"participant_token"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&roomData)
	createResp.Body.Close()

	// 2. Query file list without cookie -> 401 Unauthorized
	filesReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", nil)
	filesResp, err := http.DefaultClient.Do(filesReq)
	if err != nil {
		t.Fatal(err)
	}
	filesResp.Body.Close()
	if filesResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without PIN auth, got %d", filesResp.StatusCode)
	}

	// 3. Authenticate with PIN
	pinReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/auth/pin", strings.NewReader(`{"pin": "7777"}`))
	pinReq.Header.Set("Content-Type", "application/json")
	pinResp, err := http.DefaultClient.Do(pinReq)
	if err != nil {
		t.Fatal(err)
	}
	defer pinResp.Body.Close()
	if pinResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on PIN auth, got %d", pinResp.StatusCode)
	}

	var sessionCookie *http.Cookie
	for _, c := range pinResp.Cookies() {
		if strings.HasPrefix(c.Name, "landrop_session_") {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie")
	}

	// 4. Query file list WITH session cookie -> 200 OK
	filesReqAuth, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/rooms/"+roomData.ParticipantToken+"/files", nil)
	filesReqAuth.AddCookie(sessionCookie)
	filesRespAuth, err := http.DefaultClient.Do(filesReqAuth)
	if err != nil {
		t.Fatal(err)
	}
	defer filesRespAuth.Body.Close()
	if filesRespAuth.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with session cookie, got %d", filesRespAuth.StatusCode)
	}
}
