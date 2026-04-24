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
| `-skip-balance-check` | Bypass the pre-flight sender balance guard |
| `-retry-failed <csv>` | Rerun only FAIL rows from a prior results CSV |

**Tuning note:** all transfers share one sender DID. Internal token-locking on the node will cap effective parallelism well below `-concurrency`. Start at 10, 50, 100 to find the knee.

Pre-flight balance check: `GET /rubix/v1/dids/{sender}/balances/rbt`. If `balance < count × amount`, the run aborts before sending any transfer.

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

Each sender and each receiver appears at most once per run. Concurrency can be pushed higher than in fanouttransfer — no shared-sender token-lock bottleneck.

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

| Flag | Purpose |
|---|---|
| `-senders` | Path to file with 1 sender DID per line |
| `-receiver` | The single receiving DID |
| `-amount` / `-concurrency` / `-retry-failed` / etc. | Same as other commands |

No pre-flight balance check here — N senders would mean N extra GETs. Underfunded senders surface as FAIL rows in the CSV.

## Node address

`-addr` defaults to `localhost`; in practice only `-port` needs to be set. Override `-addr` only when targeting a remote node.

## Output

Each run writes two files to `-output`:

- **`<prefix>_<UTC-ts>.csv`** — one row per attempt. All three commands share the same 6-column schema:

  | Column | Meaning |
  |---|---|
  | `sender_did` | Initiator DID |
  | `receiver_did` | Owner DID |
  | `amount` | RBT transferred (as written to the request) |
  | `transaction_id` | `transactionID` from the signature response. Populated on SUCCESS; empty on FAIL. |
  | `status` | `SUCCESS` or `FAIL` |
  | `error` | Node message (on SUCCESS: e.g. "Transfer initiated successfully"; on FAIL: the specific error) |

  Example:

  ```csv
  sender_did,receiver_did,amount,transaction_id,status,error
  bafybmi...A,bafybmi...B,1,ba538e30...dcf472,SUCCESS,Transfer initiated successfully
  bafybmi...C,bafybmi...D,1,,FAIL,sig: password verification failed
  ```

- **`<prefix>_<UTC-ts>.log`** — timestamped progress and failure log (also mirrored to stdout). Per-FAIL lines, progress ticks every `-batch-size` completions, final summary.

Rerun failures with `-retry-failed path/to/that.csv` — sender, receiver, and amount are all preserved from the original rows.

## Success criteria

A transfer is counted as SUCCESS only when **all** of the following are true:

1. `POST /rubix/v1/tx` returns `status: true` with a signature challenge in `result`.
2. `POST /rubix/v1/signature` returns `status: true`.
3. The signature response's `result` contains a non-empty `transactionID`.

Any other outcome — network error, `status: false` at either step, missing `result`, or empty `transactionID` — is recorded as FAIL with a specific error message (`tx: ...` or `sig: ...`).

## Protocol

Each transfer is a two-step flow:

1. **`POST /rubix/v1/tx`** with:
   ```json
   {
     "initiator": "<sender_did>",
     "owner":     "<receiver_did>",
     "tokens":    { "rbt": 1.0, "ft": [], "nft": [], "smartContract": [], "transferNftOwnership": false },
     "memo":      ""
   }
   ```
   Response contains a `SignReqData` (`{id, hash}`) inside `result`.

2. **`POST /rubix/v1/signature`** with:
   ```json
   { "id": "<from-step-1>", "password": "<did-password>", "signature": "" }
   ```
   Success response:
   ```json
   { "status": true, "message": "Transfer initiated successfully", "result": { "transactionID": "ba538e30..." } }
   ```
   The `transactionID` is what gets stored in the CSV.
