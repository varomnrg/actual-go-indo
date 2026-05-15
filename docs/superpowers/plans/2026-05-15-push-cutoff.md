# Push Cutoff Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a push cutoff date that prevents transactions older than a known-synced point from being pushed to Actual Budget.

**Architecture:** Store cutoff date in new `app_settings` table. `handlePush` skips pre-cutoff approved txs. `buildReviewData` computes before-cutoff count dynamically. Settings page has htmx controls to set/advance the cutoff. Review page shows cutoff info and renders pre-cutoff txs dimmed without action controls.

**Tech Stack:** Go, SQLite, htmx, Alpine.js

---

### Task 1: `store.go` — `app_settings` table, migration, and helpers

**Files:**
- Modify: `store.go`

- [ ] **Step 1: Add `app_settings` CREATE TABLE to `runMigrations`**

After the existing `hasDescHash` migration, add the `app_settings` table creation:

```go
// store.go — inside runMigrations, after hasDescHash block:
var hasSettings bool
rows2, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name='app_settings'`)
if err != nil {
    return err
}
defer rows2.Close()
if rows2.Next() {
    hasSettings = true
}
rows2.Close()

if !hasSettings {
    _, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS app_settings (
            key   TEXT PRIMARY KEY,
            value TEXT NOT NULL DEFAULT ''
        )
    `)
    if err != nil {
        return err
    }
}
```

- [ ] **Step 2: Add `getSetting` and `setSetting` helpers at bottom of file**

```go
// store.go — after getBankAccount function:

func getSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM app_settings WHERE key=?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func setSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(`
		INSERT INTO app_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value
	`, key, value)
	return err
}
```

- [ ] **Step 3: Add `sql` import to store.go** (check if already imported — it is at line 5)

- [ ] **Step 4: Run tests to verify compilation**

Run: `go build ./...`
Expected: builds without errors

---

### Task 2: `review.go` — Add Cutoff/BeforeCutoff to review data

**Files:**
- Modify: `review.go`

- [ ] **Step 1: Add `Cutoff string` and `BeforeCutoff int` to `reviewDetailData`**

```go
// review.go — add to reviewDetailData struct:
type reviewDetailData struct {
	Import     Import
	Categories []Category
	Groups     []dateGroup
	Pending    int
	Approved   int
	Skipped    int
	Duplicate  int
	Cutoff     string
	BeforeCutoff int
	PushError  string
	PushResult string
	Warning    string
}
```

- [ ] **Step 2: Update `buildReviewData` to compute before-cutoff count**

Change `buildReviewData` to accept and use the cutoff:

```go
func buildReviewData(imp Import, txs []Transaction, cats []Category, cutoff string, pushError, pushResult string) reviewDetailData {
	pending, approved, skipped, duplicate := 0, 0, 0, 0
	beforeCutoff := 0
	for _, tx := range txs {
		switch tx.Status {
		case "pending":
			if cutoff != "" && tx.Date <= cutoff {
				beforeCutoff++
			} else {
				pending++
			}
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
		Import:       imp,
		Categories:   cats,
		Groups:       groups,
		Pending:      pending,
		Approved:     approved,
		Skipped:      skipped,
		Duplicate:    duplicate,
		Cutoff:       cutoff,
		BeforeCutoff: beforeCutoff,
		PushError:    pushError,
		PushResult:   pushResult,
	}
}
```

- [ ] **Step 3: Update all callers of `buildReviewData`**

In `handleReviewDetail`:
```go
// load cutoff
cutoff, _ := getSetting(db, "push_cutoff")
data := buildReviewData(*imp, txs, cats, cutoff, "", "")
```

In `handlePush` (in push.go):
```go
cutoff, _ := getSetting(db, "push_cutoff")
data := buildReviewData(*imp, allTxs, cats2, cutoff, pushError, pushResultStr)
```

- [ ] **Step 4: Run build to verify compilation**

Run: `go build ./...`
Expected: builds without errors

---

### Task 3: `push.go` — Cutoff check in push loop

**Files:**
- Modify: `push.go`

- [ ] **Step 1: Add `BeforeCutoff` to `pushResult`**

```go
type pushResult struct {
	Total        int
	Added        int
	Updated      int
	Errors       int
	BeforeCutoff int
	ErrorList    []string
}
```

- [ ] **Step 2: Add cutoff check in push loop**

In `handlePush`, after loading transactions and before the loop:

```go
cutoff, _ := getSetting(db, "push_cutoff")
```

Inside the loop, after `if tx.Status != "approved" { continue }` and `result.Total++`:

```go
if cutoff != "" && tx.Date <= cutoff {
    result.BeforeCutoff++
    continue
}
```

- [ ] **Step 3: Update the result message to show cutoff info**

```go
pushResultStr = fmt.Sprintf("Pushed: %d added, %d errors", result.Added, result.Errors)
if result.BeforeCutoff > 0 {
    cutoffStr := " (today)"
    if c := cutoff; c != "" {
        cutoffStr = " (cutoff: " + c + ")"
    }
    pushResultStr = fmt.Sprintf("Cutoff%s — %d before cutoff skipped. Pushed: %d added, %d errors",
        cutoffStr, result.BeforeCutoff, result.Added, result.Errors)
}
```

- [ ] **Step 4: Run build to verify compilation**

Run: `go build ./...`
Expected: builds without errors

---

### Task 4: `main.go` — Register route and add handler

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Register `POST /settings/cutoff` route**

```go
// main.go — registerRoutes, after other /settings routes:
mux.HandleFunc("POST /settings/cutoff", handleAdvanceCutoff)
```

- [ ] **Step 2: Add `handleAdvanceCutoff` handler**

```go
func handleAdvanceCutoff(w http.ResponseWriter, r *http.Request) {
	date := r.FormValue("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	} else {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			http.Error(w, "invalid date format (expected YYYY-MM-DD)", http.StatusBadRequest)
			return
		}
	}

	if err := setSetting(db, "push_cutoff", date); err != nil {
		log.Printf("set cutoff: %v", err)
		http.Error(w, "failed to save cutoff", http.StatusInternalServerError)
		return
	}

	// Render the cutoff section for htmx swap
	cutoffSectionData := struct {
		Cutoff string
	}{Cutoff: date}
	if err := tmpl.ExecuteTemplate(w, "cutoffSection", cutoffSectionData); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

- [ ] **Step 3: Run build to verify compilation**

Run: `go build ./...`
Expected: builds without errors

---

### Task 5: `templates/settings.html` — Add Push Cutoff section

**Files:**
- Modify: `templates/settings.html`

- [ ] **Step 1: Add `Cutoff string` to `settingsPageData` in `main.go`**

```go
type settingsPageData struct {
	Accounts        []BankAccount
	ActualAccounts  []ActualAccount
	PdfToText       bool
	ActualReachable bool
	Cutoff          string
	Error           string
}
```

In `handleSettingsPage`, load cutoff:
```go
cutoff, _ := getSetting(db, "push_cutoff")
data := settingsPageData{
    Accounts:        accounts,
    ActualAccounts:  actualAccounts,
    PdfToText:       pdfErr == nil,
    ActualReachable: err == nil,
    Cutoff:          cutoff,
    Error:           r.URL.Query().Get("error"),
}
```

- [ ] **Step 2: Add cutoff section to `templates/settings.html` and standalone template fragment**

Add between Dependency Status section and Account Mappings section:

```html
<section style="margin-bottom:2.5rem">
    <h2 class="section-title">Push Cutoff</h2>
    <div class="card" style="max-width:600px" id="cutoff-section">
        {{template "cutoffSection" .}}
    </div>
</section>
```

Add after the existing template definitions:

```html
{{define "cutoffSection"}}
<div style="display:flex;flex-direction:column;gap:1rem">
    <div style="display:flex;align-items:center;justify-content:space-between;gap:1rem;padding-bottom:1rem;border-bottom:1px solid var(--border)">
        <div>
            <div style="font-weight:600;font-size:.9rem">Current cutoff</div>
            <div style="font-size:.8rem;color:var(--muted)">Transactions on or before this date will be skipped during push</div>
        </div>
        {{if .Cutoff}}
        <span class="badge badge-done" style="font-size:.85rem;font-family:monospace">{{.Cutoff}}</span>
        {{else}}
        <span class="badge badge-pending" style="font-size:.8rem">Not set</span>
        {{end}}
    </div>
    {{if not .Cutoff}}
    <div style="font-size:.8rem;color:var(--muted)">No cutoff set — all approved transactions will be pushed.</div>
    {{end}}
    <div style="display:flex;gap:.75rem;align-items:flex-end;flex-wrap:wrap">
        <div>
            <label style="display:block;font-size:.8rem;font-weight:500;margin-bottom:.25rem">Advance to today</label>
            <button class="btn btn-primary" style="font-size:.85rem"
                hx-post="/settings/cutoff"
                hx-target="#cutoff-section"
                hx-swap="innerHTML"
                hx-trigger="click">Advance to today</button>
        </div>
        <div>
            <label for="cutoff-date" style="display:block;font-size:.8rem;font-weight:500;margin-bottom:.25rem">Set specific date</label>
            <div style="display:flex;gap:.5rem">
                <input type="date" id="cutoff-date" name="cutoff-date" class="form-control" style="max-width:180px">
                <button class="btn btn-secondary" style="font-size:.85rem"
                    hx-post="/settings/cutoff"
                    hx-target="#cutoff-section"
                    hx-swap="innerHTML"
                    hx-include="#cutoff-date"
                    hx-vals='{"date": document.getElementById("cutoff-date").value}'>Save</button>
            </div>
        </div>
    </div>
</div>
{{end}}
```

Wait — the hx-vals approach won't work because we need to send the date value dynamically. Better to use a form or hx-include:

```html
<div>
    <label for="cutoff-date" style="display:block;font-size:.8rem;font-weight:500;margin-bottom:.25rem">Set specific date</label>
    <div style="display:flex;gap:.5rem">
        <input type="date" id="cutoff-date" name="date" class="form-control" style="max-width:180px">
        <button class="btn btn-secondary" style="font-size:.85rem"
            hx-post="/settings/cutoff"
            hx-target="#cutoff-section"
            hx-swap="innerHTML"
            hx-include="#cutoff-date">Save</button>
    </div>
</div>
```

- [ ] **Step 3: Run build to verify compilation**

Run: `go build ./...`
Expected: builds without errors

---

### Task 6: `templates/review.html` — Cutoff display in review page

**Files:**
- Modify: `templates/review.html`

- [ ] **Step 1: Add cutoff banner near push button**

In the review detail template, after the push button and before the filter toggles:

```html
{{if .Cutoff}}
<div class="alert alert-info" style="margin-bottom:1rem;padding:.5rem .75rem;font-size:.85rem">
    Cutoff: <strong>{{.Cutoff}}</strong>
    {{if .BeforeCutoff}}
    — <strong>{{.BeforeCutoff}}</strong> transaction{{if ne .BeforeCutoff 1}}s{{end}} before cutoff will be skipped during push
    {{end}}
</div>
{{end}}
```

- [ ] **Step 2: Render pre-cutoff transactions dimmed without action controls**

In the `tx-row` section inside `reviewDetail` range (around line 190), wrap the pending controls so pre-cutoff txs show a label instead:

Replace the pending status block (lines 214-236):

```html
{{if eq .Transaction.Status "pending"}}
    {{if $.Cutoff and (le .Transaction.Date $.Cutoff)}}
    <span class="tx-status" style="color:var(--muted);font-style:italic">Before cutoff</span>
    {{else}}
    <form class="tx-actions">
        ...
    </form>
    {{end}}
{{else if eq .Transaction.Status "approved"}}
```

Wait, Go templates don't have logical operators in conditionals like `and` that work with string comparison. I need to use a template function or a different approach.

Actually, Go templates do support `and` and `or` but not `<=`. So I can't do `le .Transaction.Date $.Cutoff` in the template. I have a few options:

1. Add a template function for date comparison
2. Use a different approach — mark pre-cutoff transactions in the data prep phase

Option 2 is cleaner. In `buildReviewData` or a separate step, I can mark each `txRowData` with a `BeforeCutoff` bool.

Let me add a `BeforeCutoff bool` field to `txRowData`:

```go
type txRowData struct {
	Transaction Transaction
	Categories  []Category
	BeforeCutoff bool
}
```

In `groupByDate`, I need to pass the cutoff:

```go
func groupByDate(txs []Transaction, cats []Category, cutoff string) []dateGroup {
    ...
    for _, tx := range txs {
        beforeCutoff := cutoff != "" && tx.Date <= cutoff
        group.Transactions = append(group.Transactions, txRowData{
            Transaction:  tx,
            Categories:   cats,
            BeforeCutoff: beforeCutoff,
        })
    }
    ...
}
```

Then in the template:

```html
{{if eq .Transaction.Status "pending"}}
    {{if .BeforeCutoff}}
    <span class="tx-status" style="color:var(--muted);font-style:italic">Before cutoff</span>
    {{else}}
    <form class="tx-actions">
        ...
    </form>
    {{end}}
{{else if eq .Transaction.Status "approved"}}
```

And apply a dimmed style:
```html
style="display:flex;align-items:center;gap:.75rem;
    {{if eq .Transaction.Status "approved"}}background:#f0fdf4;{{end}}
    {{if eq .Transaction.Status "skipped"}}opacity:.5;{{end}}
    {{if eq .Transaction.Status "duplicate"}}opacity:.4;{{end}}
    {{if .BeforeCutoff}}opacity:.5;{{end}}"
```

Also add a `BeforeCutoff` filter option in the Alpine toggle group:
```html
<label><input type="checkbox" x-model="showBeforeCutoff"> Before cutoff</label>
```

And add the `showBeforeCutoff` variable to the Alpine data:
```js
showBeforeCutoff: true,
```

And add it to the `x-show` expression:
```html
x-show="
    (showPending && '{{.Transaction.Status}}' === 'pending' && !{{.BeforeCutoff}}) ||
    (showBeforeCutoff && '{{.Transaction.Status}}' === 'pending' && {{.BeforeCutoff}}) ||
    ...
"
```

- [ ] **Step 3: Run build to verify compilation**

Run: `go build ./...`
Expected: builds without errors

---

### Task 7: Tests

**Files:**
- Modify: `push.go`, `review.go`, `store.go` (already done above)
- Create/Modify test files as needed

- [ ] **Step 1: Add test for `getSetting`/`setSetting`**

In `store.go` test file or a new test:

(Actually, store.go doesn't have a test file. Let me check existing test patterns and add tests.)

Add to `review_test.go` or a new `store_test.go`:

We need to add tests. Let me write the key tests.

- [ ] **Step 2: Add cutoff test in `review_test.go`**

```go
func TestBuildReviewDataWithCutoff(t *testing.T) {
	// setup
	cfg = Config{SQLitePath: ":memory:"}
	var err error
	db, err = openDB(cfg.SQLitePath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	imp := Import{ID: "test-1", Bank: "bca"}
	txs := []Transaction{
		{ID: "tx1", Date: "2025-01-01", Amount: 1000, Status: "pending"},
		{ID: "tx2", Date: "2025-01-15", Amount: 2000, Status: "pending"},
		{ID: "tx3", Date: "2025-02-01", Amount: 3000, Status: "pending"},
		{ID: "tx4", Date: "2025-01-10", Amount: 4000, Status: "approved"},
	}

	// With cutoff at 2025-01-14
	data := buildReviewData(imp, txs, nil, "2025-01-14", "", "")
	if data.Pending != 1 {
		t.Errorf("Pending = %d, want 1 (only tx3)", data.Pending)
	}
	if data.BeforeCutoff != 2 {
		t.Errorf("BeforeCutoff = %d, want 2 (tx1, tx2)", data.BeforeCutoff)
	}
	if data.Approved != 1 {
		t.Errorf("Approved = %d, want 1 (tx4 is approved, not affected by cutoff)", data.Approved)
	}

	// With no cutoff (empty string) — existing behavior preserved
	data2 := buildReviewData(imp, txs, nil, "", "", "")
	if data2.Pending != 3 {
		t.Errorf("Pending without cutoff = %d, want 3", data2.Pending)
	}
	if data2.BeforeCutoff != 0 {
		t.Errorf("BeforeCutoff without cutoff = %d, want 0", data2.BeforeCutoff)
	}
}
```

- [ ] **Step 3: Run all tests**

Run: `go test ./... -v`
Expected: all tests pass

- [ ] **Step 4: Run build**

Run: `go build ./...`
Expected: builds without errors
