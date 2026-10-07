package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const maxDesktopBootstrapItems = 100000

type GoogleDesktopItem struct {
	NodeID         string     `json:"node_id"`
	ProviderItemID string     `json:"provider_item_id"`
	ParentNodeID   *string    `json:"parent_node_id,omitempty"`
	Name           string     `json:"name"`
	NodeType       string     `json:"node_type"`
	MIMEType       string     `json:"mime_type,omitempty"`
	SizeBytes      int64      `json:"size_bytes"`
	ModifiedAt     *time.Time `json:"modified_at,omitempty"`
	VersionID      string     `json:"version_id,omitempty"`
	Downloadable   bool       `json:"downloadable"`
}

type GoogleDesktopSnapshot struct {
	Provider      string              `json:"provider"`
	Bootstrapped  bool                `json:"bootstrapped"`
	Items         []GoogleDesktopItem `json:"items"`
}

func (s *Service) GoogleDesktopSnapshot(ctx context.Context,userID string) (GoogleDesktopSnapshot,error) {
	accountID,_,err:=s.googleCredential(ctx,userID)
	if err!=nil { return GoogleDesktopSnapshot{},err }
	if _,err:=s.EnsureGoogleRoot(ctx,userID); err!=nil { return GoogleDesktopSnapshot{},err }

	var bootstrappedAt *time.Time
	var cursor *string
	if err:=s.pool.QueryRow(ctx,
		"SELECT desktop_tree_bootstrapped_at,change_cursor FROM provider_accounts WHERE id=$1::uuid AND user_id=$2::uuid",
		accountID,userID).Scan(&bootstrappedAt,&cursor); err!=nil {
		return GoogleDesktopSnapshot{},fmt.Errorf("load desktop sync bootstrap state: %w",err)
	}

	if bootstrappedAt==nil {
		// Establish the Drive change boundary before the crawl. Any changes that
		// occur during the crawl are replayed from this cursor afterward.
		if cursor==nil || strings.TrimSpace(*cursor)=="" {
			if _,err:=s.SyncGoogleChanges(ctx,userID,1); err!=nil {
				return GoogleDesktopSnapshot{},err
			}
		}
		if _,err:=s.refreshGoogleManagedTree(ctx,userID,maxDesktopBootstrapItems); err!=nil {
			return GoogleDesktopSnapshot{},err
		}
		if _,err:=s.pool.Exec(ctx,
			"UPDATE provider_accounts SET desktop_tree_bootstrapped_at=now(),updated_at=now() WHERE id=$1::uuid",
			accountID); err!=nil {
			return GoogleDesktopSnapshot{},fmt.Errorf("mark desktop sync bootstrap complete: %w",err)
		}
		bootstrappedAt=&time.Time{}
	}

	// Drain a bounded number of change pages. The desktop will poll again if a
	// very large backlog remains; normal steady-state sync generally consumes one.
	for batch:=0;batch<5;batch++ {
		result,err:=s.SyncGoogleChanges(ctx,userID,20)
		if err!=nil { return GoogleDesktopSnapshot{},err }
		if !result.HasMore { break }
	}

	items,err:=s.listGoogleDesktopItems(ctx,userID,accountID)
	if err!=nil { return GoogleDesktopSnapshot{},err }
	return GoogleDesktopSnapshot{Provider:"google_drive",Bootstrapped:true,Items:items},nil
}

func (s *Service) refreshGoogleManagedTree(ctx context.Context,userID string,maxItems int) (int,error) {
	if maxItems<=0 { maxItems=maxDesktopBootstrapItems }
	accountID,token,err:=s.googleCredential(ctx,userID)
	if err!=nil { return 0,err }
	token,err=s.ensureGoogleAccessToken(ctx,accountID,token)
	if err!=nil { return 0,err }
	rootID,err:=s.EnsureGoogleRoot(ctx,userID)
	if err!=nil { return 0,err }

	queue:=[]string{rootID}
	seen:=map[string]bool{rootID:true}
	count:=0

	for len(queue)>0 {
		parent:=queue[0]
		queue=queue[1:]
		cursor:=""
		for {
			page,err:=s.fetchGoogleChildren(ctx,token.AccessToken,parent,cursor,maxDrivePageSize)
			if err!=nil { return count,err }
			items,err:=s.upsertGoogleFiles(ctx,userID,accountID,page.Files)
			if err!=nil { return count,err }
			count+=len(items)
			if count>maxItems {
				return count,fmt.Errorf("desktop sync bootstrap exceeds %d managed items",maxItems)
			}
			for _,item:=range items {
				if item.NodeType=="folder" && !seen[item.ProviderItemID] {
					seen[item.ProviderItemID]=true
					queue=append(queue,item.ProviderItemID)
				}
			}
			cursor=strings.TrimSpace(page.NextPageToken)
			if cursor=="" { break }
		}
	}
	return count,nil
}

func (s *Service) listGoogleDesktopItems(ctx context.Context,userID,accountID string) ([]GoogleDesktopItem,error) {
	rows,err:=s.pool.Query(ctx,`
		WITH RECURSIVE managed AS (
			SELECT
				pi.node_id,
				pi.provider_item_id,
				pi.provider_parent_item_id,
				pi.mime_type,
				pi.size_bytes,
				pi.modified_at,
				pi.provider_revision_id,
				NULL::uuid AS parent_node_id
			FROM provider_items pi
			JOIN provider_accounts pa ON pa.id=pi.provider_account_id
			JOIN nodes n ON n.id=pi.node_id
			WHERE pi.provider_account_id=$2::uuid
			  AND pa.user_id=$1::uuid
			  AND pa.root_provider_item_id IS NOT NULL
			  AND pi.provider_parent_item_id=pa.root_provider_item_id
			  AND n.deleted_at IS NULL
			  AND n.state='active'
			UNION ALL
			SELECT
				child.node_id,
				child.provider_item_id,
				child.provider_parent_item_id,
				child.mime_type,
				child.size_bytes,
				child.modified_at,
				child.provider_revision_id,
				parent.node_id
			FROM provider_items child
			JOIN managed parent
			  ON child.provider_parent_item_id=parent.provider_item_id
			JOIN nodes child_node ON child_node.id=child.node_id
			WHERE child.provider_account_id=$2::uuid
			  AND child_node.deleted_at IS NULL
			  AND child_node.state='active'
		)
		SELECT
			n.id::text,
			managed.provider_item_id,
			managed.parent_node_id::text,
			n.name,
			n.node_type,
			COALESCE(managed.mime_type,''),
			COALESCE(managed.size_bytes,0)::bigint,
			managed.modified_at,
			COALESCE(managed.provider_revision_id,'')
		FROM managed
		JOIN nodes n ON n.id=managed.node_id
		WHERE n.user_id=$1::uuid
		ORDER BY n.node_type DESC,n.name ASC,managed.provider_item_id ASC`,userID,accountID)
	if err!=nil { return nil,fmt.Errorf("list desktop sync items: %w",err) }
	defer rows.Close()

	items:=make([]GoogleDesktopItem,0)
	for rows.Next() {
		var item GoogleDesktopItem
		if err:=rows.Scan(
			&item.NodeID,&item.ProviderItemID,&item.ParentNodeID,&item.Name,&item.NodeType,
			&item.MIMEType,&item.SizeBytes,&item.ModifiedAt,&item.VersionID,
		); err!=nil {
			return nil,fmt.Errorf("scan desktop sync item: %w",err)
		}
		item.Downloadable=item.NodeType=="file" && !isGoogleWorkspaceNative(item.MIMEType)
		items=append(items,item)
	}
	if err:=rows.Err(); err!=nil { return nil,err }
	return items,nil
}

func isGoogleWorkspaceNative(mimeType string) bool {
	mimeType=strings.TrimSpace(mimeType)
	return strings.HasPrefix(mimeType,"application/vnd.google-apps.") && mimeType!=googleFolderMIME
}

func (s *Service) GoogleDesktopItem(ctx context.Context,userID,nodeID string) (GoogleDesktopItem,error) {
	accountID,_,err:=s.googleCredential(ctx,userID)
	if err!=nil { return GoogleDesktopItem{},err }
	items,err:=s.listGoogleDesktopItems(ctx,userID,accountID)
	if err!=nil { return GoogleDesktopItem{},err }
	for _,item:=range items {
		if item.NodeID==nodeID { return item,nil }
	}
	return GoogleDesktopItem{},pgx.ErrNoRows
}

func (s *Service) GoogleDesktopParentNode(ctx context.Context,userID string,parentProviderID string) (*string,error) {
	parentProviderID=strings.TrimSpace(parentProviderID)
	if parentProviderID=="" { return nil,nil }
	accountID,_,err:=s.googleCredential(ctx,userID)
	if err!=nil { return nil,err }
	var rootID *string
	if err:=s.pool.QueryRow(ctx,"SELECT root_provider_item_id FROM provider_accounts WHERE id=$1::uuid",accountID).Scan(&rootID); err!=nil {
		return nil,err
	}
	if rootID!=nil && parentProviderID==*rootID { return nil,nil }
	var nodeID string
	err=s.pool.QueryRow(ctx,
		"SELECT node_id::text FROM provider_items WHERE provider_account_id=$1::uuid AND provider_item_id=$2",
		accountID,parentProviderID).Scan(&nodeID)
	if errors.Is(err,pgx.ErrNoRows) { return nil,ErrInvalidProviderParent }
	if err!=nil { return nil,err }
	return &nodeID,nil
}
