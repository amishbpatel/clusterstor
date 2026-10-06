package providers

import (
	"context"
	"fmt"
)

type DashboardFileType struct {
	FileType string `json:"file_type"`
	FileCount int64 `json:"file_count"`
	TotalSizeBytes int64 `json:"total_size_bytes"`
}

type DashboardLargestFile struct {
	NodeID string `json:"node_id"`
	Name string `json:"name"`
	Provider string `json:"provider"`
	SizeBytes int64 `json:"size_bytes"`
}

type DashboardFileTypeFile struct {
	FileType string `json:"file_type"`
	NodeID string `json:"node_id"`
	Name string `json:"name"`
	Provider string `json:"provider"`
	SizeBytes int64 `json:"size_bytes"`
}

type DashboardFileStats struct {
	FileTypes []DashboardFileType `json:"file_types"`
	FileTypeFiles []DashboardFileTypeFile `json:"file_type_files"`
	LargestFiles []DashboardLargestFile `json:"largest_files"`
}

func (s *Service) DashboardFileStats(ctx context.Context, userID string, limit int) (DashboardFileStats, error) {
	if limit <= 0 { limit = 8 }
	if limit > 25 { limit = 25 }

	typeRows, err := s.pool.Query(ctx, `
		WITH RECURSIVE managed_items AS (
			SELECT pi.provider_account_id,pi.node_id,pi.provider_item_id,pi.provider_parent_item_id,pi.size_bytes
			FROM provider_items pi
			JOIN provider_accounts pa ON pa.id=pi.provider_account_id
			JOIN nodes root_node
			  ON root_node.id=pi.node_id
			 AND root_node.user_id=$1::uuid
			 AND root_node.state='active'
			 AND root_node.deleted_at IS NULL
			WHERE pa.user_id=$1::uuid
			  AND pa.status='connected'
			  AND pa.disconnected_at IS NULL
			  AND pa.root_provider_item_id IS NOT NULL
			  AND pi.provider_parent_item_id=pa.root_provider_item_id
			UNION ALL
			SELECT child.provider_account_id,child.node_id,child.provider_item_id,child.provider_parent_item_id,child.size_bytes
			FROM provider_items child
			JOIN managed_items parent
			  ON parent.provider_account_id=child.provider_account_id
			 AND parent.provider_item_id=child.provider_parent_item_id
			JOIN nodes child_node
			  ON child_node.id=child.node_id
			 AND child_node.user_id=$1::uuid
			 AND child_node.state='active'
			 AND child_node.deleted_at IS NULL
		)
		SELECT
			CASE
				WHEN n.name LIKE '%.%' AND right(n.name,1) <> '.'
					THEN upper(regexp_replace(n.name, '^.*\.', ''))
				ELSE 'OTHER'
			END AS file_type,
			COUNT(*)::bigint AS file_count,
			COALESCE(SUM(mi.size_bytes),0)::bigint AS total_size_bytes
		FROM managed_items mi
		JOIN nodes n ON n.id=mi.node_id
		WHERE n.user_id=$1::uuid
		  AND n.node_type='file'
		  AND n.deleted_at IS NULL
		  AND n.state='active'
		GROUP BY file_type
		ORDER BY total_size_bytes DESC, file_count DESC, file_type ASC`, userID)
	if err != nil { return DashboardFileStats{}, fmt.Errorf("dashboard file types: %w", err) }
	defer typeRows.Close()

	fileTypes := make([]DashboardFileType, 0)
	for typeRows.Next() {
		var item DashboardFileType
		if err := typeRows.Scan(&item.FileType,&item.FileCount,&item.TotalSizeBytes); err != nil {
			return DashboardFileStats{}, fmt.Errorf("scan dashboard file type: %w", err)
		}
		fileTypes = append(fileTypes,item)
	}
	if err := typeRows.Err(); err != nil { return DashboardFileStats{}, fmt.Errorf("iterate dashboard file types: %w", err) }

	fileTypeFileRows, err := s.pool.Query(ctx, `
		WITH RECURSIVE managed_items AS (
			SELECT pi.provider_account_id,pi.node_id,pi.provider_item_id,pi.provider_parent_item_id,pi.size_bytes
			FROM provider_items pi
			JOIN provider_accounts pa ON pa.id=pi.provider_account_id
			JOIN nodes root_node
			  ON root_node.id=pi.node_id
			 AND root_node.user_id=$1::uuid
			 AND root_node.state='active'
			 AND root_node.deleted_at IS NULL
			WHERE pa.user_id=$1::uuid
			  AND pa.status='connected'
			  AND pa.disconnected_at IS NULL
			  AND pa.root_provider_item_id IS NOT NULL
			  AND pi.provider_parent_item_id=pa.root_provider_item_id
			UNION ALL
			SELECT child.provider_account_id,child.node_id,child.provider_item_id,child.provider_parent_item_id,child.size_bytes
			FROM provider_items child
			JOIN managed_items parent
			  ON parent.provider_account_id=child.provider_account_id
			 AND parent.provider_item_id=child.provider_parent_item_id
			JOIN nodes child_node
			  ON child_node.id=child.node_id
			 AND child_node.user_id=$1::uuid
			 AND child_node.state='active'
			 AND child_node.deleted_at IS NULL
		),
		typed AS (
			SELECT
				CASE
					WHEN n.name LIKE '%.%' AND right(n.name,1) <> '.'
						THEN upper(regexp_replace(n.name, '^.*\.', ''))
					ELSE 'OTHER'
				END AS file_type,
				n.id::text AS node_id,
				n.name,
				pa.provider,
				COALESCE(mi.size_bytes,0)::bigint AS size_bytes
			FROM managed_items mi
			JOIN nodes n ON n.id=mi.node_id
			JOIN provider_accounts pa ON pa.id=mi.provider_account_id
			WHERE n.user_id=$1::uuid
			  AND n.node_type='file'
			  AND n.deleted_at IS NULL
			  AND n.state='active'
		),
		ranked AS (
			SELECT *, ROW_NUMBER() OVER (PARTITION BY file_type ORDER BY size_bytes DESC,name ASC) AS rank
			FROM typed
		)
		SELECT file_type,node_id,name,provider,size_bytes
		FROM ranked
		WHERE rank <= 5
		ORDER BY file_type,rank`, userID)
	if err != nil { return DashboardFileStats{}, fmt.Errorf("dashboard file type files: %w", err) }
	defer fileTypeFileRows.Close()

	fileTypeFiles := make([]DashboardFileTypeFile, 0)
	for fileTypeFileRows.Next() {
		var item DashboardFileTypeFile
		if err := fileTypeFileRows.Scan(&item.FileType,&item.NodeID,&item.Name,&item.Provider,&item.SizeBytes); err != nil {
			return DashboardFileStats{}, fmt.Errorf("scan dashboard file type file: %w", err)
		}
		fileTypeFiles = append(fileTypeFiles,item)
	}
	if err := fileTypeFileRows.Err(); err != nil { return DashboardFileStats{}, fmt.Errorf("iterate dashboard file type files: %w", err) }

	fileRows, err := s.pool.Query(ctx, `
		WITH RECURSIVE managed_items AS (
			SELECT pi.provider_account_id,pi.node_id,pi.provider_item_id,pi.provider_parent_item_id,pi.size_bytes
			FROM provider_items pi
			JOIN provider_accounts pa ON pa.id=pi.provider_account_id
			JOIN nodes root_node
			  ON root_node.id=pi.node_id
			 AND root_node.user_id=$1::uuid
			 AND root_node.state='active'
			 AND root_node.deleted_at IS NULL
			WHERE pa.user_id=$1::uuid
			  AND pa.status='connected'
			  AND pa.disconnected_at IS NULL
			  AND pa.root_provider_item_id IS NOT NULL
			  AND pi.provider_parent_item_id=pa.root_provider_item_id
			UNION ALL
			SELECT child.provider_account_id,child.node_id,child.provider_item_id,child.provider_parent_item_id,child.size_bytes
			FROM provider_items child
			JOIN managed_items parent
			  ON parent.provider_account_id=child.provider_account_id
			 AND parent.provider_item_id=child.provider_parent_item_id
			JOIN nodes child_node
			  ON child_node.id=child.node_id
			 AND child_node.user_id=$1::uuid
			 AND child_node.state='active'
			 AND child_node.deleted_at IS NULL
		)
		SELECT n.id::text,n.name,pa.provider,COALESCE(mi.size_bytes,0)::bigint
		FROM managed_items mi
		JOIN nodes n ON n.id=mi.node_id
		JOIN provider_accounts pa ON pa.id=mi.provider_account_id
		WHERE n.user_id=$1::uuid
		  AND n.node_type='file'
		  AND n.deleted_at IS NULL
		  AND n.state='active'
		ORDER BY COALESCE(mi.size_bytes,0) DESC,n.name ASC
		LIMIT $2`, userID, limit)
	if err != nil { return DashboardFileStats{}, fmt.Errorf("dashboard largest files: %w", err) }
	defer fileRows.Close()

	largest := make([]DashboardLargestFile, 0, limit)
	for fileRows.Next() {
		var item DashboardLargestFile
		if err := fileRows.Scan(&item.NodeID,&item.Name,&item.Provider,&item.SizeBytes); err != nil {
			return DashboardFileStats{}, fmt.Errorf("scan dashboard largest file: %w", err)
		}
		largest = append(largest,item)
	}
	if err := fileRows.Err(); err != nil { return DashboardFileStats{}, fmt.Errorf("iterate dashboard largest files: %w", err) }

	return DashboardFileStats{FileTypes:fileTypes,FileTypeFiles:fileTypeFiles,LargestFiles:largest},nil
}
