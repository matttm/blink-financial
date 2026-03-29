package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/matttm/blink-financial/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

const maxBodyBytes = 1 << 20

var (
	transactionBatchesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "blink_ledger_transaction_batches_total",
			Help: "Total number of transaction batches handled by outcome.",
		},
		[]string{"outcome"},
	)
	transactionItemsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "blink_ledger_transactions_total",
			Help: "Total number of transaction items observed in handled batches by outcome.",
		},
		[]string{"outcome"},
	)
	transactionBatchBytesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "blink_ledger_batch_bytes_total",
			Help: "Total bytes received in transaction batches by outcome.",
		},
		[]string{"outcome"},
	)
	transactionRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "blink_ledger_request_duration_seconds",
			Help:    "Latency for ledger HTTP request handling.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint", "outcome"},
	)
)

func init() {
	prometheus.MustRegister(
		transactionBatchesTotal,
		transactionItemsTotal,
		transactionBatchBytesTotal,
		transactionRequestDuration,
	)
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	apiMux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		if err := redisPing(ctx, cfg.RedisAddr); err != nil {
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
			transactionBatchesTotal.WithLabelValues(outcome).Inc()
			transactionItemsTotal.WithLabelValues(outcome).Add(float64(transactionCount))
			transactionBatchBytesTotal.WithLabelValues(outcome).Add(float64(batchBytes))
			transactionRequestDuration.WithLabelValues("transactions", outcome).Observe(time.Since(start).Seconds())
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

		if err := redisRPush(ctx, cfg.RedisAddr, cfg.RedisListKey, record); err != nil {
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
	rootMux.Handle("/metrics", promhttp.Handler())

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

func redisPing(ctx context.Context, addr string) error {
	return redisCommand(ctx, addr, "PING")
}

func redisRPush(ctx context.Context, addr, key, value string) error {
	return redisCommand(ctx, addr, "RPUSH", key, value)
}

func redisCommand(ctx context.Context, addr string, args ...string) error {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	if _, err := io.WriteString(conn, encodeRESP(args...)); err != nil {
		return err
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}

	if strings.HasPrefix(line, "-") {
		return fmt.Errorf("redis error: %s", strings.TrimSpace(line))
	}

	return nil
}

func encodeRESP(args ...string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "*%d\r\n", len(args))
	for _, arg := range args {
		fmt.Fprintf(&builder, "$%d\r\n%s\r\n", len(arg), arg)
	}
	return builder.String()
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
