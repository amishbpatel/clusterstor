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

type DashboardFileStats struct {
	FileTypes []DashboardFileType `json:"file_types"`
	LargestFiles []DashboardLargestFile `json:"largest_files"`
}

func (s *Service) DashboardFileStats(ctx context.Context, userID string, limit int) (DashboardFileStats, error) {
	if limit <= 0 { limit = 8 }
	if limit > 25 { limit = 25 }

	typeRows, err := s.pool.Query(ctx, `
		SELECT
			CASE
				WHEN n.name LIKE '%.%' AND right(n.name,1) <> '.'
					THEN upper(regexp_replace(n.name, '^.*\\.', ''))
				ELSE 'OTHER'
			END AS file_type,
			COUNT(*)::bigint AS file_count,
			COALESCE(SUM(pi.size_bytes),0)::bigint AS total_size_bytes
		FROM nodes n
		JOIN provider_items pi ON pi.node_id=n.id
		JOIN provider_accounts pa ON pa.id=pi.provider_account_id
		WHERE n.user_id=$1::uuid
		  AND n.node_type='file'
		  AND n.deleted_at IS NULL
		  AND n.state='active'
		  AND pa.user_id=$1::uuid
		  AND pa.status='connected'
		  AND pa.disconnected_at IS NULL
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

	fileRows, err := s.pool.Query(ctx, `
		SELECT n.id::text,n.name,pa.provider,COALESCE(pi.size_bytes,0)::bigint
		FROM nodes n
		JOIN provider_items pi ON pi.node_id=n.id
		JOIN provider_accounts pa ON pa.id=pi.provider_account_id
		WHERE n.user_id=$1::uuid
		  AND n.node_type='file'
		  AND n.deleted_at IS NULL
		  AND n.state='active'
		  AND pa.user_id=$1::uuid
		  AND pa.status='connected'
		  AND pa.disconnected_at IS NULL
		ORDER BY COALESCE(pi.size_bytes,0) DESC,n.name ASC
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

	return DashboardFileStats{FileTypes:fileTypes,LargestFiles:largest},nil
}
