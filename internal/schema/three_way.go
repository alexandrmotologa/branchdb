package schema

import (
	"fmt"
	"strings"
)

// SchemaConflict represents an incompatible schema divergence between two branches against a base.
type SchemaConflict struct {
	Table          string `json:"table"`
	Column         string `json:"column,omitempty"`
	ConflictType   string `json:"conflict_type"` // TYPE_MISMATCH, CONSTRAINT_CLASH, DROP_ALTER_CLASH, TABLE_NAME_COLLISION
	MineSummary    string `json:"mine_summary"`
	TheirsSummary  string `json:"theirs_summary"`
	Recommendation string `json:"recommendation"`
}

// ThreeWayDiffResult holds the 3-way schema comparison report.
type ThreeWayDiffResult struct {
	BaseDB           string           `json:"base_db"`
	MineDB           string           `json:"mine_db"`
	TheirsDB         string           `json:"theirs_db"`
	HasConflicts     bool             `json:"has_conflicts"`
	ConflictCount    int              `json:"conflict_count"`
	Conflicts        []SchemaConflict `json:"conflicts"`
	SafeMineChanges  []string         `json:"safe_mine_changes"`
	SafeTheirChanges []string         `json:"safe_their_changes"`
	DurationMs       int64            `json:"duration_ms"`
}

// CompareThreeWay evaluates schema divergences between a base ancestor, local branch (mine), and remote branch (theirs).
func CompareThreeWay(baseDb, mineDb, theirsDb string, base, mine, theirs *DatabaseSchema) *ThreeWayDiffResult {
	diffMine := Compare(base, mine)
	diffTheirs := Compare(base, theirs)

	var conflicts []SchemaConflict
	var safeMine []string
	var safeTheirs []string

	// Index mine changes
	mineDroppedTables := make(map[string]bool)
	for _, t := range diffMine.TablesDropped {
		mineDroppedTables[t] = true
	}
	mineAddedTables := make(map[string]Table)
	for _, t := range diffMine.TablesAdded {
		mineAddedTables[t.Name] = t
	}

	// Index theirs changes
	theirsDroppedTables := make(map[string]bool)
	for _, t := range diffTheirs.TablesDropped {
		theirsDroppedTables[t] = true
	}
	theirsAddedTables := make(map[string]Table)
	for _, t := range diffTheirs.TablesAdded {
		theirsAddedTables[t.Name] = t
	}

	// 1. Check Table Drop vs Table Modification Clashes
	for tableName, tDiff := range diffTheirs.TableDiffs {
		if mineDroppedTables[tableName] && tDiff.HasChanges() {
			conflicts = append(conflicts, SchemaConflict{
				Table:          tableName,
				ConflictType:   "DROP_ALTER_CLASH",
				MineSummary:    fmt.Sprintf("Table '%s' dropped in mine", tableName),
				TheirsSummary:  fmt.Sprintf("Table '%s' modified in theirs", tableName),
				Recommendation: "Decide whether table should be preserved or dropped before merging.",
			})
		}
	}
	for tableName, tDiff := range diffMine.TableDiffs {
		if theirsDroppedTables[tableName] && tDiff.HasChanges() {
			conflicts = append(conflicts, SchemaConflict{
				Table:          tableName,
				ConflictType:   "DROP_ALTER_CLASH",
				MineSummary:    fmt.Sprintf("Table '%s' modified in mine", tableName),
				TheirsSummary:  fmt.Sprintf("Table '%s' dropped in theirs", tableName),
				Recommendation: "Decide whether table should be preserved or dropped before merging.",
			})
		}
	}

	// 2. Check Table Addition Collisions
	for tableName, mineTable := range mineAddedTables {
		if theirsTable, exists := theirsAddedTables[tableName]; exists {
			// Both added the table: check column definitions
			mineCols := make(map[string]Column)
			for _, c := range mineTable.Columns {
				mineCols[c.Name] = c
			}

			collided := false
			for _, tc := range theirsTable.Columns {
				mc, mExists := mineCols[tc.Name]
				if !mExists {
					collided = true
					break
				}
				if !strings.EqualFold(mc.DataType, tc.DataType) {
					collided = true
					break
				}
			}

			if collided {
				conflicts = append(conflicts, SchemaConflict{
					Table:          tableName,
					ConflictType:   "TABLE_NAME_COLLISION",
					MineSummary:    fmt.Sprintf("Created table '%s' with %d columns", tableName, len(mineTable.Columns)),
					TheirsSummary:  fmt.Sprintf("Created table '%s' with different schema (%d columns)", tableName, len(theirsTable.Columns)),
					Recommendation: "Rename one table or align column types.",
				})
			} else {
				safeMine = append(safeMine, fmt.Sprintf("CREATE TABLE %s (identical in both)", tableName))
			}
		} else {
			safeMine = append(safeMine, fmt.Sprintf("CREATE TABLE %s", tableName))
		}
	}

	for tableName := range theirsAddedTables {
		if _, exists := mineAddedTables[tableName]; !exists {
			safeTheirs = append(safeTheirs, fmt.Sprintf("CREATE TABLE %s", tableName))
		}
	}

	// 3. Check Intra-Table Column Clashes
	for tableName, mineDiff := range diffMine.TableDiffs {
		theirsDiff, existsInTheirs := diffTheirs.TableDiffs[tableName]
		if !existsInTheirs {
			for _, ca := range mineDiff.ColumnsAdded {
				safeMine = append(safeMine, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", tableName, ca.Name))
			}
			continue
		}

		// Both modified the same table
		mineColsAdded := make(map[string]Column)
		for _, c := range mineDiff.ColumnsAdded {
			mineColsAdded[c.Name] = c
		}
		theirsColsAdded := make(map[string]Column)
		for _, c := range theirsDiff.ColumnsAdded {
			theirsColsAdded[c.Name] = c
		}

		// Check added column collisions
		for colName, mc := range mineColsAdded {
			if tc, cExists := theirsColsAdded[colName]; cExists {
				if !strings.EqualFold(mc.DataType, tc.DataType) || mc.IsNullable != tc.IsNullable {
					conflicts = append(conflicts, SchemaConflict{
						Table:          tableName,
						Column:         colName,
						ConflictType:   "COLUMN_TYPE_MISMATCH",
						MineSummary:    fmt.Sprintf("Added column %s %s (nullable=%v)", colName, mc.DataType, mc.IsNullable),
						TheirsSummary:  fmt.Sprintf("Added column %s %s (nullable=%v)", colName, tc.DataType, tc.IsNullable),
						Recommendation: "Align column data type and nullability constraint.",
					})
				} else {
					safeMine = append(safeMine, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s (identical)", tableName, colName))
				}
			} else {
				safeMine = append(safeMine, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", tableName, colName))
			}
		}

		for colName := range theirsColsAdded {
			if _, cExists := mineColsAdded[colName]; !cExists {
				safeTheirs = append(safeTheirs, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", tableName, colName))
			}
		}

		// Check Column Drop vs Alter
		mineDropped := make(map[string]bool)
		for _, col := range mineDiff.ColumnsDropped {
			mineDropped[col] = true
		}
		for _, alt := range theirsDiff.ColumnsAltered {
			if mineDropped[alt.ColumnName] {
				conflicts = append(conflicts, SchemaConflict{
					Table:          tableName,
					Column:         alt.ColumnName,
					ConflictType:   "DROP_ALTER_CLASH",
					MineSummary:    fmt.Sprintf("Column '%s' dropped in mine", alt.ColumnName),
					TheirsSummary:  fmt.Sprintf("Column '%s' type altered in theirs to %s", alt.ColumnName, alt.NewType),
					Recommendation: "Confirm whether column should be dropped or kept with new type.",
				})
			}
		}
	}

	return &ThreeWayDiffResult{
		BaseDB:           baseDb,
		MineDB:           mineDb,
		TheirsDB:         theirsDb,
		HasConflicts:     len(conflicts) > 0,
		ConflictCount:    len(conflicts),
		Conflicts:        conflicts,
		SafeMineChanges:  safeMine,
		SafeTheirChanges: safeTheirs,
	}
}
