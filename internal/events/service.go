package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultPageSize = 100
	maxPageSize = 500
)

type Service struct { pool *pgxpool.Pool }

type Event struct {
	ID string `json:"id"`
	Sequence int64 `json:"sequence"`
	EventType string `json:"event_type"`
	ResourceType *string `json:"resource_type,omitempty"`
	ResourceID *string `json:"resource_id,omitempty"`
	Payload json.RawMessage `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}

type Page struct {
	Events []Event `json:"events"`
	NextCursor int64 `json:"next_cursor"`
	HasMore bool `json:"has_more"`
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) List(ctx context.Context, userID string, after int64, limit int) (Page, error) {
	if after < 0 { after = 0 }
	if limit <= 0 { limit = defaultPageSize }
	if limit > maxPageSize { limit = maxPageSize }

	rows, err := s.pool.Query(ctx, `
		SELECT id::text,sequence,event_type,resource_type,resource_id::text,payload,created_at
		FROM account_events
		WHERE user_id=$1::uuid AND sequence>$2
		ORDER BY sequence ASC
		LIMIT $3`, userID, after, limit+1)
	if err != nil { return Page{}, fmt.Errorf("list account events: %w", err) }
	defer rows.Close()

	events := make([]Event, 0, limit)
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID,&event.Sequence,&event.EventType,&event.ResourceType,&event.ResourceID,&event.Payload,&event.CreatedAt); err != nil {
			return Page{}, fmt.Errorf("scan account event: %w", err)
		}
		events = append(events,event)
	}
	if err := rows.Err(); err != nil { return Page{}, fmt.Errorf("iterate account events: %w", err) }

	hasMore := len(events) > limit
	if hasMore { events = events[:limit] }
	next := after
	if len(events) > 0 { next = events[len(events)-1].Sequence }
	return Page{Events:events,NextCursor:next,HasMore:hasMore},nil
}


func (s *Service) LatestSequence(ctx context.Context, userID string) (int64, error) {
	var sequence int64
	if err := s.pool.QueryRow(ctx, "SELECT COALESCE(MAX(sequence),0) FROM account_events WHERE user_id=$1::uuid", userID).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("latest account event sequence: %w", err)
	}
	return sequence, nil
}


type RecentFileActivity struct {
	EventType string `json:"event_type"`
	NodeID string `json:"node_id"`
	Name string `json:"name"`
	Provider string `json:"provider"`
	SizeBytes int64 `json:"size_bytes"`
	Destination *string `json:"destination,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type RecentDashboardActivity struct {
	Uploads []RecentFileActivity `json:"uploads"`
	Downloads []RecentFileActivity `json:"downloads"`
}

func (s *Service) RecordDownload(ctx context.Context, userID, nodeID, provider, name string, sizeBytes int64, destination string) error {
	payload, err := json.Marshal(map[string]any{
		"provider": provider,
		"name": name,
		"size_bytes": sizeBytes,
		"destination": destination,
	})
	if err != nil { return fmt.Errorf("encode download event: %w", err) }
	_, err = s.pool.Exec(ctx,
		"INSERT INTO account_events(user_id,event_type,resource_type,resource_id,payload) VALUES ($1::uuid,'file.downloaded','node',$2::uuid,$3::jsonb)",
		userID, nodeID, string(payload))
	if err != nil { return fmt.Errorf("record download event: %w", err) }
	return nil
}

func (s *Service) RecentDashboard(ctx context.Context, userID string, limit int) (RecentDashboardActivity, error) {
	if limit <= 0 { limit = 5 }
	if limit > 20 { limit = 20 }

	query := `
		SELECT e.event_type,
		       e.resource_id::text,
		       COALESCE(n.name, e.payload->>'name', 'Unknown file') AS name,
		       COALESCE(e.payload->>'provider', pa.provider, '') AS provider,
		       COALESCE((e.payload->>'size_bytes')::bigint, pi.size_bytes, 0) AS size_bytes,
		       NULLIF(e.payload->>'destination','') AS destination,
		       e.created_at
		FROM account_events e
		LEFT JOIN nodes n ON n.id=e.resource_id
		LEFT JOIN provider_items pi ON pi.node_id=n.id
		LEFT JOIN provider_accounts pa ON pa.id=pi.provider_account_id
		WHERE e.user_id=$1::uuid
		  AND e.event_type=$2
		ORDER BY e.sequence DESC
		LIMIT $3`

	load := func(eventType string) ([]RecentFileActivity, error) {
		rows, err := s.pool.Query(ctx, query, userID, eventType, limit)
		if err != nil { return nil, fmt.Errorf("list recent %s: %w", eventType, err) }
		defer rows.Close()
		result := make([]RecentFileActivity, 0, limit)
		for rows.Next() {
			var item RecentFileActivity
			if err := rows.Scan(&item.EventType,&item.NodeID,&item.Name,&item.Provider,&item.SizeBytes,&item.Destination,&item.CreatedAt); err != nil {
				return nil, fmt.Errorf("scan recent activity: %w", err)
			}
			result = append(result,item)
		}
		if err := rows.Err(); err != nil { return nil, fmt.Errorf("iterate recent activity: %w", err) }
		return result,nil
	}

	uploads, err := load("file.version.created")
	if err != nil { return RecentDashboardActivity{}, err }
	downloads, err := load("file.downloaded")
	if err != nil { return RecentDashboardActivity{}, err }
	return RecentDashboardActivity{Uploads:uploads,Downloads:downloads},nil
}
