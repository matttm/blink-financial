package grpcapi

import (
	"strings"

	transactionsv1 "github.com/matttm/blink-financial/internal/gen/blink/transactions/v1"
	"github.com/matttm/blink-financial/internal/ingest"
	"github.com/matttm/blink-financial/internal/ledger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	transactionsv1.UnimplementedTransactionEventsServiceServer
	ingestService *ingest.Service
}

type streamFilters struct {
	source    string
	tenantID  string
	accountID string
	batchID   string
}

func NewServer(ingestService *ingest.Service) *Server {
	return &Server{
		ingestService: ingestService,
	}
}

func (s *Server) StreamTransactions(req *transactionsv1.StreamTransactionsRequest, stream transactionsv1.TransactionEventsService_StreamTransactionsServer) error {
	if req == nil {
		return status.Error(codes.InvalidArgument, "request is required")
	}

	filters := streamFilters{
		source:    strings.ToLower(strings.TrimSpace(req.GetSource())),
		tenantID:  strings.TrimSpace(req.GetTenantId()),
		accountID: strings.TrimSpace(req.GetAccountId()),
		batchID:   strings.TrimSpace(req.GetBatchId()),
	}

	events, errors := s.ingestService.SubscribeEvents(stream.Context())
	for {
		select {
		case <-stream.Context().Done():
			return nil
		case err, ok := <-errors:
			if !ok {
				errors = nil
				continue
			}
			if err != nil {
				return status.Errorf(codes.Internal, "stream decode error: %v", err)
			}
		case event, ok := <-events:
			if !ok {
				return nil
			}

			filtered := filterEvent(event, filters)
			if filtered == nil {
				continue
			}

			if err := stream.Send(toProtoEvent(*filtered)); err != nil {
				return err
			}
		}
	}
}

func filterEvent(event ingest.BatchEvent, filters streamFilters) *ingest.BatchEvent {
	if filters.source != "" && !strings.EqualFold(event.Source, filters.source) {
		return nil
	}

	if filters.batchID != "" && event.BatchID != filters.batchID {
		return nil
	}

	if filters.tenantID == "" && filters.accountID == "" {
		copyEvent := event
		copyEvent.Transactions = append([]ledger.Transaction(nil), event.Transactions...)
		copyEvent.TransactionCount = len(copyEvent.Transactions)
		return &copyEvent
	}

	filteredTransactions := make([]ledger.Transaction, 0, len(event.Transactions))
	for _, transaction := range event.Transactions {
		if filters.tenantID != "" && transaction.TenantID != filters.tenantID {
			continue
		}
		if filters.accountID != "" && transaction.AccountID != filters.accountID {
			continue
		}
		filteredTransactions = append(filteredTransactions, transaction)
	}

	if len(filteredTransactions) == 0 {
		return nil
	}

	copyEvent := event
	copyEvent.Transactions = filteredTransactions
	copyEvent.TransactionCount = len(filteredTransactions)
	return &copyEvent
}

func toProtoEvent(event ingest.BatchEvent) *transactionsv1.TransactionEvent {
	transactions := make([]*transactionsv1.Transaction, 0, len(event.Transactions))
	for _, transaction := range event.Transactions {
		metadata := transaction.Metadata
		if metadata == nil {
			metadata = map[string]string{}
		}

		transactions = append(transactions, &transactionsv1.Transaction{
			IdempotencyKey: transaction.IdempotencyKey,
			TenantId:       transaction.TenantID,
			AccountId:      transaction.AccountID,
			Type:           transaction.Type,
			Amount: &transactionsv1.Amount{
				Currency: transaction.Amount.Currency,
				Value:    transaction.Amount.Value,
			},
			Reference:  transaction.Reference,
			OccurredAt: timestamppb.New(transaction.OccurredAt),
			Metadata:   metadata,
		})
	}

	return &transactionsv1.TransactionEvent{
		Instance:         event.Instance,
		ReceivedAt:       timestamppb.New(event.ReceivedAt),
		BatchId:          event.BatchID,
		Source:           event.Source,
		TransactionCount: int32(event.TransactionCount),
		Transactions:     transactions,
	}
}
