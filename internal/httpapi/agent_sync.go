package httpapi

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/amishbpatel/clusterstor/internal/devices"
	"github.com/amishbpatel/clusterstor/internal/providers"
)

func authenticateAgentRequest(w http.ResponseWriter,r *http.Request,service *devices.Service) (devices.AuthenticatedDevice,bool) {
	header:=strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header,"Device ") {
		writeError(w,http.StatusUnauthorized,"missing_device_credential","device credential is required")
		return devices.AuthenticatedDevice{},false
	}
	credential:=strings.TrimSpace(strings.TrimPrefix(header,"Device "))
	parts:=strings.SplitN(credential,".",2)
	if len(parts)!=2 || strings.TrimSpace(parts[0])=="" || strings.TrimSpace(parts[1])=="" {
		writeError(w,http.StatusUnauthorized,"invalid_device_credential","device credential is invalid")
		return devices.AuthenticatedDevice{},false
	}
	principal,err:=service.Authenticate(r.Context(),parts[0],parts[1])
	if errors.Is(err,devices.ErrNotFound) {
		writeError(w,http.StatusUnauthorized,"invalid_device_credential","device credential is invalid or revoked")
		return devices.AuthenticatedDevice{},false
	}
	if err!=nil {
		writeError(w,http.StatusInternalServerError,"internal_error","unable to authenticate device")
		return devices.AuthenticatedDevice{},false
	}
	return principal,true
}

func handleAgentGoogleSnapshot(deviceService *devices.Service,providerService *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter,r *http.Request) {
		principal,ok:=authenticateAgentRequest(w,r,deviceService)
		if !ok { return }
		snapshot,err:=providerService.GoogleDesktopSnapshot(r.Context(),principal.UserID)
		switch {
		case errors.Is(err,providers.ErrProviderAccountNotFound):
			writeError(w,http.StatusConflict,"provider_not_connected","google drive is not connected")
		case errors.Is(err,providers.ErrProviderNotConfigured):
			writeError(w,http.StatusServiceUnavailable,"provider_not_configured","google drive is not configured")
		case errors.Is(err,providers.ErrOAuthExchange):
			writeError(w,http.StatusBadGateway,"oauth_refresh_failed","google drive credentials could not be refreshed")
		case err!=nil:
			log.Printf("agent google desktop snapshot failed for device %s: %v",principal.Device.ID,err)
			writeError(w,http.StatusBadGateway,"provider_error","google desktop snapshot failed: "+err.Error())
		default:
			writeJSON(w,http.StatusOK,snapshot)
		}
	}
}

func handleAgentCreateGoogleFolder(deviceService *devices.Service,providerService *providers.Service) http.HandlerFunc {
	type request struct {
		Name string `json:"name"`
		ParentNodeID *string `json:"parent_node_id"`
	}
	return func(w http.ResponseWriter,r *http.Request) {
		principal,ok:=authenticateAgentRequest(w,r,deviceService)
		if !ok { return }
		var body request
		if !decodeJSON(w,r,&body) { return }
		item,err:=providerService.CreateGoogleFolder(r.Context(),principal.UserID,providers.CreateFolderInput{
			Name:body.Name,ParentNodeID:cleanAgentNodeID(body.ParentNodeID),
		})
		if err!=nil {
			writeError(w,http.StatusBadGateway,"provider_error","unable to create google drive folder")
			return
		}
		writeJSON(w,http.StatusCreated,item)
	}
}

func handleAgentBeginGoogleUpload(deviceService *devices.Service,providerService *providers.Service) http.HandlerFunc {
	type request struct {
		NodeID string `json:"node_id,omitempty"`
		Name string `json:"name"`
		ContentType string `json:"content_type"`
		SizeBytes int64 `json:"size_bytes"`
		ParentNodeID *string `json:"parent_node_id"`
	}
	return func(w http.ResponseWriter,r *http.Request) {
		principal,ok:=authenticateAgentRequest(w,r,deviceService)
		if !ok { return }
		var body request
		if !decodeJSON(w,r,&body) { return }
		if strings.TrimSpace(body.NodeID)!="" {
			session,err:=providerService.BeginGoogleVersionUpload(
				r.Context(),principal.UserID,strings.TrimSpace(body.NodeID),body.ContentType,body.SizeBytes,
			)
			if err!=nil {
				writeError(w,http.StatusBadGateway,"provider_error","unable to start google drive update")
				return
			}
			writeJSON(w,http.StatusCreated,session)
			return
		}
		session,err:=providerService.BeginGoogleUpload(r.Context(),principal.UserID,providers.UploadSessionInput{
			Name:body.Name,ContentType:body.ContentType,SizeBytes:body.SizeBytes,ParentNodeID:cleanAgentNodeID(body.ParentNodeID),
		})
		if err!=nil {
			writeError(w,http.StatusBadGateway,"provider_error","unable to start google drive upload")
			return
		}
		writeJSON(w,http.StatusCreated,session)
	}
}

func handleAgentFinalizeGoogleUpload(deviceService *devices.Service,providerService *providers.Service) http.HandlerFunc {
	type request struct { ProviderItemID string `json:"provider_item_id"` }
	return func(w http.ResponseWriter,r *http.Request) {
		principal,ok:=authenticateAgentRequest(w,r,deviceService)
		if !ok { return }
		var body request
		if !decodeJSON(w,r,&body) { return }
		result,err:=providerService.FinalizeGoogleUpload(r.Context(),principal.UserID,providers.FinalizeUploadInput{ProviderItemID:body.ProviderItemID})
		if err!=nil {
			writeError(w,http.StatusBadGateway,"provider_error","unable to finalize google drive upload")
			return
		}
		writeJSON(w,http.StatusOK,result)
	}
}

func handleAgentGoogleDownload(deviceService *devices.Service,providerService *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter,r *http.Request) {
		principal,ok:=authenticateAgentRequest(w,r,deviceService)
		if !ok { return }
		stream,err:=providerService.OpenGoogleDownload(r.Context(),principal.UserID,r.PathValue("id"),r.Header.Get("Range"))
		switch {
		case errors.Is(err,providers.ErrDownloadNotFound):
			writeError(w,http.StatusNotFound,"download_not_found","file is not available for download")
		case err!=nil:
			writeError(w,http.StatusBadGateway,"provider_error","unable to open google drive download")
		default:
			defer stream.Response.Body.Close()
			for _,header:=range []string{"Content-Type","Content-Length","Content-Range","Accept-Ranges","ETag","Last-Modified"} {
				if value:=stream.Response.Header.Get(header); value!="" { w.Header().Set(header,value) }
			}
			w.Header().Set("Content-Disposition","attachment; filename*=UTF-8''"+url.PathEscape(stream.Name))
			w.Header().Set("Cache-Control","private, no-store")
			w.WriteHeader(stream.Response.StatusCode)
			_,_=io.Copy(w,stream.Response.Body)
		}
	}
}

func handleAgentMutateGoogleNode(deviceService *devices.Service,providerService *providers.Service) http.HandlerFunc {
	type request struct {
		Name string `json:"name"`
		ParentNodeID *string `json:"parent_node_id"`
	}
	return func(w http.ResponseWriter,r *http.Request) {
		principal,ok:=authenticateAgentRequest(w,r,deviceService)
		if !ok { return }
		var body request
		if !decodeJSON(w,r,&body) { return }
		nodeID:=strings.TrimSpace(r.PathValue("id"))
		if strings.TrimSpace(body.Name)!="" {
			if _,err:=providerService.RenameGoogleNode(r.Context(),principal.UserID,nodeID,body.Name); err!=nil {
				writeError(w,http.StatusBadGateway,"provider_error","unable to rename google drive item")
				return
			}
		}
		item,err:=providerService.MoveGoogleNode(r.Context(),principal.UserID,nodeID,cleanAgentNodeID(body.ParentNodeID))
		if err!=nil {
			writeError(w,http.StatusBadGateway,"provider_error","unable to move google drive item")
			return
		}
		writeJSON(w,http.StatusOK,item)
	}
}

func handleAgentDeleteGoogleNode(deviceService *devices.Service,providerService *providers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter,r *http.Request) {
		principal,ok:=authenticateAgentRequest(w,r,deviceService)
		if !ok { return }
		count,err:=providerService.DeleteGoogleNode(r.Context(),principal.UserID,r.PathValue("id"))
		if errors.Is(err,providers.ErrDeleteNotFound) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err!=nil {
			writeError(w,http.StatusBadGateway,"provider_error","unable to trash google drive item")
			return
		}
		writeJSON(w,http.StatusOK,map[string]int{"deleted_count":count})
	}
}

func cleanAgentNodeID(value *string) *string {
	if value==nil { return nil }
	clean:=strings.TrimSpace(*value)
	if clean=="" { return nil }
	return &clean
}
