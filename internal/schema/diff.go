package schema

// ColumnAlteration details what changed within an existing column.
type ColumnAlteration struct {
	ColumnName     string  `json:"column_name"`
	OldType        string  `json:"old_type"`
	NewType        string  `json:"new_type"`
	TypeChanged    bool    `json:"type_changed"`
	OldNullable    bool    `json:"old_nullable"`
	NewNullable    bool    `json:"new_nullable"`
	NullableChange bool    `json:"nullable_changed"`
	OldDefault     *string `json:"old_default,omitempty"`
	NewDefault     *string `json:"new_default,omitempty"`
	DefaultChanged bool    `json:"default_changed"`
}

// TableDiff details additions, modifications, and removals within a specific table.
type TableDiff struct {
	TableName          string             `json:"table_name"`
	ColumnsAdded       []Column           `json:"columns_added"`
	ColumnsDropped     []string           `json:"columns_dropped"`
	ColumnsAltered     []ColumnAlteration `json:"columns_altered"`
	IndexesAdded       []Index            `json:"indexes_added"`
	IndexesDropped     []string           `json:"indexes_dropped"`
	ForeignKeysAdded   []ForeignKey       `json:"foreign_keys_added"`
	ForeignKeysDropped []string           `json:"foreign_keys_dropped"`
}

// HasChanges returns true if the table has any difference.
func (td *TableDiff) HasChanges() bool {
	return len(td.ColumnsAdded) > 0 ||
		len(td.ColumnsDropped) > 0 ||
		len(td.ColumnsAltered) > 0 ||
		len(td.IndexesAdded) > 0 ||
		len(td.IndexesDropped) > 0 ||
		len(td.ForeignKeysAdded) > 0 ||
		len(td.ForeignKeysDropped) > 0
}

// SchemaDiff encapsulates all differences transitioning from Source to Target schema.
type SchemaDiff struct {
	FromDatabase  string               `json:"from_database"`
	ToDatabase    string               `json:"to_database"`
	TablesAdded   []Table              `json:"tables_added"`
	TablesDropped []string             `json:"tables_dropped"`
	TableDiffs    map[string]TableDiff `json:"table_diffs"`
}

// HasChanges returns true if any tables or structures differ.
func (sd *SchemaDiff) HasChanges() bool {
	if len(sd.TablesAdded) > 0 || len(sd.TablesDropped) > 0 {
		return true
	}
	for _, td := range sd.TableDiffs {
		if td.HasChanges() {
			return true
		}
	}
	return false
}

// Compare computes the precise structural diff between two database schemas.
func Compare(from, to *DatabaseSchema) *SchemaDiff {
	diff := &SchemaDiff{
		FromDatabase:  from.DatabaseName,
		ToDatabase:    to.DatabaseName,
		TablesAdded:   []Table{},
		TablesDropped: []string{},
		TableDiffs:    make(map[string]TableDiff),
	}

	// 1. Check for added tables
	for name, targetTable := range to.Tables {
		if _, exists := from.Tables[name]; !exists {
			diff.TablesAdded = append(diff.TablesAdded, targetTable)
		}
	}

	// 2. Check for dropped tables
	for name := range from.Tables {
		if _, exists := to.Tables[name]; !exists {
			diff.TablesDropped = append(diff.TablesDropped, name)
		}
	}

	// 3. Compare tables present in both
	for name, targetTable := range to.Tables {
		sourceTable, exists := from.Tables[name]
		if !exists {
			continue
		}

		tblDiff := TableDiff{
			TableName:          name,
			ColumnsAdded:       []Column{},
			ColumnsDropped:     []string{},
			ColumnsAltered:     []ColumnAlteration{},
			IndexesAdded:       []Index{},
			IndexesDropped:     []string{},
			ForeignKeysAdded:   []ForeignKey{},
			ForeignKeysDropped: []string{},
		}

		// Added columns
		for colName, targetCol := range targetTable.Columns {
			if _, ok := sourceTable.Columns[colName]; !ok {
				tblDiff.ColumnsAdded = append(tblDiff.ColumnsAdded, targetCol)
			}
		}

		// Dropped columns
		for colName := range sourceTable.Columns {
			if _, ok := targetTable.Columns[colName]; !ok {
				tblDiff.ColumnsDropped = append(tblDiff.ColumnsDropped, colName)
			}
		}

		// Altered columns
		for colName, targetCol := range targetTable.Columns {
			sourceCol, ok := sourceTable.Columns[colName]
			if !ok {
				continue
			}

			typeChanged := (sourceCol.DataType != targetCol.DataType)
			nullChanged := (sourceCol.IsNullable != targetCol.IsNullable)

			defChanged := false
			if (sourceCol.DefaultValue == nil && targetCol.DefaultValue != nil) ||
				(sourceCol.DefaultValue != nil && targetCol.DefaultValue == nil) {
				defChanged = true
			} else if sourceCol.DefaultValue != nil && targetCol.DefaultValue != nil {
				if *sourceCol.DefaultValue != *targetCol.DefaultValue {
					defChanged = true
				}
			}

			if typeChanged || nullChanged || defChanged {
				tblDiff.ColumnsAltered = append(tblDiff.ColumnsAltered, ColumnAlteration{
					ColumnName:     colName,
					OldType:        sourceCol.DataType,
					NewType:        targetCol.DataType,
					TypeChanged:    typeChanged,
					OldNullable:    sourceCol.IsNullable,
					NewNullable:    targetCol.IsNullable,
					NullableChange: nullChanged,
					OldDefault:     sourceCol.DefaultValue,
					NewDefault:     targetCol.DefaultValue,
					DefaultChanged: defChanged,
				})
			}
		}

		// Added & dropped indexes
		for idxName, targetIdx := range targetTable.Indexes {
			if _, ok := sourceTable.Indexes[idxName]; !ok {
				tblDiff.IndexesAdded = append(tblDiff.IndexesAdded, targetIdx)
			}
		}
		for idxName := range sourceTable.Indexes {
			if _, ok := targetTable.Indexes[idxName]; !ok {
				tblDiff.IndexesDropped = append(tblDiff.IndexesDropped, idxName)
			}
		}

		// Foreign keys
		sourceFkMap := make(map[string]ForeignKey)
		for _, fk := range sourceTable.ForeignKeys {
			sourceFkMap[fk.Name] = fk
		}
		targetFkMap := make(map[string]ForeignKey)
		for _, fk := range targetTable.ForeignKeys {
			targetFkMap[fk.Name] = fk
		}

		for fkName, targetFk := range targetFkMap {
			if _, ok := sourceFkMap[fkName]; !ok {
				tblDiff.ForeignKeysAdded = append(tblDiff.ForeignKeysAdded, targetFk)
			}
		}
		for fkName := range sourceFkMap {
			if _, ok := targetFkMap[fkName]; !ok {
				tblDiff.ForeignKeysDropped = append(tblDiff.ForeignKeysDropped, fkName)
			}
		}

		if tblDiff.HasChanges() {
			diff.TableDiffs[name] = tblDiff
		}
	}

	return diff
}
