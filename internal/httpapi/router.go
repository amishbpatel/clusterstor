package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/amishbpatel/clusterstor/internal/auth"
	"github.com/amishbpatel/clusterstor/internal/devices"
	"github.com/amishbpatel/clusterstor/internal/events"
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
	eventService := events.NewService(pool)
	eventBroker := events.NewBroker(pool)
	eventBroker.Start(context.Background())

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
	mux.Handle("POST /api/v1/providers/google_drive/changes", requireUser(authService, http.HandlerFunc(handleGoogleChanges(providerService))))
	mux.Handle("GET /api/v1/files", requireUser(authService, http.HandlerFunc(handleManagedFiles(providerService))))
	mux.Handle("POST /api/v1/providers/google_drive/root", requireUser(authService, http.HandlerFunc(handleEnsureGoogleRoot(providerService))))
	mux.Handle("POST /api/v1/providers/google_drive/folders", requireUser(authService, http.HandlerFunc(handleCreateGoogleFolder(providerService))))
	mux.Handle("POST /api/v1/providers/google_drive/uploads", requireUser(authService, http.HandlerFunc(handleBeginGoogleUpload(providerService))))
	mux.Handle("POST /api/v1/uploads/preflight", requireUser(authService, http.HandlerFunc(handleUploadPreflight(providerService))))
	mux.Handle("POST /api/v1/providers/google_drive/uploads/complete", requireUser(authService, http.HandlerFunc(handleFinalizeGoogleUpload(providerService))))
	mux.Handle("POST /api/v1/providers/google_drive/uploads/recover", requireUser(authService, http.HandlerFunc(handleRecoverGoogleUpload(providerService))))
	mux.Handle("GET /api/v1/nodes/{id}/download", requireUser(authService, http.HandlerFunc(handleDownloadNode(providerService, eventService))))
	mux.Handle("DELETE /api/v1/nodes/{id}", requireUser(authService, http.HandlerFunc(handleDeleteNode(providerService))))
	mux.Handle("PATCH /api/v1/nodes/{id}/name", requireUser(authService, http.HandlerFunc(handleRenameNode(providerService))))
	mux.Handle("POST /api/v1/nodes/{id}/move", requireUser(authService, http.HandlerFunc(handleMoveNode(providerService))))
	mux.Handle("GET /api/v1/trash", requireUser(authService, http.HandlerFunc(handleTrash(providerService))))
	mux.Handle("POST /api/v1/nodes/{id}/restore", requireUser(authService, http.HandlerFunc(handleRestoreNode(providerService))))
	mux.Handle("GET /api/v1/events", requireUser(authService, http.HandlerFunc(handleAccountEvents(eventService))))
	mux.Handle("GET /api/v1/dashboard/recent", requireUser(authService, http.HandlerFunc(handleRecentDashboard(eventService))))
	mux.Handle("GET /api/v1/dashboard/files", requireUser(authService, http.HandlerFunc(handleDashboardFileStats(providerService))))
	mux.Handle("POST /api/v1/downloads/record", requireUser(authService, http.HandlerFunc(handleRecordDownload(eventService))))
	mux.Handle("POST /api/v1/events/socket-ticket", requireUser(authService, http.HandlerFunc(handleEventSocketTicket(eventService))))
	mux.HandleFunc("GET /api/v1/events/socket", handleEventSocket(eventService, eventBroker))
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
			if target := service.PublicBaseURL(); target != "" {
				http.Redirect(w, r, target+"/dashboard?connected=google_drive", http.StatusFound)
				return
			}
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


func handleGoogleChanges(service *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		result, err := service.SyncGoogleChanges(r.Context(), user.ID, 4)
		switch {
		case errors.Is(err, providers.ErrProviderAccountNotFound):
			writeError(w,http.StatusNotFound,"provider_not_connected","google drive is not connected")
		case errors.Is(err, providers.ErrProviderNotConfigured):
			writeError(w,http.StatusServiceUnavailable,"provider_not_configured","google drive is not configured")
		case errors.Is(err, providers.ErrOAuthExchange):
			writeError(w,http.StatusBadGateway,"oauth_refresh_failed","google drive credentials could not be refreshed")
		case err != nil:
			writeError(w,http.StatusBadGateway,"provider_error","unable to sync google drive changes")
		default:
			writeJSON(w,http.StatusOK,result)
		}
	}
}


func handleManagedFiles(service *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pageSize := 0
		if raw := strings.TrimSpace(r.URL.Query().Get("page_size")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 {
				writeError(w, http.StatusBadRequest, "invalid_page_size", "page_size must be a positive integer")
				return
			}
			pageSize = value
		}
		var parentNodeID *string
		if raw := strings.TrimSpace(r.URL.Query().Get("parent_node_id")); raw != "" {
			parentNodeID = &raw
		}
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		page, err := service.ListManagedFiles(r.Context(), user.ID, parentNodeID, strings.TrimSpace(r.URL.Query().Get("cursor")), pageSize)
		switch {
		case errors.Is(err, providers.ErrInvalidProviderParent):
			writeError(w, http.StatusBadRequest, "invalid_parent", "folder must be inside a ClusterStor managed provider root")
		case errors.Is(err, providers.ErrProviderAccountNotFound):
			writeError(w, http.StatusNotFound, "provider_not_connected", "storage provider is not connected")
		case errors.Is(err, providers.ErrProviderNotConfigured):
			writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "storage provider is not configured")
		case errors.Is(err, providers.ErrOAuthExchange):
			writeError(w, http.StatusBadGateway, "oauth_refresh_failed", "storage provider credentials could not be refreshed")
		case err != nil:
			writeError(w, http.StatusBadGateway, "provider_error", "unable to list managed files")
		default:
			writeJSON(w, http.StatusOK, page)
		}
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

func handleUploadPreflight(service *providers.Service) http.HandlerFunc {
	type request struct {
		Provider string `json:"provider"`
		SizeBytes int64 `json:"size_bytes"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if !decodeJSON(w,r,&body) { return }
		if body.SizeBytes < 0 {
			writeError(w,http.StatusBadRequest,"invalid_size","size_bytes must not be negative")
			return
		}
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		result, err := service.CheckUploadCapacity(r.Context(),user.ID,body.Provider,body.SizeBytes)
		switch {
		case errors.Is(err, providers.ErrInsufficientProviderSpace):
			writeJSON(w,http.StatusOK,result)
		case errors.Is(err, providers.ErrUnsupportedProvider):
			writeError(w,http.StatusBadRequest,"unsupported_provider","storage provider is not supported")
		case errors.Is(err, providers.ErrProviderAccountNotFound):
			writeError(w,http.StatusNotFound,"provider_not_connected","storage provider is not connected")
		case errors.Is(err, providers.ErrProviderNotConfigured):
			writeError(w,http.StatusServiceUnavailable,"provider_not_configured","storage provider is not configured")
		case errors.Is(err, providers.ErrOAuthExchange):
			writeError(w,http.StatusBadGateway,"oauth_refresh_failed","storage provider credentials could not be refreshed")
		case err != nil:
			writeError(w,http.StatusBadGateway,"provider_error","unable to check provider capacity")
		default:
			writeJSON(w,http.StatusOK,result)
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
		case errors.Is(err, providers.ErrInsufficientProviderSpace):
			writeError(w, http.StatusConflict, "insufficient_storage", "not enough free space in Google Drive for this upload")
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


func handleRecoverGoogleUpload(service *providers.Service) http.HandlerFunc {
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
		result, err := service.RecoverGoogleUpload(r.Context(), user.ID, providers.UploadSessionInput{
			Name: body.Name, ContentType: body.ContentType, SizeBytes: body.SizeBytes, ParentNodeID: body.ParentNodeID,
		})
		switch {
		case errors.Is(err, providers.ErrDownloadNotFound):
			writeError(w, http.StatusNotFound, "upload_not_found", "uploaded file could not be verified in Google Drive")
		case errors.Is(err, providers.ErrInvalidProviderParent):
			writeError(w, http.StatusBadRequest, "invalid_parent", "parent folder must be inside the ClusterStor provider root")
		case errors.Is(err, providers.ErrProviderAccountNotFound):
			writeError(w, http.StatusNotFound, "provider_not_connected", "google drive is not connected")
		case err != nil:
			writeError(w, http.StatusBadGateway, "provider_error", "unable to recover google drive upload")
		default:
			writeJSON(w, http.StatusOK, result)
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
 

func handleRenameNode(service *providers.Service) http.HandlerFunc {
	type request struct { Name string `json:"name"` }
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if !decodeJSON(w, r, &body) { return }
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		item, err := service.RenameGoogleNode(r.Context(), user.ID, r.PathValue("id"), body.Name)
		switch {
		case errors.Is(err, providers.ErrDeleteNotFound):
			writeError(w, http.StatusNotFound, "node_not_found", "file or folder was not found")
		case err != nil:
			writeError(w, http.StatusBadGateway, "provider_error", "unable to rename google drive item")
		default:
			writeJSON(w, http.StatusOK, item)
		}
	}
}

func handleMoveNode(service *providers.Service) http.HandlerFunc {
	type request struct { ParentNodeID *string `json:"parent_node_id"` }
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if !decodeJSON(w, r, &body) { return }
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		item, err := service.MoveGoogleNode(r.Context(), user.ID, r.PathValue("id"), body.ParentNodeID)
		switch {
		case errors.Is(err, providers.ErrDeleteNotFound):
			writeError(w, http.StatusNotFound, "node_not_found", "file or folder was not found")
		case errors.Is(err, providers.ErrInvalidProviderParent):
			writeError(w, http.StatusBadRequest, "invalid_parent", "destination folder is not valid")
		case err != nil:
			writeError(w, http.StatusBadGateway, "provider_error", "unable to move google drive item")
		default:
			writeJSON(w, http.StatusOK, item)
		}
	}
}

func handleTrash(service *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		items, err := service.ListGoogleTrash(r.Context(), user.ID)
		if err != nil {
			writeError(w, http.StatusBadGateway, "provider_error", "unable to load trash")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func handleRestoreNode(service *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		restored, err := service.RestoreGoogleNode(r.Context(), user.ID, r.PathValue("id"))
		switch {
		case errors.Is(err, providers.ErrDeleteNotFound):
			writeError(w, http.StatusNotFound, "restore_not_found", "file or folder was not found in trash")
		case err != nil:
			writeError(w, http.StatusBadGateway, "provider_error", "unable to restore google drive item")
		default:
			writeJSON(w, http.StatusOK, map[string]any{"restored": restored})
		}
	}
}


func handleDeleteNode(service *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		deleted, err := service.DeleteGoogleNode(r.Context(), user.ID, r.PathValue("id"))
		switch {
		case errors.Is(err, providers.ErrDeleteNotFound):
			writeError(w, http.StatusNotFound, "delete_not_found", "file or folder was not found")
		case errors.Is(err, providers.ErrInvalidProviderParent):
			writeError(w, http.StatusBadRequest, "invalid_delete_target", "the ClusterStor provider root cannot be deleted")
		case errors.Is(err, providers.ErrProviderAccountNotFound):
			writeError(w, http.StatusNotFound, "provider_not_connected", "google drive is not connected")
		case errors.Is(err, providers.ErrProviderNotConfigured):
			writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "google drive is not configured")
		case errors.Is(err, providers.ErrOAuthExchange):
			writeError(w, http.StatusBadGateway, "oauth_refresh_failed", "google drive credentials could not be refreshed")
		case err != nil:
			writeError(w, http.StatusBadGateway, "provider_error", "unable to delete google drive item")
		default:
			writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
		}
	}
}


func handleDownloadNode(service *providers.Service, eventService *events.Service) http.HandlerFunc {
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
			written, copyErr := io.Copy(w, stream.Response.Body)
			if copyErr == nil && written > 0 && strings.TrimSpace(r.Header.Get("Range")) == "" && strings.TrimSpace(r.Header.Get("X-ClusterStor-Suppress-Download-Event")) == "" {
				_ = eventService.RecordDownload(r.Context(), user.ID, r.PathValue("id"), "google_drive", stream.Name, stream.SizeBytes, "Web browser")
			}
		}
	}
}


func handleRecordDownload(service *events.Service) http.HandlerFunc {
	type request struct {
		Name string `json:"name"`
		Provider string `json:"provider"`
		SizeBytes int64 `json:"size_bytes"`
		Destination string `json:"destination"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if !decodeJSON(w, r, &body) { return }
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		name := strings.TrimSpace(body.Name)
		provider := strings.TrimSpace(body.Provider)
		destination := strings.TrimSpace(body.Destination)
		if name == "" || provider == "" {
			writeError(w, http.StatusBadRequest, "invalid_download_activity", "name and provider are required")
			return
		}
		if destination == "" { destination = "Web browser" }
		if err := service.RecordArchiveDownload(r.Context(), user.ID, provider, name, body.SizeBytes, destination); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to record download activity")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}


func handleDashboardFileStats(service *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		result, err := service.DashboardFileStats(r.Context(), user.ID, 8)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to load dashboard file statistics")
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}


func handleRecentDashboard(service *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		result, err := service.RecentDashboard(r.Context(), user.ID, 5)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to load recent file activity")
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}


func handleAccountEvents(service *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		after := int64(0)
		if raw := strings.TrimSpace(r.URL.Query().Get("after")); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < 0 {
				writeError(w, http.StatusBadRequest, "invalid_cursor", "after must be a non-negative integer sequence")
				return
			}
			after = value
		}
		limit := 0
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 {
				writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer")
				return
			}
			limit = value
		}
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		page, err := service.List(r.Context(), user.ID, after, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to list account events")
			return
		}
		writeJSON(w, http.StatusOK, page)
	}
}


func handleEventSocketTicket(service *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(userContextKey{}).(auth.User)
		ticket, expiresAt, err := service.CreateSocketTicket(r.Context(), user.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to create socket ticket")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"ticket": ticket, "expires_at": expiresAt})
	}
}

func handleEventSocket(service *events.Service, broker *events.Broker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := service.ConsumeSocketTicket(r.Context(), strings.TrimSpace(r.URL.Query().Get("ticket")))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_socket_ticket", "socket ticket is invalid or expired")
			return
		}
		conn, err := upgradeWebSocket(w, r)
		if err != nil {
			return
		}
		defer conn.Close()

		notifications, unsubscribe := broker.Subscribe(userID)
		defer unsubscribe()

		latest, err := service.LatestSequence(r.Context(), userID)
		if err != nil { return }
		ready, _ := json.Marshal(map[string]any{"type":"ready","latest_sequence":latest})
		if err := conn.WriteText(ready); err != nil { return }

		ping := time.NewTicker(30 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case sequence, ok := <-notifications:
				if !ok { return }
				message, _ := json.Marshal(map[string]any{"type":"events_available","sequence":sequence})
				if err := conn.WriteText(message); err != nil { return }
			case <-ping.C:
				if err := conn.WritePing(); err != nil { return }
			}
		}
	}
}
