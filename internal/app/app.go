package app

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/i1k3r/HAMAL/internal/cleanup"
	"github.com/i1k3r/HAMAL/internal/config"
	"github.com/i1k3r/HAMAL/internal/database"
	"github.com/i1k3r/HAMAL/internal/file"
	"github.com/i1k3r/HAMAL/internal/room"
	"github.com/i1k3r/HAMAL/internal/share"
	"github.com/i1k3r/HAMAL/internal/storage"
)

//go:embed templates/*.html static/*
var assets embed.FS

// ParticipantRecord tracks an active participant connected to a room.
type ParticipantRecord struct {
	ID       string    `json:"id"`
	IP       string    `json:"ip"`
	Name     string    `json:"name"`
	LastSeen time.Time `json:"last_seen"`
}

type App struct {
	cfg             config.Config
	db              *sql.DB
	paths           storage.Paths
	rooms           *room.Store
	files           *file.Store
	shares          *share.Store
	cleanupWorker   *cleanup.Worker
	logger          *slog.Logger
	handler         http.Handler
	trustedProxies  []*net.IPNet
	roomLimiter     *IPRateLimiter
	authLimiter     *IPRateLimiter
	uploadLimiter   *IPRateLimiter
	downloadLimiter *IPRateLimiter
	participantMu   sync.RWMutex
	participants    map[string]map[string]ParticipantRecord
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	paths, err := storage.Initialize(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	secret, err := storage.ResolveServerSecret(paths, cfg.ServerSecret)
	if err != nil {
		return nil, err
	}
	cfg.ServerSecret = secret
	db, err := database.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	roomStore := room.NewStore(db, cfg.ServerSecret)
	shareStore := share.NewStore(db, cfg.ServerSecret)
	quotaManager := file.NewQuotaManager()
	fileStore := file.NewStore(db, paths, quotaManager, file.StoreOptions{
		MaxTotalStorage: cfg.MaxTotalStorage,
		MinFreeSpace:    cfg.MinFreeSpace,
	})
	cleanupWorker := cleanup.NewWorker(db, paths, cleanup.Options{
		Interval:            cfg.CleanupInterval,
		BatchSize:           cfg.CleanupBatchSize,
		StagingMaxAge:       cfg.StagingMaxAge,
		OrphanGracePeriod:   cfg.OrphanGracePeriod,
		ClosedRoomRetention: cfg.ClosedRoomRetention,
	}, logger)

	var trustedProxies []*net.IPNet
	for _, proxy := range cfg.TrustedProxies {
		_, ipNet, err := net.ParseCIDR(strings.TrimSpace(proxy))
		if err == nil && ipNet != nil {
			trustedProxies = append(trustedProxies, ipNet)
		}
	}

	roomLimiter := NewIPRateLimiter(cfg.ShareManagementRateLimit, cfg.ShareManagementRateLimit/3)
	authLimiter := NewIPRateLimiter(cfg.ShareManagementRateLimit, cfg.ShareManagementRateLimit/3)
	uploadLimiter := NewIPRateLimiter(cfg.ShareManagementRateLimit*2, cfg.ShareManagementRateLimit)
	downloadLimiter := NewIPRateLimiter(cfg.ShareAccessRateLimit, cfg.ShareAccessRateLimit/6)

	a := &App{
		cfg:             cfg,
		db:              db,
		paths:           paths,
		rooms:           roomStore,
		files:           fileStore,
		shares:          shareStore,
		cleanupWorker:   cleanupWorker,
		logger:          logger,
		trustedProxies:  trustedProxies,
		roomLimiter:     roomLimiter,
		authLimiter:     authLimiter,
		uploadLimiter:   uploadLimiter,
		downloadLimiter: downloadLimiter,
		participants:    make(map[string]map[string]ParticipantRecord),
	}
	routesHandler, err := a.routes()
	if err != nil {
		db.Close()
		return nil, err
	}
	a.handler = a.securityHeadersMiddleware(routesHandler)
	return a, nil
}

func (a *App) recordParticipant(roomID, ip, userAgent string) {
	if roomID == "" || ip == "" {
		return
	}
	a.participantMu.Lock()
	defer a.participantMu.Unlock()
	if a.participants == nil {
		a.participants = make(map[string]map[string]ParticipantRecord)
	}
	if a.participants[roomID] == nil {
		a.participants[roomID] = make(map[string]ParticipantRecord)
	}

	name := "Mobile Device"
	if strings.Contains(userAgent, "iPhone") || strings.Contains(userAgent, "iPad") {
		name = "Apple iOS Device"
	} else if strings.Contains(userAgent, "Android") {
		name = "Android Device"
	} else if strings.Contains(userAgent, "Windows") {
		name = "Windows Device"
	} else if strings.Contains(userAgent, "Macintosh") {
		name = "macOS Device"
	} else if strings.Contains(userAgent, "Linux") {
		name = "Linux Device"
	}

	a.participants[roomID][ip] = ParticipantRecord{
		ID:       ip,
		IP:       ip,
		Name:     name,
		LastSeen: time.Now().UTC(),
	}
}

func (a *App) getActiveParticipants(roomID string) []ParticipantRecord {
	a.participantMu.Lock()
	defer a.participantMu.Unlock()
	if a.participants == nil || a.participants[roomID] == nil {
		return []ParticipantRecord{}
	}

	cutoff := time.Now().UTC().Add(-60 * time.Second)
	var active []ParticipantRecord
	for k, p := range a.participants[roomID] {
		if p.LastSeen.After(cutoff) {
			active = append(active, p)
		} else {
			delete(a.participants[roomID], k)
		}
	}
	if active == nil {
		active = []ParticipantRecord{}
	}
	return active
}

func (a *App) getActiveParticipantCount(roomID string) int {
	return len(a.getActiveParticipants(roomID))
}

func (a *App) trackParticipantFromRequest(r *http.Request, rm *room.Room, role room.Role) {
	if role != room.RoleParticipant || rm == nil {
		return
	}
	clientIP := a.clientIP(r)
	if clientIP == "" {
		return
	}
	a.recordParticipant(rm.ID, clientIP, r.UserAgent())
}

func (a *App) Close() error {
	return a.db.Close()
}

func (a *App) Handler() http.Handler {
	return a.handler
}

func (a *App) RunCleanupOnce(ctx context.Context) (cleanup.Stats, error) {
	return a.cleanupWorker.RunOnce(ctx)
}

func (a *App) StartCleanup(ctx context.Context) {
	a.cleanupWorker.Run(ctx)
}

func (a *App) CleanupWorker() *cleanup.Worker {
	return a.cleanupWorker
}

func (a *App) Ready() error {
	if err := database.Check(a.db); err != nil {
		return err
	}
	return storage.Check(a.paths)
}

// clientIP extracts the verified client IP. If the request comes from a configured trusted proxy,
// it inspects X-Forwarded-For or X-Real-IP. Otherwise it strictly returns RemoteAddr.
func (a *App) clientIP(r *http.Request) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = strings.TrimSpace(r.RemoteAddr)
	} else {
		remoteHost = strings.TrimSpace(remoteHost)
	}

	remoteIP := net.ParseIP(remoteHost)
	if remoteIP == nil || len(a.trustedProxies) == 0 {
		return remoteHost
	}

	isTrusted := false
	for _, ipNet := range a.trustedProxies {
		if ipNet.Contains(remoteIP) {
			isTrusted = true
			break
		}
	}

	if !isTrusted {
		return remoteHost
	}

	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for _, part := range parts {
			candidate := strings.TrimSpace(part)
			if ip := net.ParseIP(candidate); ip != nil {
				return candidate
			}
		}
	}

	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if ip := net.ParseIP(xri); ip != nil {
			return xri
		}
	}

	return remoteHost
}

func (a *App) checkRateLimit(w http.ResponseWriter, r *http.Request, limiter *IPRateLimiter) bool {
	if limiter == nil {
		return true
	}
	ip := a.clientIP(r)
	allowed, retryAfter := limiter.Allow(ip)
	if !allowed {
		if retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
		}
		writeJSONError(w, "too many requests, please try again later", http.StatusTooManyRequests)
		return false
	}
	return true
}

func (a *App) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'self'; form-action 'self';")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")

		if a.isHTTPS(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

type idleTimeoutReader struct {
	r       io.Reader
	rc      *http.ResponseController
	timeout time.Duration
	useRC   bool
}

func newIdleTimeoutReader(r io.Reader, rc *http.ResponseController, timeout time.Duration) io.Reader {
	itr := &idleTimeoutReader{
		r:       r,
		rc:      rc,
		timeout: timeout,
	}
	if rc != nil && timeout > 0 {
		if err := rc.SetReadDeadline(time.Now().Add(timeout)); err == nil {
			itr.useRC = true
		}
	}
	return itr
}

func (itr *idleTimeoutReader) Read(p []byte) (int, error) {
	if itr.timeout <= 0 {
		return itr.r.Read(p)
	}

	if itr.useRC {
		_ = itr.rc.SetReadDeadline(time.Now().Add(itr.timeout))
		n, err := itr.r.Read(p)
		if err == io.EOF {
			_ = itr.rc.SetReadDeadline(time.Time{})
		} else if n > 0 {
			_ = itr.rc.SetReadDeadline(time.Now().Add(itr.timeout))
		}
		return n, err
	}

	// Fallback for mocked readers or test recorders without socket ResponseController
	type readResult struct {
		n   int
		err error
	}
	ch := make(chan readResult, 1)
	go func() {
		n, err := itr.r.Read(p)
		ch <- readResult{n: n, err: err}
	}()

	timer := time.NewTimer(itr.timeout)
	defer timer.Stop()

	select {
	case res := <-ch:
		return res.n, res.err
	case <-timer.C:
		return 0, os.ErrDeadlineExceeded
	}
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") || strings.Contains(errStr, "i/o timeout")
}

func (a *App) isHTTPS(r *http.Request) bool {
	if a.cfg.SecureCookies == "true" {
		return true
	}
	if a.cfg.SecureCookies == "false" {
		return false
	}
	if r.TLS != nil {
		return true
	}
	// In auto mode, check if request is coming from a trusted reverse proxy
	if len(a.trustedProxies) > 0 {
		remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			remoteHost = strings.TrimSpace(r.RemoteAddr)
		} else {
			remoteHost = strings.TrimSpace(remoteHost)
		}
		clientIP := net.ParseIP(remoteHost)
		if clientIP != nil {
			for _, ipNet := range a.trustedProxies {
				if ipNet.Contains(clientIP) {
					if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
						return true
					}
					break
				}
			}
		}
	}
	return false
}

func (a *App) baseURL(r *http.Request) string {
	if a.cfg.BaseURL != "" {
		return strings.TrimRight(a.cfg.BaseURL, "/")
	}
	scheme := "http"
	if a.isHTTPS(r) {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

func (a *App) shareURL(r *http.Request, token string) string {
	if a.cfg.PublicBaseURL != "" {
		return fmt.Sprintf("%s/s/%s", strings.TrimRight(a.cfg.PublicBaseURL, "/"), token)
	}
	return fmt.Sprintf("%s/s/%s", a.baseURL(r), token)
}

func (a *App) getSessionCookie(r *http.Request, roomID string) string {
	cookieName := "landrop_session_" + roomID
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func (a *App) isParticipantAuthenticated(ctx context.Context, rm *room.Room, r *http.Request) bool {
	if !rm.PinRequired {
		return true
	}
	token := a.getSessionCookie(r, rm.ID)
	if token == "" {
		return false
	}
	valid, err := a.rooms.ValidateSession(ctx, rm.ID, token)
	return err == nil && valid
}

func (a *App) routes() (http.Handler, error) {
	tmpl, err := template.ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	static, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		data, err := assets.ReadFile("static/favicon.ico")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	})

	// Health & Readiness
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := a.Ready(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"not_ready"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	// HTML Views
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.ExecuteTemplate(w, "index.html", map[string]any{
			"Year": time.Now().Year(),
		})
	})

	mux.HandleFunc("GET /c/{token}", func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		rm, role, err := a.rooms.GetByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, room.ErrRoomNotFound) {
				http.NotFound(w, r)
				return
			}
			if errors.Is(err, room.ErrRoomExpired) || errors.Is(err, room.ErrRoomClosed) {
				w.WriteHeader(http.StatusGone)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_ = tmpl.ExecuteTemplate(w, "creator.html", map[string]any{
					"Year":               time.Now().Year(),
					"CreatorToken":       token,
					"ParticipantToken":   "",
					"ParticipantURL":     "",
					"ExpiresAtRFC3339":   "",
					"Inactive":           true,
					"PinRequired":        false,
					"IsLocked":           false,
					"Files":              []file.File{},
					"GlobalShareEnabled": a.cfg.GlobalShareEnabled,
					"Shares":             []share.Share{},
				})
				return
			}
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if role != room.RoleCreator {
			http.NotFound(w, r)
			return
		}

		participantToken := room.DeriveParticipantToken(token)
		participantURL := fmt.Sprintf("%s/r/%s", a.baseURL(r), participantToken)

		filesList, _ := a.files.ListReadyFiles(r.Context(), rm.ID)
		if filesList == nil {
			filesList = []file.File{}
		}

		var sharesList []share.Share
		if a.cfg.GlobalShareEnabled {
			sharesList, _ = a.shares.ListRoomShares(r.Context(), rm.ID)
		}
		if sharesList == nil {
			sharesList = []share.Share{}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.ExecuteTemplate(w, "creator.html", map[string]any{
			"Year":               time.Now().Year(),
			"CreatorToken":       token,
			"ParticipantToken":   participantToken,
			"ParticipantURL":     participantURL,
			"ExpiresAtRFC3339":   rm.ExpiresAt.Format(time.RFC3339),
			"Inactive":           false,
			"PinRequired":        rm.PinRequired,
			"IsLocked":           rm.IsLocked(),
			"Files":              filesList,
			"GlobalShareEnabled": a.cfg.GlobalShareEnabled,
			"Shares":             sharesList,
		})
	})

	mux.HandleFunc("GET /r/{token}", func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		rm, role, err := a.rooms.GetByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, room.ErrRoomNotFound) {
				http.NotFound(w, r)
				return
			}
			if errors.Is(err, room.ErrRoomExpired) || errors.Is(err, room.ErrRoomClosed) {
				w.WriteHeader(http.StatusGone)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_ = tmpl.ExecuteTemplate(w, "participant.html", map[string]any{
					"Year":              time.Now().Year(),
					"ParticipantToken":  token,
					"ExpiresAtRFC3339":  "",
					"Inactive":          true,
					"PinRequired":       false,
					"PinAuthenticated":  false,
					"IsLocked":          false,
					"RetryAfterSeconds": 0,
					"Files":             []file.File{},
				})
				return
			}
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if role != room.RoleParticipant {
			http.NotFound(w, r)
			return
		}

		a.trackParticipantFromRequest(r, rm, role)
		activeCount := a.getActiveParticipantCount(rm.ID)

		isAuth := a.isParticipantAuthenticated(r.Context(), rm, r)

		var filesList []file.File
		if !rm.PinRequired || isAuth {
			filesList, _ = a.files.ListReadyFiles(r.Context(), rm.ID)
		}
		if filesList == nil {
			filesList = []file.File{}
		}

		var closeDeadlineStr string
		if rm.CloseDeadline != nil {
			closeDeadlineStr = rm.CloseDeadline.Format(time.RFC3339)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		_ = tmpl.ExecuteTemplate(w, "participant.html", map[string]any{
			"Year":                    time.Now().Year(),
			"ParticipantToken":        token,
			"ExpiresAtRFC3339":        rm.ExpiresAt.Format(time.RFC3339),
			"Status":                  rm.Status,
			"CloseDeadline":           closeDeadlineStr,
			"ClosingRemainingSeconds": rm.ClosingRemainingSeconds(),
			"Inactive":                false,
			"PinRequired":             rm.PinRequired,
			"PinAuthenticated":        isAuth,
			"IsLocked":                rm.IsLocked(),
			"RetryAfterSeconds":       rm.LockoutRemainingSeconds(),
			"ParticipantCount":        activeCount,
			"Files":                   filesList,
		})
	})

	// JSON API Endpoints
	mux.HandleFunc("POST /api/v1/rooms", func(w http.ResponseWriter, r *http.Request) {
		if !a.checkRateLimit(w, r, a.roomLimiter) {
			return
		}

		var req struct {
			TTLSeconds int    `json:"ttl_seconds"`
			PIN        string `json:"pin"`
		}

		contentType := r.Header.Get("Content-Type")
		if strings.HasPrefix(contentType, "application/json") {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
				writeJSONError(w, "invalid request payload", http.StatusBadRequest)
				return
			}
		} else {
			_ = r.ParseForm()
			if val := r.FormValue("ttl_seconds"); val != "" {
				parsed, err := strconv.Atoi(val)
				if err == nil {
					req.TTLSeconds = parsed
				}
			}
			req.PIN = r.FormValue("pin")
		}

		ttl := a.cfg.DefaultTTL
		if req.TTLSeconds > 0 {
			ttl = time.Duration(req.TTLSeconds) * time.Second
		}

		if ttl < a.cfg.MinTTL || ttl > a.cfg.MaxTTL {
			writeJSONError(w, fmt.Sprintf("TTL must be between %s and %s", a.cfg.MinTTL, a.cfg.MaxTTL), http.StatusBadRequest)
			return
		}

		created, err := a.rooms.Create(r.Context(), ttl, a.cfg.MaxRoomSize, a.cfg.MaxFileSize, a.cfg.MaxFilesPerRoom, req.PIN)
		if err != nil {
			if errors.Is(err, room.ErrInvalidPIN) {
				writeJSONError(w, "PIN must be between 4 and 8 characters", http.StatusBadRequest)
				return
			}
			a.logger.Error("failed to create room", "error", err)
			writeJSONError(w, "failed to create room", http.StatusInternalServerError)
			return
		}

		baseURL := a.baseURL(r)
		resp := map[string]any{
			"room_id":           created.ID,
			"creator_token":     created.CreatorToken,
			"creator_url":       fmt.Sprintf("%s/c/%s", baseURL, created.CreatorToken),
			"participant_token": created.ParticipantToken,
			"participant_url":   fmt.Sprintf("%s/r/%s", baseURL, created.ParticipantToken),
			"expires_at":        created.ExpiresAt.Format(time.RFC3339),
			"ttl_seconds":       created.TTLSeconds,
			"pin_required":      created.PinRequired,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Phase 4: Participant PIN verification
	mux.HandleFunc("POST /api/v1/rooms/{token}/auth/pin", func(w http.ResponseWriter, r *http.Request) {
		if !a.checkRateLimit(w, r, a.authLimiter) {
			return
		}

		token := r.PathValue("token")
		var req struct {
			PIN string `json:"pin"`
		}

		contentType := r.Header.Get("Content-Type")
		if strings.HasPrefix(contentType, "application/json") {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
				writeJSONError(w, "invalid request payload", http.StatusBadRequest)
				return
			}
		} else {
			_ = r.ParseForm()
			req.PIN = r.FormValue("pin")
		}

		ok, remaining, retryAfterSec, err := a.rooms.VerifyAndRecordPINAttempt(r.Context(), token, req.PIN)
		if err != nil {
			if errors.Is(err, room.ErrRoomNotFound) {
				writeJSONError(w, "room not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, room.ErrRoomExpired) {
				writeJSONError(w, "room has expired", http.StatusGone)
				return
			}
			if errors.Is(err, room.ErrRoomClosed) {
				writeJSONError(w, "room is closed", http.StatusGone)
				return
			}
			if errors.Is(err, room.ErrUnauthorized) {
				writeJSONError(w, "unauthorized", http.StatusForbidden)
				return
			}
			if errors.Is(err, room.ErrRoomLocked) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":               "too many failed attempts, please wait before trying again",
					"retry_after_seconds": retryAfterSec,
				})
				return
			}
			if errors.Is(err, room.ErrIncorrectPIN) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":              "incorrect PIN",
					"remaining_attempts": remaining,
				})
				return
			}
			a.logger.Error("failed to verify pin", "error", err)
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":              "incorrect PIN",
				"remaining_attempts": remaining,
			})
			return
		}

		rm, _, err := a.rooms.GetByToken(r.Context(), token)
		if err != nil {
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		a.trackParticipantFromRequest(r, rm, room.RoleParticipant)

		sessionToken, err := a.rooms.CreateSession(r.Context(), rm.ID, rm.ExpiresAt)
		if err != nil {
			a.logger.Error("failed to create participant session", "error", err)
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		maxAge := int(time.Until(rm.ExpiresAt).Seconds())
		if maxAge < 0 {
			maxAge = 0
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "landrop_session_" + rm.ID,
			Value:    sessionToken,
			Path:     "/",
			HttpOnly: true,
			Secure:   a.isHTTPS(r),
			SameSite: http.SameSiteLaxMode,
			Expires:  rm.ExpiresAt,
			MaxAge:   maxAge,
		})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "authenticated",
		})
	})

	// Phase 4: Creator unlock endpoint
	mux.HandleFunc("POST /api/v1/rooms/{token}/unlock", func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		err := a.rooms.UnlockRoom(r.Context(), token)
		if err != nil {
			if errors.Is(err, room.ErrRoomNotFound) {
				writeJSONError(w, "room not found or unauthorized", http.StatusNotFound)
				return
			}
			if errors.Is(err, room.ErrRoomClosed) {
				writeJSONError(w, "room is closed", http.StatusGone)
				return
			}
			a.logger.Error("failed to unlock room", "error", err)
			writeJSONError(w, "failed to unlock room", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "unlocked"})
	})

	mux.HandleFunc("GET /api/v1/rooms/{token}", func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		rm, role, err := a.rooms.GetByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, room.ErrRoomNotFound) {
				writeJSONError(w, "room not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, room.ErrRoomExpired) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusGone)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status":            "expired",
					"error":             "room has expired",
					"remaining_seconds": 0,
				})
				return
			}
			if errors.Is(err, room.ErrRoomClosed) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusGone)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status":            "closed",
					"error":             "room is closed",
					"remaining_seconds": 0,
				})
				return
			}
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		remainingSeconds := int(time.Until(rm.ExpiresAt).Seconds())
		if remainingSeconds < 0 {
			remainingSeconds = 0
		}

		a.trackParticipantFromRequest(r, rm, role)
		activeCount := a.getActiveParticipantCount(rm.ID)
		activeList := a.getActiveParticipants(rm.ID)

		isAuth := (role == room.RoleCreator) || a.isParticipantAuthenticated(r.Context(), rm, r)

		var closeDeadlineStr string
		if rm.CloseDeadline != nil {
			closeDeadlineStr = rm.CloseDeadline.Format(time.RFC3339)
		}

		resp := map[string]any{
			"room_id":                   rm.ID,
			"status":                    rm.Status,
			"is_creator":                (role == room.RoleCreator),
			"expires_at":                rm.ExpiresAt.Format(time.RFC3339),
			"close_deadline":            closeDeadlineStr,
			"closing_remaining_seconds": rm.ClosingRemainingSeconds(),
			"remaining_seconds":         remainingSeconds,
			"pin_required":              rm.PinRequired,
			"pin_authenticated":         isAuth,
			"is_locked":                 rm.IsLocked(),
			"retry_after_seconds":       rm.LockoutRemainingSeconds(),
			"participant_count":         activeCount,
			"participants":              activeList,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("POST /api/v1/rooms/{token}/close", func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		rm, role, err := a.rooms.GetByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, room.ErrRoomNotFound) {
				writeJSONError(w, "room not found or unauthorized", http.StatusNotFound)
				return
			}
			if errors.Is(err, room.ErrRoomExpired) || errors.Is(err, room.ErrRoomClosed) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "closed", "remaining_seconds": 0})
				return
			}
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if role == room.RoleCreator {
			err = a.rooms.CloseByRoomID(r.Context(), rm.ID)
			if err != nil && !errors.Is(err, room.ErrRoomClosed) {
				writeJSONError(w, "failed to close room", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "closed", "remaining_seconds": 0})
			return
		}

		if role == room.RoleParticipant && rm.PinRequired {
			if !a.isParticipantAuthenticated(r.Context(), rm, r) {
				writeJSONError(w, "PIN authentication required", http.StatusUnauthorized)
				return
			}
		}

		closingRoom, err := a.rooms.StartClosing(r.Context(), rm.ID, 10*time.Second)
		if err != nil {
			if errors.Is(err, room.ErrRoomClosed) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "closed", "remaining_seconds": 0})
				return
			}
			writeJSONError(w, "failed to close room", http.StatusInternalServerError)
			return
		}

		var closeDeadlineStr string
		if closingRoom.CloseDeadline != nil {
			closeDeadlineStr = closingRoom.CloseDeadline.Format(time.RFC3339)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                    closingRoom.Status,
			"close_deadline":            closeDeadlineStr,
			"closing_remaining_seconds": closingRoom.ClosingRemainingSeconds(),
			"remaining_seconds":         closingRoom.ClosingRemainingSeconds(),
		})
	})

	mux.HandleFunc("GET /api/v1/rooms/{token}/qr.svg", func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		rm, role, err := a.rooms.GetByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, room.ErrRoomNotFound) {
				http.NotFound(w, r)
				return
			}
			if errors.Is(err, room.ErrRoomExpired) || errors.Is(err, room.ErrRoomClosed) {
				http.Error(w, "room is inactive", http.StatusGone)
				return
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		_ = rm
		var participantToken string
		if role == room.RoleCreator {
			participantToken = room.DeriveParticipantToken(token)
		} else {
			participantToken = token
		}

		participantURL := fmt.Sprintf("%s/r/%s", a.baseURL(r), participantToken)
		svgBytes, err := room.GenerateSVG(participantURL, 280)
		if err != nil {
			http.Error(w, "failed to generate qr code", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "private, max-age=60")
		_, _ = w.Write(svgBytes)
	})

	// Phase 3A: True streaming file upload endpoint (PIN protected for participants)
	mux.HandleFunc("POST /api/v1/rooms/{token}/files", func(w http.ResponseWriter, r *http.Request) {
		if !a.checkRateLimit(w, r, a.uploadLimiter) {
			return
		}

		token := r.PathValue("token")
		rm, role, err := a.rooms.GetByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, room.ErrRoomNotFound) {
				writeJSONError(w, "room not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, room.ErrRoomExpired) {
				writeJSONError(w, "room has expired", http.StatusGone)
				return
			}
			if errors.Is(err, room.ErrRoomClosed) {
				writeJSONError(w, "room is closed", http.StatusGone)
				return
			}
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if rm.Status == "closing" {
			writeJSONError(w, "room is closing, uploads not allowed", http.StatusConflict)
			return
		}

		a.trackParticipantFromRequest(r, rm, role)

		if role == room.RoleParticipant && rm.PinRequired {
			if !a.isParticipantAuthenticated(r.Context(), rm, r) {
				writeJSONError(w, "PIN authentication required", http.StatusUnauthorized)
				return
			}
		}

		// Request body bounded to MaxFileSize + 10MB envelope
		r.Body = http.MaxBytesReader(w, r.Body, rm.MaxFileSize+10<<20)

		mr, err := r.MultipartReader()
		if err != nil {
			writeJSONError(w, "expected multipart/form-data payload", http.StatusBadRequest)
			return
		}

		for {
			part, err := mr.NextPart()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				writeJSONError(w, fmt.Sprintf("read multipart part: %v", err), http.StatusBadRequest)
				return
			}

			if part.FormName() == "file" {
				filename := part.FileName()
				contentType := part.Header.Get("Content-Type")

				declaredSize := int64(0)
				if r.ContentLength > 0 {
					declaredSize = r.ContentLength
				}

				var bodyReader io.Reader = part
				if a.cfg.UploadIdleTimeout > 0 {
					rc := http.NewResponseController(w)
					_ = rc.SetReadDeadline(time.Now().Add(a.cfg.UploadIdleTimeout))
					defer func() {
						_ = rc.SetReadDeadline(time.Time{})
					}()
					bodyReader = newIdleTimeoutReader(part, rc, a.cfg.UploadIdleTimeout)
				}

				uploaded, err := a.files.StreamUpload(
					r.Context(),
					rm.ID,
					filename,
					contentType,
					bodyReader,
					declaredSize,
					rm.MaxFileSize,
					rm.MaxRoomSize,
					rm.MaxFiles,
				)
				if err != nil {
					if errors.Is(err, file.ErrEmptyFile) {
						writeJSONError(w, "uploaded file is empty", http.StatusBadRequest)
						return
					}
					if errors.Is(err, file.ErrFileLimitReached) {
						writeJSONError(w, "room file count limit reached", http.StatusBadRequest)
						return
					}
					if errors.Is(err, file.ErrFileTooLarge) || errors.Is(err, file.ErrQuotaExceeded) || errors.Is(err, file.ErrGlobalStorageExceeded) || errors.Is(err, file.ErrInsufficientStorage) {
						writeJSONError(w, "file exceeds maximum size or storage quota", http.StatusRequestEntityTooLarge)
						return
					}
					if isTimeoutError(err) {
						writeJSONError(w, "upload timed out due to inactivity", http.StatusRequestTimeout)
						return
					}
					a.logger.Error("failed to stream upload", "error", err)
					writeJSONError(w, "failed to save uploaded file", http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(uploaded)
				return
			}
		}

		writeJSONError(w, "missing 'file' form field", http.StatusBadRequest)
	})

	// Phase 3A: File listing endpoint (PIN protected for participants)
	mux.HandleFunc("GET /api/v1/rooms/{token}/files", func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		rm, role, err := a.rooms.GetByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, room.ErrRoomNotFound) {
				writeJSONError(w, "room not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, room.ErrRoomExpired) {
				writeJSONError(w, "room has expired", http.StatusGone)
				return
			}
			if errors.Is(err, room.ErrRoomClosed) {
				writeJSONError(w, "room is closed", http.StatusGone)
				return
			}
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if role == room.RoleParticipant && rm.PinRequired {
			if !a.isParticipantAuthenticated(r.Context(), rm, r) {
				writeJSONError(w, "PIN authentication required", http.StatusUnauthorized)
				return
			}
		}

		a.trackParticipantFromRequest(r, rm, role)

		filesList, err := a.files.ListReadyFiles(r.Context(), rm.ID)
		if err != nil {
			a.logger.Error("failed to list files", "error", err)
			writeJSONError(w, "failed to list files", http.StatusInternalServerError)
			return
		}

		usedBytes, count, err := a.files.GetRoomUsageAndCount(r.Context(), rm.ID)
		if err != nil {
			writeJSONError(w, "failed to calculate room usage", http.StatusInternalServerError)
			return
		}

		if filesList == nil {
			filesList = []file.File{}
		}

		var closeDeadlineStr string
		if rm.CloseDeadline != nil {
			closeDeadlineStr = rm.CloseDeadline.Format(time.RFC3339)
		}

		resp := map[string]any{
			"room_id":                   rm.ID,
			"status":                    rm.Status,
			"close_deadline":            closeDeadlineStr,
			"closing_remaining_seconds": rm.ClosingRemainingSeconds(),
			"files":                     filesList,
			"total_size_bytes":          usedBytes,
			"file_count":                count,
			"max_room_size":             rm.MaxRoomSize,
			"max_files":                 rm.MaxFiles,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Phase 3B: File download endpoint (PIN protected for participants)
	downloadHandler := func(w http.ResponseWriter, r *http.Request) {
		if !a.checkRateLimit(w, r, a.downloadLimiter) {
			return
		}

		token := r.PathValue("token")
		fileID := r.PathValue("file_id")

		rm, role, err := a.rooms.GetByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, room.ErrRoomNotFound) {
				writeJSONError(w, "room not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, room.ErrRoomExpired) {
				writeJSONError(w, "room has expired", http.StatusGone)
				return
			}
			if errors.Is(err, room.ErrRoomClosed) {
				writeJSONError(w, "room is closed", http.StatusGone)
				return
			}
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if rm.Status == "closing" {
			writeJSONError(w, "room is closing, downloads not allowed", http.StatusGone)
			return
		}

		if role == room.RoleParticipant && rm.PinRequired {
			if !a.isParticipantAuthenticated(r.Context(), rm, r) {
				writeJSONError(w, "PIN authentication required", http.StatusUnauthorized)
				return
			}
		}

		a.trackParticipantFromRequest(r, rm, role)

		readyFile, err := a.files.GetReadyFile(r.Context(), rm.ID, fileID)
		if err != nil {
			if errors.Is(err, file.ErrFileNotFound) {
				writeJSONError(w, "file not found", http.StatusNotFound)
				return
			}
			a.logger.Error("failed to get ready file", "error", err)
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		f, err := a.files.OpenStorageFile(readyFile.StorageID)
		if err != nil {
			if errors.Is(err, file.ErrFileNotFound) || errors.Is(err, file.ErrInvalidFilename) {
				a.logger.Error("storage object missing on disk", "storage_id", readyFile.StorageID, "error", err)
				writeJSONError(w, "file not found", http.StatusNotFound)
				return
			}
			a.logger.Error("failed to open storage file", "error", err)
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}
		defer f.Close()

		w.Header().Set("Content-Type", readyFile.ContentType)
		w.Header().Set("Content-Disposition", file.SanitizeContentDisposition(readyFile.OriginalFilename))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-transform, max-age=0, must-revalidate")

		http.ServeContent(w, r, readyFile.OriginalFilename, readyFile.CreatedAt, f)
	}

	mux.HandleFunc("GET /api/v1/rooms/{token}/files/{file_id}", downloadHandler)
	mux.HandleFunc("HEAD /api/v1/rooms/{token}/files/{file_id}", downloadHandler)

	// Global Share endpoints (creator management & public download capability)
	mux.HandleFunc("POST /api/v1/rooms/{token}/files/{file_id}/share", func(w http.ResponseWriter, r *http.Request) {
		if !a.cfg.GlobalShareEnabled {
			http.NotFound(w, r)
			return
		}
		if !a.checkRateLimit(w, r, a.roomLimiter) {
			return
		}

		token := r.PathValue("token")
		fileID := r.PathValue("file_id")

		rm, role, err := a.rooms.GetByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, room.ErrRoomNotFound) {
				writeJSONError(w, "room not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, room.ErrRoomExpired) {
				writeJSONError(w, "room has expired", http.StatusGone)
				return
			}
			if errors.Is(err, room.ErrRoomClosed) {
				writeJSONError(w, "room is closed", http.StatusGone)
				return
			}
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if role != room.RoleCreator {
			writeJSONError(w, "room not found", http.StatusNotFound)
			return
		}

		requestedTTL := a.cfg.DefaultShareTTL
		if r.Body != nil && r.ContentLength > 0 {
			var payload struct {
				TTLSeconds int `json:"ttl_seconds"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err == nil && payload.TTLSeconds > 0 {
				requestedTTL = time.Duration(payload.TTLSeconds) * time.Second
			}
		}

		sh, shareToken, err := a.shares.CreateShare(
			r.Context(),
			rm.ID,
			fileID,
			requestedTTL,
			rm.ExpiresAt,
			a.cfg.MaxShareTTL,
			a.cfg.MaxSharesPerRoom,
		)
		if err != nil {
			if errors.Is(err, file.ErrFileNotFound) {
				writeJSONError(w, "file not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, share.ErrShareLimitReached) {
				writeJSONError(w, "room share limit reached", http.StatusBadRequest)
				return
			}
			if errors.Is(err, share.ErrRoomInactive) {
				writeJSONError(w, "room is inactive", http.StatusGone)
				return
			}
			a.logger.Error("failed to create share", "error", err)
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"share_id":   sh.ID,
			"share_url":  a.shareURL(r, shareToken),
			"expires_at": sh.ExpiresAt.Format(time.RFC3339),
			"status":     sh.Status,
		})
	})

	mux.HandleFunc("POST /api/v1/rooms/{token}/shares/{share_id}/revoke", func(w http.ResponseWriter, r *http.Request) {
		if !a.cfg.GlobalShareEnabled {
			http.NotFound(w, r)
			return
		}
		if !a.checkRateLimit(w, r, a.roomLimiter) {
			return
		}

		token := r.PathValue("token")
		shareID := r.PathValue("share_id")

		rm, role, err := a.rooms.GetByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, room.ErrRoomNotFound) {
				writeJSONError(w, "room not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, room.ErrRoomExpired) {
				writeJSONError(w, "room has expired", http.StatusGone)
				return
			}
			if errors.Is(err, room.ErrRoomClosed) {
				writeJSONError(w, "room is closed", http.StatusGone)
				return
			}
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if role != room.RoleCreator {
			writeJSONError(w, "room not found", http.StatusNotFound)
			return
		}

		err = a.shares.RevokeShare(r.Context(), rm.ID, shareID)
		if err != nil {
			if errors.Is(err, share.ErrShareNotFound) {
				writeJSONError(w, "share not found", http.StatusNotFound)
				return
			}
			a.logger.Error("failed to revoke share", "error", err)
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "revoked",
		})
	})

	mux.HandleFunc("GET /s/{token}", func(w http.ResponseWriter, r *http.Request) {
		if !a.cfg.GlobalShareEnabled {
			http.NotFound(w, r)
			return
		}
		if !a.checkRateLimit(w, r, a.downloadLimiter) {
			return
		}

		token := r.PathValue("token")
		sh, f, err := a.shares.GetByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, share.ErrShareNotFound) || errors.Is(err, share.ErrInvalidToken) {
				http.NotFound(w, r)
				return
			}
			if errors.Is(err, share.ErrShareExpired) || errors.Is(err, share.ErrShareRevoked) || errors.Is(err, share.ErrRoomInactive) {
				w.WriteHeader(http.StatusGone)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_ = tmpl.ExecuteTemplate(w, "share.html", map[string]any{
					"Year":             time.Now().Year(),
					"ShareToken":       token,
					"ExpiresAtRFC3339": "",
					"Filename":         "",
					"FormattedSize":    "",
					"ContentType":      "",
					"Inactive":         true,
				})
				return
			}
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.ExecuteTemplate(w, "share.html", map[string]any{
			"Year":             time.Now().Year(),
			"ShareToken":       token,
			"ExpiresAtRFC3339": sh.ExpiresAt.Format(time.RFC3339),
			"Filename":         f.OriginalFilename,
			"FormattedSize":    formatBytes(f.SizeBytes),
			"ContentType":      f.ContentType,
			"Inactive":         false,
		})
	})

	shareDownloadHandler := func(w http.ResponseWriter, r *http.Request) {
		if !a.cfg.GlobalShareEnabled {
			http.NotFound(w, r)
			return
		}
		if !a.checkRateLimit(w, r, a.downloadLimiter) {
			return
		}

		token := r.PathValue("token")
		_, f, err := a.shares.GetByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, share.ErrShareNotFound) || errors.Is(err, share.ErrInvalidToken) {
				http.NotFound(w, r)
				return
			}
			if errors.Is(err, share.ErrShareExpired) || errors.Is(err, share.ErrShareRevoked) || errors.Is(err, share.ErrRoomInactive) {
				writeJSONError(w, "share has expired or is inactive", http.StatusGone)
				return
			}
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		storageFile, err := a.files.OpenStorageFile(f.StorageID)
		if err != nil {
			if errors.Is(err, file.ErrFileNotFound) || errors.Is(err, file.ErrInvalidFilename) {
				a.logger.Error("share storage object missing on disk", "storage_id", f.StorageID, "error", err)
				writeJSONError(w, "file not found", http.StatusNotFound)
				return
			}
			a.logger.Error("failed to open share storage file", "error", err)
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}
		defer storageFile.Close()

		w.Header().Set("Content-Type", f.ContentType)
		w.Header().Set("Content-Disposition", file.SanitizeContentDisposition(f.OriginalFilename))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-transform, max-age=0, must-revalidate")

		http.ServeContent(w, r, f.OriginalFilename, f.CreatedAt, storageFile)
	}

	mux.HandleFunc("GET /s/{token}/download", shareDownloadHandler)
	mux.HandleFunc("HEAD /s/{token}/download", shareDownloadHandler)

	return requestLogging(mux, a.logger), nil
}

func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 Bytes"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d Bytes", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": message,
	})
}
