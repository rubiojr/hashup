package api

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/dustin/go-humanize"
	_ "github.com/mattn/go-sqlite3"
)

type ExtensionStat struct {
	Extension string `json:"extension"`
	Count     int64  `json:"count"`
	Size      int64  `json:"size"`
	SizeHuman string `json:"size_human"`
}

type ExtensionStats struct {
	Stats      []*ExtensionStat `json:"stats"`
	TotalCount int64            `json:"total_count"`
	TotalSize  int64            `json:"total_size"`
}

type Stats struct {
	Host           string           `json:"host,omitempty"`
	Extensions     []*ExtensionStat `json:"extensions"`
	TotalCount     int64            `json:"total_count"`
	TotalSize      int64            `json:"total_size"`
	Count          int64            `json:"count"`
	Size           int64            `json:"size"`
	TotalSizeHuman string           `json:"total_size_human"`
	Limit          int              `json:"limit"`
	OtherCount     int64            `json:"other_count,omitempty"`
	OtherSize      int64            `json:"other_size,omitempty"`
	OtherSizeHuman string           `json:"other_size_human,omitempty"`
}

func fileStats(db *sql.DB, orderBy string, descending bool, host string, limit int) (*Stats, error) {
	validColumns := map[string]string{
		"file_size": "total_size",
		"size":      "total_size",
		"count":     "count",
		"extension": "extension",
	}

	column, ok := validColumns[orderBy]
	if !ok {
		column = "total_size"
	}

	sortOrder := "ASC"
	if descending {
		sortOrder = "DESC"
	}

	var rows *sql.Rows
	var err error

	if host == "" {
		query := fmt.Sprintf(`
			SELECT extension, COUNT(*) as count, SUM(file_size) AS total_size
			FROM file_info
			GROUP BY extension COLLATE NOCASE
			ORDER BY %s %s
		`, column, sortOrder)

		rows, err = db.Query(query)
	} else {
		query := fmt.Sprintf(`
			SELECT extension, COUNT(*) as count, SUM(file_size) AS total_size
			FROM file_info
			WHERE host = ?
			GROUP BY extension COLLATE NOCASE
			ORDER BY %s %s
		`, column, sortOrder)

		rows, err = db.Query(query, host)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query database: %v", err)
	}
	defer rows.Close()

	all := &ExtensionStats{
		TotalSize:  0,
		TotalCount: 0,
		Stats:      []*ExtensionStat{},
	}

	for rows.Next() {
		var count, sum int64
		var extension string

		err := rows.Scan(&extension, &count, &sum)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %v", err)
		}

		all.TotalCount += count
		all.TotalSize += sum

		if extension == "" || extension == "unknown" {
			//extension = "[no extension]"
			continue
		}

		all.Stats = append(all.Stats, &ExtensionStat{
			Extension: strings.ToLower(extension),
			Count:     count,
			Size:      sum,
			SizeHuman: humanize.Bytes(uint64(sum)),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %v", err)
	}

	return jsonStats(all, host, limit)
}

func jsonStats(estats *ExtensionStats, host string, limit int) (*Stats, error) {
	stats := estats.Stats
	count := len(estats.Stats)

	var otherCount, otherSize int64
	if len(stats) > limit {
		for i := limit; i < len(stats); i++ {
			otherCount += stats[i].Count
			otherSize += stats[i].Size
		}
		// Trim the stats list to the limit
		stats = stats[:limit]
	}

	if count > limit {
		count = limit
	}

	response := &Stats{
		Extensions:     stats,
		Count:          int64(count),
		Size:           estats.TotalSize - otherSize,
		TotalCount:     estats.TotalCount,
		TotalSize:      estats.TotalSize,
		TotalSizeHuman: humanize.Bytes(uint64(estats.TotalSize)),
		Limit:          limit,
	}

	if host != "" {
		response.Host = host
	}

	// Include "Other" category in the JSON response if there are items beyond the limit
	if otherCount > 0 {
		response.OtherCount = otherCount
		response.OtherSize = otherSize
		response.OtherSizeHuman = humanize.Bytes(uint64(otherSize))
	}

	return response, nil
}
