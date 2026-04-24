package runner

import (
	"bufio"
	"encoding/csv"
	"os"
	"strconv"
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

// LoadFailedTasks reads a results CSV written by this tool and returns Tasks
// for rows whose status is FAIL. Expected columns:
// sender, receiver, amount, transaction_id, status, error.
func LoadFailedTasks(path string) ([]Task, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // tolerate ragged rows just in case
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var tasks []Task
	for i, row := range records {
		if i == 0 || len(row) < 5 {
			continue
		}
		if row[4] != "FAIL" {
			continue
		}
		amt, _ := strconv.ParseFloat(row[2], 64)
		tasks = append(tasks, Task{
			Sender:   row[0],
			Receiver: row[1],
			Amount:   amt,
		})
	}
	return tasks, nil
}
