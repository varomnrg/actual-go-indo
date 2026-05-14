package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type reviewListData struct {
	Imports []importWithCount
}

type importWithCount struct {
	Import
	PendingCount  int
	ApprovedCount int
	SkippedCount  int
	DuplicateCount int
}

type reviewDetailData struct {
	Import     Import
	Categories []Category
	Groups     []dateGroup
	Pending    int
	Approved   int
	Skipped    int
	Duplicate  int
	PushError  string
	PushResult string
	Warning    string
}

type dateGroup struct {
	Date         string
	Transactions []txRowData
}

type txRowData struct {
	Transaction Transaction
	Categories  []Category
}

var tmplFuncs = template.FuncMap{
	"rp": func(amount int64) string {
		abs := amount
		prefix := "-"
		if amount >= 0 {
			prefix = ""
			abs = amount
		} else {
			abs = -amount
		}
		s := fmt.Sprintf("%d", abs)
		n := len(s)
		if n <= 3 {
			return prefix + "Rp " + s
		}
		var parts []string
		for i := n; i > 0; i -= 3 {
			start := i - 3
			if start < 0 {
				start = 0
			}
			parts = append([]string{s[start:i]}, parts...)
		}
		return prefix + "Rp " + strings.Join(parts, ".")
	},
	"displayDate": func(dateStr string) string {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return dateStr
		}
		return t.Format("02 Jan 2006")
	},
	"catName": func(categoryID string, cats []Category) string {
		for _, c := range cats {
			if c.ID == categoryID {
				return c.Name
			}
		}
		return ""
	},
	"suggestCatID": func(description string, cats []Category) string {
		name := SuggestCategory(description)
		if name == "" {
			return ""
		}
		for _, c := range cats {
			if c.Name == name {
				return c.ID
			}
		}
		return ""
	},
	"abs": func(n int64) int64 {
		if n < 0 {
			return -n
		}
		return n
	},
	"upper": strings.ToUpper,
}

func handleReviewList(w http.ResponseWriter, r *http.Request) {
	imports, err := listImports(db)
	if err != nil {
		log.Printf("list imports: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data := reviewListData{}
	for _, imp := range imports {
		pending, approved, skipped, duplicate := countTxStatuses(db, imp.ID)
		data.Imports = append(data.Imports, importWithCount{
			Import:         imp,
			PendingCount:   pending,
			ApprovedCount:  approved,
			SkippedCount:   skipped,
			DuplicateCount: duplicate,
		})
	}

	if err := tmpl.ExecuteTemplate(w, "reviewList", data); err != nil {
		log.Printf("render review list: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func handleReviewDetail(w http.ResponseWriter, r *http.Request) {
	importID := r.PathValue("importID")
	imp, err := getImport(db, importID)
	if err != nil {
		http.Error(w, "import not found", http.StatusNotFound)
		return
	}

	txs, err := listTransactions(db, importID)
	if err != nil {
		log.Printf("list transactions: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	cats, err := listCategories(db)
	if err != nil {
		log.Printf("list categories: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data := buildReviewData(*imp, txs, cats, "", "")
	if imp.Account != "" {
		if _, mappingErr := getBankAccount(db, imp.Bank, imp.Account); mappingErr == sql.ErrNoRows {
			data.Warning = fmt.Sprintf(
				"Account %q (%s) has no mapping. Go to /settings to add one before pushing.",
				imp.Account, imp.Bank,
			)
		}
	}
	if err := tmpl.ExecuteTemplate(w, "reviewDetail", data); err != nil {
		log.Printf("render review detail: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func buildReviewData(imp Import, txs []Transaction, cats []Category, pushError, pushResult string) reviewDetailData {
	pending, approved, skipped, duplicate := 0, 0, 0, 0
	for _, tx := range txs {
		switch tx.Status {
		case "pending":
			pending++
		case "approved":
			approved++
		case "skipped":
			skipped++
		case "duplicate":
			duplicate++
		}
	}

	groups := groupByDate(txs, cats)

	return reviewDetailData{
		Import:     imp,
		Categories: cats,
		Groups:     groups,
		Pending:    pending,
		Approved:   approved,
		Skipped:    skipped,
		Duplicate:  duplicate,
		PushError:  pushError,
		PushResult: pushResult,
	}
}

func groupByDate(txs []Transaction, cats []Category) []dateGroup {
	if len(txs) == 0 {
		return nil
	}
	var groups []dateGroup
	currentDate := txs[0].Date
	group := dateGroup{Date: currentDate}

	for _, tx := range txs {
		if tx.Date != currentDate {
			groups = append(groups, group)
			currentDate = tx.Date
			group = dateGroup{Date: currentDate}
		}
		group.Transactions = append(group.Transactions, txRowData{
			Transaction: tx,
			Categories:  cats,
		})
	}
	groups = append(groups, group)
	return groups
}

func countTxStatuses(db *sql.DB, importID string) (pending, approved, skipped, duplicate int) {
	txs, err := listTransactions(db, importID)
	if err != nil {
		return 0, 0, 0, 0
	}
	for _, tx := range txs {
		switch tx.Status {
		case "pending":
			pending++
		case "approved":
			approved++
		case "skipped":
			skipped++
		case "duplicate":
			duplicate++
		}
	}
	return
}

func handleApprove(w http.ResponseWriter, r *http.Request) {
	txID := r.FormValue("tx_id")
	categoryID := r.FormValue("category_id")

	if txID == "" {
		http.Error(w, "missing tx_id", http.StatusBadRequest)
		return
	}

	if err := updateTransactionStatus(db, txID, "approved", categoryID); err != nil {
		log.Printf("approve tx %s: %v", txID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	cats, err := listCategories(db)
	if err != nil {
		log.Printf("list categories: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	tx, err := getTransaction(db, txID)
	if err != nil {
		log.Printf("get tx %s: %v", txID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	tmpl.ExecuteTemplate(w, "txRow", txRowData{Transaction: *tx, Categories: cats})
}

func handleSkip(w http.ResponseWriter, r *http.Request) {
	txID := r.FormValue("tx_id")

	if txID == "" {
		http.Error(w, "missing tx_id", http.StatusBadRequest)
		return
	}

	if err := updateTransactionStatus(db, txID, "skipped", ""); err != nil {
		log.Printf("skip tx %s: %v", txID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	cats, err := listCategories(db)
	if err != nil {
		log.Printf("list categories: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	tx, err := getTransaction(db, txID)
	if err != nil {
		log.Printf("get tx %s: %v", txID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	tmpl.ExecuteTemplate(w, "txRow", txRowData{Transaction: *tx, Categories: cats})
}

func getTransaction(db *sql.DB, id string) (*Transaction, error) {
	tx := &Transaction{}
	err := db.QueryRow(`
		SELECT id, import_id, date, time, description, notes, amount, balance,
		       reference_id, category_id, transfer_to_account, status, created_at
		FROM transactions WHERE id=?`, id,
	).Scan(&tx.ID, &tx.ImportID, &tx.Date, &tx.Time, &tx.Description,
		&tx.Notes, &tx.Amount, &tx.Balance, &tx.ReferenceID, &tx.CategoryID,
		&tx.TransferToAccount, &tx.Status, &tx.CreatedAt)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	imports, err := listImports(db)
	if err != nil {
		log.Printf("list imports: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data := reviewListData{}
	for _, imp := range imports {
		pending, approved, skipped, duplicate := countTxStatuses(db, imp.ID)
		data.Imports = append(data.Imports, importWithCount{
			Import:         imp,
			PendingCount:   pending,
			ApprovedCount:  approved,
			SkippedCount:   skipped,
			DuplicateCount: duplicate,
		})
	}

	if err := tmpl.ExecuteTemplate(w, "historyPage", data); err != nil {
		log.Printf("render history: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func handleListCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := listCategories(db)
	if err != nil {
		log.Printf("list categories: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cats)
}

func handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}
	if _, err := insertCategory(db, name); err != nil {
		log.Printf("create category: %v", err)
		http.Error(w, "category creation failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func handleRetry(w http.ResponseWriter, r *http.Request) {
	importID := r.PathValue("importID")
	imp, err := getImport(db, importID)
	if err != nil {
		http.Error(w, "import not found", http.StatusNotFound)
		return
	}

	if imp.FilePath == "" {
		http.Error(w, "no file to retry", http.StatusBadRequest)
		return
	}

	updateImportStatus(db, importID, "parsing", "", 0)

	var txs []Transaction
	var account, period string

	switch imp.Bank {
	case "bca":
		f, err := os.Open(imp.FilePath)
		if err != nil {
			updateImportStatus(db, importID, "error", err.Error(), 0)
			log.Printf("retry open file: %v", err)
			http.Redirect(w, r, "/review/"+importID, http.StatusSeeOther)
			return
		}
		defer f.Close()
		txs, account, period, err = ParseBCA(f)
		if err != nil {
			updateImportStatus(db, importID, "error", err.Error(), 0)
			log.Printf("retry parse: %v", err)
			http.Redirect(w, r, "/review/"+importID, http.StatusSeeOther)
			return
		}
	case "mandiri":
		password := r.FormValue("password")
		txs, account, period, err = ParseMandiri(imp.FilePath, password)
		if err != nil {
			updateImportStatus(db, importID, "error", err.Error(), 0)
			http.Redirect(w, r, "/review/"+importID, http.StatusSeeOther)
			return
		}
	case "jago":
		f, err := os.Open(imp.FilePath)
		if err != nil {
			updateImportStatus(db, importID, "error", err.Error(), 0)
			http.Redirect(w, r, "/review/"+importID, http.StatusSeeOther)
			return
		}
		defer f.Close()
		txs, account, period, err = ParseJago(f)
		if err != nil {
			updateImportStatus(db, importID, "error", err.Error(), 0)
			http.Redirect(w, r, "/review/"+importID, http.StatusSeeOther)
			return
		}
	default:
		http.Error(w, "unknown bank", http.StatusBadRequest)
		return
	}

	if err := replaceImportTransactions(db, importID, txs); err != nil {
		log.Printf("replace transactions: %v", err)
		updateImportStatus(db, importID, "error", err.Error(), 0)
		http.Redirect(w, r, "/review/"+importID, http.StatusSeeOther)
		return
	}

	updateImportStatus(db, importID, "pending", "", len(txs))
	if account != "" || period != "" {
		db.Exec(`UPDATE imports SET account=?, period=? WHERE id=?`, account, period, importID)
	}

	http.Redirect(w, r, "/review/"+importID, http.StatusSeeOther)
}

func replaceImportTransactions(db *sql.DB, importID string, txs []Transaction) error {
	if _, err := db.Exec(`DELETE FROM transactions WHERE import_id=?`, importID); err != nil {
		return fmt.Errorf("delete old transactions: %w", err)
	}
	for i := range txs {
		txs[i].ImportID = importID
		exists, err := referenceIDExists(db, txs[i].ReferenceID)
		if err != nil {
			return err
		}
		if exists {
			txs[i].Status = "duplicate"
		}
		if _, err := insertTransaction(db, &txs[i]); err != nil {
			return fmt.Errorf("insert tx: %w", err)
		}
	}
	return nil
}

func handleUploadSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	bank := r.FormValue("bank")
	if bank == "" {
		http.Error(w, "missing bank", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	savedPath, err := saveUploadedFile(file, header.Filename)
	if err != nil {
		log.Printf("save file: %v", err)
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}

	impID, err := insertImport(db, &Import{
		Filename: header.Filename,
		Bank:     bank,
		FilePath: savedPath,
		Status:   "parsing",
	})
	if err != nil {
		log.Printf("insert import: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	reopened, err := os.Open(savedPath)
	if err != nil {
		updateImportStatus(db, impID, "error", err.Error(), 0)
		log.Printf("reopen file: %v", err)
		http.Redirect(w, r, "/review/"+impID, http.StatusSeeOther)
		return
	}
	defer reopened.Close()

	var txs []Transaction
	var account, period string
	var parseErr error

	switch bank {
	case "bca":
		txs, account, period, parseErr = ParseBCA(reopened)
	case "mandiri":
		password := r.FormValue("password")
		txs, account, period, parseErr = ParseMandiri(savedPath, password)
	case "jago":
		txs, account, period, parseErr = ParseJago(reopened)
	default:
		updateImportStatus(db, impID, "error", "unsupported bank", 0)
		http.Error(w, "unsupported bank", http.StatusBadRequest)
		return
	}

	if parseErr != nil {
		updateImportStatus(db, impID, "error", parseErr.Error(), 0)
		log.Printf("parse error: %v", parseErr)
		http.Redirect(w, r, "/review/"+impID, http.StatusSeeOther)
		return
	}

	for i := range txs {
		txs[i].ImportID = impID
		exists, err := referenceIDExists(db, txs[i].ReferenceID)
		if err != nil {
			log.Printf("check ref id: %v", err)
			continue
		}
		if exists {
			txs[i].Status = "duplicate"
		}
		if _, err := insertTransaction(db, &txs[i]); err != nil {
			log.Printf("insert tx: %v", err)
		}
	}

	updateImportStatus(db, impID, "pending", "", len(txs))
	if account != "" || period != "" {
		db.Exec(`UPDATE imports SET account=?, period=? WHERE id=?`, account, period, impID)
	}

	http.Redirect(w, r, "/review/"+impID, http.StatusSeeOther)
}

func saveUploadedFile(src io.Reader, filename string) (string, error) {
	name := fmt.Sprintf("%d_%s", time.Now().UnixNano(), filename)
	path := cfg.UploadDir + "/" + name

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, src); err != nil {
		return "", err
	}
	return path, nil
}
