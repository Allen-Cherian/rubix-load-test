// bulktransfer transfers RBT from each DID in a senders file to a single
// receiver DID. Inverse of cmd/fanouttransfer.
//
// Build:
//
//	go build -o bulktransfer ./cmd/bulktransfer
//
// Run:
//
//	./bulktransfer \
//	    -senders senders.txt \
//	    -receiver <receiver_did> \
//	    -port 20000 \
//	    -concurrency 50 \
//	    -amount 1.0 \
//	    -password mypassword \
//	    -output results/
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/rubixchain/rubix-loadtest/internal/rubix"
	"github.com/rubixchain/rubix-loadtest/internal/runner"
)

func main() {
	sendersFile := flag.String("senders", "", "Path to file with one sender DID per line (required unless -retry-failed)")
	receiver := flag.String("receiver", "", "Receiver DID (required)")
	addr := flag.String("addr", "localhost", "Node address")
	port := flag.Int("port", 20000, "Node port")
	concurrency := flag.Int("concurrency", 50, "Max parallel transfers")
	amount := flag.Float64("amount", 1.0, "RBT amount per transfer")
	password := flag.String("password", "mypassword", "DID unlock password")
	memo := flag.String("memo", "", "Optional memo attached to each transfer")
	outputDir := flag.String("output", "results/", "Output directory for CSV and log")
	retryFailed := flag.String("retry-failed", "", "Path to a previous results CSV; only retry FAIL rows")
	batchSize := flag.Int("batch-size", 1000, "Log progress every N completions")
	timeout := flag.Duration("timeout", 2*time.Minute, "Per-request HTTP timeout")
	flag.Parse()

	if *sendersFile == "" && *retryFailed == "" {
		fmt.Fprintln(os.Stderr, "error: -senders or -retry-failed is required")
		flag.Usage()
		os.Exit(1)
	}
	if *receiver == "" {
		fmt.Fprintln(os.Stderr, "error: -receiver is required")
		flag.Usage()
		os.Exit(1)
	}

	var senders []string
	var err error
	if *retryFailed != "" {
		senders, err = runner.LoadFailedDIDs(*retryFailed)
		if err != nil {
			log.Fatalf("cannot read retry CSV: %v", err)
		}
		log.Printf("Retry mode: loaded %d failed senders from %s", len(senders), *retryFailed)
	} else {
		senders, err = runner.LoadDIDs(*sendersFile, *receiver)
		if err != nil {
			log.Fatalf("cannot read senders file: %v", err)
		}
		log.Printf("Loaded %d senders from %s (deduped, excluding receiver)", len(senders), *sendersFile)
	}

	if len(senders) == 0 {
		log.Fatalf("No senders to process.")
	}

	c := rubix.NewClient(*addr, *port, *timeout)

	log.Printf("Starting bulk transfer: %d senders → receiver=%.20s...  concurrency=%d  node=%s:%d",
		len(senders), *receiver, *concurrency, *addr, *port)

	tasks := make([]runner.Task, len(senders))
	for i, s := range senders {
		tasks[i] = runner.Task{DID: s}
	}

	fn := func(t runner.Task) runner.Result {
		res := c.Transfer(t.DID, *receiver, *amount, *password, *memo)
		status := "FAIL"
		if res.Status {
			status = "SUCCESS"
		}
		return runner.Result{
			DID:     t.DID,
			Status:  status,
			ReqID:   res.ReqID,
			Message: res.Message,
		}
	}

	_, fails, _, err := runner.Run(tasks, fn, runner.Config{
		Concurrency: *concurrency,
		BatchSize:   *batchSize,
		OutputDir:   *outputDir,
		LogPrefix:   "bulk_transfer",
		CSVHeader:   []string{"sender_did", "status", "req_id", "error"},
	})
	if err != nil {
		log.Fatalf("run failed: %v", err)
	}
	if fails > 0 {
		os.Exit(1)
	}
}
