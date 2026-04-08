package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

type Service struct {
	transactionBatchesTotal    *prometheus.CounterVec
	transactionItemsTotal      *prometheus.CounterVec
	transactionBatchBytesTotal *prometheus.CounterVec
	transactionRequestDuration *prometheus.HistogramVec
	handler                    http.Handler
}

func NewService(registerer prometheus.Registerer, gatherer prometheus.Gatherer) *Service {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	if gatherer == nil {
		gatherer = prometheus.DefaultGatherer
	}

	service := &Service{
		transactionBatchesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "blink_ledger_transaction_batches_total",
				Help: "Total number of transaction batches handled by outcome.",
			},
			[]string{"outcome"},
		),
		transactionItemsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "blink_ledger_transactions_total",
				Help: "Total number of transaction items observed in handled batches by outcome.",
			},
			[]string{"outcome"},
		),
		transactionBatchBytesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "blink_ledger_batch_bytes_total",
				Help: "Total bytes received in transaction batches by outcome.",
			},
			[]string{"outcome"},
		),
		transactionRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "blink_ledger_request_duration_seconds",
				Help:    "Latency for ledger HTTP request handling.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"endpoint", "outcome"},
		),
		handler: promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}),
	}

	registerer.MustRegister(
		service.transactionBatchesTotal,
		service.transactionItemsTotal,
		service.transactionBatchBytesTotal,
		service.transactionRequestDuration,
	)

	return service
}

func (s *Service) Handler() http.Handler {
	return s.handler
}

func (s *Service) RecordTransactionBatch(endpoint, outcome string, transactionCount int, batchBytes int, duration time.Duration) {
	s.transactionBatchesTotal.WithLabelValues(outcome).Inc()
	s.transactionItemsTotal.WithLabelValues(outcome).Add(float64(transactionCount))
	s.transactionBatchBytesTotal.WithLabelValues(outcome).Add(float64(batchBytes))
	s.transactionRequestDuration.WithLabelValues(endpoint, outcome).Observe(duration.Seconds())
}

func (s *Service) RegisterRedisPoolStats(registerer prometheus.Registerer, statsFn func() *redis.PoolStats) {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	registerer.MustRegister(
		prometheus.NewCounterFunc(
			prometheus.CounterOpts{
				Name: "blink_redis_pool_hits_total",
				Help: "Total number of Redis pool cache hits.",
			},
			func() float64 {
				return float64(statsFn().Hits)
			},
		),
		prometheus.NewCounterFunc(
			prometheus.CounterOpts{
				Name: "blink_redis_pool_misses_total",
				Help: "Total number of Redis pool misses.",
			},
			func() float64 {
				return float64(statsFn().Misses)
			},
		),
		prometheus.NewCounterFunc(
			prometheus.CounterOpts{
				Name: "blink_redis_pool_timeouts_total",
				Help: "Total number of Redis pool timeouts.",
			},
			func() float64 {
				return float64(statsFn().Timeouts)
			},
		),
		prometheus.NewCounterFunc(
			prometheus.CounterOpts{
				Name: "blink_redis_pool_wait_count_total",
				Help: "Total number of times the Redis client waited for a pooled connection.",
			},
			func() float64 {
				return float64(statsFn().WaitCount)
			},
		),
		prometheus.NewCounterFunc(
			prometheus.CounterOpts{
				Name: "blink_redis_pool_unusable_total",
				Help: "Total number of Redis connections marked unusable.",
			},
			func() float64 {
				return float64(statsFn().Unusable)
			},
		),
		prometheus.NewCounterFunc(
			prometheus.CounterOpts{
				Name: "blink_redis_pool_wait_duration_seconds_total",
				Help: "Total time spent waiting for Redis pool connections.",
			},
			func() float64 {
				return float64(statsFn().WaitDurationNs) / float64(time.Second)
			},
		),
		prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Name: "blink_redis_pool_total_conns",
				Help: "Current number of total Redis pool connections.",
			},
			func() float64 {
				return float64(statsFn().TotalConns)
			},
		),
		prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Name: "blink_redis_pool_idle_conns",
				Help: "Current number of idle Redis pool connections.",
			},
			func() float64 {
				return float64(statsFn().IdleConns)
			},
		),
		prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Name: "blink_redis_pool_stale_conns",
				Help: "Current number of stale Redis pool connections.",
			},
			func() float64 {
				return float64(statsFn().StaleConns)
			},
		),
	)
}
