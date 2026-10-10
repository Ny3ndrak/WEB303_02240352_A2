package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	pb "equipbooking/proto/equipment"
)

// Store is this service's OWN database. Nothing else reads or writes
// this file directly — that is the Database-per-Service pattern.
type Store struct {
	db *sql.DB
}

func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	schema := `CREATE TABLE IF NOT EXISTS equipment (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		category TEXT NOT NULL DEFAULT '',
		location TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		added_at TEXT NOT NULL
	);`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Create(id string, req *pb.CreateEquipmentRequest) (*pb.Equipment, error) {
	e := &pb.Equipment{
		Id:          id,
		Name:        req.Name,
		Description: req.Description,
		Category:    req.Category,
		Location:    req.Location,
		Status:      req.Status,
		AddedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	_, err := s.db.Exec(
		`INSERT INTO equipment (id, name, description, category, location, status, added_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.Id, e.Name, e.Description, e.Category, e.Location, e.Status, e.AddedAt,
	)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Store) Get(id string) (*pb.Equipment, error) {
	row := s.db.QueryRow(`SELECT id, name, description, category, location, status, added_at FROM equipment WHERE id = ?`, id)
	return scanEquipment(row)
}

func (s *Store) List(status string) ([]*pb.Equipment, error) {
	var rows *sql.Rows
	var err error
	if status == "" {
		rows, err = s.db.Query(`SELECT id, name, description, category, location, status, added_at FROM equipment ORDER BY added_at DESC`)
	} else {
		rows, err = s.db.Query(`SELECT id, name, description, category, location, status, added_at FROM equipment WHERE status = ? ORDER BY added_at DESC`, status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*pb.Equipment
	for rows.Next() {
		e := &pb.Equipment{}
		if err := rows.Scan(&e.Id, &e.Name, &e.Description, &e.Category, &e.Location, &e.Status, &e.AddedAt); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

func (s *Store) Update(req *pb.UpdateEquipmentRequest) (*pb.Equipment, error) {
	res, err := s.db.Exec(
		`UPDATE equipment SET name = ?, description = ?, category = ?, location = ?, status = ? WHERE id = ?`,
		req.Name, req.Description, req.Category, req.Location, req.Status, req.Id,
	)
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
	res, err := s.db.Exec(`DELETE FROM equipment WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEquipment(row rowScanner) (*pb.Equipment, error) {
	e := &pb.Equipment{}
	if err := row.Scan(&e.Id, &e.Name, &e.Description, &e.Category, &e.Location, &e.Status, &e.AddedAt); err != nil {
		return nil, err
	}
	return e, nil
}
