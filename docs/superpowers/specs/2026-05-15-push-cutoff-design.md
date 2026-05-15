# Push Cutoff — Design Spec

**Issue:** `.scratch/banktoactual/issues/11-push-cutoff.md`
**Date:** 2026-05-15
**Status:** Approved

## Problem

When bootstrapping Actual Budget with existing bank history, transactions already in Actual have no `imported_id`, so there's no way to detect duplicates. Users need a push cutoff date: everything before the cutoff is assumed already synced, only post-cutoff transactions get pushed.

## Design

### 1. `store.go` — `app_settings` table + helpers

Add via `runMigrations`:
```sql
CREATE TABLE IF NOT EXISTS app_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);
```

Helpers:
```go
func getSetting(db *sql.DB, key string) (string, error)
func setSetting(db *sql.DB, key, value string) error  // upsert
```

Cutoff stored as `key="push_cutoff"`, `value="YYYY-MM-DD"` or `""` (no cutoff → push everything).

### 2. `push.go` — skip pre-cutoff transactions

`handlePush` loads cutoff, skips approved txs with `date <= cutoff`, counts as `BeforeCutoff`. Result message shows cutoff and skipped count.

### 3. `review.go` — cutoff in review page

`reviewDetailData` gains `Cutoff string` and `BeforeCutoff int`. `buildReviewData` computes `BeforeCutoff` from stored setting. Pre-cutoff pending txs excluded from `Pending` count. Value is derived dynamically — always reflects current cutoff regardless of last push.

### 4. `main.go` — settings endpoint

```
POST /settings/cutoff
```

Reads `date` from form (defaults to `time.Now().Format("2006-01-02")`). Validates YYYY-MM-DD format. Calls `setSetting`. Returns updated cutoff section via htmx.

`settingsPageData` gains `Cutoff string`.

### 5. Templates

**`settings.html`**: New Push Cutoff section below Dependency Status. Shows current cutoff or "No cutoff set". Htmx "Advance to today" button + manual date input with Save button.

**`review.html`**: Shows cutoff banner near push button: "Cutoff: YYYY-MM-DD — N transactions before cutoff will be skipped." Pre-cutoff pending txs render dimmed with "(before cutoff)" label instead of approve/skip controls. Alpine filter toggles include a cutoff group.

### 6. Pre-cutoff transactions

Keep DB `status="pending"` unchanged (no migration needed). The UI and push logic derive cutoff behavior from the setting. Pre-cutoff txs appear dimmed without action controls. They don't count toward `Pending` total.

## Key Decisions

- **Before-cutoff is always current**: `buildReviewData` computes it from stored setting every time the review page loads, not cached from the last push.
- **No status changes on existing records**: Pre-cutoff transactions remain `pending` in DB. The cutoff is purely a UI/push filter.
- **Cutoff preserves semantics**: `""` means no cutoff → existing behavior unchanged.

## Files Changed

- `store.go` — table + helpers + migration
- `push.go` — cutoff check in push loop + result
- `review.go` — cutoff in data struct + build logic
- `main.go` — route + handler
- `templates/settings.html` — cutoff section
- `templates/review.html` — cutoff display + pre-cutoff rendering
