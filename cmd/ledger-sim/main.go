package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/matttm/blink-financial/internal/config"
	transactionsv1 "github.com/matttm/blink-financial/internal/gen/blink/transactions/v1"
	grpcapi "github.com/matttm/blink-financial/internal/grpcapi"
	"github.com/matttm/blink-financial/internal/ingest"
	"github.com/matttm/blink-financial/internal/metrics"
	"github.com/matttm/blink-financial/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
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
	ingestService := ingest.NewService(cfg.InstanceID, cfg.RedisListKey, redisQueue)

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

	apiMux.Handle("/transactions", transactionValidationMiddleware(metricsService, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		validatedBatch, ok := transactionBatchFromContext(r.Context())
		if !ok {
			http.Error(w, "validated transaction batch missing from request context", http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
		defer cancel()

		if err := ingestService.EnqueueBatch(ctx, validatedBatch.Batch); err != nil {
			if validatedBatch.Outcome != nil {
				*validatedBatch.Outcome = "redis_error"
			}
			http.Error(w, "failed to enqueue transaction batch", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(
			w,
			`{"status":"accepted","instance":"%s","batch_id":"%s","transaction_count":%d}`+"\n",
			cfg.InstanceID,
			validatedBatch.Batch.BatchID,
			len(validatedBatch.Batch.Transactions),
		)
	})))

	rootMux := http.NewServeMux()
	rootMux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiMux))
	rootMux.Handle("/metrics", metricsService.Handler())

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           requestLogger(rootMux),
		ReadHeaderTimeout: 2 * time.Second,
	}

	grpcListener, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("listen grpc: %v", err)
	}

	grpcServer := grpc.NewServer()
	transactionsv1.RegisterTransactionEventsServiceServer(grpcServer, grpcapi.NewServer(ingestService))
	reflection.Register(grpcServer)

	go func() {
		log.Printf("blink ledger grpc listening on :%s, redis=%s", cfg.GRPCPort, cfg.RedisAddr)
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatalf("grpc serve: %v", err)
		}
	}()

	log.Printf("blink ledger http listening on :%s, redis=%s", cfg.Port, cfg.RedisAddr)
	log.Fatal(server.ListenAndServe())
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
