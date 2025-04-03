package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rubiojr/hashup/cmd/hs/types"
	hsdb "github.com/rubiojr/hashup/internal/db"
)

func TestSearchHandler(t *testing.T) {
	// Create a temporary database for testing
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Create the database and add some test data
	db, err := hsdb.OpenDatabase(dbPath)
	assert.NoError(t, err)
	defer os.Remove(dbPath)
	defer db.Close()

	// Insert test data
	now := time.Now()
	_, err = db.Exec(`
		INSERT INTO file_hashes (id, file_hash) VALUES
		(1, 'hash1'),
		(2, 'hash2'),
		(3, 'hash3')
	`)
	assert.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO file_info (file_path, file_size, modified_date, hash_id, host, extension, file_hash) VALUES
		('/path/to/testfile1.txt', 1000, ?, 1, 'testhost', 'txt', 'hash1'),
		('/path/to/testfile2.pdf', 2000, ?, 2, 'testhost', 'pdf', 'hash2'),
		('/path/to/testfile3.doc', 3000, ?, 3, 'otherhost', 'doc', 'hash3')
	`, now.Format("2006-01-02 15:04:05"), now.Format("2006-01-02 15:04:05"), now.Format("2006-01-02 15:04:05"))
	assert.NoError(t, err)

	// Create a handler for testing
	handler := searchHandler(dbPath)

	// Test cases
	testCases := []struct {
		name           string
		query          string
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "Search with valid query",
			query:          "testfile",
			expectedStatus: http.StatusOK,
			expectedCount:  3,
		},
		{
			name:           "Search with specific extension",
			query:          "pdf",
			expectedStatus: http.StatusOK,
			expectedCount:  1,
		},
		{
			name:           "Search with no results",
			query:          "nonexistentfile",
			expectedStatus: http.StatusOK,
			expectedCount:  0,
		},
		{
			name:           "Search with empty query",
			query:          "",
			expectedStatus: http.StatusBadRequest,
			expectedCount:  0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a request
			req, err := http.NewRequest("GET", "/search?q="+tc.query, nil)
			assert.NoError(t, err)

			// Create a response recorder
			rr := httptest.NewRecorder()

			// Serve the request
			handler.ServeHTTP(rr, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, rr.Code)

			// If successful, check the response content
			if tc.expectedStatus == http.StatusOK {
				// Parse the response JSON
				var results []*types.FileResult
				err = json.Unmarshal(rr.Body.Bytes(), &results)
				assert.NoError(t, err)

				// Check the number of results
				assert.Equal(t, tc.expectedCount, len(results))

				// If results are expected, check some fields of the first result
				if tc.expectedCount > 0 {
					assert.Contains(t, results[0].FilePath, "testfile")
					assert.NotEmpty(t, results[0].FileHash)
				}
			}
		})
	}
}
