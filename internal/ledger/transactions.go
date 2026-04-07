package ledger

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var amountPattern = regexp.MustCompile(`^\d+(\.\d{2})?$`)

type TransactionBatchRequest struct {
	BatchID      string        `json:"batch_id"`
	Source       string        `json:"source"`
	Transactions []Transaction `json:"transactions"`
}

type Transaction struct {
	IdempotencyKey string            `json:"idempotency_key"`
	TenantID       string            `json:"tenant_id"`
	AccountID      string            `json:"account_id"`
	Type           string            `json:"type"`
	Amount         Amount            `json:"amount"`
	Reference      string            `json:"reference,omitempty"`
	OccurredAt     time.Time         `json:"occurred_at"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type Amount struct {
	Currency string `json:"currency"`
	Value    string `json:"value"`
}

func (r *TransactionBatchRequest) ValidateAndNormalize() error {
	r.BatchID = strings.TrimSpace(r.BatchID)
	r.Source = strings.TrimSpace(strings.ToLower(r.Source))

	if r.BatchID == "" {
		return fmt.Errorf("batch_id is required")
	}

	if r.Source == "" {
		return fmt.Errorf("source is required")
	}

	if len(r.Transactions) == 0 {
		return fmt.Errorf("transactions must contain at least one item")
	}

	for i := range r.Transactions {
		if err := r.Transactions[i].ValidateAndNormalize(i); err != nil {
			return err
		}
	}

	return nil
}

func (t *Transaction) ValidateAndNormalize(index int) error {
	t.IdempotencyKey = strings.TrimSpace(t.IdempotencyKey)
	t.TenantID = strings.TrimSpace(t.TenantID)
	t.AccountID = strings.TrimSpace(t.AccountID)
	t.Type = strings.ToLower(strings.TrimSpace(t.Type))
	t.Reference = strings.TrimSpace(t.Reference)
	t.Amount.Currency = strings.ToUpper(strings.TrimSpace(t.Amount.Currency))
	t.Amount.Value = strings.TrimSpace(t.Amount.Value)

	if t.IdempotencyKey == "" {
		return fmt.Errorf("transactions[%d].idempotency_key is required", index)
	}

	if t.TenantID == "" {
		return fmt.Errorf("transactions[%d].tenant_id is required", index)
	}

	if t.AccountID == "" {
		return fmt.Errorf("transactions[%d].account_id is required", index)
	}

	switch t.Type {
	case "credit", "debit":
	default:
		return fmt.Errorf("transactions[%d].type must be credit or debit", index)
	}

	if len(t.Amount.Currency) != 3 {
		return fmt.Errorf("transactions[%d].amount.currency must be a 3-letter currency code", index)
	}

	if !amountPattern.MatchString(t.Amount.Value) {
		return fmt.Errorf("transactions[%d].amount.value must be a positive decimal string like 12.50", index)
	}

	amountValue, err := strconv.ParseFloat(t.Amount.Value, 64)
	if err != nil || amountValue <= 0 {
		return fmt.Errorf("transactions[%d].amount.value must be greater than zero", index)
	}

	if t.OccurredAt.IsZero() {
		return fmt.Errorf("transactions[%d].occurred_at is required", index)
	}

	return nil
}
