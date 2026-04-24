package runner

import (
	"encoding/csv"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Task describes one transfer to execute.
type Task struct {
	Sender   string
	Receiver string
	Amount   float64
}

// Result is the outcome of one Task. TransactionID is populated only on
// SUCCESS (a non-empty transactionID returned by the signature response).
type Result struct {
	Sender        string
	Receiver      string
	Amount        float64
	TransactionID string
	Status        string // "SUCCESS" or "FAIL"
	Message       string
}

// TransferFn executes one task and returns its result.
type TransferFn func(t Task) Result

// Config controls how a run is executed and logged.
type Config struct {
	Concurrency int
	BatchSize   int    // log progress every N completions
	OutputDir   string // CSV + log destination
	LogPrefix   string // e.g. "fanout_transfer" or "bulk_transfer"
	CSVHeader   []string
}

// Run executes the given tasks through fn with a bounded worker pool. It writes
// a CSV and a log file named "<prefix>_<UTC-ts>.{csv,log}" inside cfg.OutputDir,
// mirrors the log to stdout, and returns (successes, failures, total).
func Run(tasks []Task, fn TransferFn, cfg Config) (int64, int64, int64, error) {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 50
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return 0, 0, 0, err
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	logPath := filepath.Join(cfg.OutputDir, cfg.LogPrefix+"_"+ts+".log")
	csvPath := filepath.Join(cfg.OutputDir, cfg.LogPrefix+"_"+ts+".csv")

	logFile, err := os.Create(logPath)
	if err != nil {
		return 0, 0, 0, err
	}
	defer logFile.Close()

	stdLog := log.New(os.Stdout, "", log.LstdFlags)
	fileLog := log.New(logFile, "", log.LstdFlags)
	logBoth := func(format string, args ...any) {
		stdLog.Printf(format, args...)
		fileLog.Printf(format, args...)
	}

	csvFile, err := os.Create(csvPath)
	if err != nil {
		return 0, 0, 0, err
	}
	defer csvFile.Close()

	csvWriter := csv.NewWriter(csvFile)
	_ = csvWriter.Write(cfg.CSVHeader)

	var (
		mu      sync.Mutex
		success int64
		fails   int64
		done    int64
		total   = int64(len(tasks))
	)

	tStart := time.Now()
	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup
	resultCh := make(chan Result, cfg.Concurrency*2)

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for r := range resultCh {
			row := []string{
				r.Sender,
				r.Receiver,
				strconv.FormatFloat(r.Amount, 'f', -1, 64),
				r.TransactionID,
				r.Status,
				r.Message,
			}
			mu.Lock()
			_ = csvWriter.Write(row)
			mu.Unlock()

			n := atomic.AddInt64(&done, 1)
			if r.Status == "SUCCESS" {
				atomic.AddInt64(&success, 1)
			} else {
				atomic.AddInt64(&fails, 1)
				logBoth("FAIL  sender=%.20s... receiver=%.20s... error=%s",
					r.Sender, r.Receiver, r.Message)
			}
			if n%int64(cfg.BatchSize) == 0 && total > 0 {
				elapsed := time.Since(tStart).Seconds()
				tps := float64(n) / elapsed
				eta := float64(total-n) / tps
				logBoth("Progress %d/%d  success=%d  fail=%d  tps=%.1f  eta=%.0fs",
					n, total, atomic.LoadInt64(&success), atomic.LoadInt64(&fails), tps, eta)
			}
		}
	}()

	for _, t := range tasks {
		wg.Add(1)
		sem <- struct{}{}
		go func(task Task) {
			defer wg.Done()
			defer func() { <-sem }()
			resultCh <- fn(task)
		}(t)
	}

	wg.Wait()
	close(resultCh)
	<-writerDone
	csvWriter.Flush()

	elapsed := time.Since(tStart).Seconds()
	s := atomic.LoadInt64(&success)
	f := atomic.LoadInt64(&fails)
	tps := float64(total) / elapsed
	logBoth("Done: %d success, %d fail out of %d in %.1fs (%.1f tps)", s, f, total, elapsed, tps)
	logBoth("Results CSV : %s", csvPath)
	logBoth("Log file    : %s", logPath)
	return s, f, total, nil
}

// PickSubset returns up to count items from all using the given mode
// ("head" or "random"). Seed 0 means time-based randomness.
func PickSubset(all []string, count int, mode string, seed int64) []string {
	if count <= 0 || count >= len(all) {
		return all
	}
	switch mode {
	case "random":
		if seed == 0 {
			seed = time.Now().UnixNano()
		}
		r := rand.New(rand.NewSource(seed))
		cp := make([]string, len(all))
		copy(cp, all)
		r.Shuffle(len(cp), func(i, j int) { cp[i], cp[j] = cp[j], cp[i] })
		return cp[:count]
	default:
		return all[:count]
	}
}
