# Banking System -- Double-Entry Ledger Backend

A production-grade banking ledger backend written in Go with PostgreSQL. Implements double-entry bookkeeping, serializable transactions, pessimistic locking, idempotency, reversals, and a full audit log.

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [Project Structure](#project-structure)
3. [Startup Flow](#startup-flow)
4. [Database Schema](#database-schema)
5. [Domain Model](#domain-model)
6. [Repository Layer](#repository-layer)
7. [Service Layer](#service-layer)
8. [Transfer Flow (Step-by-Step)](#transfer-flow-step-by-step)
9. [Deposit Flow](#deposit-flow)
10. [Withdrawal Flow](#withdrawal-flow)
11. [Reversal Flow](#reversal-flow)
12. [Failure Auditing (failTransaction)](#failure-auditing-failtransaction)
13. [Concurrency Safety](#concurrency-safety)
14. [Idempotency](#idempotency)
15. [Double-Entry Bookkeeping](#double-entry-bookkeeping)
16. [HTTP Handler Layer](#http-handler-layer)
17. [API Reference](#api-reference)
18. [Error Handling](#error-handling)
19. [ErrCloser Cleanup Pattern](#errcloser-cleanup-pattern)
20. [Running Tests](#running-tests)
21. [Design Decisions and Trade-offs](#design-decisions-and-trade-offs)

---

## Quick Start

```bash
# Start PostgreSQL
docker-compose up -d postgres

# Build and run (applies schema + seed data automatically)
make run

# Or run directly
go run ./cmd/server

# The server listens on :8080 by default
# Swagger UI: http://localhost:8080/swagger/index.html#/

# To check data in db
psql -h localhost -p 5433 -U banking -d banking
```

### Useful DB Queries

```sql
-- All accounts with balances
SELECT id, name, currency, balance, created_at FROM accounts ORDER BY created_at;

-- All transactions
SELECT id, type, status, amount, from_account_id, to_account_id, note, created_at
FROM transactions ORDER BY created_at DESC;

-- Journal entries for a specific transaction
SELECT je.id, je.account_id, a.name, je.amount, je.direction
FROM journal_entries je
JOIN accounts a ON a.id = je.account_id
WHERE je.transaction_id = '<transaction-uuid>';

-- Verify double-entry invariant (should return zero rows)
SELECT t.id, t.type, SUM(je.amount) AS journal_sum
FROM transactions t
JOIN journal_entries je ON je.transaction_id = t.id
WHERE t.status = 'SUCCESS'
GROUP BY t.id, t.type
HAVING SUM(je.amount) != 0;

-- Verify account balances match journal entries (should return zero rows)
SELECT a.id, a.name, a.balance AS stored_balance, COALESCE(SUM(je.amount), 0) AS computed_balance
FROM accounts a
LEFT JOIN journal_entries je ON je.account_id = a.id
LEFT JOIN transactions t ON t.id = je.transaction_id AND t.status = 'SUCCESS'
GROUP BY a.id, a.name, a.balance
HAVING a.balance != COALESCE(SUM(je.amount), 0);
```

---

## Project Structure

```
banking-system/
├── cmd/server/                  # Application entry point + DI wiring
│   ├── main.go                  # main() -> run() -> serve()
│   ├── flags.go                 # CLI flags with env var fallbacks
│   ├── closeutils.go            # ErrCloser, OnErrFunc, Wrap, LogOnErr
│   ├── di.go                    # diContainer struct + newDIContainer()
│   ├── db.go                    # Database DI provider
│   ├── account_repo.go          # AccountRepository DI provider
│   ├── tx_repo.go               # TransactionRepository DI provider
│   ├── account_svc.go           # AccountService DI provider
│   ├── ledger_svc.go            # LedgerService DI provider
│   ├── http_handler.go          # HTTP handler DI provider
│   ├── di_test.go               # Integration test (skips without DB)
│   ├── di_provider_test.go      # DI provider unit tests with sqlmock
│   ├── closeutils_test.go       # ErrCloser tests
│   └── flags_test.go            # Flag/env tests
├── internal/
│   ├── domain/
│   │   ├── models.go            # Account, Transaction, JournalEntry, request types
│   │   ├── errors.go            # Sentinel domain errors
│   │   ├── domain_test.go       # Model JSON round-trip tests
│   │   └── errors_test.go       # Error distinctness tests
│   ├── handler/
│   │   ├── handler.go           # HTTP routes, handlers, CORS middleware
│   │   └── handler_test.go      # Handler tests with mock services
│   ├── repository/
│   │   ├── interfaces.go        # AccountRepository, TransactionRepository, DB
│   │   └── postgres/
│   │       ├── db.go            # Open() -- connection pool setup
│   │       ├── account.go       # AccountRepo (CRUD + SELECT FOR UPDATE)
│   │       └── transaction.go   # TransactionRepo (CRUD + journal entries)
│   ├── service/
│   │   ├── account.go           # AccountService (list, get, create)
│   │   ├── ledger.go            # LedgerService (transfer, deposit, withdraw, reverse)
│   │   ├── account_test.go      # AccountService unit tests
│   │   └── ledger_test.go       # LedgerService unit tests with sqlmock
│   └── migrations/
│       ├── migrations.go        # go:embed for schema.sql
│       └── schema.sql           # DDL + indexes + seed data
├── docs/                        # Auto-generated Swagger docs
│   ├── docs.go
│   ├── swagger.json
│   └── swagger.yaml
├── docker-compose.yml           # PostgreSQL 16 + app service
├── Dockerfile                   # Multi-stage Go build
├── Makefile                     # Dev shortcuts
├── go.mod
└── go.sum
```

The code follows a **layered architecture**: Domain -> Repository -> Service -> Handler. Each layer only depends on the one below it. The DI container in `cmd/server/` wires them together at startup.

---

## Startup Flow

When you run `go run ./cmd/server`, here is exactly what happens:

```
main()
  └─> run()
        ├─ getFlags()                  -- parse CLI flags / env vars
        ├─ newDIContainer(flags)       -- register all lazy DI providers
        │    (nothing is created yet -- providers are just closures)
        ├─ dic.httpHandler()           -- first call triggers the full chain:
        │    ├─ dic.accountSvc()
        │    │    └─ dic.accountRepo()
        │    │         └─ dic.db()     -- NOW the DB connects:
        │    │              ├─ postgres.Open(dsn)  -- open pool, ping
        │    │              └─ db.Exec(schema.sql) -- create tables + seed
        │    ├─ dic.ledgerSvc()
        │    │    ├─ dic.db()          -- returns cached *sql.DB (already created)
        │    │    ├─ dic.accountRepo() -- returns cached repo
        │    │    └─ dic.txRepo()
        │    │         └─ dic.db()     -- cached
        │    └─ handler.New(accountSvc, ledgerSvc) -- register HTTP routes
        └─ serve(addr, handler)
              ├─ create http.Server with timeouts
              ├─ start ListenAndServe in goroutine
              ├─ block on SIGINT/SIGTERM
              └─ srv.Shutdown(ctx) with 10s deadline
```

Key insight: **nothing is created until it is first accessed**. The DI container holds closures, not values. The first call to `dic.httpHandler()` cascades through the entire dependency graph, creating each component exactly once.


---

## Database Schema

The schema lives in `internal/migrations/schema.sql` and is embedded into the binary via `go:embed`. It is applied automatically on startup via `db.ExecContext(ctx, migrations.Schema)`.

### Tables

**`accounts`**
| Column | Type | Notes |
|---|---|---|
| id | UUID (PK) | Auto-generated via `gen_random_uuid()` |
| name | VARCHAR(255) | Account holder name |
| currency | CHAR(3) | ISO currency code, default `INR` |
| balance | BIGINT | Balance in smallest currency unit (paise), `CHECK (balance >= 0)` |
| created_at | TIMESTAMPTZ | Auto-set |
| updated_at | TIMESTAMPTZ | Updated on balance change |

**`transactions`**
| Column | Type | Notes |
|---|---|---|
| id | UUID (PK) | Auto-generated |
| type | VARCHAR(20) | `DEPOSIT`, `WITHDRAWAL`, `TRANSFER`, `REVERSAL` |
| status | VARCHAR(10) | `PENDING`, `SUCCESS`, `FAILED` |
| amount | BIGINT | Always positive, `CHECK (amount > 0)` |
| from_account_id | UUID (FK) | Source account (nullable for deposits) |
| to_account_id | UUID (FK) | Destination account (nullable for withdrawals) |
| reversal_of | UUID (FK) | Points to original transaction if this is a reversal |
| idempotency_key | VARCHAR(255) | `UNIQUE` -- prevents duplicate operations |
| note | TEXT | Optional user note |
| error_message | TEXT | Populated on FAILED transactions |
| created_at | TIMESTAMPTZ | Auto-set |

**`journal_entries`**
| Column | Type | Notes |
|---|---|---|
| id | UUID (PK) | Auto-generated |
| transaction_id | UUID (FK) | Parent transaction |
| account_id | UUID (FK) | Which account is affected |
| amount | BIGINT | Signed: negative = debit, positive = credit |
| direction | VARCHAR(6) | `DEBIT` or `CREDIT` |
| created_at | TIMESTAMPTZ | Auto-set |

### Indexes

- `idx_journal_entries_transaction` -- look up entries by transaction
- `idx_journal_entries_account` -- look up entries by account
- `idx_transactions_from_account` -- filter transactions by source
- `idx_transactions_to_account` -- filter transactions by destination
- `idx_transactions_reversal_of` -- find reversals of a transaction

### Seed Data

Three demo accounts are inserted on startup (idempotent via `ON CONFLICT DO NOTHING`):

| ID | Name | Currency | Balance |
|---|---|---|---|
| `11111111-1111-1111-1111-111111111111` | Alice | INR | 1000 |
| `22222222-2222-2222-2222-222222222222` | Bob | INR | 500 |
| `33333333-3333-3333-3333-333333333333` | Charlie | INR | 250 |

---

## Domain Model

All domain types live in `internal/domain/models.go`.

**Money representation**: All amounts are stored as `int64` in the **smallest currency unit** (e.g. paise for INR, cents for USD). This avoids floating-point precision errors. The API accepts and returns integer values.

**Key types**:
- `Account` -- holds current balance (denormalized cache of journal entry sums)
- `Transaction` -- top-level record for every operation attempt (including failures)
- `JournalEntry` -- one side of a double-entry pair, linked to a transaction and an account
- `TransferRequest`, `DepositRequest`, `WithdrawalRequest`, `ReversalRequest` -- input DTOs

**Domain errors** (`internal/domain/errors.go`):
- `ErrAccountNotFound` -- account UUID doesn't exist
- `ErrInsufficientFunds` -- balance < requested amount
- `ErrInvalidAmount` -- amount <= 0
- `ErrSameAccount` -- transfer source == destination
- `ErrTransactionNotFound` -- transaction UUID doesn't exist
- `ErrAlreadyReversed` -- a SUCCESS reversal already exists for this transaction
- `ErrCannotReverseType` -- only SUCCESS transactions can be reversed
- `ErrIdempotentReplay` -- idempotency key was already used

---

## Repository Layer

Defined by interfaces in `internal/repository/interfaces.go`, implemented in `internal/repository/postgres/`.

### `AccountRepository`

| Method | What it does |
|---|---|
| `GetByID(ctx, id)` | Read-only fetch by UUID |
| `GetAll(ctx)` | List all accounts ordered by `created_at` |
| `Create(ctx, name, currency)` | Insert new account, return with generated UUID |
| `GetByIDForUpdate(ctx, tx, id)` | `SELECT ... FOR UPDATE` inside an existing DB transaction -- acquires a **row lock** |
| `UpdateBalance(ctx, tx, id, delta)` | `UPDATE accounts SET balance = balance + $delta` -- the DB `CHECK` constraint rejects negative balances |

### `TransactionRepository`

| Method | What it does |
|---|---|
| `Create(ctx, tx, t)` | Insert a new transaction record |
| `UpdateStatus(ctx, tx, id, status, errMsg)` | Set status to SUCCESS or FAILED |
| `GetByID(ctx, id)` | Fetch by UUID |
| `GetAll(ctx)` | List last 200 transactions, newest first |
| `GetByIdempotencyKey(ctx, key)` | Look up by idempotency key (returns `nil, nil` if not found) |
| `IsReversed(ctx, originalID)` | Check if a SUCCESS reversal exists targeting this transaction |
| `CreateJournalEntry(ctx, tx, entry)` | Insert a journal entry |
| `GetJournalEntriesByTransaction(ctx, txID)` | List all journal entries for a transaction |



---

## Service Layer

### AccountService (`internal/service/account.go`)

Simple pass-through to the repository with default value handling:

- `GetAccount(ctx, id)` -- delegates to `accounts.GetByID`
- `ListAccounts(ctx)` -- delegates to `accounts.GetAll`
- `CreateAccount(ctx, name, currency)` -- defaults name to `"Unnamed"` and currency to `"INR"` if empty

### LedgerService (`internal/service/ledger.go`)

This is the core of the system. Every balance-changing operation goes through here. It coordinates:
1. Input validation
2. Idempotency checks
3. Database transactions at `SERIALIZABLE` isolation
4. Pessimistic row locking (`SELECT FOR UPDATE`)
5. Balance updates
6. Double-entry journal entry creation
7. Status tracking (PENDING -> SUCCESS or FAILED)
8. Failure auditing on a separate DB connection

---

## Transfer Flow (Step-by-Step)

This is the most complex operation. Here is what happens when `POST /api/transfers` is called:

```
Client sends: { from_account_id, to_account_id, amount, note, idempotency_key }
```

### Step 1: Input Validation (handler)
- Parse JSON body
- Parse UUIDs for `from_account_id` and `to_account_id`
- If parsing fails, return `400 Bad Request`

### Step 2: Input Validation (service)
- Check `amount > 0`, else return `ErrInvalidAmount`
- Check `from != to`, else return `ErrSameAccount`

### Step 3: Idempotency Check
```go
if req.IdempotencyKey != "" {
    existing, err := s.txRepo.GetByIdempotencyKey(ctx, req.IdempotencyKey)
    if existing != nil {
        return existing, nil  // return the cached result
    }
}
```
If the key was already used, return the original transaction record. The client gets the same response as the first call.

### Step 4: Begin Serializable Transaction
```go
tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
defer tx.Rollback()
```
PostgreSQL's `SERIALIZABLE` isolation ensures that concurrent transactions behave as if they ran sequentially.

### Step 5: Create PENDING Transaction Record
```go
txRecord := &domain.Transaction{
    ID:     uuid.New(),
    Type:   domain.TypeTransfer,
    Status: domain.StatusPending,
    Amount: req.Amount,
    ...
}
s.txRepo.Create(ctx, tx, txRecord)
```
The transaction record is created immediately in PENDING status so it appears in the audit log even if something fails later.

### Step 6: Lock Accounts in Consistent Order
```go
first, second := req.FromAccountID, req.ToAccountID
if first.String() > second.String() {
    first, second = second, first
}
s.accounts.GetByIDForUpdate(ctx, tx, first)
s.accounts.GetByIDForUpdate(ctx, tx, second)
```
Both accounts are locked via `SELECT ... FOR UPDATE`. They are always locked in **lexicographic UUID order** regardless of which is source/destination. This prevents deadlocks when two concurrent transfers go A->B and B->A simultaneously.

### Step 7: Check Sufficient Funds
```go
src, _ := s.accounts.GetByIDForUpdate(ctx, tx, req.FromAccountID)
if src.Balance < req.Amount {
    s.failTransaction(ctx, tx, txRecord, domain.ErrInsufficientFunds)
    return txRecord, domain.ErrInsufficientFunds
}
```
If the source doesn't have enough, the transaction is marked FAILED (via a separate DB connection -- see [Failure Auditing](#failure-auditing-failtransaction)) and the error is returned.

### Step 8: Apply Balance Changes
```go
s.accounts.UpdateBalance(ctx, tx, req.FromAccountID, -req.Amount)  // subtract from source
s.accounts.UpdateBalance(ctx, tx, req.ToAccountID,   req.Amount)   // add to destination
```
The `UpdateBalance` method does `SET balance = balance + $delta`. The DB `CHECK (balance >= 0)` constraint acts as a second safety net.

### Step 9: Write Journal Entries
```go
s.writeJournalPair(ctx, tx, txRecord.ID, req.FromAccountID, req.ToAccountID, req.Amount)
```
This creates two entries:
- **DEBIT** on source account: amount = `-req.Amount`
- **CREDIT** on destination account: amount = `+req.Amount`

For every transfer, `sum(journal_entries.amount) = 0`. This is the double-entry invariant.

### Step 10: Mark SUCCESS and Commit
```go
s.txRepo.UpdateStatus(ctx, tx, txRecord.ID, domain.StatusSuccess, "")
tx.Commit()
txRecord.Status = domain.StatusSuccess
return txRecord, nil
```

### Step 11: HTTP Response
The handler returns the transaction record as JSON with status `201 Created`.

### Visual Summary

```
Client POST /api/transfers
  │
  ├─ Validate input
  ├─ Check idempotency key (return cached if exists)
  │
  ├─ BEGIN SERIALIZABLE TX
  │   ├─ INSERT transaction (PENDING)
  │   ├─ SELECT FOR UPDATE account_1 (lower UUID)
  │   ├─ SELECT FOR UPDATE account_2 (higher UUID)
  │   ├─ Check source.balance >= amount
  │   ├─ UPDATE source.balance -= amount
  │   ├─ UPDATE dest.balance   += amount
  │   ├─ INSERT journal_entry (DEBIT,  source, -amount)
  │   ├─ INSERT journal_entry (CREDIT, dest,   +amount)
  │   ├─ UPDATE transaction -> SUCCESS
  │   └─ COMMIT
  │
  └─ Return 201 { transaction }
```

---

## Deposit Flow

Deposits represent external money entering the system.

```
Client POST /api/deposits { account_id, amount, note, idempotency_key }
  │
  ├─ Validate amount > 0
  ├─ Check idempotency
  │
  ├─ BEGIN SERIALIZABLE TX
  │   ├─ INSERT transaction (DEPOSIT, PENDING, to_account_id = account_id)
  │   ├─ SELECT FOR UPDATE account
  │   ├─ UPDATE account.balance += amount
  │   ├─ INSERT journal_entry (CREDIT, account, +amount)
  │   ├─ UPDATE transaction -> SUCCESS
  │   └─ COMMIT
  │
  └─ Return 201 { transaction }
```

Only one journal entry is created (CREDIT). In a full production system, there would be a corresponding DEBIT on an "external funds" contra account, but this implementation keeps it simple.

---

## Withdrawal Flow

Withdrawals represent money leaving the system.

```
Client POST /api/withdrawals { account_id, amount, note, idempotency_key }
  │
  ├─ Validate amount > 0
  ├─ Check idempotency
  │
  ├─ BEGIN SERIALIZABLE TX
  │   ├─ INSERT transaction (WITHDRAWAL, PENDING, from_account_id = account_id)
  │   ├─ SELECT FOR UPDATE account
  │   ├─ Check account.balance >= amount
  │   ├─ UPDATE account.balance -= amount
  │   ├─ INSERT journal_entry (DEBIT, account, -amount)
  │   ├─ UPDATE transaction -> SUCCESS
  │   └─ COMMIT
  │
  └─ Return 201 { transaction }
```

---

## Reversal Flow

Reversals undo a completed transaction by creating an inverse operation.

```
Client POST /api/reversals { transaction_id, note, idempotency_key }
  │
  ├─ Check idempotency
  ├─ Fetch original transaction
  ├─ Verify original.status == SUCCESS (else ErrCannotReverseType)
  ├─ Check not already reversed (else ErrAlreadyReversed)
  │
  ├─ BEGIN SERIALIZABLE TX
  │   ├─ INSERT transaction (REVERSAL, PENDING, reversal_of = original.id)
  │   │
  │   ├─ Switch on original.type:
  │   │
  │   │   TRANSFER:
  │   │   ├─ Lock both accounts (consistent order)
  │   │   ├─ Check dest.balance >= original.amount
  │   │   ├─ UPDATE dest.balance   -= original.amount (undo the credit)
  │   │   ├─ UPDATE source.balance += original.amount (undo the debit)
  │   │   ├─ INSERT journal_entry (DEBIT,  dest,   -amount)
  │   │   └─ INSERT journal_entry (CREDIT, source, +amount)
  │   │
  │   │   DEPOSIT:
  │   │   ├─ Lock account
  │   │   ├─ Check account.balance >= original.amount
  │   │   ├─ UPDATE account.balance -= original.amount
  │   │   └─ INSERT journal_entry (DEBIT, account, -amount)
  │   │
  │   │   WITHDRAWAL:
  │   │   ├─ Lock account
  │   │   ├─ UPDATE account.balance += original.amount
  │   │   └─ INSERT journal_entry (CREDIT, account, +amount)
  │   │
  │   ├─ UPDATE transaction -> SUCCESS
  │   └─ COMMIT
  │
  └─ Return 201 { transaction }
```

Safeguards:
- Only `SUCCESS` transactions can be reversed
- Each transaction can only be reversed once (`IsReversed` check)
- Insufficient funds in the reversal target will cause FAILED status

---

## Failure Auditing (failTransaction)

When an operation fails mid-transaction (e.g., insufficient funds after the PENDING record was already inserted), the main DB transaction will be rolled back by `defer tx.Rollback()`. This means the PENDING record would also be lost.

To preserve the audit trail, `failTransaction` **rolls back the main transaction first** (so account `FOR UPDATE` locks and the uncommitted PENDING row are released), then opens a **separate database connection** and writes the FAILED row. Rolling back before the audit insert avoids a **self-deadlock**: the audit `INSERT` for the same transaction id would otherwise block on the still-open main transaction that already inserted that id.


This ensures that **every operation attempt is recorded**, whether it succeeded or failed. The `ON CONFLICT` upsert handles both cases:
- If the PENDING record was already committed (unlikely), it updates it to FAILED
- If the PENDING record will be rolled back, it inserts a new FAILED record

---

## Concurrency Safety

The system uses three complementary mechanisms:

### 1. Serializable Isolation
```go
tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
```
PostgreSQL's `SERIALIZABLE` level detects conflicts between concurrent transactions and aborts one of them, guaranteeing that the result is equivalent to some serial execution.

### 2. Pessimistic Locking (SELECT FOR UPDATE)
```go
s.accounts.GetByIDForUpdate(ctx, tx, accountID)
// SQL: SELECT ... FROM accounts WHERE id = $1 FOR UPDATE
```
This acquires an **exclusive row lock** on the account row. Other transactions trying to lock the same row will block until this transaction commits or rolls back.

### 3. Consistent Lock Ordering
```go
first, second := req.FromAccountID, req.ToAccountID
if first.String() > second.String() {
    first, second = second, first
}
// Always lock the lexicographically smaller UUID first
```
When a transfer involves two accounts, they are always locked in the same order (by UUID string comparison). This prevents **deadlocks**: if Transfer A->B and Transfer B->A run concurrently, both will try to lock the lower UUID first, so one will block on the other rather than creating a cycle.

---

## Idempotency

Every mutation endpoint accepts an optional `idempotency_key`. The flow:

1. Before starting the main transaction, check `transactions` table for an existing record with this key
2. If found, return the existing result immediately (no side effects)
3. If not found, proceed with the operation and store the key in the new transaction record
4. The `UNIQUE` constraint on `idempotency_key` in the database prevents race conditions

This means a client can safely retry a failed network request without risk of double-processing.

---

## Double-Entry Bookkeeping

Every balance change creates journal entries that satisfy:

```
For any transfer:  SUM(journal_entries.amount) = 0
```

| Operation | Journal Entries |
|---|---|
| Transfer 500 A->B | DEBIT A: -500, CREDIT B: +500 |
| Deposit 1000 to A | CREDIT A: +1000 |
| Withdraw 250 from A | DEBIT A: -250 |
| Reverse Transfer A->B | DEBIT B: -500, CREDIT A: +500 |

The `accounts.balance` column is a **denormalized cache** of the sum of all journal entries for that account. It is updated atomically alongside journal entry creation within the same database transaction, so they can never drift apart.

---

## HTTP Handler Layer

The handler (`internal/handler/handler.go`) is responsible for:

1. **Route registration** -- maps URL paths to handler functions using `http.ServeMux`
2. **Request parsing** -- decodes JSON bodies, parses UUID path parameters
3. **Service delegation** -- calls the appropriate service method
4. **Response formatting** -- serializes results to JSON with appropriate HTTP status codes
5. **Error mapping** -- translates domain errors to HTTP status codes
6. **CORS middleware** -- allows cross-origin requests from any origin

The handler defines its own interfaces for the services it depends on (`AccountService`, `LedgerService`). It accepts concrete types from the `service` package in its constructor, but internally works through these interfaces. This makes the handler testable with mocks.

---

## API Reference

Interactive API docs are available at **http://localhost:8080/swagger/index.html#/** when the server is running.

### Accounts

**`GET /api/accounts`** -- List all accounts
```json
// Response 200
[
  {
    "id": "11111111-1111-1111-1111-111111111111",
    "name": "Alice",
    "currency": "INR",
    "balance": 1000,
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  }
]
```

**`GET /api/accounts/:id`** -- Get account by ID
```json
// Response 200 (or 404 if not found)
{
  "id": "11111111-1111-1111-1111-111111111111",
  "name": "Alice",
  "currency": "INR",
  "balance": 1000,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

**`POST /api/accounts`** -- Create a new account
```json
// Request
{ "name": "Dave", "currency": "INR" }

// Response 201
{
  "id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
  "name": "Dave",
  "currency": "INR",
  "balance": 0,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

### Ledger Operations

**`POST /api/transfers`** -- Transfer between accounts
```json
// Request
{
  "from_account_id": "11111111-1111-1111-1111-111111111111",
  "to_account_id": "22222222-2222-2222-2222-222222222222",
  "amount": 500,
  "note": "Lunch payment",
  "idempotency_key": "txn-abc-123"
}

// Response 201
{
  "id": "...",
  "type": "TRANSFER",
  "status": "SUCCESS",
  "amount": 500,
  "from_account_id": "11111111-...",
  "to_account_id": "22222222-...",
  "idempotency_key": "txn-abc-123",
  "note": "Lunch payment",
  "created_at": "..."
}
```

**`POST /api/deposits`** -- Deposit into account
```json
// Request
{
  "account_id": "11111111-1111-1111-1111-111111111111",
  "amount": 1000,
  "note": "Salary",
  "idempotency_key": "dep-001"
}
```

**`POST /api/withdrawals`** -- Withdraw from account
```json
// Request
{
  "account_id": "11111111-1111-1111-1111-111111111111",
  "amount": 250,
  "note": "ATM withdrawal",
  "idempotency_key": "wd-001"
}
```

**`POST /api/reversals`** -- Reverse a transaction
```json
// Request
{
  "transaction_id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "note": "Customer refund",
  "idempotency_key": "rev-001"
}
```

### Audit Log

**`GET /api/transactions`** -- List last 200 transactions (newest first)

**`GET /api/transactions/:id`** -- Get transaction by ID

---

## Error Handling

Domain errors are mapped to HTTP status codes in the handler:

| Domain Error | HTTP Status |
|---|---|
| `ErrAccountNotFound` | 404 Not Found |
| `ErrTransactionNotFound` | 404 Not Found |
| `ErrInsufficientFunds` | 422 Unprocessable Entity |
| `ErrInvalidAmount` | 422 Unprocessable Entity |
| `ErrSameAccount` | 422 Unprocessable Entity |
| `ErrAlreadyReversed` | 422 Unprocessable Entity |
| `ErrCannotReverseType` | 422 Unprocessable Entity |
| `ErrIdempotentReplay` | 200 OK |
| (everything else) | 500 Internal Server Error |

Error responses are always JSON:
```json
{ "error": "insufficient funds" }
```

---

## ErrCloser Cleanup Pattern

The `ErrCloser` type is a function `func(onErr OnErrFunc)` that cleans up a resource and reports errors through a callback:

```go
type OnErrFunc func(err error)
type ErrCloser func(onErr OnErrFunc)
```

- `Nil()` -- returns a no-op closer (used before the resource is created)
- `Wrap(label)` -- adds a label prefix to any error before forwarding
- `LogOnErr` -- a built-in callback that logs errors with `log.Printf`

In the DB provider, the closer starts as `Nil()` and is replaced with a real closer once the DB is opened:

```go
closeDB := ec.Nil()             // no-op until DB is actually created
provider := func() (*sql.DB, error) {
    db, err = newDB(flg)
    closeDB = func(onErr OnErrFunc) {  // now it closes the real DB
        onErr(db.Close())
    }
}
```

At shutdown: `closeDIC(LogOnErr)` -> `closeDB.Wrap("database")(LogOnErr)` -> `db.Close()`.

---

## Running Tests

```bash
# Run all tests (no DB required -- integration test skips automatically)
go test ./...

# Verbose output
go test ./... -v

# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out

# HTML coverage report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
open coverage.html

# Run specific package tests
go test ./internal/handler/... -v
go test ./internal/service/... -v
go test ./cmd/server/... -v

# Integration test (requires running PostgreSQL)
DATABASE_URL="postgres://banking:banking@localhost:5433/banking?sslmode=disable" \
  go test ./cmd/server/ -run TestDIContainer -v
```

---

## Design Decisions and Trade-offs

### Money as int64 (smallest currency unit)
Floating-point arithmetic introduces rounding errors (`0.1 + 0.2 != 0.3`). By storing everything as integers in the smallest currency unit (paise for INR, cents for USD), arithmetic is exact. The API accepts and returns these integer values; the client is responsible for display formatting.

### Denormalized balance on accounts
The `accounts.balance` column is redundant -- it could always be recomputed from `SUM(journal_entries.amount)`. However, keeping it denormalized means reads are O(1) instead of scanning the journal. The balance is always updated in the same DB transaction as the journal entries, so consistency is guaranteed.

### failTransaction uses a separate connection
When an operation fails, the main transaction must be rolled back before the audit write: otherwise the uncommitted PENDING row blocks the audit `INSERT` on another connection and can deadlock. After `Rollback`, `failTransaction` opens a **new** database transaction to insert the FAILED row so every attempt is logged.

### Idempotency check outside the serializable transaction
The idempotency lookup runs before `BeginTx(SERIALIZABLE)`. This is a pragmatic choice: checking idempotency in a read-only query is cheap and avoids holding a serializable transaction open while doing a lookup. There is a small race window where two requests with the same key could both pass the check, but the `UNIQUE` constraint on `idempotency_key` in the database prevents both from succeeding.

### No external framework for HTTP
The handler uses Go's standard `net/http.ServeMux`. This keeps dependencies minimal. Path parameter parsing is done manually by stripping known prefixes. For a larger project, a router like chi or gorilla/mux would be more appropriate.

### Single currency simplification
Accounts have a `currency` field but the system does not enforce currency matching between accounts in a transfer. In a production system, cross-currency transfers would require exchange rate handling.
