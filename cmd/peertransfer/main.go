// peertransfer transfers RBT from N senders to N receivers, paired.
//
// Build:
//
//	go build -o peertransfer ./cmd/peertransfer
//
// Run:
//
//	./peertransfer \
//	    -senders senders.txt \
//	    -receivers receivers.txt \
//	    -count 500 \
//	    -pair zip \
//	    -amount 1.0 \
//	    -concurrency 50 \
//	    -port 20000 \
//	    -password mypassword \
//	    -output results/
package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/rubixchain/rubix-loadtest/internal/rubix"
	"github.com/rubixchain/rubix-loadtest/internal/runner"
)

func main() {
	sendersFile := flag.String("senders", "", "Path to file with one sender DID per line (required unless -retry-failed)")
	receiversFile := flag.String("receivers", "", "Path to file with one receiver DID per line (required unless -retry-failed)")
	count := flag.Int("count", 500, "Number of pairs to execute (capped by min(|senders|, |receivers|))")
	pairMode := flag.String("pair", "zip", "Pairing strategy: zip | random")
	seed := flag.Int64("random-seed", 0, "Seed for -pair random (0 = time-based)")
	addr := flag.String("addr", "localhost", "Node address")
	port := flag.Int("port", 20000, "Node port")
	concurrency := flag.Int("concurrency", 50, "Max parallel transfers")
	amount := flag.Float64("amount", 1.0, "RBT amount per transfer")
	password := flag.String("password", "mypassword", "DID unlock password")
	memo := flag.String("memo", "", "Optional memo attached to each transfer")
	outputDir := flag.String("output", "results/", "Output directory for CSV and log")
	retryFailed := flag.String("retry-failed", "", "Path to a previous results CSV; only retry FAIL rows")
	batchSize := flag.Int("batch-size", 100, "Log progress every N completions")
	timeout := flag.Duration("timeout", 2*time.Minute, "Per-request HTTP timeout")
	flag.Parse()

	if *retryFailed == "" && (*sendersFile == "" || *receiversFile == "") {
		fmt.Fprintln(os.Stderr, "error: both -senders and -receivers are required (or use -retry-failed)")
		flag.Usage()
		os.Exit(1)
	}
	if *pairMode != "zip" && *pairMode != "random" {
		fmt.Fprintln(os.Stderr, "error: -pair must be 'zip' or 'random'")
		os.Exit(1)
	}

	var tasks []runner.Task
	var err error

	if *retryFailed != "" {
		tasks, err = runner.LoadFailedTasks(*retryFailed)
		if err != nil {
			log.Fatalf("cannot read retry CSV: %v", err)
		}
		log.Printf("Retry mode: loaded %d failed pairs from %s", len(tasks), *retryFailed)
	} else {
		senders, err := runner.LoadDIDs(*sendersFile, "")
		if err != nil {
			log.Fatalf("cannot read senders file: %v", err)
		}
		receivers, err := runner.LoadDIDs(*receiversFile, "")
		if err != nil {
			log.Fatalf("cannot read receivers file: %v", err)
		}
		log.Printf("Loaded %d senders, %d receivers (deduped)", len(senders), len(receivers))

		tasks = buildPairs(senders, receivers, *count, *pairMode, *seed, *amount)
		log.Printf("Built %d pairs using pair=%s", len(tasks), *pairMode)
	}

	if len(tasks) == 0 {
		log.Fatalf("No pairs to process.")
	}

	c := rubix.NewClient(*addr, *port, *timeout)

	log.Printf("Planned: %d transfers × %.4f RBT = %.4f RBT total",
		len(tasks), *amount, float64(len(tasks))*(*amount))
	log.Printf("Starting peer transfer: pairs=%d  concurrency=%d  node=%s:%d",
		len(tasks), *concurrency, *addr, *port)

	fn := func(t runner.Task) runner.Result {
		res := c.Transfer(t.Sender, t.Receiver, t.Amount, *password, *memo)
		status := "FAIL"
		if res.Status {
			status = "SUCCESS"
		}
		return runner.Result{
			Sender:        t.Sender,
			Receiver:      t.Receiver,
			Amount:        t.Amount,
			TransactionID: res.TransactionID,
			Status:        status,
			Message:       res.Message,
		}
	}

	_, fails, _, err := runner.Run(tasks, fn, runner.Config{
		Concurrency: *concurrency,
		BatchSize:   *batchSize,
		OutputDir:   *outputDir,
		LogPrefix:   "peer_transfer",
		CSVHeader:   []string{"sender_did", "receiver_did", "amount", "transaction_id", "status", "error"},
	})
	if err != nil {
		log.Fatalf("run failed: %v", err)
	}
	if fails > 0 {
		os.Exit(1)
	}
}

// buildPairs produces up to count (sender, receiver) tasks. zip pairs
// index-for-index after truncating both lists to the same length. random
// shuffles receivers first, then zips — so each sender and each receiver
// still appears at most once per run.
func buildPairs(senders, receivers []string, count int, mode string, seed int64, amount float64) []runner.Task {
	n := len(senders)
	if len(receivers) < n {
		n = len(receivers)
	}
	if count > 0 && count < n {
		n = count
	}
	if n == 0 {
		return nil
	}

	rcv := receivers[:n]
	snd := senders[:n]

	if mode == "random" {
		if seed == 0 {
			seed = time.Now().UnixNano()
		}
		r := rand.New(rand.NewSource(seed))
		cp := make([]string, len(receivers))
		copy(cp, receivers)
		r.Shuffle(len(cp), func(i, j int) { cp[i], cp[j] = cp[j], cp[i] })
		rcv = cp[:n]
	}

	tasks := make([]runner.Task, n)
	for i := 0; i < n; i++ {
		tasks[i] = runner.Task{Sender: snd[i], Receiver: rcv[i], Amount: amount}
	}
	return tasks
}
