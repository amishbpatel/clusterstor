package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/amishbpatel/clusterstor/internal/auth"
	"github.com/amishbpatel/clusterstor/internal/devices"
	"github.com/amishbpatel/clusterstor/internal/providers"
	"github.com/jackc/pgx/v5/pgxpool"
)

type healthResponse struct {
	Status string `json:"status"`
	Time time.Time `json:"time"`
}

type errorResponse struct {
	Code string `json:"code"`
	Message string `json:"message"`
}

type userContextKey struct{}
type tokenContextKey struct{}

func NewRouter(pool *pgxpool.Pool, providerService *providers.Service) http.Handler {
	authService := auth.NewService(pool)
	deviceService := devices.NewService(pool)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Time: time.Now().UTC()})
	})
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"service": "clusterstor-api", "api_version": "v1"})
	})

	mux.HandleFunc("POST /api/v1/auth/signup", handleSignup(authService))
	mux.HandleFunc("POST /api/v1/auth/login", handleLogin(authService))
	mux.Handle("POST /api/v1/auth/logout", requireUser(authService, http.HandlerFunc(handleLogout(authService))))
	mux.Handle("GET /api/v1/me", requireUser(authService, http.HandlerFunc(handleMe)))
	mux.Handle("POST /api/v1/devices", requireUser(authService, http.HandlerFunc(handleRegisterDevice(deviceService))))
	mux.Handle("GET /api/v1/devices", requireUser(authService, http.HandlerFunc(handleListDevices(deviceService))))
	mux.Handle("DELETE /api/v1/devices/{id}", requireUser(authService, http.HandlerFunc(handleRevokeDevice(deviceService))))
	mux.Handle("POST /api/v1/providers/google_drive/oauth/start", requireUser(authService, http.HandlerFunc(handleGoogleOAuthStart(providerService))))
	mux.HandleFunc("GET /api/v1/providers/google_drive/callback", handleGoogleOAuthCallback(providerService))
	mux.Handle("GET /api/v1/providers", requireUser(authService, http.HandlerFunc(handleListProviders(providerService))))
	mux.Handle("GET /api/v1/providers/google_drive/files", requireUser(authService, http.HandlerFunc(handleGoogleDriveFiles(providerService))))
	mux.Handle("POST /api/v1/providers/google_drive/root", requireUser(authService, http.HandlerFunc(handleEnsureGoogleRoot(providerService))))
	mux.Handle("POST /api/v1/providers/google_drive/folders", requireUser(authService, http.HandlerFunc(handleCreateGoogleFolder(providerService))))
	mux.Handle("POST /api/v1/providers/google_drive/uploads", requireUser(authService, http.HandlerFunc(handleBeginGoogleUpload(providerService))))
	mux.Handle("POST /api/v1/providers/google_drive/uploads/complete", requireUser(authService, http.HandlerFunc(handleFinalizeGoogleUpload(providerService))))
	mux.Handle("GET /api/v1/nodes/{id}/download", requireUser(authService, http.HandlerFunc(handleDownloadNode(providerService))))
	return mux
}

func handleSignup(service *auth.Service) http.HandlerFunc {
	type request struct {
		Email string `json:"email"`
		Password string `json:"password"`
		DisplayName *string `json:"display_name"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if !decodeJSON(w, r, &body) { return }
		session, err := service.Signup(r.Context(), body.Email, body.Password, body.DisplayName)
		switch {
		case errors.Is(err, auth.ErrInvalidEmail):
			writeError(w, http.StatusBadRequest, "invalid_email", err.Error())
		case errors.Is(err, auth.ErrWeakPassword):
			writeError(w, http.StatusBadRequest, "weak_password", err.Error())
		case errors.Is(err, auth.ErrEmailInUse):
			writeError(w, http.StatusConflict, "email_in_use", err.Error())
		case err != nil:
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to create account")
		default:
			writeJSON(w, http.StatusCreated, session)
		}
	}
}

func handleLogin(service *auth.Service) http.HandlerFunc {
	type request struct {
		Email string `json:"email"`
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if !decodeJSON(w, r, &body) { return }
		session, err := service.Login(r.Context(), body.Email, body.Password)
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to sign in")
			return
		}
		writeJSON(w, http.StatusOK, session)
	}
}

func handleLogout(service *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, _ := r.Context().Value(tokenContextKey{}).(string)
		if err := service.RevokeSession(r.Context(), token); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_session", "session is not valid")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := r.Context().Value(userContextKey{}).(auth.User)
	writeJSON(w, http.StatusOK, user)
}

func handleRegisterDevice(service *devices.Service) http.HandlerFunc {
	type request struct {
		Name string `json:"name"`
		Platform string `json:"platform"`
		AgentVersion *string `json:"agent_version"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if !decodeJSON(w, r, &body) { return }
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		registration, err := service.Register(r.Context(), user.ID, devices.RegisterInput{Name: body.Name, Platform: body.Platform, AgentVersion: body.AgentVersion})
		if errors.Is(err, devices.ErrInvalidDevice) {
			writeError(w, http.StatusBadRequest, "invalid_device", "name and platform are required")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to register device")
			return
		}
		writeJSON(w, http.StatusCreated, registration)
	}
}

func handleListDevices(service *devices.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		result, err := service.List(r.Context(), user.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to list devices")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"devices": result})
	}
}

func handleRevokeDevice(service *devices.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		if err := service.Revoke(r.Context(), user.ID, r.PathValue("id")); err != nil {
			if errors.Is(err, devices.ErrNotFound) {
				writeError(w, http.StatusNotFound, "device_not_found", "device not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to revoke device")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func requireUser(service *auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing_session", "bearer session token is required")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		user, err := service.Authenticate(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_session", "session is not valid")
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey{}, user)
		ctx = context.WithValue(ctx, tokenContextKey{}, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Code: code, Message: message})
}


func handleGoogleOAuthStart(service *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		result, err := service.StartGoogleOAuth(r.Context(), user.ID)
		if errors.Is(err, providers.ErrProviderNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "google drive is not configured")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to start google drive connection")
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func handleGoogleOAuthCallback(service *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if oauthErr := strings.TrimSpace(r.URL.Query().Get("error")); oauthErr != "" {
			writeError(w, http.StatusBadRequest, "oauth_denied", "google drive authorization was not completed")
			return
		}
		account, err := service.CompleteGoogleOAuth(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"))
		switch {
		case errors.Is(err, providers.ErrInvalidOAuthState):
			writeError(w, http.StatusBadRequest, "invalid_oauth_state", "oauth state is invalid or expired")
		case errors.Is(err, providers.ErrProviderNotConfigured):
			writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "google drive is not configured")
		case errors.Is(err, providers.ErrOAuthExchange):
			writeError(w, http.StatusBadGateway, "oauth_exchange_failed", "google drive token exchange failed")
		case err != nil:
			writeError(w, http.StatusBadGateway, "provider_error", "unable to complete google drive connection")
		default:
			writeJSON(w, http.StatusOK, account)
		}
	}
}

func handleListProviders(service *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		accounts, err := service.ListAccounts(r.Context(), user.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to list provider accounts")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"providers": accounts})
	}
}


func handleGoogleDriveFiles(service *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		pageSize := 0
		if raw := strings.TrimSpace(r.URL.Query().Get("page_size")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 {
				writeError(w, http.StatusBadRequest, "invalid_page_size", "page_size must be a positive integer")
				return
			}
			pageSize = value
		}

		page, err := service.SyncGoogleDrive(r.Context(), user.ID, r.URL.Query().Get("cursor"), pageSize)
		switch {
		case errors.Is(err, providers.ErrProviderAccountNotFound):
			writeError(w, http.StatusNotFound, "provider_not_connected", "google drive is not connected")
		case errors.Is(err, providers.ErrProviderNotConfigured):
			writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "google drive is not configured")
		case errors.Is(err, providers.ErrOAuthExchange):
			writeError(w, http.StatusBadGateway, "oauth_refresh_failed", "google drive credentials could not be refreshed")
		case err != nil:
			writeError(w, http.StatusBadGateway, "provider_error", "unable to list google drive files")
		default:
			writeJSON(w, http.StatusOK, page)
		}
	}
}


func handleEnsureGoogleRoot(service *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		rootID, err := service.EnsureGoogleRoot(r.Context(), user.ID)
		switch {
		case errors.Is(err, providers.ErrProviderAccountNotFound):
			writeError(w, http.StatusNotFound, "provider_not_connected", "google drive is not connected")
		case errors.Is(err, providers.ErrProviderNotConfigured):
			writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "google drive is not configured")
		case errors.Is(err, providers.ErrOAuthExchange):
			writeError(w, http.StatusBadGateway, "oauth_refresh_failed", "google drive credentials could not be refreshed")
		case err != nil:
			writeError(w, http.StatusBadGateway, "provider_error", "unable to ensure ClusterStor root folder")
		default:
			writeJSON(w, http.StatusOK, map[string]string{"root_provider_item_id": rootID, "name": "ClusterStor"})
		}
	}
}


func handleCreateGoogleFolder(service *providers.Service) http.HandlerFunc {
	type request struct {
		Name string `json:"name"`
		ParentNodeID *string `json:"parent_node_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if !decodeJSON(w, r, &body) { return }
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		item, err := service.CreateGoogleFolder(r.Context(), user.ID, providers.CreateFolderInput{Name: body.Name, ParentNodeID: body.ParentNodeID})
		switch {
		case errors.Is(err, providers.ErrInvalidProviderParent):
			writeError(w, http.StatusBadRequest, "invalid_parent", "parent folder must be inside the ClusterStor provider root")
		case errors.Is(err, providers.ErrProviderAccountNotFound):
			writeError(w, http.StatusNotFound, "provider_not_connected", "google drive is not connected")
		case errors.Is(err, providers.ErrProviderNotConfigured):
			writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "google drive is not configured")
		case errors.Is(err, providers.ErrOAuthExchange):
			writeError(w, http.StatusBadGateway, "oauth_refresh_failed", "google drive credentials could not be refreshed")
		case err != nil:
			writeError(w, http.StatusBadGateway, "provider_error", "unable to create google drive folder")
		default:
			writeJSON(w, http.StatusCreated, item)
		}
	}
}

func handleBeginGoogleUpload(service *providers.Service) http.HandlerFunc {
	type request struct {
		Name string `json:"name"`
		ContentType string `json:"content_type"`
		SizeBytes int64 `json:"size_bytes"`
		ParentNodeID *string `json:"parent_node_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if !decodeJSON(w, r, &body) { return }
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		session, err := service.BeginGoogleUpload(r.Context(), user.ID, providers.UploadSessionInput{
			Name: body.Name, ContentType: body.ContentType, SizeBytes: body.SizeBytes, ParentNodeID: body.ParentNodeID,
		})
		switch {
		case errors.Is(err, providers.ErrInvalidProviderParent):
			writeError(w, http.StatusBadRequest, "invalid_parent", "parent folder must be inside the ClusterStor provider root")
		case errors.Is(err, providers.ErrProviderAccountNotFound):
			writeError(w, http.StatusNotFound, "provider_not_connected", "google drive is not connected")
		case errors.Is(err, providers.ErrProviderNotConfigured):
			writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "google drive is not configured")
		case errors.Is(err, providers.ErrOAuthExchange):
			writeError(w, http.StatusBadGateway, "oauth_refresh_failed", "google drive credentials could not be refreshed")
		case err != nil:
			writeError(w, http.StatusBadGateway, "provider_error", "unable to start google drive upload")
		default:
			writeJSON(w, http.StatusCreated, session)
		}
	}
}


func handleFinalizeGoogleUpload(service *providers.Service) http.HandlerFunc {
	type request struct {
		ProviderItemID string `json:"provider_item_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if !decodeJSON(w, r, &body) { return }
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		result, err := service.FinalizeGoogleUpload(r.Context(), user.ID, providers.FinalizeUploadInput{ProviderItemID: body.ProviderItemID})
		switch {
		case errors.Is(err, providers.ErrUploadedFileOutsideRoot):
			writeError(w, http.StatusBadRequest, "outside_clusterstor_root", "uploaded file must be inside the ClusterStor provider root")
		case errors.Is(err, providers.ErrProviderAccountNotFound):
			writeError(w, http.StatusNotFound, "provider_not_connected", "google drive is not connected")
		case errors.Is(err, providers.ErrProviderNotConfigured):
			writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "google drive is not configured")
		case errors.Is(err, providers.ErrOAuthExchange):
			writeError(w, http.StatusBadGateway, "oauth_refresh_failed", "google drive credentials could not be refreshed")
		case err != nil:
			writeError(w, http.StatusBadGateway, "provider_error", "unable to finalize google drive upload")
		default:
			writeJSON(w, http.StatusCreated, result)
		}
	}
}


func handleDownloadNode(service *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		stream, err := service.OpenGoogleDownload(r.Context(), user.ID, r.PathValue("id"), r.Header.Get("Range"))
		switch {
		case errors.Is(err, providers.ErrDownloadNotFound):
			writeError(w, http.StatusNotFound, "download_not_found", "file is not available for download")
		case errors.Is(err, providers.ErrProviderNotConfigured):
			writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "google drive is not configured")
		case errors.Is(err, providers.ErrOAuthExchange):
			writeError(w, http.StatusBadGateway, "oauth_refresh_failed", "google drive credentials could not be refreshed")
		case err != nil:
			writeError(w, http.StatusBadGateway, "provider_error", "unable to open google drive download")
		default:
			defer stream.Response.Body.Close()
			for _, header := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
				if value := stream.Response.Header.Get(header); value != "" { w.Header().Set(header, value) }
			}
			w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(stream.Name))
			w.Header().Set("Cache-Control", "private, no-store")
			w.WriteHeader(stream.Response.StatusCode)
			_, _ = io.Copy(w, stream.Response.Body)
		}
	}
}
