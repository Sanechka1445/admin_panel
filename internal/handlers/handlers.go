package handlers

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"

	"colorshapes-admin/backupmanager"
	"colorshapes-admin/internal/auth"
	"colorshapes-admin/internal/models"
)

// App holds all dependencies.
type App struct {
	DB        *sql.DB
	Auth      *auth.Store
	Colors    *models.ColorStore
	Shapes    *models.ShapeStore
	Backup    *backupmanager.Manager
	BackupCfg backupmanager.Config
	Tmpl      *template.Template
}

// TemplateData is passed to all templates.
type TemplateData struct {
	Title string
	Flash string
	Error string
	Data  interface{}
}

// shapeFormData is used by both ShapesNew and ShapesEdit.
type shapeFormData struct {
	Shape  *models.Shape
	Colors []models.Color
}

func (a *App) render(w http.ResponseWriter, name string, td TemplateData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.Tmpl.ExecuteTemplate(w, name, td); err != nil {
		// Headers already sent by ExecuteTemplate, only log the error
		fmt.Printf("template error rendering %s: %v\n", name, err)
	}
}

// RequireAuth middleware
func (a *App) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := a.Auth.GetAdminFromRequest(r); err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// ------- Auth -------

func (a *App) LoginPage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		email := r.FormValue("email")
		pass := r.FormValue("password")
		admin, err := a.Auth.Authenticate(email, pass)
		if err != nil {
			a.render(w, "login.html", TemplateData{Title: "Вход", Error: "Неверный email или пароль"})
			return
		}
		if err := a.Auth.CreateSession(w, admin.ID); err != nil {
			a.render(w, "login.html", TemplateData{Title: "Вход", Error: "Ошибка сессии"})
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.render(w, "login.html", TemplateData{Title: "Вход"})
}

func (a *App) Logout(w http.ResponseWriter, r *http.Request) {
	a.Auth.Logout(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ------- Dashboard -------

func (a *App) Dashboard(w http.ResponseWriter, r *http.Request) {
	colors, _ := a.Colors.List()
	shapes, _ := a.Shapes.List()
	type stats struct {
		Colors int
		Shapes int
	}
	a.render(w, "dashboard.html", TemplateData{
		Title: "Панель управления",
		Data:  stats{Colors: len(colors), Shapes: len(shapes)},
	})
}

// ------- Colors -------

func (a *App) ColorsList(w http.ResponseWriter, r *http.Request) {
	colors, err := a.Colors.List()
	flash := r.URL.Query().Get("flash")
	if err != nil {
		a.render(w, "colors_list.html", TemplateData{Title: "Цвета", Error: err.Error()})
		return
	}
	a.render(w, "colors_list.html", TemplateData{Title: "Цвета", Data: colors, Flash: flash})
}

func (a *App) ColorsNew(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		hex := r.FormValue("hex")
		if err := a.Colors.Create(name, hex); err != nil {
			a.render(w, "colors_form.html", TemplateData{Title: "Новый цвет", Error: err.Error()})
			return
		}
		http.Redirect(w, r, "/colors?flash=Цвет+добавлен", http.StatusSeeOther)
		return
	}
	a.render(w, "colors_form.html", TemplateData{Title: "Новый цвет"})
}

func (a *App) ColorsEdit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(filepath.Base(r.URL.Path))
	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		hex := r.FormValue("hex")
		if err := a.Colors.Update(id, name, hex); err != nil {
			a.render(w, "colors_form.html", TemplateData{Title: "Редактировать цвет", Error: err.Error()})
			return
		}
		http.Redirect(w, r, "/colors?flash=Цвет+обновлён", http.StatusSeeOther)
		return
	}
	c, err := a.Colors.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.render(w, "colors_form.html", TemplateData{Title: "Редактировать цвет", Data: c})
}

func (a *App) ColorsDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(filepath.Base(r.URL.Path))
	if err := a.Colors.Delete(id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/colors?flash=Цвет+удалён", http.StatusSeeOther)
}

// ------- Shapes -------

func (a *App) ShapesList(w http.ResponseWriter, r *http.Request) {
	shapes, err := a.Shapes.List()
	flash := r.URL.Query().Get("flash")
	if err != nil {
		a.render(w, "shapes_list.html", TemplateData{Title: "Фигуры", Error: err.Error()})
		return
	}
	a.render(w, "shapes_list.html", TemplateData{Title: "Фигуры", Data: shapes, Flash: flash})
}

func (a *App) ShapesNew(w http.ResponseWriter, r *http.Request) {
	colors, _ := a.Colors.List()
	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		colorID, _ := strconv.Atoi(r.FormValue("color_id"))
		if err := a.Shapes.Create(name, colorID); err != nil {
			a.render(w, "shapes_form.html", TemplateData{
				Title: "Новая фигура",
				Error: err.Error(),
				Data:  shapeFormData{Colors: colors},
			})
			return
		}
		http.Redirect(w, r, "/shapes?flash=Фигура+добавлена", http.StatusSeeOther)
		return
	}
	a.render(w, "shapes_form.html", TemplateData{
		Title: "Новая фигура",
		Data:  shapeFormData{Colors: colors},
	})
}

func (a *App) ShapesEdit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(filepath.Base(r.URL.Path))
	colors, _ := a.Colors.List()
	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		colorID, _ := strconv.Atoi(r.FormValue("color_id"))
		if err := a.Shapes.Update(id, name, colorID); err != nil {
			a.render(w, "shapes_form.html", TemplateData{
				Title: "Редактировать фигуру",
				Error: err.Error(),
				Data:  shapeFormData{Colors: colors},
			})
			return
		}
		http.Redirect(w, r, "/shapes?flash=Фигура+обновлена", http.StatusSeeOther)
		return
	}
	sh, err := a.Shapes.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.render(w, "shapes_form.html", TemplateData{
		Title: "Редактировать фигуру",
		Data:  shapeFormData{Shape: sh, Colors: colors},
	})
}

func (a *App) ShapesDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(filepath.Base(r.URL.Path))
	if err := a.Shapes.Delete(id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/shapes?flash=Фигура+удалена", http.StatusSeeOther)
}

// ------- Backup -------

type backupPageData struct {
	Storages []backupmanager.Storage
	Backups  map[string][]string
	Flash    string
	Error    string
}

func (a *App) BackupPage(w http.ResponseWriter, r *http.Request) {
	storages := a.Backup.ListStorages()
	backups := make(map[string][]string)
	for _, s := range storages {
		files, _ := a.Backup.ListBackups(s.Name)
		backups[s.Name] = files
	}
	flash := r.URL.Query().Get("flash")
	errMsg := r.URL.Query().Get("error")

	var storeList []backupmanager.Storage
	for _, s := range storages {
		storeList = append(storeList, *s)
	}

	a.render(w, "backup.html", TemplateData{
		Title: "Резервные копии",
		Data: backupPageData{
			Storages: storeList,
			Backups:  backups,
			Flash:    flash,
			Error:    errMsg,
		},
	})
}

func (a *App) BackupCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/backup", http.StatusSeeOther)
		return
	}
	storageName := r.FormValue("storage")
	archiveFile, err := a.Backup.Backup(a.BackupCfg, storageName)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/backup?error=%s", err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/backup?flash=Бэкап+создан:+%s", archiveFile), http.StatusSeeOther)
}

func (a *App) BackupRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/backup", http.StatusSeeOther)
		return
	}
	storageName := r.FormValue("storage")
	filename := r.FormValue("filename")
	if err := a.Backup.Restore(a.BackupCfg, storageName, filename); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/backup?error=%s", err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/backup?flash=Восстановление+завершено", http.StatusSeeOther)
}

func (a *App) BackupDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/backup", http.StatusSeeOther)
		return
	}
	storageName := r.FormValue("storage")
	filename := r.FormValue("filename")
	if err := a.Backup.DeleteBackup(storageName, filename); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/backup?error=%s", err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/backup?flash=Бэкап+удалён", http.StatusSeeOther)
}

func (a *App) StorageAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/backup", http.StatusSeeOther)
		return
	}
	name := r.FormValue("name")
	path := r.FormValue("path")
	if err := a.Backup.AddStorage(name, path); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/backup?error=%s", err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/backup?flash=Хранилище+добавлено", http.StatusSeeOther)
}

func (a *App) StorageRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/backup", http.StatusSeeOther)
		return
	}
	name := r.FormValue("name")
	if err := a.Backup.RemoveStorage(name); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/backup?error=%s", err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/backup?flash=Хранилище+удалено", http.StatusSeeOther)
}
