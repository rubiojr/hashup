package types

import "time"

type FileResult struct {
	FilePath     string    `json:"file_path"`
	FileSize     int64     `json:"file_size"`
	ModifiedDate time.Time `json:"modified_date"`
	Host         string    `json:"host"`
	Extension    string    `json:"extension"`
	FileHash     string    `json:"file_hash"`
}
