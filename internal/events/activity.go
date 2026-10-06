package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ActivityQuery struct {
	Before int64
	Limit int
	EventType string
	Provider string
	Search string
	From *time.Time
	To *time.Time
}

type ActivityItem struct {
	ID string `json:"id"`
	Sequence int64 `json:"sequence"`
	EventType string `json:"event_type"`
	ResourceType *string `json:"resource_type,omitempty"`
	ResourceID *string `json:"resource_id,omitempty"`
	Name string `json:"name"`
	NodeType string `json:"node_type,omitempty"`
	Provider string `json:"provider,omitempty"`
	SizeBytes int64 `json:"size_bytes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Payload json.RawMessage `json:"payload"`
}

type ActivityPage struct {
	Items []ActivityItem `json:"items"`
	NextCursor int64 `json:"next_cursor,omitempty"`
	HasMore bool `json:"has_more"`
}

func (s *Service) Activity(ctx context.Context, userID string, query ActivityQuery) (ActivityPage,error) {
	limit:=query.Limit
	if limit<=0 { limit=50 }
	if limit>200 { limit=200 }

	args:=[]any{userID}
	where:=[]string{"e.user_id=$1::uuid"}
	add:=func(clause string,value any) {
		args=append(args,value)
		where=append(where,fmt.Sprintf(clause,len(args)))
	}

	if query.Before>0 { add("e.sequence<$%d",query.Before) }
	if value:=strings.TrimSpace(query.EventType); value!="" { add("e.event_type=$%d",value) }
	if value:=strings.TrimSpace(query.Provider); value!="" {
		add("COALESCE(NULLIF(e.payload->>'provider',''),pa.provider,'')=$%d",value)
	}
	if value:=strings.TrimSpace(query.Search); value!="" {
		args=append(args,"%"+value+"%")
		where=append(where,fmt.Sprintf("COALESCE(n.name,e.payload->>'name','') ILIKE $%d",len(args)))
	}
	if query.From!=nil { add("e.created_at >= $%d",*query.From) }
	if query.To!=nil { add("e.created_at < $%d",*query.To) }

	args=append(args,limit+1)
	sql:=fmt.Sprintf(`
		SELECT e.id::text,e.sequence,e.event_type,e.resource_type,e.resource_id::text,
		       COALESCE(n.name,e.payload->>'name',
		         CASE
		           WHEN e.resource_type='archive' THEN 'Archive download'
		           ELSE 'Unknown item'
		         END) AS name,
		       COALESCE(n.node_type,'') AS node_type,
		       COALESCE(NULLIF(e.payload->>'provider',''),pa.provider,'') AS provider,
		       COALESCE(NULLIF(e.payload->>'size_bytes','')::bigint,pi.size_bytes,0) AS size_bytes,
		       e.created_at,e.payload
		FROM account_events e
		LEFT JOIN nodes n ON n.id=e.resource_id
		LEFT JOIN provider_items pi ON pi.node_id=n.id
		LEFT JOIN provider_accounts pa ON pa.id=pi.provider_account_id
		WHERE %s
		ORDER BY e.sequence DESC
		LIMIT $%d`,strings.Join(where," AND "),len(args))

	rows,err:=s.pool.Query(ctx,sql,args...)
	if err!=nil { return ActivityPage{},fmt.Errorf("list activity: %w",err) }
	defer rows.Close()

	items:=make([]ActivityItem,0,limit)
	for rows.Next() {
		var item ActivityItem
		if err:=rows.Scan(
			&item.ID,&item.Sequence,&item.EventType,&item.ResourceType,&item.ResourceID,
			&item.Name,&item.NodeType,&item.Provider,&item.SizeBytes,&item.CreatedAt,&item.Payload,
		); err!=nil {
			return ActivityPage{},fmt.Errorf("scan activity: %w",err)
		}
		items=append(items,item)
	}
	if err:=rows.Err(); err!=nil { return ActivityPage{},fmt.Errorf("iterate activity: %w",err) }

	hasMore:=len(items)>limit
	if hasMore { items=items[:limit] }
	var next int64
	if len(items)>0 { next=items[len(items)-1].Sequence }
	return ActivityPage{Items:items,NextCursor:next,HasMore:hasMore},nil
}
