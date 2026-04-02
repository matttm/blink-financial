package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

func (s *Service) RecordTransactionBatch(outcome string, transactionCount int, batchBytes int, duration time.Duration) {
	s.transactionBatchesTotal.WithLabelValues(outcome).Inc()
	s.transactionItemsTotal.WithLabelValues(outcome).Add(float64(transactionCount))
	s.transactionBatchBytesTotal.WithLabelValues(outcome).Add(float64(batchBytes))
	s.transactionRequestDuration.WithLabelValues("transactions", outcome).Observe(duration.Seconds())
}
