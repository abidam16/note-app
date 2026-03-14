package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type FolderSiblingUniquenessConflict struct {
	WorkspaceID    string
	ParentID       *string
	NormalizedName string
	ConflictCount  int
	FolderIDs      []string
}

func RunFolderSiblingUniquenessPreflight(dsn string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := NewPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	conflicts, err := listFolderSiblingUniquenessConflicts(ctx, pool)
	if err != nil {
		return err
	}
	if len(conflicts) == 0 {
		return nil
	}

	return fmt.Errorf("folder sibling-name uniqueness preflight failed:\n%s", formatFolderSiblingUniquenessConflicts(conflicts))
}

func listFolderSiblingUniquenessConflicts(ctx context.Context, pool *pgxpool.Pool) ([]FolderSiblingUniquenessConflict, error) {
	rows, err := pool.Query(ctx, `
		SELECT
			workspace_id::text,
			parent_id::text,
			LOWER(TRIM(name)) AS normalized_name,
			COUNT(*) AS conflict_count,
			ARRAY_AGG(id::text ORDER BY created_at ASC, id ASC) AS folder_ids
		FROM folders
		GROUP BY workspace_id, parent_id, LOWER(TRIM(name))
		HAVING COUNT(*) > 1
		ORDER BY conflict_count DESC, workspace_id ASC, parent_id ASC NULLS FIRST, normalized_name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query folder sibling-name conflicts: %w", err)
	}
	defer rows.Close()

	conflicts := make([]FolderSiblingUniquenessConflict, 0)
	for rows.Next() {
		var conflict FolderSiblingUniquenessConflict
		if err := rows.Scan(
			&conflict.WorkspaceID,
			&conflict.ParentID,
			&conflict.NormalizedName,
			&conflict.ConflictCount,
			&conflict.FolderIDs,
		); err != nil {
			return nil, fmt.Errorf("scan folder sibling-name conflict: %w", err)
		}
		conflicts = append(conflicts, conflict)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate folder sibling-name conflicts: %w", err)
	}

	return conflicts, nil
}

func formatFolderSiblingUniquenessConflicts(conflicts []FolderSiblingUniquenessConflict) string {
	const maxLines = 20

	lines := make([]string, 0, min(len(conflicts), maxLines)+1)
	for idx, conflict := range conflicts {
		if idx == maxLines {
			lines = append(lines, fmt.Sprintf("... and %d more conflict group(s)", len(conflicts)-maxLines))
			break
		}

		parentID := "<root>"
		if conflict.ParentID != nil && strings.TrimSpace(*conflict.ParentID) != "" {
			parentID = *conflict.ParentID
		}

		lines = append(lines, fmt.Sprintf(
			"- workspace_id=%s parent_id=%s normalized_name=%q count=%d folder_ids=%s",
			conflict.WorkspaceID,
			parentID,
			conflict.NormalizedName,
			conflict.ConflictCount,
			strings.Join(conflict.FolderIDs, ","),
		))
	}

	return strings.Join(lines, "\n")
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
