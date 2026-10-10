// equipment-service is Service B: it owns Equipment in its own SQLite
// database and exposes full CRUD over gRPC. It never calls any other
// service.
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

	pb "equipbooking/proto/equipment"
)

var validStatuses = map[string]bool{"available": true, "booked": true, "maintenance": true}

type server struct {
	pb.UnimplementedEquipmentServiceServer
	store *Store
}

func (s *server) CreateEquipment(ctx context.Context, req *pb.CreateEquipmentRequest) (*pb.Equipment, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.Status == "" {
		req.Status = "available"
	}
	if !validStatuses[req.Status] {
		return nil, status.Error(codes.InvalidArgument, "status must be one of: available, booked, maintenance")
	}
	e, err := s.store.Create(uuid.NewString(), req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not create equipment: %v", err)
	}
	return e, nil
}

func (s *server) GetEquipment(ctx context.Context, req *pb.GetEquipmentRequest) (*pb.Equipment, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	e, err := s.store.Get(req.Id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "equipment %q not found", req.Id)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not get equipment: %v", err)
	}
	return e, nil
}

func (s *server) ListEquipment(ctx context.Context, req *pb.ListEquipmentRequest) (*pb.ListEquipmentResponse, error) {
	if req.Status != "" && !validStatuses[req.Status] {
		return nil, status.Error(codes.InvalidArgument, "status must be one of: available, booked, maintenance")
	}
	list, err := s.store.List(req.Status)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not list equipment: %v", err)
	}
	return &pb.ListEquipmentResponse{Equipment: list}, nil
}

func (s *server) UpdateEquipment(ctx context.Context, req *pb.UpdateEquipmentRequest) (*pb.Equipment, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if !validStatuses[req.Status] {
		return nil, status.Error(codes.InvalidArgument, "status must be one of: available, booked, maintenance")
	}
	e, err := s.store.Update(req)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "equipment %q not found", req.Id)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not update equipment: %v", err)
	}
	return e, nil
}

func (s *server) DeleteEquipment(ctx context.Context, req *pb.DeleteEquipmentRequest) (*pb.DeleteEquipmentResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	ok, err := s.store.Delete(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not delete equipment: %v", err)
	}
	if !ok {
		return nil, status.Errorf(codes.NotFound, "equipment %q not found", req.Id)
	}
	return &pb.DeleteEquipmentResponse{Success: true}, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	port := getenv("EQUIPMENT_SERVICE_PORT", "50051")
	dbPath := getenv("EQUIPMENT_DB_PATH", "equipment.db")

	store, err := NewStore(dbPath)
	if err != nil {
		log.Fatalf("could not open store at %s: %v", dbPath, err)
	}
	defer store.Close()

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("could not listen on :%s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterEquipmentServiceServer(grpcServer, &server{store: store})

	log.Printf("equipment-service listening on :%s (db: %s)", port, dbPath)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve failed: %v", err)
	}
}
