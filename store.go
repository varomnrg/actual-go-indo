package main

import (
	"crypto/sha1"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Import struct {
	ID        string
	Filename  string
	Bank      string
	Account   string
	Period    string
	FilePath  string
	Status    string
	ErrorMsg  string
	TxCount   int
	CreatedAt time.Time
}

type Transaction struct {
	ID                string
	ImportID          string
	Date              string
	Time              string
	Description       string
	Notes             string
	Amount            int64
	Balance           int64
	ReferenceID       string
	CategoryID        string
	TransferToAccount string
	Status            string
	CreatedAt         time.Time
}

type Category struct {
	ID          string
	Name        string
	ActualCatID string
	SortOrder   int
}

type BankAccount struct {
	ID              string
	Bank            string
	Identifier      string
	ActualAccountID string
}

const schema = `
CREATE TABLE IF NOT EXISTS imports (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    filename    TEXT NOT NULL,
    bank        TEXT NOT NULL CHECK (bank IN ('bca', 'mandiri', 'jago')),
    account     TEXT NOT NULL DEFAULT '',
    period      TEXT NOT NULL DEFAULT '',
    file_path   TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'parsing' CHECK (status IN ('parsing', 'pending', 'done', 'error')),
    error_msg   TEXT NOT NULL DEFAULT '',
    tx_count    INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS transactions (
    id                  TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    import_id           TEXT NOT NULL REFERENCES imports(id),
    date                TEXT NOT NULL,
    time                TEXT NOT NULL DEFAULT '',
    description         TEXT NOT NULL,
    notes               TEXT NOT NULL DEFAULT '',
    amount              INTEGER NOT NULL,
    balance             INTEGER NOT NULL DEFAULT 0,
    reference_id        TEXT NOT NULL DEFAULT '',
    description_hash    TEXT NOT NULL DEFAULT '',
    category_id         TEXT NOT NULL DEFAULT '',
    transfer_to_account TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'skipped', 'duplicate')),
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS categories (
    id              TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    name            TEXT NOT NULL UNIQUE,
    actual_cat_id   TEXT NOT NULL DEFAULT '',
    sort_order      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS bank_accounts (
    id                TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    bank              TEXT NOT NULL,
    identifier        TEXT NOT NULL,
    actual_account_id TEXT NOT NULL DEFAULT '',
    UNIQUE (bank, identifier)
);

CREATE INDEX IF NOT EXISTS idx_tx_import ON transactions(import_id);
CREATE INDEX IF NOT EXISTS idx_tx_status ON transactions(status);
CREATE INDEX IF NOT EXISTS idx_tx_date ON transactions(date);
`

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("run schema: %w", err)
	}
	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return db, nil
}

func runMigrations(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(transactions)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var hasDescHash bool
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == "description_hash" {
			hasDescHash = true
		}
	}
	if !hasDescHash {
		_, err := db.Exec(`ALTER TABLE transactions ADD COLUMN description_hash TEXT NOT NULL DEFAULT ''`)
		if err != nil {
			return err
		}
	}
	return nil
}

// --- imports ---

func insertImport(db *sql.DB, imp *Import) (string, error) {
	var id string
	err := db.QueryRow(`
		INSERT INTO imports (filename, bank, account, period, file_path, status)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id`,
		imp.Filename, imp.Bank, imp.Account, imp.Period, imp.FilePath, imp.Status,
	).Scan(&id)
	return id, err
}

func updateImportStatus(db *sql.DB, id, status, errMsg string, txCount int) error {
	_, err := db.Exec(
		`UPDATE imports SET status=?, error_msg=?, tx_count=? WHERE id=?`,
		status, errMsg, txCount, id,
	)
	return err
}

func getImport(db *sql.DB, id string) (*Import, error) {
	imp := &Import{}
	err := db.QueryRow(`
		SELECT id, filename, bank, account, period, file_path, status, error_msg, tx_count, created_at
		FROM imports WHERE id=?`, id,
	).Scan(&imp.ID, &imp.Filename, &imp.Bank, &imp.Account, &imp.Period,
		&imp.FilePath, &imp.Status, &imp.ErrorMsg, &imp.TxCount, &imp.CreatedAt)
	if err != nil {
		return nil, err
	}
	return imp, nil
}

func listImports(db *sql.DB) ([]Import, error) {
	rows, err := db.Query(`SELECT id, filename, bank, account, period, status, tx_count, created_at FROM imports ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var imports []Import
	for rows.Next() {
		var imp Import
		if err := rows.Scan(&imp.ID, &imp.Filename, &imp.Bank, &imp.Account, &imp.Period, &imp.Status, &imp.TxCount, &imp.CreatedAt); err != nil {
			return nil, err
		}
		imports = append(imports, imp)
	}
	return imports, rows.Err()
}

// --- transactions ---

func insertTransaction(db *sql.DB, tx *Transaction) (string, error) {
	descHash := fmt.Sprintf("%x", sha1.Sum([]byte(tx.Description)))
	var id string
	err := db.QueryRow(`
		INSERT INTO transactions (import_id, date, time, description, notes, amount, balance, reference_id, description_hash, category_id, transfer_to_account, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id`,
		tx.ImportID, tx.Date, tx.Time, tx.Description, tx.Notes,
		tx.Amount, tx.Balance, tx.ReferenceID, descHash, tx.CategoryID, tx.TransferToAccount, tx.Status,
	).Scan(&id)
	return id, err
}

func listTransactions(db *sql.DB, importID string) ([]Transaction, error) {
	rows, err := db.Query(`
		SELECT id, import_id, date, time, description, notes, amount, balance,
		       reference_id, category_id, transfer_to_account, status, created_at
		FROM transactions WHERE import_id=? ORDER BY date, created_at`, importID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var txs []Transaction
	for rows.Next() {
		var tx Transaction
		if err := rows.Scan(&tx.ID, &tx.ImportID, &tx.Date, &tx.Time, &tx.Description,
			&tx.Notes, &tx.Amount, &tx.Balance, &tx.ReferenceID, &tx.CategoryID,
			&tx.TransferToAccount, &tx.Status, &tx.CreatedAt); err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}
	return txs, rows.Err()
}

func updateTransactionStatus(db *sql.DB, id, status, categoryID string) error {
	_, err := db.Exec(`UPDATE transactions SET status=?, category_id=? WHERE id=?`, status, categoryID, id)
	return err
}

func referenceIDExists(db *sql.DB, referenceID string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(1) FROM transactions WHERE reference_id=? AND reference_id != ''`, referenceID).Scan(&count)
	return count > 0, err
}

func descriptionHashExists(db *sql.DB, importID, bank, date string, amount int64, descHash string) (bool, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(1) FROM transactions t
		JOIN imports i ON i.id = t.import_id
		WHERE t.import_id != ?
		  AND i.bank = ?
		  AND t.date = ?
		  AND t.amount = ?
		  AND t.description_hash = ?
	`, importID, bank, date, amount, descHash).Scan(&count)
	return count > 0, err
}

// --- categories ---

func listCategories(db *sql.DB) ([]Category, error) {
	rows, err := db.Query(`SELECT id, name, actual_cat_id, sort_order FROM categories ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.ActualCatID, &c.SortOrder); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

func insertCategory(db *sql.DB, name string) (string, error) {
	var id string
	err := db.QueryRow(`INSERT INTO categories (name) VALUES (?) RETURNING id`, name).Scan(&id)
	return id, err
}

func updateCategoryActualID(db *sql.DB, id, actualCatID string) error {
	_, err := db.Exec(`UPDATE categories SET actual_cat_id=? WHERE id=?`, actualCatID, id)
	return err
}

// --- bank_accounts ---

func listBankAccounts(db *sql.DB) ([]BankAccount, error) {
	rows, err := db.Query(`SELECT id, bank, identifier, actual_account_id FROM bank_accounts ORDER BY bank, identifier`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []BankAccount
	for rows.Next() {
		var a BankAccount
		if err := rows.Scan(&a.ID, &a.Bank, &a.Identifier, &a.ActualAccountID); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func upsertBankAccount(db *sql.DB, bank, identifier, actualAccountID string) error {
	_, err := db.Exec(`
		INSERT INTO bank_accounts (bank, identifier, actual_account_id) VALUES (?, ?, ?)
		ON CONFLICT(bank, identifier) DO UPDATE SET actual_account_id=excluded.actual_account_id`,
		bank, identifier, actualAccountID,
	)
	return err
}

func getBankAccount(db *sql.DB, bank, identifier string) (*BankAccount, error) {
	a := &BankAccount{}
	err := db.QueryRow(`SELECT id, bank, identifier, actual_account_id FROM bank_accounts WHERE bank=? AND identifier=?`, bank, identifier).
		Scan(&a.ID, &a.Bank, &a.Identifier, &a.ActualAccountID)
	if err != nil {
		return nil, err
	}
	return a, nil
}
