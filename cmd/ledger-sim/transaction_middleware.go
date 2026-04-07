package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/matttm/blink-financial/internal/ledger"
	"github.com/matttm/blink-financial/internal/metrics"
)

type requestContextKey string

const transactionBatchContextKey requestContextKey = "transactionBatch"

type validatedTransactionBatch struct {
	Batch      ledger.TransactionBatchRequest
	BatchBytes int
	Outcome    *string
}

func transactionValidationMiddleware(metricsService *metrics.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		outcome := "accepted"
		transactionCount := 0
		batchBytes := 0

		defer func() {
			metricsService.RecordTransactionBatch(outcome, transactionCount, batchBytes, time.Since(start))
		}()

		if r.Method != http.MethodPost {
			outcome = "method_not_allowed"
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "" && !strings.Contains(strings.ToLower(contentType), "application/json") {
			outcome = "invalid_content_type"
			http.Error(w, "content type must be application/json", http.StatusBadRequest)
			return
		}

		defer r.Body.Close()

		body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
		if err != nil {
			outcome = "read_error"
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		batchBytes = len(body)

		if len(strings.TrimSpace(string(body))) == 0 {
			outcome = "empty_batch"
			http.Error(w, "empty batch", http.StatusBadRequest)
			return
		}

		var batch ledger.TransactionBatchRequest
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&batch); err != nil {
			outcome = "invalid_json"
			http.Error(w, "invalid transaction batch JSON", http.StatusBadRequest)
			return
		}

		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			outcome = "invalid_json"
			http.Error(w, "request body must contain a single JSON object", http.StatusBadRequest)
			return
		}

		transactionCount = len(batch.Transactions)
		if err := batch.ValidateAndNormalize(); err != nil {
			outcome = "invalid_batch"
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		ctx := context.WithValue(r.Context(), transactionBatchContextKey, validatedTransactionBatch{
			Batch:      batch,
			BatchBytes: batchBytes,
			Outcome:    &outcome,
		})

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func transactionBatchFromContext(ctx context.Context) (validatedTransactionBatch, bool) {
	batch, ok := ctx.Value(transactionBatchContextKey).(validatedTransactionBatch)
	return batch, ok
}
