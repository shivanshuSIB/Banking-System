Plan to implement                                                                                                                         │
│                                                                                                                                           │
│ Banking System Backend — Implementation Plan                                                                                              │
│                                                                                                                                           │
│ Context                                                                                                                                   │
│                                                                                                                                           │
│ Build a banking ledger backend at /Users/shivanshusingh/go/src/Golang/banking-system/ using the DTSL DI pattern (lazy sync.Mutex + nil    │
│ providers, ErrCloser cleanup) as seen in DTSL/email-sending/cmd/sent-events-consumer/di.go and the existing project-bank repo. Backend    │
│ only for now (no web UI in this pass).                                                                                                    │
│                                                                                                                                           │
│ The existing project-bank at /Users/shivanshusingh/go/src/Golang/project-bank/ already implements this exact system — we'll use it as the │
│  blueprint and copy it into the new location with a clean structure.                                                                      │
│                                                                                                                                           │
│ Approach: Copy & Clean from project-bank                                                                                                  │
│                                                                                                                                           │
│ Since project-bank already has the complete working implementation with the correct DI pattern, the most efficient approach is to         │
│ replicate its code into banking-system/ with the same module name and structure.                                                          │
│                                                                                                                                           │
│ File Structure                                                                                                                            │
│                                                                                                                                           │
│ banking-system/                                                                                                                           │
│ ├── cmd/server/                                                                                                                           │
│ │   ├── main.go              # Entry point, run(), serve()                                                                                │
│ │   ├── flags.go             # CLI flags + env fallbacks                                                                                  │
│ │   ├── closeutils.go        # ErrCloser, OnErrFunc, Wrap, LogOnErr                                                                       │
│ │   ├── di.go                # diContainer struct + newDIContainer()                                                                      │
│ │   ├── db.go                # newDBDIProvider, newDB                                                                                     │
│ │   ├── account_repo.go      # newAccountRepoDIProvider                                                                                   │
│ │   ├── tx_repo.go           # newTxRepoDIProvider                                                                                        │
│ │   ├── account_svc.go       # newAccountSvcDIProvider                                                                                    │
│ │   ├── ledger_svc.go        # newLedgerSvcDIProvider                                                                                     │
│ │   ├── http_handler.go      # newHTTPHandlerDIProvider                                                                                   │
│ │   └── di_test.go           # DI container smoke test                                                                                    │
│ ├── internal/                                                                                                                             │
│ │   ├── domain/                                                                                                                           │
│ │   │   ├── models.go        # Account, Transaction, JournalEntry, request types                                                          │
│ │   │   └── errors.go        # Domain errors                                                                                              │
│ │   ├── handler/                                                                                                                          │
│ │   │   └── handler.go       # HTTP routes + handlers + CORS                                                                              │
│ │   ├── repository/                                                                                                                       │
│ │   │   ├── interfaces.go    # AccountRepository, TransactionRepository, DB interfaces                                                    │
│ │   │   └── postgres/                                                                                                                     │
│ │   │       ├── db.go        # Open(), connection pool                                                                                    │
│ │   │       ├── account.go   # AccountRepo implementation                                                                                 │
│ │   │       └── transaction.go # TransactionRepo implementation                                                                           │
│ │   ├── service/                                                                                                                          │
│ │   │   ├── account.go       # AccountService                                                                                             │
│ │   │   └── ledger.go        # LedgerService (transfer, deposit, withdraw, reverse)                                                       │
│ │   └── migrations/                                                                                                                       │
│ │       ├── migrations.go    # go:embed schema.sql                                                                                        │
│ │       └── schema.sql       # DDL: accounts, transactions, journal_entries + seed data                                                   │
│ ├── docker-compose.yml       # PostgreSQL 16 + app                                                                                        │
│ ├── Dockerfile               # Multi-stage Go build                                                                                       │
│ ├── go.mod                   # module banking-system                                                                                      │
│ └── Makefile                 # dev shortcuts                                                                                              │
│                                                                                                                                           │
│ Key Files to Create (sourced from project-bank)                                                                                           │
│                                                                                                                                           │
│ All files will be copied from /Users/shivanshusingh/go/src/Golang/project-bank/ with the module name banking-system preserved (it already │
│  matches).                                                                                                                                │
│                                                                                                                                           │
│ DI Pattern (DTSL style)                                                                                                                   │
│                                                                                                                                           │
│ - Each dependency is a func() (*T, error) field on diContainer                                                                            │
│ - Each has a newXxxDIProvider(dic) that returns a closure with sync.Mutex + nil lazy init                                                 │
│ - DB provider also returns an ErrCloser for cleanup                                                                                       │
│ - newDIContainer() wires all providers and composes closers                                                                               │
│                                                                                                                                           │
│ Core Business Logic                                                                                                                       │
│                                                                                                                                           │
│ - Double-entry ledger: Every balance change creates journal entries (debit + credit)                                                      │
│ - Serializable transactions: sql.LevelSerializable for all balance-changing ops                                                           │
│ - SELECT FOR UPDATE: Pessimistic row locking with consistent ordering to avoid deadlocks                                                  │
│ - Idempotency keys: Prevent duplicate operations                                                                                          │
│ - Reversal: Invert original operation, check IsReversed to prevent double-reversal                                                        │
│ - Audit log: Transaction table records every attempt (SUCCESS and FAILED) with error messages                                             │
│ - failTransaction: Writes failure record via a separate DB transaction so audit survives rollback                                         │
│                                                                                                                                           │
│ API Endpoints                                                                                                                             │
│                                                                                                                                           │
│ - GET/POST /api/accounts — list/create accounts                                                                                           │
│ - GET /api/accounts/:id — get account                                                                                                     │
│ - POST /api/transfers — transfer between accounts                                                                                         │
│ - POST /api/deposits — deposit into account                                                                                               │
│ - POST /api/withdrawals — withdraw from account                                                                                           │
│ - POST /api/reversals — reverse a transaction                                                                                             │
│ - GET /api/transactions — list audit log                                                                                                  │
│ - GET /api/transactions/:id — get transaction detail                                                                                      │
│                                                                                                                                           │
│ Steps                                                                                                                                     │
│                                                                                                                                           │
│ 1. Initialize Go module at banking-system/                                                                                                │
│ 2. Create internal/domain/ — models.go, errors.go                                                                                         │
│ 3. Create internal/migrations/ — schema.sql, migrations.go                                                                                │
│ 4. Create internal/repository/ — interfaces.go, postgres/db.go, postgres/account.go, postgres/transaction.go                              │
│ 5. Create internal/service/ — account.go, ledger.go                                                                                       │
│ 6. Create internal/handler/ — handler.go                                                                                                  │
│ 7. Create cmd/server/ — all DI files, flags.go, main.go, closeutils.go                                                                    │
│ 8. Create cmd/server/di_test.go — smoke test                                                                                              │
│ 9. Create docker-compose.yml, Dockerfile, Makefile                                                                                        │
│ 10. Run go mod tidy, verify compilation                                                                                                   │
│ 11. Run docker-compose up -d postgres, then go run ./cmd/server to verify                                                                 │
│                                                                                                                                           │
│ Verification                                                                                                                              │
│                                                                                                                                           │
│ # Start postgres                                                                                                                          │
│ cd /Users/shivanshusingh/go/src/Golang/banking-system                                                                                     │
│ docker-compose up -d postgres                                                                                                             │
│                                                                                                                                           │
│ # Build and run                                                                                                                           │
│ go build ./cmd/server && ./server                                                                                                         │
│                                                                                                                                           │
│ # Test endpoints                                                                                                                          │
│ curl http://localhost:8080/api/accounts                                                                                                   │
│ curl -X POST http://localhost:8080/api/accounts -d '{"name":"Test","currency":"USD"}'                                                     │
│ curl -X POST http://localhost:8080/api/deposits -d '{"account_id":"...","amount":10000}'                                                  │
│ curl -X POST http://localhost:8080/api/transfers -d '{"from_account_id":"...","to_account_id":"...","amount":5000}'                       │
│ curl http://localhost:8080/api/transactions                                                                                               │
│                                                                                                                                           │
│ # Run DI test                                                                                                                             │
│ go test ./cmd/server/ -run TestDIContainer