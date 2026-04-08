package ingest

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/matttm/blink-financial/internal/ledger"
	"github.com/matttm/blink-financial/internal/store"
)

type Service struct {
	instanceID        string
	redisListKey      string
	redisEventChannel string
	redisQueue        *store.RedisQueue
}

type BatchEvent struct {
	Instance         string               `json:"instance"`
	ReceivedAt       time.Time            `json:"received_at"`
	BatchID          string               `json:"batch_id"`
	Source           string               `json:"source"`
	TransactionCount int                  `json:"transaction_count"`
	Transactions     []ledger.Transaction `json:"transactions"`
}

func NewService(instanceID, redisListKey string, redisQueue *store.RedisQueue) *Service {
	return &Service{
		instanceID:        instanceID,
		redisListKey:      redisListKey,
		redisEventChannel: strings.TrimSpace(redisListKey) + ":events",
		redisQueue:        redisQueue,
	}
}

func (s *Service) EnqueueBatch(ctx context.Context, batch ledger.TransactionBatchRequest) error {
	event := BatchEvent{
		Instance:         s.instanceID,
		ReceivedAt:       time.Now().UTC(),
		BatchID:          batch.BatchID,
		Source:           batch.Source,
		TransactionCount: len(batch.Transactions),
		Transactions:     batch.Transactions,
	}

	record, err := json.Marshal(event)
	if err != nil {
		return err
	}

	if err := s.redisQueue.Enqueue(ctx, s.redisListKey, string(record)); err != nil {
		return err
	}

	// Event streaming is an observer path for tools like the TUI; failed fan-out
	// should not reject the primary ingest path after the batch is already queued.
	_ = s.redisQueue.Publish(ctx, s.redisEventChannel, string(record))
	return nil
}

func (s *Service) SubscribeEvents(ctx context.Context) (<-chan BatchEvent, <-chan error) {
	pubsub := s.redisQueue.Subscribe(ctx, s.redisEventChannel)
	output := make(chan BatchEvent)
	errors := make(chan error, 1)

	go func() {
		defer close(output)
		defer close(errors)
		defer pubsub.Close()

		messages := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case message, ok := <-messages:
				if !ok {
					return
				}

				var event BatchEvent
				if err := json.Unmarshal([]byte(message.Payload), &event); err != nil {
					select {
					case errors <- err:
					default:
					}
					continue
				}

				select {
				case output <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return output, errors
}
