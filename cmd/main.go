package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"

	"colorshapes-admin/backupmanager"
	"colorshapes-admin/internal/auth"
	"colorshapes-admin/internal/handlers"
	"colorshapes-admin/internal/models"
)

func main() {
	backupmanager.LoadEnv(".env")

	dbHost := getEnv("DB_HOST", "db")
	dbPort := getEnv("DB_PORT", "3306")
	dbUser := getEnv("DB_USER", "root")
	dbPass := getEnv("DB_PASSWORD", "secret")
	dbName := getEnv("DB_NAME", "colorshapes")
	appPort := getEnv("APP_PORT", "8080")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Local",
		dbUser, dbPass, dbHost, dbPort, dbName)

	db := waitForDB(dsn)
	defer db.Close()

	if err := migrate(db); err != nil {
		log.Fatalf("migration: %v", err)
	}
	if err := seedAdmin(db); err != nil {
		log.Fatalf("seed admin: %v", err)
	}

	// Templates
	tmpl := template.Must(template.ParseGlob("internal/templates/*.html"))

	// Backup manager
	backupDir := getEnv("BACKUP_DIR", "dbBackup")
	_ = os.MkdirAll(backupDir, 0o755)
	bm, err := backupmanager.New(filepath.Join(backupDir, "storages.json"), backupDir)
	if err != nil {
		log.Fatalf("backupmanager: %v", err)
	}

	app := &handlers.App{
		DB:     db,
		Auth:   &auth.Store{DB: db},
		Colors: &models.ColorStore{DB: db},
		Shapes: &models.ShapeStore{DB: db},
		Backup: bm,
		BackupCfg: backupmanager.Config{
			Host:     dbHost,
			Port:     dbPort,
			User:     dbUser,
			Password: dbPass,
			DBName:   dbName,
		},
		Tmpl: tmpl,
	}

	mux := http.NewServeMux()

	// Auth
	mux.HandleFunc("/login", app.LoginPage)
	mux.HandleFunc("/logout", app.RequireAuth(app.Logout))

	// Dashboard
	mux.HandleFunc("/", app.RequireAuth(app.Dashboard))

	// Colors
	mux.HandleFunc("/colors", app.RequireAuth(app.ColorsList))
	mux.HandleFunc("/colors/new", app.RequireAuth(app.ColorsNew))
	mux.HandleFunc("/colors/", app.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case endsWith(r.URL.Path, "/edit"):
			app.ColorsEdit(w, r)
		case endsWith(r.URL.Path, "/delete"):
			app.ColorsDelete(w, r)
		default:
			http.NotFound(w, r)
		}
	}))

	// Shapes
	mux.HandleFunc("/shapes", app.RequireAuth(app.ShapesList))
	mux.HandleFunc("/shapes/new", app.RequireAuth(app.ShapesNew))
	mux.HandleFunc("/shapes/", app.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case endsWith(r.URL.Path, "/edit"):
			app.ShapesEdit(w, r)
		case endsWith(r.URL.Path, "/delete"):
			app.ShapesDelete(w, r)
		default:
			http.NotFound(w, r)
		}
	}))

	// Backup
	mux.HandleFunc("/backup", app.RequireAuth(app.BackupPage))
	mux.HandleFunc("/backup/create", app.RequireAuth(app.BackupCreate))
	mux.HandleFunc("/backup/restore", app.RequireAuth(app.BackupRestore))
	mux.HandleFunc("/backup/delete", app.RequireAuth(app.BackupDelete))
	mux.HandleFunc("/backup/storage/add", app.RequireAuth(app.StorageAdd))
	mux.HandleFunc("/backup/storage/remove", app.RequireAuth(app.StorageRemove))

	log.Printf("Server starting on :%s", appPort)
	if err := http.ListenAndServe(":"+appPort, mux); err != nil {
		log.Fatal(err)
	}
}

func waitForDB(dsn string) *sql.DB {
	var db *sql.DB
	var err error
	for i := 0; i < 30; i++ {
		db, err = sql.Open("mysql", dsn)
		if err == nil {
			if err = db.Ping(); err == nil {
				log.Println("Database connected")
				return db
			}
		}
		log.Printf("Waiting for DB... (%d/30): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("Could not connect to database: %v", err)
	return nil
}

func migrate(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS admins (
			id            INT AUTO_INCREMENT PRIMARY KEY,
			email         VARCHAR(255) NOT NULL UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS sessions (
			id         INT AUTO_INCREMENT PRIMARY KEY,
			token      VARCHAR(128) NOT NULL UNIQUE,
			admin_id   INT NOT NULL,
			expires_at DATETIME NOT NULL,
			FOREIGN KEY (admin_id) REFERENCES admins(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS colors (
			id         INT AUTO_INCREMENT PRIMARY KEY,
			name       VARCHAR(100) NOT NULL,
			hex        VARCHAR(7)   NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS shapes (
			id         INT AUTO_INCREMENT PRIMARY KEY,
			name       VARCHAR(100) NOT NULL,
			color_id   INT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (color_id) REFERENCES colors(id) ON DELETE RESTRICT
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func seedAdmin(db *sql.DB) error {
	var count int
	_ = db.QueryRow("SELECT COUNT(*) FROM admins WHERE email='admin@admin.ru'").Scan(&count)
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("111111"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO admins (email, password_hash) VALUES (?, ?)", "admin@admin.ru", string(hash))
	return err
}

func endsWith(path, suffix string) bool {
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
