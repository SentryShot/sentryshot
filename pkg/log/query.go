package log

import (
	"database/sql"
	"log"
	"strconv"
)

// Query database query.
type Query struct {
	Levels   []Level
	Time     UnixMillisecond
	Sources  []string
	Monitors []string
	Limit    int
}

// Query for logs in database.
func (l *Logger) Query(q Query) (*[]Log, error) {
	sqlStmt := "SELECT time,level,src,monitor,msg FROM logs"
	sqlStmt += " WHERE level " + genIN(len(q.Levels))
	sqlStmt += " AND src " + genIN(len(q.Sources))

	if len(q.Monitors) != 0 {
		sqlStmt += " AND monitor " + genIN(len(q.Monitors))
	}

	if q.Time != 0 {
		sqlStmt += " AND time < (?)"
	}

	sqlStmt += " ORDER BY time DESC"

	if q.Limit != 0 {
		sqlStmt += " LIMIT " + strconv.Itoa(q.Limit)
	}

	stmt, err := l.db.Prepare(sqlStmt)
	if err != nil {
		return nil, err
	}

	args := []interface{}{}
	args = append(args, levelsToInterfaces(q.Levels)...)
	args = append(args, stringsToInterfaces(q.Sources)...)
	args = append(args, stringsToInterfaces(q.Monitors)...)
	if q.Time != 0 {
		args = append(args, q.Time)
	}

	rows, err := stmt.Query(args...)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	return parseRows(rows)
}

func parseRows(rows *sql.Rows) (*[]Log, error) {
	var logs []Log
	for rows.Next() {
		var t UnixMillisecond
		var level uint8
		var src string
		var monitor string
		var msg string

		err := rows.Scan(&t, &level, &src, &monitor, &msg)
		if err != nil {
			return nil, err
		}

		log := Log{
			Time:    t,
			Level:   Level(level),
			Src:     src,
			Monitor: monitor,
			Msg:     msg,
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &logs, nil
}

func genIN(n int) string {
	// Input: 1 Output: "IN (?)"
	// Input: 2 Output: "IN (?, ?)"
	output := "IN ("
	for i := 1; i <= n; i++ {
		if i != n {
			output += "?, "
		} else {
			output += "?"
		}
	}
	return output + ")"
}

// GO1.18 Generics
func levelsToInterfaces(slice []Level) []interface{} {
	output := make([]interface{}, len(slice))
	for i, v := range slice {
		output[i] = v
	}
	return output
}

func stringsToInterfaces(slice []string) []interface{} {
	output := make([]interface{}, len(slice))
	for i, v := range slice {
		output[i] = v
	}
	return output
}
