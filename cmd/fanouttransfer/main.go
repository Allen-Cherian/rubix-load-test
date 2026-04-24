// fanouttransfer transfers RBT from ONE sender DID to N receiver DIDs picked
// from a large receivers file.
//
// Build:
//
//	go build -o fanouttransfer ./cmd/fanouttransfer
//
// Run:
//
//	./fanouttransfer \
//	    -sender <sender_did> \
//	    -receivers receivers.txt \
//	    -count 500 \
//	    -select head \
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
	sender := flag.String("sender", "", "Sender DID holding the tokens (required)")
	receiversFile := flag.String("receivers", "", "Path to file with one receiver DID per line (required unless -retry-failed)")
	count := flag.Int("count", 500, "Number of receivers to pick from the receivers file")
	selectMode := flag.String("select", "head", "How to pick receivers: head | random")
	seed := flag.Int64("random-seed", 0, "Seed for -select random (0 = time-based)")
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
	skipBalanceCheck := flag.Bool("skip-balance-check", false, "Skip the pre-flight sender balance check")
	flag.Parse()

	if *sender == "" {
		fmt.Fprintln(os.Stderr, "error: -sender is required")
		flag.Usage()
		os.Exit(1)
	}
	if *receiversFile == "" && *retryFailed == "" {
		fmt.Fprintln(os.Stderr, "error: -receivers or -retry-failed is required")
		flag.Usage()
		os.Exit(1)
	}
	if *selectMode != "head" && *selectMode != "random" {
		fmt.Fprintln(os.Stderr, "error: -select must be 'head' or 'random'")
		os.Exit(1)
	}

	var receivers []string
	var err error
	if *retryFailed != "" {
		receivers, err = runner.LoadFailedDIDs(*retryFailed)
		if err != nil {
			log.Fatalf("cannot read retry CSV: %v", err)
		}
		log.Printf("Retry mode: loaded %d failed receivers from %s", len(receivers), *retryFailed)
	} else {
		all, err := runner.LoadDIDs(*receiversFile, *sender)
		if err != nil {
			log.Fatalf("cannot read receivers file: %v", err)
		}
		log.Printf("Loaded %d receivers from %s (deduped, excluding sender)", len(all), *receiversFile)
		receivers = runner.PickSubset(all, *count, *selectMode, *seed)
		log.Printf("Picked %d receivers using select=%s", len(receivers), *selectMode)
	}

	if len(receivers) == 0 {
		log.Fatalf("No receivers to process.")
	}

	c := rubix.NewClient(*addr, *port, *timeout)

	needed := float64(len(receivers)) * (*amount)
	if !*skipBalanceCheck {
		bal, raw, err := c.GetRBTBalance(*sender)
		switch {
		case err != nil:
			log.Printf("WARN: balance check failed (%v) — proceeding anyway", err)
		case bal == nil:
			log.Printf("WARN: balance check returned no data (status=%v message=%s) — proceeding anyway",
				raw.Status, raw.Message)
		default:
			log.Printf("Sender balance: free=%.4f locked=%.4f", bal.Balance, bal.Locked)
			if bal.Balance < needed {
				log.Fatalf("insufficient balance: need %.4f RBT but sender has %.4f free (use -skip-balance-check to override)",
					needed, bal.Balance)
			}
		}
	}
	log.Printf("Planned: %d × %.4f RBT = %.4f RBT total", len(receivers), *amount, needed)
	log.Printf("Starting fan-out: sender=%.20s...  → %d receivers  concurrency=%d  node=%s:%d",
		*sender, len(receivers), *concurrency, *addr, *port)

	tasks := make([]runner.Task, len(receivers))
	for i, r := range receivers {
		tasks[i] = runner.Task{DID: r}
	}

	fn := func(t runner.Task) runner.Result {
		res := c.Transfer(*sender, t.DID, *amount, *password, *memo)
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
		LogPrefix:   "fanout_transfer",
		CSVHeader:   []string{"receiver_did", "status", "req_id", "error"},
	})
	if err != nil {
		log.Fatalf("run failed: %v", err)
	}
	if fails > 0 {
		os.Exit(1)
	}
}
