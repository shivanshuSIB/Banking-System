package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"banking-system/internal/domain"
	"banking-system/internal/service"

	"github.com/google/uuid"
	httpSwagger "github.com/swaggo/http-swagger"
)

// CreateAccountRequest represents the request body for creating a new account.
type CreateAccountRequest struct {
	Name     string `json:"name" example:"Alice"`
	Currency string `json:"currency" example:"USD"`
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error string `json:"error" example:"account not found"`
}

// AccountService is the subset of service.AccountService used by the handler.
type AccountService interface {
	GetAccount(ctx context.Context, id uuid.UUID) (*domain.Account, error)
	ListAccounts(ctx context.Context) ([]*domain.Account, error)
	CreateAccount(ctx context.Context, name, currency string) (*domain.Account, error)
}

// LedgerService is the subset of service.LedgerService used by the handler.
type LedgerService interface {
	Transfer(ctx context.Context, req domain.TransferRequest) (*domain.Transaction, error)
	Deposit(ctx context.Context, req domain.DepositRequest) (*domain.Transaction, error)
	Withdraw(ctx context.Context, req domain.WithdrawalRequest) (*domain.Transaction, error)
	Reverse(ctx context.Context, req domain.ReversalRequest) (*domain.Transaction, error)
	GetTransaction(ctx context.Context, id uuid.UUID) (*domain.Transaction, error)
	ListTransactions(ctx context.Context) ([]*domain.Transaction, error)
}

// Handler wires all HTTP routes.
type Handler struct {
	accounts AccountService
	ledger   LedgerService
	mux      *http.ServeMux
}

func New(accounts *service.AccountService, ledger *service.LedgerService) http.Handler {
	h := &Handler{
		accounts: accounts,
		ledger:   ledger,
		mux:      http.NewServeMux(),
	}
	h.registerRoutes()
	return corsMiddleware(h.mux)
}

func NewWithFrontend(accounts *service.AccountService, ledger *service.LedgerService, frontendDir string) http.Handler {
	h := &Handler{
		accounts: accounts,
		ledger:   ledger,
		mux:      http.NewServeMux(),
	}
	h.registerRoutes()
	// Serve static frontend files at root with no-cache headers
	fs := http.FileServer(http.Dir(frontendDir))
	h.mux.Handle("/", noCacheMiddleware(fs))
	return corsMiddleware(h.mux)
}

func (h *Handler) registerRoutes() {
	// Accounts
	h.mux.HandleFunc("/api/accounts", h.handleAccounts)
	h.mux.HandleFunc("/api/accounts/", h.handleAccountByID)

	// Ledger operations
	h.mux.HandleFunc("/api/transfers", h.handleTransfer)
	h.mux.HandleFunc("/api/deposits", h.handleDeposit)
	h.mux.HandleFunc("/api/withdrawals", h.handleWithdrawal)
	h.mux.HandleFunc("/api/reversals", h.handleReversal)

	// Audit log / transactions
	h.mux.HandleFunc("/api/transactions", h.handleTransactions)
	h.mux.HandleFunc("/api/transactions/", h.handleTransactionByID)

	// Swagger UI
	h.mux.Handle("/swagger/", httpSwagger.WrapHandler)
}

// -- Account handlers --------------------------------------------------------


func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	h.handleAccounts(w, r)
}


func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	h.handleAccounts(w, r)
}

func (h *Handler) handleAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		accounts, err := h.accounts.ListAccounts(r.Context())
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
		writeJSON(w, accounts, http.StatusOK)

	case http.MethodPost:
		var body struct {
			Name     string `json:"name"`
			Currency string `json:"currency"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		acct, err := h.accounts.CreateAccount(r.Context(), body.Name, body.Currency)
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
		writeJSON(w, acct, http.StatusCreated)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}


func (h *Handler) handleAccountByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := parseUUID(r.URL.Path, "/api/accounts/")
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	acct, err := h.accounts.GetAccount(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrAccountNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, err, status)
		return
	}
	writeJSON(w, acct, http.StatusOK)
}

// -- Ledger handlers ---------------------------------------------------------


func (h *Handler) handleTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		FromAccountID  string `json:"from_account_id"`
		ToAccountID    string `json:"to_account_id"`
		Amount         int64  `json:"amount"`
		Note           string `json:"note"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	fromID, err := uuid.Parse(body.FromAccountID)
	if err != nil {
		writeError(w, errors.New("invalid from_account_id"), http.StatusBadRequest)
		return
	}
	toID, err := uuid.Parse(body.ToAccountID)
	if err != nil {
		writeError(w, errors.New("invalid to_account_id"), http.StatusBadRequest)
		return
	}

	tx, err := h.ledger.Transfer(r.Context(), domain.TransferRequest{
		FromAccountID:  fromID,
		ToAccountID:    toID,
		Amount:         body.Amount,
		Note:           body.Note,
		IdempotencyKey: body.IdempotencyKey,
	})
	if err != nil {
		writeError(w, err, domainStatus(err))
		return
	}
	writeJSON(w, tx, http.StatusCreated)
}

func (h *Handler) handleDeposit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		AccountID      string `json:"account_id"`
		Amount         int64  `json:"amount"`
		Note           string `json:"note"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	accountID, err := uuid.Parse(body.AccountID)
	if err != nil {
		writeError(w, errors.New("invalid account_id"), http.StatusBadRequest)
		return
	}

	tx, err := h.ledger.Deposit(r.Context(), domain.DepositRequest{
		AccountID:      accountID,
		Amount:         body.Amount,
		Note:           body.Note,
		IdempotencyKey: body.IdempotencyKey,
	})
	if err != nil {
		writeError(w, err, domainStatus(err))
		return
	}
	writeJSON(w, tx, http.StatusCreated)
}

func (h *Handler) handleWithdrawal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		AccountID      string `json:"account_id"`
		Amount         int64  `json:"amount"`
		Note           string `json:"note"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	accountID, err := uuid.Parse(body.AccountID)
	if err != nil {
		writeError(w, errors.New("invalid account_id"), http.StatusBadRequest)
		return
	}

	tx, err := h.ledger.Withdraw(r.Context(), domain.WithdrawalRequest{
		AccountID:      accountID,
		Amount:         body.Amount,
		Note:           body.Note,
		IdempotencyKey: body.IdempotencyKey,
	})
	if err != nil {
		writeError(w, err, domainStatus(err))
		return
	}
	writeJSON(w, tx, http.StatusCreated)
}

// handleReversal godoc
// @Summary Reverse a transaction
// @Description Reverse a previously completed transaction
// @Tags ledger
// @Accept json
// @Produce json
// @Param request body domain.ReversalRequest true "Reversal request"
// @Success 201 {object} domain.Transaction
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /reversals [post]
func (h *Handler) handleReversal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		TransactionID  string `json:"transaction_id"`
		Note           string `json:"note"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	txID, err := uuid.Parse(body.TransactionID)
	if err != nil {
		writeError(w, errors.New("invalid transaction_id"), http.StatusBadRequest)
		return
	}

	tx, err := h.ledger.Reverse(r.Context(), domain.ReversalRequest{
		TransactionID:  txID,
		Note:           body.Note,
		IdempotencyKey: body.IdempotencyKey,
	})
	if err != nil {
		writeError(w, err, domainStatus(err))
		return
	}
	writeJSON(w, tx, http.StatusCreated)
}


func (h *Handler) handleTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	txs, err := h.ledger.ListTransactions(r.Context())
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, txs, http.StatusOK)
}


func (h *Handler) handleTransactionByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := parseUUID(r.URL.Path, "/api/transactions/")
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	tx, err := h.ledger.GetTransaction(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrTransactionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, err, status)
		return
	}
	writeJSON(w, tx, http.StatusOK)
}

// -- helpers -----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, err error, status int) {
	log.Printf("handler error [%d]: %v", status, err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}) //nolint:errcheck
}

func parseUUID(path, prefix string) (uuid.UUID, error) {
	raw := strings.TrimPrefix(path, prefix)
	raw = strings.TrimSuffix(raw, "/")
	return uuid.Parse(raw)
}

func domainStatus(err error) int {
	switch {
	case errors.Is(err, domain.ErrAccountNotFound),
		errors.Is(err, domain.ErrTransactionNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrInsufficientFunds),
		errors.Is(err, domain.ErrInvalidAmount),
		errors.Is(err, domain.ErrSameAccount),
		errors.Is(err, domain.ErrAlreadyReversed),
		errors.Is(err, domain.ErrCannotReverseType):
		return http.StatusUnprocessableEntity
	case errors.Is(err, domain.ErrIdempotentReplay):
		return http.StatusOK
	default:
		return http.StatusInternalServerError
	}
}

func noCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
