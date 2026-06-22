package models

import (
	"database/sql"
	"fmt"
)

// Color represents a named color in the system.
type Color struct {
	ID   int
	Name string
	Hex  string
}

// Shape represents a geometric shape associated with a Color.
type Shape struct {
	ID      int
	Name    string
	ColorID int
	Color   *Color // populated by joins
}

// ColorStore provides CRUD operations for colors.
type ColorStore struct{ DB *sql.DB }

func (s *ColorStore) List() ([]Color, error) {
	rows, err := s.DB.Query("SELECT id, name, hex FROM colors ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Color
	for rows.Next() {
		var c Color
		if err := rows.Scan(&c.ID, &c.Name, &c.Hex); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *ColorStore) Get(id int) (*Color, error) {
	var c Color
	err := s.DB.QueryRow("SELECT id, name, hex FROM colors WHERE id=?", id).
		Scan(&c.ID, &c.Name, &c.Hex)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *ColorStore) Create(name, hex string) error {
	_, err := s.DB.Exec("INSERT INTO colors (name, hex) VALUES (?, ?)", name, hex)
	return err
}

func (s *ColorStore) Update(id int, name, hex string) error {
	res, err := s.DB.Exec("UPDATE colors SET name=?, hex=? WHERE id=?", name, hex, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("color %d not found", id)
	}
	return nil
}

func (s *ColorStore) Delete(id int) error {
	_, err := s.DB.Exec("DELETE FROM colors WHERE id=?", id)
	return err
}

// ShapeStore provides CRUD operations for shapes.
type ShapeStore struct{ DB *sql.DB }

func (s *ShapeStore) List() ([]Shape, error) {
	rows, err := s.DB.Query(`
		SELECT sh.id, sh.name, sh.color_id, c.name, c.hex
		FROM shapes sh
		JOIN colors c ON c.id = sh.color_id
		ORDER BY sh.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Shape
	for rows.Next() {
		var sh Shape
		sh.Color = &Color{}
		if err := rows.Scan(&sh.ID, &sh.Name, &sh.ColorID, &sh.Color.Name, &sh.Color.Hex); err != nil {
			return nil, err
		}
		sh.Color.ID = sh.ColorID
		out = append(out, sh)
	}
	return out, rows.Err()
}

func (s *ShapeStore) Get(id int) (*Shape, error) {
	var sh Shape
	sh.Color = &Color{}
	err := s.DB.QueryRow(`
		SELECT sh.id, sh.name, sh.color_id, c.name, c.hex
		FROM shapes sh JOIN colors c ON c.id = sh.color_id
		WHERE sh.id=?`, id).
		Scan(&sh.ID, &sh.Name, &sh.ColorID, &sh.Color.Name, &sh.Color.Hex)
	if err != nil {
		return nil, err
	}
	sh.Color.ID = sh.ColorID
	return &sh, nil
}

func (s *ShapeStore) Create(name string, colorID int) error {
	_, err := s.DB.Exec("INSERT INTO shapes (name, color_id) VALUES (?, ?)", name, colorID)
	return err
}

func (s *ShapeStore) Update(id int, name string, colorID int) error {
	res, err := s.DB.Exec("UPDATE shapes SET name=?, color_id=? WHERE id=?", name, colorID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("shape %d not found", id)
	}
	return nil
}

func (s *ShapeStore) Delete(id int) error {
	_, err := s.DB.Exec("DELETE FROM shapes WHERE id=?", id)
	return err
}
