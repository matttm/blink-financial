package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/matttm/blink-financial/internal/config"
	"github.com/matttm/blink-financial/internal/metrics"
	"github.com/matttm/blink-financial/internal/store"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const maxBodyBytes = 1 << 20

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	metricsService := metrics.NewService(nil, nil)
	redisQueue := store.NewRedisQueue(cfg.RedisAddr)
	defer redisQueue.Close()
	metricsService.RegisterRedisPoolStats(nil, redisQueue.PoolStats)

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	apiMux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		if err := redisQueue.Ping(ctx); err != nil {
			http.Error(w, "redis unavailable", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	apiMux.HandleFunc("/transactions", func(w http.ResponseWriter, r *http.Request) {
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

		defer r.Body.Close()

		body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
		if err != nil {
			outcome = "read_error"
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		batchBytes = len(body)
		transactionCount = countTransactionItems(body)

		if len(body) == 0 {
			outcome = "empty_batch"
			http.Error(w, "empty batch", http.StatusBadRequest)
			return
		}

		payload := strings.TrimSpace(string(body))
		record := fmt.Sprintf(`{"instance":"%s","received_at":"%s","payload":%q}`,
			cfg.InstanceID,
			time.Now().UTC().Format(time.RFC3339Nano),
			payload,
		)

		ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
		defer cancel()

		if err := redisQueue.Enqueue(ctx, cfg.RedisListKey, record); err != nil {
			outcome = "redis_error"
			http.Error(w, "failed to enqueue transaction batch", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(w, `{"status":"accepted","instance":"%s"}`+"\n", cfg.InstanceID)
	})

	// 2. Create the main, root ServeMux.
	rootMux := http.NewServeMux()

	// 3. Register the apiMux with the rootMux, using http.StripPrefix.
	// Requests to "/api/v1/" (and anything under it) will be passed to apiMux,
	// but the "/api/v1" part of the path will be stripped first.
	rootMux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiMux))
	rootMux.Handle("/metrics", metricsService.Handler())

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           requestLogger(rootMux),
		ReadHeaderTimeout: 2 * time.Second,
	}

	log.Printf("blink ledger simulator listening on :%s, redis=%s", cfg.Port, cfg.RedisAddr)
	log.Fatal(server.ListenAndServe())
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func countTransactionItems(body []byte) int {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return 0
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	token, err := decoder.Token()
	if err != nil {
		return 1
	}

	delim, ok := token.(json.Delim)
	if !ok || delim != '[' {
		return 1
	}

	count := 0
	for decoder.More() {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if count == 0 {
				return 1
			}
			return count
		}
		count++
	}

	return count
}
