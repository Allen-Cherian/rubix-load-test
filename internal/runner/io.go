package runner

import (
	"bufio"
	"encoding/csv"
	"os"
	"strings"
)

// LoadDIDs reads one DID per line, trimming blanks, deduping, and optionally
// excluding a specific DID (e.g. the sender in a fan-out).
func LoadDIDs(path, exclude string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	seen := make(map[string]struct{})
	var dids []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		d := strings.TrimSpace(scanner.Text())
		if d == "" || d == exclude {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		dids = append(dids, d)
	}
	return dids, scanner.Err()
}

// LoadFailedDIDs reads a results CSV written by this tool and returns the DID
// column for rows whose status is FAIL. Column 0 is the DID, column 1 status.
func LoadFailedDIDs(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var dids []string
	for i, row := range records {
		if i == 0 {
			continue
		}
		if len(row) >= 2 && row[1] == "FAIL" {
			dids = append(dids, row[0])
		}
	}
	return dids, nil
}
