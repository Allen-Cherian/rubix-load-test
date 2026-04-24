# rubix-loadtest

Standalone load-testing CLIs for a Rubix node. No dependency on `rubixgoplatform` — talks to the node via its HTTP API directly (`POST /rubix/v1/tx` and `POST /rubix/v1/signature`).

## Build

```sh
go build -o bin/fanouttransfer ./cmd/fanouttransfer
go build -o bin/bulktransfer   ./cmd/bulktransfer
go build -o bin/peertransfer   ./cmd/peertransfer
```

## fanouttransfer (1 sender → N receivers)

For the 150k-token-sender → 500-receivers scenario:

```sh
./bin/fanouttransfer \
  -sender <sender_did> \
  -receivers dids_100k.txt \
  -count 500 \
  -select head \
  -amount 1.0 \
  -concurrency 50 \
  -addr localhost -port 20000 \
  -password <pw> \
  -output results/
```

Key flags:

| Flag | Purpose |
|---|---|
| `-sender` | DID holding the tokens |
| `-receivers` | Path to file with 1 DID per line |
| `-count` | How many receivers to pick (default 500) |
| `-select` | `head` (first N) or `random` |
| `-random-seed` | Seed for `-select random` (0 = time-based) |
| `-amount` | RBT per transfer |
| `-concurrency` | Max parallel transfers |
| `-retry-failed <csv>` | Rerun only FAIL rows from a prior results CSV |

**Tuning note:** all 500 transfers share one sender DID. Internal token-locking on the node will cap effective parallelism well below `-concurrency`. Start at 10, 50, 100 to find the knee.

## peertransfer (N senders → N receivers, paired)

```sh
./bin/peertransfer \
  -senders senders.txt \
  -receivers receivers.txt \
  -count 500 \
  -pair zip \
  -amount 1.0 \
  -concurrency 50 \
  -port 20000 \
  -password <pw> \
  -output results/
```

| Flag | Purpose |
|---|---|
| `-senders` / `-receivers` | Path to each list (1 DID per line) |
| `-count` | How many pairs to run (capped by `min(\|senders\|, \|receivers\|)`) |
| `-pair` | `zip` (index-for-index) or `random` (shuffle receivers, then zip) |
| `-random-seed` | Seed for `-pair random` |
| `-amount` / `-concurrency` / `-retry-failed` / etc. | Same as other commands |

Each sender and each receiver appears at most once per run.

CSV adds a column: `sender_did, receiver_did, status, req_id, error`.

## bulktransfer (N senders → 1 receiver)

```sh
./bin/bulktransfer \
  -senders senders.txt \
  -receiver <receiver_did> \
  -amount 1.0 \
  -concurrency 50 \
  -addr localhost -port 20000 \
  -password <pw> \
  -output results/
```

## Output

Each run writes two files to `-output`:

- `<prefix>_<UTC-ts>.csv` — one row per attempt: `did, status (SUCCESS|FAIL), req_id, error`
- `<prefix>_<UTC-ts>.log` — timestamped progress and failure log (also mirrored to stdout)

Rerun failures with `-retry-failed path/to/that.csv`.

## Protocol

Each transfer is a two-step flow:

1. `POST /rubix/v1/tx` with `{initiator, owner, tokens:{rbt,...}, memo}`.
   - If the response has `result: null`, the transfer completed.
   - If the response has a `result` object (a signature challenge), proceed to step 2.
2. `POST /rubix/v1/signature` with `{id, password, signature:""}` — the node signs with the unlocked DID.
