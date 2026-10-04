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
