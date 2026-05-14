package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type actualTxPayload struct {
	Date       string `json:"date"`
	Amount     int64  `json:"amount"`
	PayeeName  string `json:"payee_name"`
	Notes      string `json:"notes"`
	ImportedID string `json:"imported_id"`
	Category   string `json:"category,omitempty"`
}

type pushResult struct {
	Total     int
	Added     int
	Updated   int
	Errors    int
	ErrorList []string
}

func handlePush(w http.ResponseWriter, r *http.Request) {
	importID := r.PathValue("importID")
	imp, err := getImport(db, importID)
	if err != nil {
		http.Error(w, "import not found", http.StatusNotFound)
		return
	}

	bankAccount, err := getBankAccount(db, imp.Bank, imp.Account)
	if err != nil {
		log.Printf("get bank account for %s/%s: %v", imp.Bank, imp.Account, err)
		http.Error(w, "no account mapping found — configure in Settings", http.StatusBadRequest)
		return
	}

	if bankAccount.ActualAccountID == "" {
		http.Error(w, "account not mapped to Actual", http.StatusBadRequest)
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

	catMap := make(map[string]string)
	for _, c := range cats {
		catMap[c.ID] = c.ActualCatID
	}

	result := pushResult{}
	for _, tx := range txs {
		if tx.Status != "approved" {
			continue
		}
		result.Total++

		amount := tx.Amount * 1000
		payload := actualTxPayload{
			Date:       tx.Date,
			Amount:     amount,
			PayeeName:  tx.Description,
			Notes:      tx.Notes,
			ImportedID: tx.ReferenceID,
		}
		if actualCatID, ok := catMap[tx.CategoryID]; ok && actualCatID != "" {
			payload.Category = actualCatID
		}

		// TODO(issue-05): when tx.TransferToAccount != "", create a native Actual transfer
		// instead of a regular transaction (requires looking up both pocket account IDs and
		// calling the Actual Budget transfer endpoint).

		if err := pushTransaction(bankAccount.ActualAccountID, payload); err != nil {
			result.Errors++
			result.ErrorList = append(result.ErrorList, fmt.Sprintf("%s: %v", tx.Description, err))
		} else {
			result.Added++
		}
	}

	var pushError, pushResultStr string
	if result.Errors > 0 {
		pushError = fmt.Sprintf("%d errors during push", result.Errors)
	}
	pushResultStr = fmt.Sprintf("Pushed: %d added, %d errors", result.Added, result.Errors)

	if result.Total == 0 {
		pushResultStr = "No approved transactions to push"
	}

	updateImportStatus(db, importID, "done", pushError, len(txs))

	cats2, _ := listCategories(db)
	allTxs, _ := listTransactions(db, importID)
	pending, approved, skipped, duplicate := 0, 0, 0, 0
	for _, tx := range allTxs {
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

	data := reviewDetailData{
		Import:     *imp,
		Categories: cats2,
		Groups:     groupByDate(allTxs, cats2),
		Pending:    pending,
		Approved:   approved,
		Skipped:    skipped,
		Duplicate:  duplicate,
		PushError:  pushError,
		PushResult: pushResultStr,
	}

	tmpl.ExecuteTemplate(w, "reviewDetail", data)
}

func pushTransaction(accountID string, payload actualTxPayload) error {
	body, err := json.Marshal(struct {
		Transaction actualTxPayload `json:"transaction"`
	}{Transaction: payload})
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	url := cfg.ActualAPIURL + "/v1/budgets/" + cfg.ActualBudgetSyncID + "/accounts/" + accountID + "/transactions"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.ActualAPIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("push to Actual: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Actual API returned %d", resp.StatusCode)
	}

	return nil
}
