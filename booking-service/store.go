package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	pb "equipbooking/proto/bookings"
)

// Store is Service A's OWN database — separate from equipment-service's.
// Bookings only ever hold an equipment_id (a string); they never read
// Service B's table directly.
type Store struct {
	db *sql.DB
}

func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	schema := `CREATE TABLE IF NOT EXISTS bookings (
		id TEXT PRIMARY KEY,
		equipment_id TEXT NOT NULL,
		requester_name TEXT NOT NULL,
		requester_contact TEXT NOT NULL DEFAULT '',
		purpose TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Create(id string, req *pb.CreateBookingRequest) (*pb.Booking, error) {
	b := &pb.Booking{
		Id:               id,
		EquipmentId:      req.EquipmentId,
		RequesterName:    req.RequesterName,
		RequesterContact: req.RequesterContact,
		Purpose:          req.Purpose,
		Status:           "pending",
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	}
	_, err := s.db.Exec(
		`INSERT INTO bookings (id, equipment_id, requester_name, requester_contact, purpose, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		b.Id, b.EquipmentId, b.RequesterName, b.RequesterContact, b.Purpose, b.Status, b.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Store) Get(id string) (*pb.Booking, error) {
	row := s.db.QueryRow(`SELECT id, equipment_id, requester_name, requester_contact, purpose, status, created_at FROM bookings WHERE id = ?`, id)
	return scanBooking(row)
}

func (s *Store) List(status string) ([]*pb.Booking, error) {
	var rows *sql.Rows
	var err error
	if status == "" {
		rows, err = s.db.Query(`SELECT id, equipment_id, requester_name, requester_contact, purpose, status, created_at FROM bookings ORDER BY created_at DESC`)
	} else {
		rows, err = s.db.Query(`SELECT id, equipment_id, requester_name, requester_contact, purpose, status, created_at FROM bookings WHERE status = ? ORDER BY created_at DESC`, status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*pb.Booking
	for rows.Next() {
		b := &pb.Booking{}
		if err := rows.Scan(&b.Id, &b.EquipmentId, &b.RequesterName, &b.RequesterContact, &b.Purpose, &b.Status, &b.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func (s *Store) Update(req *pb.UpdateBookingRequest) (*pb.Booking, error) {
	res, err := s.db.Exec(`UPDATE bookings SET status = ?, purpose = ? WHERE id = ?`, req.Status, req.Purpose, req.Id)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, sql.ErrNoRows
	}
	return s.Get(req.Id)
}

func (s *Store) Delete(id string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM bookings WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanBooking(row rowScanner) (*pb.Booking, error) {
	b := &pb.Booking{}
	if err := row.Scan(&b.Id, &b.EquipmentId, &b.RequesterName, &b.RequesterContact, &b.Purpose, &b.Status, &b.CreatedAt); err != nil {
		return nil, err
	}
	return b, nil
}
