package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTransactionTypeConstants(t *testing.T) {
	tests := []struct {
		got  TransactionType
		want string
	}{
		{TypeDeposit, "DEPOSIT"},
		{TypeWithdrawal, "WITHDRAWAL"},
		{TypeTransfer, "TRANSFER"},
		{TypeReversal, "REVERSAL"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}

func TestTransactionStatusConstants(t *testing.T) {
	tests := []struct {
		got  TransactionStatus
		want string
	}{
		{StatusPending, "PENDING"},
		{StatusSuccess, "SUCCESS"},
		{StatusFailed, "FAILED"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}

func TestDirectionConstants(t *testing.T) {
	if DirectionDebit != "DEBIT" {
		t.Errorf("got %q, want DEBIT", DirectionDebit)
	}
	if DirectionCredit != "CREDIT" {
		t.Errorf("got %q, want CREDIT", DirectionCredit)
	}
}

func TestAccountJSON(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	acct := Account{
		ID:        id,
		Name:      "Alice",
		Currency:  "USD",
		Balance:   10000,
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(acct)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Account
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != acct.ID || got.Name != acct.Name || got.Currency != acct.Currency || got.Balance != acct.Balance {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func TestTransactionJSON_OmitsNilFields(t *testing.T) {
	tx := Transaction{
		ID:     uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		Type:   TypeDeposit,
		Status: StatusSuccess,
		Amount: 5000,
	}

	data, err := json.Marshal(tx)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	if _, ok := raw["from_account_id"]; ok {
		t.Error("expected from_account_id to be omitted when nil")
	}
	if _, ok := raw["to_account_id"]; ok {
		t.Error("expected to_account_id to be omitted when nil")
	}
	if _, ok := raw["reversal_of"]; ok {
		t.Error("expected reversal_of to be omitted when nil")
	}
	if _, ok := raw["idempotency_key"]; ok {
		t.Error("expected idempotency_key to be omitted when nil")
	}
}

func TestTransactionJSON_IncludesOptionalFields(t *testing.T) {
	fromID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	toID := uuid.MustParse("00000000-0000-0000-0000-000000000004")
	key := "key-123"
	tx := Transaction{
		ID:             uuid.MustParse("00000000-0000-0000-0000-000000000005"),
		Type:           TypeTransfer,
		Status:         StatusSuccess,
		Amount:         1000,
		FromAccountID:  &fromID,
		ToAccountID:    &toID,
		IdempotencyKey: &key,
	}

	data, err := json.Marshal(tx)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Transaction
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.FromAccountID == nil || *got.FromAccountID != fromID {
		t.Error("from_account_id mismatch")
	}
	if got.ToAccountID == nil || *got.ToAccountID != toID {
		t.Error("to_account_id mismatch")
	}
	if got.IdempotencyKey == nil || *got.IdempotencyKey != key {
		t.Error("idempotency_key mismatch")
	}
}

func TestJournalEntryJSON(t *testing.T) {
	entry := JournalEntry{
		ID:            uuid.New(),
		TransactionID: uuid.New(),
		AccountID:     uuid.New(),
		Amount:        -500,
		Direction:     DirectionDebit,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got JournalEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Amount != -500 || got.Direction != DirectionDebit {
		t.Errorf("round-trip mismatch: got amount=%d direction=%s", got.Amount, got.Direction)
	}
}

func TestRequestStructsJSON(t *testing.T) {
	t.Run("TransferRequest", func(t *testing.T) {
		req := TransferRequest{
			FromAccountID: uuid.New(),
			ToAccountID:   uuid.New(),
			Amount:        1000,
			Note:          "test",
		}
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		var got TransferRequest
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if got.Amount != req.Amount || got.FromAccountID != req.FromAccountID {
			t.Error("round-trip mismatch")
		}
	})

	t.Run("DepositRequest", func(t *testing.T) {
		req := DepositRequest{AccountID: uuid.New(), Amount: 500}
		data, _ := json.Marshal(req)
		var got DepositRequest
		json.Unmarshal(data, &got)
		if got.Amount != 500 {
			t.Error("amount mismatch")
		}
	})

	t.Run("WithdrawalRequest", func(t *testing.T) {
		req := WithdrawalRequest{AccountID: uuid.New(), Amount: 300}
		data, _ := json.Marshal(req)
		var got WithdrawalRequest
		json.Unmarshal(data, &got)
		if got.Amount != 300 {
			t.Error("amount mismatch")
		}
	})

	t.Run("ReversalRequest", func(t *testing.T) {
		req := ReversalRequest{TransactionID: uuid.New(), Note: "undo"}
		data, _ := json.Marshal(req)
		var got ReversalRequest
		json.Unmarshal(data, &got)
		if got.Note != "undo" {
			t.Error("note mismatch")
		}
	})
}
