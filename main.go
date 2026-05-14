package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

//go:embed templates/*
var templateFS embed.FS

var (
	cfg  Config
	db   *sql.DB
	tmpl *template.Template
)

func main() {
	cfg = loadConfig()

	checkDependencies()

	if err := os.MkdirAll(cfg.UploadDir, 0o755); err != nil {
		log.Fatalf("create upload dir: %v", err)
	}

	var err error
	db, err = openDB(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	tmpl = template.Must(template.New("").Funcs(tmplFuncs).ParseFS(templateFS, "templates/*.html"))

	if err := seedCategories(db); err != nil {
		log.Printf("seed categories: %v", err)
	}

	mux := http.NewServeMux()
	registerRoutes(mux)

	log.Printf("Listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func checkDependencies() {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		log.Fatal("pdftotext not found: install poppler-utils (apt install poppler-utils / brew install poppler)")
	}
}

func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/upload", http.StatusFound)
	})

	mux.HandleFunc("GET /upload", handleUploadPage)
	mux.HandleFunc("POST /upload", handleUploadSubmit)

	mux.HandleFunc("GET /review", handleReviewList)
	mux.HandleFunc("GET /review/{importID}", handleReviewDetail)
	mux.HandleFunc("POST /review/{importID}/approve", handleApprove)
	mux.HandleFunc("POST /review/{importID}/skip", handleSkip)
	mux.HandleFunc("POST /review/{importID}/push", handlePush)

	mux.HandleFunc("POST /import/{importID}/retry", handleRetry)

	mux.HandleFunc("GET /history", handleHistory)

	mux.HandleFunc("GET /settings", handleSettingsPage)
	mux.HandleFunc("POST /settings/accounts", handleSaveAccount)

	mux.HandleFunc("GET /api/categories", handleListCategories)
	mux.HandleFunc("POST /api/categories", handleCreateCategory)
}

func handleUploadPage(w http.ResponseWriter, r *http.Request) {
	if err := tmpl.ExecuteTemplate(w, "uploadPage", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type ActualAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type settingsPageData struct {
	Accounts        []BankAccount
	ActualAccounts  []ActualAccount
	PdfToText       bool
	ActualReachable bool
	Error           string
}

func fetchActualAccounts() ([]ActualAccount, error) {
	req, err := http.NewRequest("GET", cfg.ActualAPIURL+"/v1/budgets/"+cfg.ActualBudgetSyncID+"/accounts", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", cfg.ActualAPIKey)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Data []ActualAccount `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	accounts, _ := listBankAccounts(db)
	actualAccounts, err := fetchActualAccounts()
	_, pdfErr := exec.LookPath("pdftotext")
	data := settingsPageData{
		Accounts:        accounts,
		ActualAccounts:  actualAccounts,
		PdfToText:       pdfErr == nil,
		ActualReachable: err == nil,
		Error:           r.URL.Query().Get("error"),
	}
	if err := tmpl.ExecuteTemplate(w, "settingsPage", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleSaveAccount(w http.ResponseWriter, r *http.Request) {
	bank := r.FormValue("bank")
	identifier := r.FormValue("identifier")
	actualAccountID := r.FormValue("actual_account_id")

	if bank == "" || identifier == "" || actualAccountID == "" {
		http.Redirect(w, r, "/settings?error=missing-fields", http.StatusSeeOther)
		return
	}

	existing, err := getBankAccount(db, bank, identifier)
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/settings?error=db-error", http.StatusSeeOther)
		return
	}
	if existing != nil {
		http.Redirect(w, r, "/settings?error=duplicate", http.StatusSeeOther)
		return
	}

	if err := upsertBankAccount(db, bank, identifier, actualAccountID); err != nil {
		http.Redirect(w, r, "/settings?error=save-failed", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}
