// Command diagserver runs the unchanged HAMAL application handler with a test-only upload
// observer. It exists only for the e2e harness (Phase 0 diagnostics for the intermittent WebKit
// "uploaded file is empty" failure) and is never built into the product binary or image.
//
// Startup mirrors cmd/lan-drop (same config loading, app.New, cleanup worker and http.Server
// timeouts). The only difference: when HAMAL_DIAG_UPLOAD_LOG is set, every
// POST /api/v1/rooms/{token}/files request body is copied as the application reads it (a tee; the
// handler still reads the original stream), and after the handler returns, the copy is parsed
// with mime/multipart and one JSON line is appended to that file with the request headers, the
// response status, and for every part: form name, filename, part headers, byte count and an
// FNV-1a hash. File contents are never logged.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/i1k3r/HAMAL/internal/app"
	"github.com/i1k3r/HAMAL/internal/config"
)

// Uploads in the diagnosed test are a few bytes; cap the copy so a large upload cannot exhaust memory.
const maxCopy = 4 << 20

var uploadPath = regexp.MustCompile(`^/api/v1/rooms/([^/]+)/files$`)

type partRecord struct {
	FormName  string            `json:"form_name"`
	FileName  string            `json:"file_name"`
	Headers   map[string]string `json:"headers"`
	Bytes     int64             `json:"bytes"`
	FNV1a32   string            `json:"fnv1a32"`
	ReadError string            `json:"read_error,omitempty"`
}

type uploadRecord struct {
	Time             string       `json:"time"`
	Token            string       `json:"token"`
	QueryFilename    string       `json:"query_filename"`
	ContentType      string       `json:"content_type"`
	ContentLength    int64        `json:"content_length"`
	TransferEncoding []string     `json:"transfer_encoding"`
	BodyBytesRead    int64        `json:"body_bytes_read_by_handler"`
	CopyTruncated    bool         `json:"copy_truncated"`
	Status           int          `json:"response_status"`
	ResponseBody     string       `json:"response_body"`
	Parts            []partRecord `json:"parts"`
	ParseError       string       `json:"parse_error,omitempty"`
	DurationMS       int64        `json:"duration_ms"`
}

type teeBody struct {
	r         io.ReadCloser
	buf       bytes.Buffer
	n         int64
	truncated bool
}

func (t *teeBody) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 {
		t.n += int64(n)
		room := maxCopy - t.buf.Len()
		if room > 0 {
			if n <= room {
				t.buf.Write(p[:n])
			} else {
				t.buf.Write(p[:room])
				t.truncated = true
			}
		} else {
			t.truncated = true
		}
	}
	return n, err
}

func (t *teeBody) Close() error { return t.r.Close() }

type recorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if r.body.Len() < 1024 {
		r.body.Write(p[:min(len(p), 1024-r.body.Len())])
	}
	return r.ResponseWriter.Write(p)
}

// Unwrap lets http.ResponseController (used by the handler for read deadlines) reach the real writer.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func observeUploads(next http.Handler, logPath string) http.Handler {
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := uploadPath.FindStringSubmatch(r.URL.Path)
		if r.Method != http.MethodPost || m == nil {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		tee := &teeBody{r: r.Body}
		r.Body = tee
		rec := &recorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		out := uploadRecord{
			Time:             start.UTC().Format(time.RFC3339Nano),
			Token:            m[1],
			QueryFilename:    r.URL.Query().Get("filename"),
			ContentType:      r.Header.Get("Content-Type"),
			ContentLength:    r.ContentLength,
			TransferEncoding: r.TransferEncoding,
			BodyBytesRead:    tee.n,
			CopyTruncated:    tee.truncated,
			Status:           rec.status,
			ResponseBody:     strings.TrimSpace(rec.body.String()),
			DurationMS:       time.Since(start).Milliseconds(),
		}
		out.Parts, out.ParseError = parseCopy(out.ContentType, tee.buf.Bytes())

		line, _ := json.Marshal(out)
		mu.Lock()
		defer mu.Unlock()
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return
		}
		_, _ = f.Write(append(line, '\n'))
		_ = f.Close()
	})
}

// parseCopy parses the copied bytes with the same library the handler uses. Only the bytes the
// handler actually read are available, so a part the handler never reached is reported as such.
func parseCopy(contentType string, body []byte) ([]partRecord, string) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "content-type: " + err.Error()
	}
	mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var parts []partRecord
	for {
		p, err := mr.NextPart()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return parts, ""
			}
			return parts, "next part: " + err.Error()
		}
		h := fnv.New32a()
		n, readErr := io.Copy(h, p)
		rec := partRecord{
			FormName: p.FormName(),
			FileName: p.FileName(),
			Headers:  map[string]string{},
			Bytes:    n,
			FNV1a32:  strings.ToLower(hex32(h.Sum32())),
		}
		for k, v := range p.Header {
			rec.Headers[k] = strings.Join(v, ", ")
		}
		if readErr != nil {
			rec.ReadError = readErr.Error()
		}
		parts = append(parts, rec)
	}
}

func hex32(v uint32) string {
	const digits = "0123456789abcdef"
	b := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		b[i] = digits[v&0xf]
		v >>= 4
	}
	return string(b)
}

func main() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, nil)).Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	logger := app.NewLogger(cfg.LogFormat, cfg.LogLevel)
	application, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("startup failed", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_, _ = application.RunCleanupOnce(ctx)
	cancel()
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	go application.StartCleanup(workerCtx)

	handler := application.Handler()
	if logPath := os.Getenv("HAMAL_DIAG_UPLOAD_LOG"); logPath != "" {
		handler = observeUploads(handler, logPath)
	}
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stop:
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}
