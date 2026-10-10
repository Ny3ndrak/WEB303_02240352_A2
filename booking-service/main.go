// booking-service is Service A: it owns Bookings in its own SQLite
// database. CreateBooking calls equipment-service over gRPC (through
// EquipmentClient, with timeout + retry + circuit breaker) to confirm
// the equipment exists before a booking is stored, satisfying
// Requirement 2.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net"
	"os"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "equipbooking/proto/bookings"
	"equipbooking/resilience"
)

var validBookingStatuses = map[string]bool{"pending": true, "approved": true, "rejected": true}

type server struct {
	pb.UnimplementedBookingServiceServer
	store           *Store
	equipmentClient *EquipmentClient
}

func (s *server) CreateBooking(ctx context.Context, req *pb.CreateBookingRequest) (*pb.Booking, error) {
	if strings.TrimSpace(req.EquipmentId) == "" {
		return nil, status.Error(codes.InvalidArgument, "equipment_id is required")
	}
	if strings.TrimSpace(req.RequesterName) == "" {
		return nil, status.Error(codes.InvalidArgument, "requester_name is required")
	}

	// Confirm the equipment exists via Service B's gRPC API — never read its DB directly.
	_, err := s.equipmentClient.GetEquipment(ctx, req.EquipmentId)
	if err != nil {
		if errors.Is(err, resilience.ErrCircuitOpen) {
			return nil, status.Error(codes.Unavailable, "equipment-service is temporarily unavailable (circuit breaker open), please try again shortly")
		}
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.NotFound:
				return nil, status.Errorf(codes.InvalidArgument, "equipment %q does not exist", req.EquipmentId)
			case codes.DeadlineExceeded, codes.Unavailable:
				return nil, status.Error(codes.Unavailable, "equipment-service did not respond in time, please try again")
			}
		}
		return nil, status.Errorf(codes.Internal, "could not verify equipment: %v", err)
	}

	b, err := s.store.Create(uuid.NewString(), req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not create booking: %v", err)
	}
	return b, nil
}

func (s *server) GetBooking(ctx context.Context, req *pb.GetBookingRequest) (*pb.Booking, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	b, err := s.store.Get(req.Id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "booking %q not found", req.Id)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not get booking: %v", err)
	}
	return b, nil
}

func (s *server) ListBookings(ctx context.Context, req *pb.ListBookingsRequest) (*pb.ListBookingsResponse, error) {
	if req.Status != "" && !validBookingStatuses[req.Status] {
		return nil, status.Error(codes.InvalidArgument, "status must be one of: pending, approved, rejected")
	}
	list, err := s.store.List(req.Status)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not list bookings: %v", err)
	}
	return &pb.ListBookingsResponse{Bookings: list}, nil
}

func (s *server) UpdateBooking(ctx context.Context, req *pb.UpdateBookingRequest) (*pb.Booking, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if !validBookingStatuses[req.Status] {
		return nil, status.Error(codes.InvalidArgument, "status must be one of: pending, approved, rejected")
	}
	b, err := s.store.Update(req)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "booking %q not found", req.Id)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not update booking: %v", err)
	}
	return b, nil
}

func (s *server) DeleteBooking(ctx context.Context, req *pb.DeleteBookingRequest) (*pb.DeleteBookingResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	ok, err := s.store.Delete(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not delete booking: %v", err)
	}
	if !ok {
		return nil, status.Errorf(codes.NotFound, "booking %q not found", req.Id)
	}
	return &pb.DeleteBookingResponse{Success: true}, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	port := getenv("BOOKING_SERVICE_PORT", "50052")
	dbPath := getenv("BOOKING_DB_PATH", "bookings.db")
	equipmentServiceAddr := getenv("EQUIPMENT_SERVICE_ADDR", "localhost:50051")

	store, err := NewStore(dbPath)
	if err != nil {
		log.Fatalf("could not open store at %s: %v", dbPath, err)
	}
	defer store.Close()

	equipmentClient, err := NewEquipmentClient(equipmentServiceAddr)
	if err != nil {
		log.Fatalf("could not create equipment-service client for %s: %v", equipmentServiceAddr, err)
	}

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("could not listen on :%s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterBookingServiceServer(grpcServer, &server{store: store, equipmentClient: equipmentClient})

	log.Printf("booking-service listening on :%s (db: %s, equipment-service: %s)", port, dbPath, equipmentServiceAddr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve failed: %v", err)
	}
}
