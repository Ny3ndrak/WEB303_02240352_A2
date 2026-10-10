package main

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	equipmentpb "equipbooking/proto/equipment"
	"equipbooking/resilience"
)

// EquipmentClient wraps calls to equipment-service (Service B) with a
// timeout, retries with backoff, and a circuit breaker — Requirement 5.
type EquipmentClient struct {
	client  equipmentpb.EquipmentServiceClient
	breaker *resilience.CircuitBreaker
}

func NewEquipmentClient(addr string) (*EquipmentClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &EquipmentClient{
		client:  equipmentpb.NewEquipmentServiceClient(conn),
		breaker: resilience.NewCircuitBreaker(3, 5*time.Second), // trip after 3 failures, cool down 5s
	}, nil
}

// isRetryable: only retry conditions that might clear up on their own.
// A bad request (InvalidArgument) or missing equipment (NotFound) will
// never succeed on retry, so those fail fast instead of wasting attempts.
func isRetryable(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return true // network-level error, not a gRPC status: worth retrying
	}
	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}

// GetEquipment confirms equipment exists before booking-service stores a
// booking against it. 3 attempts, 2s timeout per attempt, exponential
// backoff starting at 100ms (100ms, 200ms), all guarded by the breaker.
func (ec *EquipmentClient) GetEquipment(ctx context.Context, id string) (*equipmentpb.Equipment, error) {
	var e *equipmentpb.Equipment
	err := resilience.Call(ctx, ec.breaker, 3, 2*time.Second, 100*time.Millisecond, isRetryable,
		func(attemptCtx context.Context) error {
			res, err := ec.client.GetEquipment(attemptCtx, &equipmentpb.GetEquipmentRequest{Id: id})
			if err != nil {
				return err
			}
			e = res
			return nil
		},
	)
	return e, err
}
