package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"banking-system/internal/domain"

	"github.com/google/uuid"
)

// --- mock services ----------------------------------------------------------

type stubAccountService struct {
	getAccountFn   func(ctx context.Context, id uuid.UUID) (*domain.Account, error)
	listAccountsFn func(ctx context.Context) ([]*domain.Account, error)
	createAccountFn func(ctx context.Context, name, currency string) (*domain.Account, error)
}

func (s *stubAccountService) GetAccount(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	return s.getAccountFn(ctx, id)
}
func (s *stubAccountService) ListAccounts(ctx context.Context) ([]*domain.Account, error) {
	return s.listAccountsFn(ctx)
}
func (s *stubAccountService) CreateAccount(ctx context.Context, name, currency string) (*domain.Account, error) {
	return s.createAccountFn(ctx, name, currency)
}

type stubLedgerService struct {
	transferFn         func(ctx context.Context, req domain.TransferRequest) (*domain.Transaction, error)
	depositFn          func(ctx context.Context, req domain.DepositRequest) (*domain.Transaction, error)
	withdrawFn         func(ctx context.Context, req domain.WithdrawalRequest) (*domain.Transaction, error)
	reverseFn          func(ctx context.Context, req domain.ReversalRequest) (*domain.Transaction, error)
	getTransactionFn   func(ctx context.Context, id uuid.UUID) (*domain.Transaction, error)
	listTransactionsFn func(ctx context.Context) ([]*domain.Transaction, error)
}

func (s *stubLedgerService) Transfer(ctx context.Context, req domain.TransferRequest) (*domain.Transaction, error) {
	return s.transferFn(ctx, req)
}
func (s *stubLedgerService) Deposit(ctx context.Context, req domain.DepositRequest) (*domain.Transaction, error) {
	return s.depositFn(ctx, req)
}
func (s *stubLedgerService) Withdraw(ctx context.Context, req domain.WithdrawalRequest) (*domain.Transaction, error) {
	return s.withdrawFn(ctx, req)
}
func (s *stubLedgerService) Reverse(ctx context.Context, req domain.ReversalRequest) (*domain.Transaction, error) {
	return s.reverseFn(ctx, req)
}
func (s *stubLedgerService) GetTransaction(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	return s.getTransactionFn(ctx, id)
}
func (s *stubLedgerService) ListTransactions(ctx context.Context) ([]*domain.Transaction, error) {
	return s.listTransactionsFn(ctx)
}

// newTestHandler creates a Handler wired with stubs and returns its mux.
func newTestHandler(acctSvc *stubAccountService, ledgerSvc *stubLedgerService) *Handler {
	h := &Handler{
		accounts: acctSvc,
		ledger:   ledgerSvc,
		mux:      http.NewServeMux(),
	}
	h.registerRoutes()
	return h
}

// --- Account endpoint tests -------------------------------------------------

func TestListAccounts_OK(t *testing.T) {
	acctSvc := &stubAccountService{
		listAccountsFn: func(context.Context) ([]*domain.Account, error) {
			return []*domain.Account{{ID: uuid.New(), Name: "Alice"}}, nil
		},
	}
	h := newTestHandler(acctSvc, &stubLedgerService{})

	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var accounts []domain.Account
	json.NewDecoder(w.Body).Decode(&accounts)
	if len(accounts) != 1 || accounts[0].Name != "Alice" {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestListAccounts_Error(t *testing.T) {
	acctSvc := &stubAccountService{
		listAccountsFn: func(context.Context) ([]*domain.Account, error) {
			return nil, errors.New("db error")
		},
	}
	h := newTestHandler(acctSvc, &stubLedgerService{})

	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCreateAccount_OK(t *testing.T) {
	acctSvc := &stubAccountService{
		createAccountFn: func(_ context.Context, name, currency string) (*domain.Account, error) {
			return &domain.Account{ID: uuid.New(), Name: name, Currency: currency, Balance: 0}, nil
		},
	}
	h := newTestHandler(acctSvc, &stubLedgerService{})

	body := `{"name":"Bob","currency":"USD"}`
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var acct domain.Account
	json.NewDecoder(w.Body).Decode(&acct)
	if acct.Name != "Bob" || acct.Currency != "USD" {
		t.Errorf("unexpected account: %+v", acct)
	}
}

func TestCreateAccount_BadJSON(t *testing.T) {
	h := newTestHandler(&stubAccountService{}, &stubLedgerService{})

	req := httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAccounts_MethodNotAllowed(t *testing.T) {
	h := newTestHandler(&stubAccountService{}, &stubLedgerService{})

	req := httptest.NewRequest(http.MethodDelete, "/api/accounts", nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestGetAccountByID_OK(t *testing.T) {
	id := uuid.New()
	acctSvc := &stubAccountService{
		getAccountFn: func(_ context.Context, gotID uuid.UUID) (*domain.Account, error) {
			return &domain.Account{ID: gotID, Name: "Alice"}, nil
		},
	}
	h := newTestHandler(acctSvc, &stubLedgerService{})

	req := httptest.NewRequest(http.MethodGet, "/api/accounts/"+id.String(), nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetAccountByID_InvalidUUID(t *testing.T) {
	h := newTestHandler(&stubAccountService{}, &stubLedgerService{})

	req := httptest.NewRequest(http.MethodGet, "/api/accounts/not-a-uuid", nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetAccountByID_NotFound(t *testing.T) {
	acctSvc := &stubAccountService{
		getAccountFn: func(context.Context, uuid.UUID) (*domain.Account, error) {
			return nil, domain.ErrAccountNotFound
		},
	}
	h := newTestHandler(acctSvc, &stubLedgerService{})

	req := httptest.NewRequest(http.MethodGet, "/api/accounts/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetAccountByID_MethodNotAllowed(t *testing.T) {
	h := newTestHandler(&stubAccountService{}, &stubLedgerService{})

	req := httptest.NewRequest(http.MethodPost, "/api/accounts/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// --- Transfer endpoint tests ------------------------------------------------

func TestHandleTransfer_OK(t *testing.T) {
	fromID := uuid.New()
	toID := uuid.New()
	ledgerSvc := &stubLedgerService{
		transferFn: func(_ context.Context, req domain.TransferRequest) (*domain.Transaction, error) {
			return &domain.Transaction{ID: uuid.New(), Type: domain.TypeTransfer, Status: domain.StatusSuccess, Amount: req.Amount}, nil
		},
	}
	h := newTestHandler(&stubAccountService{}, ledgerSvc)

	body, _ := json.Marshal(map[string]interface{}{
		"from_account_id": fromID.String(),
		"to_account_id":   toID.String(),
		"amount":          1000,
		"note":            "test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/transfers", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTransfer_InvalidFromID(t *testing.T) {
	h := newTestHandler(&stubAccountService{}, &stubLedgerService{})

	body := `{"from_account_id":"bad","to_account_id":"` + uuid.New().String() + `","amount":100}`
	req := httptest.NewRequest(http.MethodPost, "/api/transfers", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleTransfer_InvalidToID(t *testing.T) {
	h := newTestHandler(&stubAccountService{}, &stubLedgerService{})

	body := `{"from_account_id":"` + uuid.New().String() + `","to_account_id":"bad","amount":100}`
	req := httptest.NewRequest(http.MethodPost, "/api/transfers", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleTransfer_MethodNotAllowed(t *testing.T) {
	h := newTestHandler(&stubAccountService{}, &stubLedgerService{})

	req := httptest.NewRequest(http.MethodGet, "/api/transfers", nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleTransfer_DomainErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"insufficient funds", domain.ErrInsufficientFunds, http.StatusUnprocessableEntity},
		{"invalid amount", domain.ErrInvalidAmount, http.StatusUnprocessableEntity},
		{"same account", domain.ErrSameAccount, http.StatusUnprocessableEntity},
		{"account not found", domain.ErrAccountNotFound, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ledgerSvc := &stubLedgerService{
				transferFn: func(context.Context, domain.TransferRequest) (*domain.Transaction, error) {
					return nil, tt.err
				},
			}
			h := newTestHandler(&stubAccountService{}, ledgerSvc)

			body, _ := json.Marshal(map[string]interface{}{
				"from_account_id": uuid.New().String(),
				"to_account_id":   uuid.New().String(),
				"amount":          100,
			})
			req := httptest.NewRequest(http.MethodPost, "/api/transfers", bytes.NewBuffer(body))
			w := httptest.NewRecorder()
			h.mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

// --- Deposit endpoint tests -------------------------------------------------

func TestHandleDeposit_OK(t *testing.T) {
	ledgerSvc := &stubLedgerService{
		depositFn: func(_ context.Context, req domain.DepositRequest) (*domain.Transaction, error) {
			return &domain.Transaction{ID: uuid.New(), Type: domain.TypeDeposit, Status: domain.StatusSuccess}, nil
		},
	}
	h := newTestHandler(&stubAccountService{}, ledgerSvc)

	body, _ := json.Marshal(map[string]interface{}{
		"account_id": uuid.New().String(),
		"amount":     500,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/deposits", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestHandleDeposit_InvalidAccountID(t *testing.T) {
	h := newTestHandler(&stubAccountService{}, &stubLedgerService{})

	body := `{"account_id":"bad","amount":100}`
	req := httptest.NewRequest(http.MethodPost, "/api/deposits", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleDeposit_MethodNotAllowed(t *testing.T) {
	h := newTestHandler(&stubAccountService{}, &stubLedgerService{})

	req := httptest.NewRequest(http.MethodGet, "/api/deposits", nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// --- Withdrawal endpoint tests ----------------------------------------------

func TestHandleWithdrawal_OK(t *testing.T) {
	ledgerSvc := &stubLedgerService{
		withdrawFn: func(_ context.Context, req domain.WithdrawalRequest) (*domain.Transaction, error) {
			return &domain.Transaction{ID: uuid.New(), Type: domain.TypeWithdrawal, Status: domain.StatusSuccess}, nil
		},
	}
	h := newTestHandler(&stubAccountService{}, ledgerSvc)

	body, _ := json.Marshal(map[string]interface{}{
		"account_id": uuid.New().String(),
		"amount":     300,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/withdrawals", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestHandleWithdrawal_InvalidAccountID(t *testing.T) {
	h := newTestHandler(&stubAccountService{}, &stubLedgerService{})

	body := `{"account_id":"nope","amount":100}`
	req := httptest.NewRequest(http.MethodPost, "/api/withdrawals", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleWithdrawal_MethodNotAllowed(t *testing.T) {
	h := newTestHandler(&stubAccountService{}, &stubLedgerService{})

	req := httptest.NewRequest(http.MethodGet, "/api/withdrawals", nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// --- Reversal endpoint tests ------------------------------------------------

func TestHandleReversal_OK(t *testing.T) {
	ledgerSvc := &stubLedgerService{
		reverseFn: func(_ context.Context, req domain.ReversalRequest) (*domain.Transaction, error) {
			return &domain.Transaction{ID: uuid.New(), Type: domain.TypeReversal, Status: domain.StatusSuccess}, nil
		},
	}
	h := newTestHandler(&stubAccountService{}, ledgerSvc)

	body, _ := json.Marshal(map[string]interface{}{
		"transaction_id": uuid.New().String(),
		"note":           "undo",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/reversals", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestHandleReversal_InvalidTxID(t *testing.T) {
	h := newTestHandler(&stubAccountService{}, &stubLedgerService{})

	body := `{"transaction_id":"bad"}`
	req := httptest.NewRequest(http.MethodPost, "/api/reversals", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleReversal_DomainErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"already reversed", domain.ErrAlreadyReversed, http.StatusUnprocessableEntity},
		{"cannot reverse type", domain.ErrCannotReverseType, http.StatusUnprocessableEntity},
		{"tx not found", domain.ErrTransactionNotFound, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ledgerSvc := &stubLedgerService{
				reverseFn: func(context.Context, domain.ReversalRequest) (*domain.Transaction, error) {
					return nil, tt.err
				},
			}
			h := newTestHandler(&stubAccountService{}, ledgerSvc)

			body, _ := json.Marshal(map[string]interface{}{
				"transaction_id": uuid.New().String(),
			})
			req := httptest.NewRequest(http.MethodPost, "/api/reversals", bytes.NewBuffer(body))
			w := httptest.NewRecorder()
			h.mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

// --- Transaction endpoint tests ---------------------------------------------

func TestListTransactions_OK(t *testing.T) {
	ledgerSvc := &stubLedgerService{
		listTransactionsFn: func(context.Context) ([]*domain.Transaction, error) {
			return []*domain.Transaction{{ID: uuid.New()}}, nil
		},
	}
	h := newTestHandler(&stubAccountService{}, ledgerSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/transactions", nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestListTransactions_MethodNotAllowed(t *testing.T) {
	h := newTestHandler(&stubAccountService{}, &stubLedgerService{})

	req := httptest.NewRequest(http.MethodPost, "/api/transactions", nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestGetTransactionByID_OK(t *testing.T) {
	id := uuid.New()
	ledgerSvc := &stubLedgerService{
		getTransactionFn: func(_ context.Context, gotID uuid.UUID) (*domain.Transaction, error) {
			return &domain.Transaction{ID: gotID, Type: domain.TypeDeposit}, nil
		},
	}
	h := newTestHandler(&stubAccountService{}, ledgerSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/transactions/"+id.String(), nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetTransactionByID_InvalidUUID(t *testing.T) {
	h := newTestHandler(&stubAccountService{}, &stubLedgerService{})

	req := httptest.NewRequest(http.MethodGet, "/api/transactions/not-uuid", nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetTransactionByID_NotFound(t *testing.T) {
	ledgerSvc := &stubLedgerService{
		getTransactionFn: func(context.Context, uuid.UUID) (*domain.Transaction, error) {
			return nil, domain.ErrTransactionNotFound
		},
	}
	h := newTestHandler(&stubAccountService{}, ledgerSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/transactions/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// --- Helper function tests --------------------------------------------------

func TestParseUUID(t *testing.T) {
	id := uuid.New()
	got, err := parseUUID("/api/accounts/"+id.String(), "/api/accounts/")
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Errorf("expected %v, got %v", id, got)
	}
}

func TestParseUUID_TrailingSlash(t *testing.T) {
	id := uuid.New()
	got, err := parseUUID("/api/accounts/"+id.String()+"/", "/api/accounts/")
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Errorf("expected %v, got %v", id, got)
	}
}

func TestParseUUID_Invalid(t *testing.T) {
	_, err := parseUUID("/api/accounts/garbage", "/api/accounts/")
	if err == nil {
		t.Error("expected error for invalid UUID")
	}
}

func TestDomainStatus(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{domain.ErrAccountNotFound, http.StatusNotFound},
		{domain.ErrTransactionNotFound, http.StatusNotFound},
		{domain.ErrInsufficientFunds, http.StatusUnprocessableEntity},
		{domain.ErrInvalidAmount, http.StatusUnprocessableEntity},
		{domain.ErrSameAccount, http.StatusUnprocessableEntity},
		{domain.ErrAlreadyReversed, http.StatusUnprocessableEntity},
		{domain.ErrCannotReverseType, http.StatusUnprocessableEntity},
		{domain.ErrIdempotentReplay, http.StatusOK},
		{errors.New("unknown"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		got := domainStatus(tt.err)
		if got != tt.want {
			t.Errorf("domainStatus(%v) = %d, want %d", tt.err, got, tt.want)
		}
	}
}

func TestCORSMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := corsMiddleware(inner)

	// Regular request
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin header")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// OPTIONS preflight
	req = httptest.NewRequest(http.MethodOptions, "/", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS, got %d", w.Code)
	}
}

func TestNoCacheMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := noCacheMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Cache-Control") == "" {
		t.Error("missing Cache-Control header")
	}
	if w.Header().Get("Pragma") != "no-cache" {
		t.Error("missing Pragma header")
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, map[string]string{"key": "value"}, http.StatusOK)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["key"] != "value" {
		t.Errorf("unexpected body: %v", body)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, errors.New("something broke"), http.StatusBadRequest)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "something broke" {
		t.Errorf("unexpected error message: %v", body)
	}
}
