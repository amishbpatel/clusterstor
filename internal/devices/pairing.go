package devices

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const pairingTTL = 10 * time.Minute

var (
	ErrPairingNotFound = errors.New("device pairing not found")
	ErrPairingExpired = errors.New("device pairing expired")
	ErrPairingPending = errors.New("device pairing pending approval")
	ErrPairingConsumed = errors.New("device pairing already consumed")
)

type PairingStart struct {
	UserCode string `json:"user_code"`
	PollToken string `json:"poll_token"`
	VerificationURL string `json:"verification_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type PairingPreview struct {
	UserCode string `json:"user_code"`
	Name string `json:"name"`
	Platform string `json:"platform"`
	AgentVersion *string `json:"agent_version,omitempty"`
	PeerContributionEnabled bool `json:"peer_contribution_enabled"`
	PeerContributionBytes int64 `json:"peer_contribution_bytes"`
	ExpiresAt time.Time `json:"expires_at"`
	Approved bool `json:"approved"`
}

type PairingPoll struct {
	Status string `json:"status"`
	Registration *Registration `json:"registration,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Service) StartPairing(ctx context.Context, name, platform string, agentVersion *string, peerEnabled bool, peerBytes int64, verificationBaseURL string) (PairingStart,error) {
	name=strings.TrimSpace(name)
	platform=strings.TrimSpace(platform)
	if name=="" || platform=="" || peerBytes<0 || (!peerEnabled && peerBytes!=0) || (peerEnabled && peerBytes<=0) {
		return PairingStart{},ErrInvalidDevice
	}
	pollRaw:=make([]byte,32)
	if _,err:=rand.Read(pollRaw); err!=nil { return PairingStart{},fmt.Errorf("create pairing poll token: %w",err) }
	pollToken:=base64.RawURLEncoding.EncodeToString(pollRaw)
	pollHash:=sha256.Sum256([]byte(pollToken))
	expiresAt:=time.Now().UTC().Add(pairingTTL)

	var userCode string
	for attempt:=0;attempt<5;attempt++ {
		code,err:=generatePairingCode()
		if err!=nil { return PairingStart{},err }
		codeHash:=sha256.Sum256([]byte(normalizePairingCode(code)))
		_,err=s.pool.Exec(ctx,`
			INSERT INTO device_pairings(
				user_code_hash,poll_token_hash,requested_name,platform,agent_version,
				peer_contribution_enabled,peer_contribution_bytes,expires_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			codeHash[:],pollHash[:],name,platform,cleanString(agentVersion),peerEnabled,peerBytes,expiresAt)
		if err==nil {
			userCode=code
			break
		}
		if !isUniqueViolation(err) { return PairingStart{},fmt.Errorf("store device pairing: %w",err) }
	}
	if userCode=="" { return PairingStart{},errors.New("unable to allocate unique pairing code") }

	base:=strings.TrimRight(strings.TrimSpace(verificationBaseURL),"/")
	url:=base+"/devices?pair="+userCode
	return PairingStart{UserCode:userCode,PollToken:pollToken,VerificationURL:url,ExpiresAt:expiresAt},nil
}

func (s *Service) PreviewPairing(ctx context.Context, code string) (PairingPreview,error) {
	normalized:=normalizePairingCode(code)
	if normalized=="" { return PairingPreview{},ErrPairingNotFound }
	hash:=sha256.Sum256([]byte(normalized))
	var preview PairingPreview
	var approvedAt *time.Time
	err:=s.pool.QueryRow(ctx,`
		SELECT requested_name,platform,agent_version,peer_contribution_enabled,peer_contribution_bytes,expires_at,approved_at
		FROM device_pairings
		WHERE user_code_hash=$1 AND consumed_at IS NULL`,hash[:]).
		Scan(&preview.Name,&preview.Platform,&preview.AgentVersion,&preview.PeerContributionEnabled,&preview.PeerContributionBytes,&preview.ExpiresAt,&approvedAt)
	if errors.Is(err,pgx.ErrNoRows) { return PairingPreview{},ErrPairingNotFound }
	if err!=nil { return PairingPreview{},fmt.Errorf("preview device pairing: %w",err) }
	if time.Now().UTC().After(preview.ExpiresAt) { return PairingPreview{},ErrPairingExpired }
	preview.UserCode=formatPairingCode(normalized)
	preview.Approved=approvedAt!=nil
	return preview,nil
}

func (s *Service) ApprovePairing(ctx context.Context, userID, code string) (PairingPreview,error) {
	normalized:=normalizePairingCode(code)
	if normalized=="" { return PairingPreview{},ErrPairingNotFound }
	hash:=sha256.Sum256([]byte(normalized))
	var preview PairingPreview
	err:=s.pool.QueryRow(ctx,`
		UPDATE device_pairings
		SET approved_by_user_id=$1::uuid,approved_at=COALESCE(approved_at,now())
		WHERE user_code_hash=$2
		  AND expires_at>now()
		  AND consumed_at IS NULL
		  AND (approved_by_user_id IS NULL OR approved_by_user_id=$1::uuid)
		RETURNING requested_name,platform,agent_version,peer_contribution_enabled,peer_contribution_bytes,expires_at`,
		userID,hash[:]).
		Scan(&preview.Name,&preview.Platform,&preview.AgentVersion,&preview.PeerContributionEnabled,&preview.PeerContributionBytes,&preview.ExpiresAt)
	if errors.Is(err,pgx.ErrNoRows) {
		var expiresAt time.Time
		var approvedUser *string
		checkErr:=s.pool.QueryRow(ctx,`
			SELECT expires_at,approved_by_user_id::text FROM device_pairings WHERE user_code_hash=$1`,hash[:]).
			Scan(&expiresAt,&approvedUser)
		if errors.Is(checkErr,pgx.ErrNoRows) { return PairingPreview{},ErrPairingNotFound }
		if checkErr==nil && time.Now().UTC().After(expiresAt) { return PairingPreview{},ErrPairingExpired }
		return PairingPreview{},ErrPairingNotFound
	}
	if err!=nil { return PairingPreview{},fmt.Errorf("approve device pairing: %w",err) }
	preview.UserCode=formatPairingCode(normalized)
	preview.Approved=true
	return preview,nil
}

func (s *Service) PollPairing(ctx context.Context, pollToken string) (PairingPoll,error) {
	pollToken=strings.TrimSpace(pollToken)
	if pollToken=="" { return PairingPoll{},ErrPairingNotFound }
	pollHash:=sha256.Sum256([]byte(pollToken))

	tx,err:=s.pool.Begin(ctx)
	if err!=nil { return PairingPoll{},fmt.Errorf("begin pairing poll: %w",err) }
	defer tx.Rollback(ctx)

	var pairingID,name,platform string
	var agentVersion *string
	var peerEnabled bool
	var peerBytes int64
	var approvedUserID,deviceID *string
	var delivery []byte
	var expiresAt time.Time
	err=tx.QueryRow(ctx,`
		SELECT id::text,requested_name,platform,agent_version,peer_contribution_enabled,peer_contribution_bytes,
		       approved_by_user_id::text,device_id::text,delivery_ciphertext,expires_at
		FROM device_pairings
		WHERE poll_token_hash=$1
		FOR UPDATE`,pollHash[:]).
		Scan(&pairingID,&name,&platform,&agentVersion,&peerEnabled,&peerBytes,&approvedUserID,&deviceID,&delivery,&expiresAt)
	if errors.Is(err,pgx.ErrNoRows) { return PairingPoll{},ErrPairingNotFound }
	if err!=nil { return PairingPoll{},fmt.Errorf("load device pairing: %w",err) }
	if time.Now().UTC().After(expiresAt) { return PairingPoll{},ErrPairingExpired }
	if approvedUserID==nil {
		return PairingPoll{Status:"pending",ExpiresAt:expiresAt},nil
	}

	if deviceID!=nil && len(delivery)>0 {
		secret,err:=decryptPairingDelivery(pollToken,delivery)
		if err!=nil { return PairingPoll{},fmt.Errorf("decrypt pairing delivery: %w",err) }
		device,err:=loadDeviceByID(ctx,tx,*approvedUserID,*deviceID)
		if err!=nil { return PairingPoll{},err }
		if err:=tx.Commit(ctx); err!=nil { return PairingPoll{},fmt.Errorf("commit pairing replay: %w",err) }
		return PairingPoll{Status:"approved",Registration:&Registration{Device:device,Secret:secret},ExpiresAt:expiresAt},nil
	}

	raw:=make([]byte,32)
	if _,err:=rand.Read(raw); err!=nil { return PairingPoll{},fmt.Errorf("create device credential: %w",err) }
	secret:=base64.RawURLEncoding.EncodeToString(raw)
	secretHash:=sha256.Sum256([]byte(secret))

	var device Device
	err=tx.QueryRow(ctx,`
		INSERT INTO devices(user_id,name,platform,agent_version,peer_contribution_enabled,peer_contribution_bytes)
		VALUES ($1::uuid,$2,$3,$4,$5,$6)
		RETURNING id::text,name,platform,agent_version,status,peer_contribution_enabled,peer_contribution_bytes,last_seen_at,created_at,revoked_at`,
		*approvedUserID,name,platform,cleanString(agentVersion),peerEnabled,peerBytes).
		Scan(&device.ID,&device.Name,&device.Platform,&device.AgentVersion,&device.Status,&device.PeerContributionEnabled,&device.PeerContributionBytes,&device.LastSeenAt,&device.CreatedAt,&device.RevokedAt)
	if err!=nil { return PairingPoll{},fmt.Errorf("create paired device: %w",err) }
	if _,err:=tx.Exec(ctx,`INSERT INTO device_credentials(device_id,secret_hash) VALUES ($1::uuid,$2)`,device.ID,secretHash[:]); err!=nil {
		return PairingPoll{},fmt.Errorf("store paired credential: %w",err)
	}
	delivery,err=encryptPairingDelivery(pollToken,secret)
	if err!=nil { return PairingPoll{},err }
	if _,err:=tx.Exec(ctx,`
		UPDATE device_pairings SET device_id=$1::uuid,delivery_ciphertext=$2,consumed_at=now() WHERE id=$3::uuid`,
		device.ID,delivery,pairingID); err!=nil {
		return PairingPoll{},fmt.Errorf("complete device pairing: %w",err)
	}
	if err:=tx.Commit(ctx); err!=nil { return PairingPoll{},fmt.Errorf("commit device pairing: %w",err) }
	return PairingPoll{Status:"approved",Registration:&Registration{Device:device,Secret:secret},ExpiresAt:expiresAt},nil
}

func generatePairingCode() (string,error) {
	const alphabet="ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	raw:=make([]byte,8)
	if _,err:=rand.Read(raw); err!=nil { return "",fmt.Errorf("generate pairing code: %w",err) }
	out:=make([]byte,8)
	for i,b:=range raw { out[i]=alphabet[int(b)%len(alphabet)] }
	return string(out[:4])+"-"+string(out[4:]),nil
}

func normalizePairingCode(code string) string {
	code=strings.ToUpper(strings.TrimSpace(code))
	code=strings.ReplaceAll(code,"-","")
	code=strings.ReplaceAll(code," ","")
	if len(code)!=8 { return "" }
	return code
}

func formatPairingCode(normalized string) string {
	if len(normalized)!=8 { return normalized }
	return normalized[:4]+"-"+normalized[4:]
}

func encryptPairingDelivery(pollToken,secret string) ([]byte,error) {
	key:=sha256.Sum256([]byte("clusterstor-pairing-delivery:"+pollToken))
	block,err:=aes.NewCipher(key[:])
	if err!=nil { return nil,err }
	gcm,err:=cipher.NewGCM(block)
	if err!=nil { return nil,err }
	nonce:=make([]byte,gcm.NonceSize())
	if _,err:=rand.Read(nonce); err!=nil { return nil,err }
	return gcm.Seal(nonce,nonce,[]byte(secret),nil),nil
}

func decryptPairingDelivery(pollToken string,ciphertext []byte) (string,error) {
	key:=sha256.Sum256([]byte("clusterstor-pairing-delivery:"+pollToken))
	block,err:=aes.NewCipher(key[:])
	if err!=nil { return "",err }
	gcm,err:=cipher.NewGCM(block)
	if err!=nil { return "",err }
	if len(ciphertext)<gcm.NonceSize() { return "",errors.New("pairing delivery ciphertext is too short") }
	plain,err:=gcm.Open(nil,ciphertext[:gcm.NonceSize()],ciphertext[gcm.NonceSize():],nil)
	if err!=nil { return "",err }
	return string(plain),nil
}

func isUniqueViolation(err error) bool {
	type sqlState interface{ SQLState() string }
	var state sqlState
	return errors.As(err,&state) && state.SQLState()=="23505"
}

type deviceRow interface {
	Scan(...any) error
}

func loadDeviceByID(ctx context.Context,q interface{ QueryRow(context.Context,string,...any) pgx.Row },userID,deviceID string) (Device,error) {
	var device Device
	err:=q.QueryRow(ctx,`
		SELECT id::text,name,platform,agent_version,status,peer_contribution_enabled,peer_contribution_bytes,last_seen_at,created_at,revoked_at
		FROM devices WHERE id=$1::uuid AND user_id=$2::uuid`,deviceID,userID).
		Scan(&device.ID,&device.Name,&device.Platform,&device.AgentVersion,&device.Status,&device.PeerContributionEnabled,&device.PeerContributionBytes,&device.LastSeenAt,&device.CreatedAt,&device.RevokedAt)
	if err!=nil { return Device{},fmt.Errorf("load paired device: %w",err) }
	return device,nil
}
