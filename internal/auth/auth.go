package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "cs_session"

// Admin represents an administrator account.
type Admin struct {
	ID           int
	Email        string
	PasswordHash string
}

// Store handles admin auth and sessions.
type Store struct {
	DB *sql.DB
}

// Authenticate verifies credentials and returns the admin or an error.
func (s *Store) Authenticate(email, password string) (*Admin, error) {
	var a Admin
	err := s.DB.QueryRow("SELECT id, email, password_hash FROM admins WHERE email=?", email).
		Scan(&a.ID, &a.Email, &a.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("invalid credentials")
	}
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	return &a, nil
}

// CreateSession creates a DB session and sets the cookie.
func (s *Store) CreateSession(w http.ResponseWriter, adminID int) error {
	token, err := randomHex(32)
	if err != nil {
		return err
	}
	expiry := time.Now().Add(24 * time.Hour)
	_, err = s.DB.Exec(
		"INSERT INTO sessions (token, admin_id, expires_at) VALUES (?, ?, ?)",
		token, adminID, expiry,
	)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Expires:  expiry,
	})
	return nil
}

// GetAdminFromRequest validates the session cookie and returns the admin ID.
func (s *Store) GetAdminFromRequest(r *http.Request) (int, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return 0, fmt.Errorf("no session")
	}
	var adminID int
	var expires time.Time
	err = s.DB.QueryRow(
		"SELECT admin_id, expires_at FROM sessions WHERE token=?", c.Value,
	).Scan(&adminID, &expires)
	if errors.Is(err, sql.ErrNoRows) || time.Now().After(expires) {
		return 0, fmt.Errorf("session invalid or expired")
	}
	if err != nil {
		return 0, err
	}
	return adminID, nil
}

// Logout deletes the session.
func (s *Store) Logout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(sessionCookie)
	if err == nil {
		_, _ = s.DB.Exec("DELETE FROM sessions WHERE token=?", c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:    sessionCookie,
		Value:   "",
		Path:    "/",
		Expires: time.Unix(0, 0),
	})
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
