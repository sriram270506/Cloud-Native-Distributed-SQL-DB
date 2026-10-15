package sql

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sriram270506/cloud-native-distributed-sql-db/mvcc"
	"github.com/sriram270506/cloud-native-distributed-sql-db/storage"
)

type Table struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
}

type Result struct {
	Columns []string            `json:"columns,omitempty"`
	Rows    []map[string]string `json:"rows,omitempty"`
	Message string              `json:"message,omitempty"`
}

type Engine struct {
	store *storage.Store
	mvcc  *mvcc.Engine
}

func New(store *storage.Store) (*Engine, error) {
	return &Engine{store: store, mvcc: mvcc.New()}, nil
}

func (e *Engine) Execute(statement string) (Result, error) {
	statement = strings.TrimSpace(strings.TrimSuffix(statement, ";"))
	if statement == "" {
		return Result{}, fmt.Errorf("empty SQL statement")
	}
	lower := strings.ToLower(statement)
	switch {
	case strings.HasPrefix(lower, "create table"):
		return e.create(statement)
	case strings.HasPrefix(lower, "insert"):
		return e.insert(statement)
	case strings.HasPrefix(lower, "select"):
		return e.selectRows(statement)
	case strings.HasPrefix(lower, "update"):
		return e.update(statement)
	case strings.HasPrefix(lower, "delete"):
		return e.delete(statement)
	case strings.HasPrefix(lower, "drop table"):
		return e.dropTable(statement)
	default:
		return Result{}, fmt.Errorf("unsupported SQL statement")
	}
}

func (e *Engine) create(statement string) (Result, error) {
	re := regexp.MustCompile(`(?is)^create\s+table\s+(\w+)\s*\((.+)\)$`)
	match := re.FindStringSubmatch(statement)
	if match == nil {
		return Result{}, fmt.Errorf("invalid CREATE TABLE syntax")
	}
	columns := make([]string, 0)
	for _, definition := range splitCSV(match[2]) {
		parts := strings.Fields(definition)
		if len(parts) == 0 {
			return Result{}, fmt.Errorf("invalid column definition")
		}
		columns = append(columns, strings.ToLower(parts[0]))
	}
	table := Table{Name: strings.ToLower(match[1]), Columns: columns}
	data, _ := json.Marshal(table)
	if err := e.store.Put(schemaKey(table.Name), data); err != nil {
		return Result{}, err
	}
	return Result{Message: "table " + table.Name + " created"}, nil
}

func (e *Engine) insert(statement string) (Result, error) {
	re := regexp.MustCompile(`(?is)^insert\s+into\s+(\w+)\s*\(([^)]*)\)\s*values\s*\((.*)\)$`)
	match := re.FindStringSubmatch(statement)
	if match == nil {
		return Result{}, fmt.Errorf("invalid INSERT syntax")
	}
	table, err := e.table(match[1])
	if err != nil {
		return Result{}, err
	}
	columns := normalizeList(match[2])
	values := splitCSV(match[3])
	if len(columns) != len(values) {
		return Result{}, fmt.Errorf("INSERT column/value count mismatch")
	}
	row := make(map[string]string, len(columns))
	for i, column := range columns {
		if !contains(table.Columns, column) {
			return Result{}, fmt.Errorf("unknown column %q", column)
		}
		row[column] = unquote(values[i])
	}
	id := row[table.Columns[0]]
	if id == "" {
		id = nextID(e.store, table.Name)
		row[table.Columns[0]] = id
	}
	if _, ok := e.store.Get(rowKey(table.Name, id)); ok {
		return Result{}, fmt.Errorf("duplicate primary key %q", id)
	}
	if err := e.commitRow(table.Name, id, row); err != nil {
		return Result{}, err
	}
	return Result{Message: "1 row inserted"}, nil
}

func (e *Engine) selectRows(statement string) (Result, error) {
	re := regexp.MustCompile(`(?is)^select\s+(.*?)\s+from\s+(\w+)(?:\s+(?:inner\s+)?join\s+(\w+)\s+on\s+([\w.]+)\s*=\s*([\w.]+))?(?:\s+where\s+(.*?))?(?:\s+group\s+by\s+([\w.,\s]+?))?(?:\s+having\s+(.*?))?(?:\s+order\s+by\s+([\w.]+)(?:\s+(asc|desc))?)?(?:\s+limit\s+(\d+))?(?:\s+offset\s+(\d+))?$`)
	match := re.FindStringSubmatch(statement)
	if match == nil {
		return Result{}, fmt.Errorf("invalid SELECT syntax")
	}
	table, err := e.table(match[2])
	if err != nil {
		return Result{}, err
	}
	availableColumns := table.Columns
	var joinedTable Table
	if match[3] != "" {
		joinedTable, err = e.table(match[3])
		if err != nil {
			return Result{}, err
		}
		availableColumns = append(append([]string{}, table.Columns...), joinedTable.Columns...)
	}
	columns, aggregates, err := selectColumns(match[1], availableColumns)
	if err != nil {
		return Result{}, err
	}
	filter, err := parseFilter(match[6], availableColumns)
	if err != nil {
		return Result{}, err
	}
	rows := make([]map[string]string, 0)
	for _, data := range e.store.Scan(rowPrefix(table.Name)) {
		var row map[string]string
		if err := json.Unmarshal(data, &row); err != nil {
			return Result{}, err
		}
		if match[3] == "" {
			if filter(row) {
				rows = append(rows, row)
			}
			continue
		}
		leftColumn, rightColumn := strings.ToLower(match[4]), strings.ToLower(match[5])
		for _, joinedData := range e.store.Scan(rowPrefix(joinedTable.Name)) {
			var rightRow map[string]string
			if err := json.Unmarshal(joinedData, &rightRow); err != nil {
				return Result{}, err
			}
			combined := combineRows(row, rightRow, table.Name, joinedTable.Name)
			if combined[columnValueKey(combined, table.Name, leftColumn)] ==
				combined[columnValueKey(combined, joinedTable.Name, rightColumn)] && filter(combined) {
				rows = append(rows, combined)
			}
		}
	}
	if match[7] != "" {
		groupColumns := normalizeList(match[7])
		for _, column := range groupColumns {
			if !contains(availableColumns, unqualifiedColumn(column)) {
				return Result{}, fmt.Errorf("unknown GROUP BY column %q", column)
			}
		}
		rows = groupRows(rows, groupColumns, aggregates)
	} else if len(aggregates) > 0 {
		rows = []map[string]string{aggregateRows(rows, aggregates)}
	}
	if match[8] != "" {
		having, err := parseFilter(match[8], append(append([]string{}, availableColumns...), aggregateNames(aggregates)...))
		if err != nil {
			return Result{}, err
		}
		filtered := rows[:0]
		for _, row := range rows {
			if having(row) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	if match[9] != "" {
		orderColumn := strings.ToLower(match[9])
		if !contains(availableColumns, unqualifiedColumn(orderColumn)) && !contains(aggregateNames(aggregates), orderColumn) {
			return Result{}, fmt.Errorf("unknown ORDER BY column %q", orderColumn)
		}
		desc := strings.EqualFold(match[10], "desc")
		sort.SliceStable(rows, func(i, j int) bool {
			left, right := rows[i][columnValueKey(rows[i], "", orderColumn)], rows[j][columnValueKey(rows[j], "", orderColumn)]
			if leftNumber, leftErr := strconv.ParseFloat(left, 64); leftErr == nil {
				if rightNumber, rightErr := strconv.ParseFloat(right, 64); rightErr == nil {
					if desc {
						return leftNumber > rightNumber
					}
					return leftNumber < rightNumber
				}
			}
			if desc {
				return left > right
			}
			return left < right
		})
	}
	offset, limit := 0, 0
	if match[11] != "" {
		limit, _ = strconv.Atoi(match[11])
	}
	if match[12] != "" {
		offset, _ = strconv.Atoi(match[12])
	}
	if offset > len(rows) {
		offset = len(rows)
	}
	rows = rows[offset:]
	if limit > 0 && limit < len(rows) {
		rows = rows[:limit]
	}
	selectedRows := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		selected := make(map[string]string, len(columns))
		for _, column := range columns {
			selected[column] = row[column]
		}
		selectedRows = append(selectedRows, selected)
	}
	return Result{Columns: columns, Rows: selectedRows}, nil
}

type aggregate struct {
	function string
	column   string
	name     string
}

func selectColumns(expression string, tableColumns []string) ([]string, []aggregate, error) {
	if strings.TrimSpace(expression) == "*" {
		return tableColumns, nil, nil
	}
	columns := make([]string, 0)
	aggregates := make([]aggregate, 0)
	for _, item := range splitCSV(expression) {
		item = strings.TrimSpace(item)
		aggregateMatch := regexp.MustCompile(`(?i)^(count|sum|avg|min|max)\s*\(\s*(\*|[\w]+)\s*\)(?:\s+as\s+([\w]+))?$`).FindStringSubmatch(item)
		if aggregateMatch != nil {
			column := strings.ToLower(aggregateMatch[2])
			if column != "*" && !contains(tableColumns, unqualifiedColumn(column)) {
				return nil, nil, fmt.Errorf("unknown column %q", column)
			}
			name := strings.ToLower(aggregateMatch[3])
			if name == "" {
				name = strings.ToLower(aggregateMatch[1]) + "_" + column
			}
			aggregates = append(aggregates, aggregate{strings.ToLower(aggregateMatch[1]), column, name})
			columns = append(columns, name)
			continue
		}
		column := strings.ToLower(item)
		if !contains(tableColumns, unqualifiedColumn(column)) {
			return nil, nil, fmt.Errorf("unknown column %q", column)
		}
		columns = append(columns, column)
	}
	return columns, aggregates, nil
}

func parseFilter(expression string, tableColumns []string) (func(map[string]string) bool, error) {
	if strings.TrimSpace(expression) == "" {
		return func(map[string]string) bool { return true }, nil
	}
	orGroups := regexp.MustCompile(`(?i)\s+or\s+`).Split(strings.TrimSpace(expression), -1)
	groups := make([][]func(map[string]string) bool, 0, len(orGroups))
	for _, orGroup := range orGroups {
		parts := regexp.MustCompile(`(?i)\s+and\s+`).Split(strings.TrimSpace(orGroup), -1)
		conditions := make([]func(map[string]string) bool, 0, len(parts))
		for _, part := range parts {
			match := regexp.MustCompile(`(?is)^([\w.]+)\s*(>=|<=|!=|<>|=|>|<|like)\s*(.+)$`).FindStringSubmatch(strings.TrimSpace(part))
			if match == nil {
				return nil, fmt.Errorf("invalid WHERE condition %q", part)
			}
			column := strings.ToLower(match[1])
			if !contains(tableColumns, unqualifiedColumn(column)) && !contains(tableColumns, column) {
				return nil, fmt.Errorf("unknown column %q", column)
			}
			operator, expected := strings.ToLower(match[2]), unquote(match[3])
			conditions = append(conditions, func(row map[string]string) bool {
				actual := row[columnValueKey(row, "", column)]
				if actualNumber, actualErr := strconv.ParseFloat(actual, 64); actualErr == nil {
					if expectedNumber, expectedErr := strconv.ParseFloat(expected, 64); expectedErr == nil {
						switch operator {
						case ">":
							return actualNumber > expectedNumber
						case ">=":
							return actualNumber >= expectedNumber
						case "<":
							return actualNumber < expectedNumber
						case "<=":
							return actualNumber <= expectedNumber
						}
					}
				}
				switch operator {
				case "=":
					return actual == expected
				case "!=", "<>":
					return actual != expected
				case "like":
					pattern := strings.ReplaceAll(regexp.QuoteMeta(expected), "%", ".*")
					matched, _ := regexp.MatchString("(?i)^"+pattern+"$", actual)
					return matched
				case ">":
					return actual > expected
				case ">=":
					return actual >= expected
				case "<":
					return actual < expected
				case "<=":
					return actual <= expected
				default:
					return false
				}
			})
		}
		groups = append(groups, conditions)
	}
	return func(row map[string]string) bool {
		for _, conditions := range groups {
			matches := true
			for _, condition := range conditions {
				if !condition(row) {
					matches = false
					break
				}
			}
			if matches {
				return true
			}
		}
		return false
	}, nil
}

func groupRows(rows []map[string]string, columns []string, aggregates []aggregate) []map[string]string {
	groups := make(map[string][]map[string]string)
	order := make([]string, 0)
	for _, row := range rows {
		keyParts := make([]string, 0, len(columns))
		for _, column := range columns {
			keyParts = append(keyParts, row[column])
		}
		key := strings.Join(keyParts, "\x00")
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		groups[key] = append(groups[key], row)
	}
	result := make([]map[string]string, 0, len(order))
	for _, key := range order {
		group := groups[key]
		row := make(map[string]string)
		for _, column := range columns {
			row[column] = group[0][column]
		}
		result = append(result, mergeAggregates(row, group, aggregates))
	}
	return result
}

func aggregateRows(rows []map[string]string, aggregates []aggregate) map[string]string {
	return mergeAggregates(make(map[string]string), rows, aggregates)
}

func mergeAggregates(row map[string]string, rows []map[string]string, aggregates []aggregate) map[string]string {
	for _, item := range aggregates {
		if item.function == "count" {
			row[item.name] = strconv.Itoa(len(rows))
			continue
		}
		total := 0.0
		values := make([]float64, 0)
		for _, source := range rows {
			value, err := strconv.ParseFloat(source[item.column], 64)
			if err == nil {
				total += value
				values = append(values, value)
			}
		}
		switch item.function {
		case "sum":
			row[item.name] = strconv.FormatFloat(total, 'f', -1, 64)
		case "avg":
			if len(values) > 0 {
				row[item.name] = strconv.FormatFloat(total/float64(len(values)), 'f', -1, 64)
			}
		case "min", "max":
			if len(values) > 0 {
				sort.Float64s(values)
				index := 0
				if item.function == "max" {
					index = len(values) - 1
				}
				row[item.name] = strconv.FormatFloat(values[index], 'f', -1, 64)
			}
		}
	}
	return row
}

func aggregateNames(aggregates []aggregate) []string {
	names := make([]string, 0, len(aggregates))
	for _, item := range aggregates {
		names = append(names, item.name)
	}
	return names
}

func (e *Engine) update(statement string) (Result, error) {
	re := regexp.MustCompile(`(?is)^update\s+(\w+)\s+set\s+(\w+)\s*=\s*('(?:''|[^'])*'|[^\s]+)\s+where\s+(\w+)\s*=\s*('(?:''|[^'])*'|[^\s]+)$`)
	match := re.FindStringSubmatch(statement)
	if match == nil {
		return Result{}, fmt.Errorf("invalid UPDATE syntax")
	}
	table, err := e.table(match[1])
	if err != nil {
		return Result{}, err
	}
	if !contains(table.Columns, strings.ToLower(match[2])) || !contains(table.Columns, strings.ToLower(match[4])) {
		return Result{}, fmt.Errorf("unknown column")
	}
	count := 0
	for key, data := range e.store.Scan(rowPrefix(table.Name)) {
		var row map[string]string
		if err := json.Unmarshal(data, &row); err != nil {
			return Result{}, err
		}
		if row[strings.ToLower(match[4])] != unquote(match[5]) {
			continue
		}
		row[strings.ToLower(match[2])] = unquote(match[3])
		id := strings.TrimPrefix(key, rowPrefix(table.Name))
		if err := e.commitRow(table.Name, id, row); err != nil {
			return Result{}, err
		}
		count++
	}
	return Result{Message: fmt.Sprintf("%d rows updated", count)}, nil
}

func (e *Engine) delete(statement string) (Result, error) {
	re := regexp.MustCompile(`(?is)^delete\s+from\s+(\w+)\s+where\s+(\w+)\s*=\s*('(?:''|[^'])*'|[^\s]+)$`)
	match := re.FindStringSubmatch(statement)
	if match == nil {
		return Result{}, fmt.Errorf("invalid DELETE syntax")
	}
	table, err := e.table(match[1])
	if err != nil {
		return Result{}, err
	}
	if !contains(table.Columns, strings.ToLower(match[2])) {
		return Result{}, fmt.Errorf("unknown column")
	}
	count := 0
	for key, data := range e.store.Scan(rowPrefix(table.Name)) {
		var row map[string]string
		if err := json.Unmarshal(data, &row); err != nil {
			return Result{}, err
		}
		if row[strings.ToLower(match[2])] == unquote(match[3]) {
			if err := e.store.Delete(key); err != nil {
				return Result{}, err
			}
			count++
		}
	}
	return Result{Message: fmt.Sprintf("%d rows deleted", count)}, nil
}

func (e *Engine) dropTable(statement string) (Result, error) {
	re := regexp.MustCompile(`(?is)^drop\s+table\s+(\w+)$`)
	match := re.FindStringSubmatch(statement)
	if match == nil {
		return Result{}, fmt.Errorf("invalid DROP TABLE syntax")
	}
	table := strings.ToLower(match[1])
	if _, err := e.table(table); err != nil {
		return Result{}, err
	}
	if err := e.store.Delete(schemaKey(table)); err != nil {
		return Result{}, err
	}
	for _, key := range e.store.Keys(rowPrefix(table)) {
		if err := e.store.Delete(key); err != nil {
			return Result{}, err
		}
	}
	return Result{Message: "table " + table + " dropped"}, nil
}

func (e *Engine) table(name string) (Table, error) {
	data, ok := e.store.Get(schemaKey(strings.ToLower(name)))
	if !ok {
		return Table{}, fmt.Errorf("table %q does not exist", name)
	}
	var table Table
	if err := json.Unmarshal(data, &table); err != nil {
		return Table{}, err
	}
	return table, nil
}

func (e *Engine) commitRow(table, id string, row map[string]string) error {
	tx := e.mvcc.Begin()
	data, _ := json.Marshal(row)
	tx.Put(rowKey(table, id), data)
	if _, err := tx.Commit(); err != nil {
		return err
	}
	return e.store.Put(rowKey(table, id), data)
}

func schemaKey(table string) string  { return "schema:" + table }
func rowPrefix(table string) string  { return "row:" + table + ":" }
func rowKey(table, id string) string { return rowPrefix(table) + id }

func nextID(store *storage.Store, table string) string {
	max := 0
	for key := range store.Scan(rowPrefix(table)) {
		id, _ := strconv.Atoi(strings.TrimPrefix(key, rowPrefix(table)))
		if id > max {
			max = id
		}
	}
	return strconv.Itoa(max + 1)
}
func normalizeList(value string) []string {
	result := make([]string, 0)
	for _, item := range strings.Split(value, ",") {
		result = append(result, strings.ToLower(strings.TrimSpace(item)))
	}
	return result
}

func unqualifiedColumn(column string) string {
	if index := strings.LastIndex(column, "."); index >= 0 {
		return column[index+1:]
	}
	return column
}

func columnValueKey(row map[string]string, table, column string) string {
	column = strings.ToLower(column)
	if _, ok := row[column]; ok {
		return column
	}
	if table != "" {
		qualified := strings.ToLower(table) + "." + unqualifiedColumn(column)
		if _, ok := row[qualified]; ok {
			return qualified
		}
	}
	return unqualifiedColumn(column)
}

func combineRows(left, right map[string]string, leftTable, rightTable string) map[string]string {
	combined := make(map[string]string, len(left)+len(right)*2)
	for column, value := range left {
		combined[column] = value
		combined[strings.ToLower(leftTable)+"."+column] = value
	}
	for column, value := range right {
		combined[strings.ToLower(rightTable)+"."+column] = value
		if _, exists := combined[column]; !exists {
			combined[column] = value
		}
	}
	return combined
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func unquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'")
	}
	return value
}
func splitCSV(value string) []string {
	result := make([]string, 0)
	start, quoted := 0, false
	for i, char := range value {
		if char == '\'' {
			quoted = !quoted
		}
		if char == ',' && !quoted {
			result = append(result, strings.TrimSpace(value[start:i]))
			start = i + 1
		}
	}
	result = append(result, strings.TrimSpace(value[start:]))
	return result
}

func (e *Engine) Tables() []Table {
	result := make([]Table, 0)
	for key, data := range e.store.Scan("schema:") {
		var table Table
		if json.Unmarshal(data, &table) == nil && strings.HasPrefix(key, "schema:") {
			result = append(result, table)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
