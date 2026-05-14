package main

import (
	"strings"
	"testing"
)

func TestParseBCAText(t *testing.T) {
	input := `                              DAFTAR TRANSAKSI
                              =================
PERIODE : SEPTEMBER 2025
NO. REKENING : 2730270111

TANGGAL  KETERANGAN                    CBG           MUTASI               SALDO
                                             SALDO AWAL                             500,000.00
30/09    CREDIT INTEREST              1111             1,234.56          501,234.56
30/09    TRANSFER BIFAST KE BANK      1111        50,000.00 DB          451,234.56
        107669341641
30/09    BCA VIRTUAL ACCOUNT          2222       100,000.00             551,234.56

SALDO AWAL                   500,000.00
MUTASI CR                   101,234.56
MUTASI DB                    50,000.00
SALDO AKHIR                 551,234.56`

	txs, account, period, err := parseBCAText(strings.Split(input, "\n"))
	if err != nil {
		t.Fatalf("parseBCAText: %v", err)
	}

	if account != "2730270111" {
		t.Errorf("account = %q, want 2730270111", account)
	}
	if period != "SEPTEMBER 2025" {
		t.Errorf("period = %q, want SEPTEMBER 2025", period)
	}

	if len(txs) != 3 {
		t.Fatalf("got %d transactions, want 3", len(txs))
	}

	// Transaction 1: credit interest
	tx1 := txs[0]
	if tx1.Date != "2025-09-30" {
		t.Errorf("tx1 date = %q, want 2025-09-30", tx1.Date)
	}
	if tx1.Description != "CREDIT INTEREST" {
		t.Errorf("tx1 desc = %q, want CREDIT INTEREST", tx1.Description)
	}
	if tx1.Amount != 1234 {
		t.Errorf("tx1 amount = %d, want 1234", tx1.Amount)
	}
	if tx1.Balance != 501234 {
		t.Errorf("tx1 balance = %d, want 501234", tx1.Balance)
	}
	if tx1.Status != "pending" {
		t.Errorf("tx1 status = %q, want pending", tx1.Status)
	}

	// Transaction 2: transfer (debit)
	tx2 := txs[1]
	if tx2.Date != "2025-09-30" {
		t.Errorf("tx2 date = %q, want 2025-09-30", tx2.Date)
	}
	if tx2.Amount != -50000 {
		t.Errorf("tx2 amount = %d, want -50000 (debit)", tx2.Amount)
	}
	if tx2.Balance != 451234 {
		t.Errorf("tx2 balance = %d, want 451234", tx2.Balance)
	}
	if !strings.Contains(tx2.Description, "TRANSFER BIFAST") {
		t.Errorf("tx2 desc = %q, should contain TRANSFER BIFAST", tx2.Description)
	}
	if !strings.Contains(tx2.Description, "107669341641") {
		t.Errorf("tx2 desc = %q, should contain continuation 107669341641", tx2.Description)
	}

	// Transaction 3: credit
	tx3 := txs[2]
	if tx3.Amount != 100000 {
		t.Errorf("tx3 amount = %d, want 100000", tx3.Amount)
	}
	if tx3.Balance != 551234 {
		t.Errorf("tx3 balance = %d, want 551234", tx3.Balance)
	}
}

func TestParseBCASaldoAwalSkip(t *testing.T) {
	input := `PERIODE : OKTOBER 2025
NO. REKENING : 1234567890

TANGGAL  KETERANGAN                    CBG           MUTASI               SALDO
                                             SALDO AWAL                           1,000,000.00
01/10    SOME TRANSACTION             0000           500,000.00          1,500,000.00

SALDO AWAL                 1,000,000.00
MUTASI CR                   500,000.00
MUTASI DB                         0.00
SALDO AKHIR                1,500,000.00`

	txs, account, period, err := parseBCAText(strings.Split(input, "\n"))
	if err != nil {
		t.Fatalf("parseBCAText: %v", err)
	}
	if account != "1234567890" {
		t.Errorf("account = %q, want 1234567890", account)
	}
	if period != "OKTOBER 2025" {
		t.Errorf("period = %q, want OKTOBER 2025", period)
	}
	if len(txs) != 1 {
		t.Fatalf("got %d transactions, want 1", len(txs))
	}
	if txs[0].Amount != 500000 {
		t.Errorf("amount = %d, want 500000", txs[0].Amount)
	}
}

func TestParseBCAEmptyNoTransactions(t *testing.T) {
	input := `PERIODE : JANUARY 2025
NO. REKENING : 1111111111

TANGGAL  KETERANGAN                    CBG           MUTASI               SALDO
                                             SALDO AWAL                           1,000,000.00

SALDO AWAL                 1,000,000.00
MUTASI CR                         0.00
MUTASI DB                         0.00
SALDO AKHIR                1,000,000.00`

	txs, _, _, err := parseBCAText(strings.Split(input, "\n"))
	if err != nil {
		t.Fatalf("parseBCAText: %v", err)
	}
	if len(txs) != 0 {
		t.Fatalf("got %d transactions, want 0", len(txs))
	}
}

func TestBuildBCARefID(t *testing.T) {
	refID := buildBCARefID("2730270111", "30/09", 50000, "TRANSFER BIFAST KE BANK JAGO")
	if !strings.HasPrefix(refID, "bca-2730270111-30/09-50000-") {
		t.Errorf("refID = %q, want prefix bca-2730270111-30/09-50000-", refID)
	}
	if len(refID) != len("bca-2730270111-30/09-50000-")+8 {
		t.Errorf("refID length = %d, want %d", len(refID), len("bca-2730270111-30/09-50000-")+8)
	}
}

func TestParseBCAAmount(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1,234.56", 1234},
		{"500,000.00", 500000},
		{"1,000,000.00", 1000000},
		{"0.00", 0},
		{"100", 100},
		{"", 0},
	}
	for _, tt := range tests {
		got := parseBCAAmount(tt.input)
		if got != tt.want {
			t.Errorf("parseBCAAmount(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseBCAMultiLineDescription(t *testing.T) {
	input := `PERIODE : SEPTEMBER 2025
NO. REKENING : 2730270111

TANGGAL  KETERANGAN                    CBG           MUTASI               SALDO
                                             SALDO AWAL                             500,000.00
30/09    CREDIT INTEREST              1111             1,234.56          501,234.56
30/09    TRANSFER BIFAST KE BANK      1111        50,000.00 DB          451,234.56
        JAGO
        REF:12345

SALDO AWAL                   500,000.00
MUTASI CR                   101,234.56
MUTASI DB                    50,000.00
SALDO AKHIR                 551,234.56`

	txs, _, _, err := parseBCAText(strings.Split(input, "\n"))
	if err != nil {
		t.Fatalf("parseBCAText: %v", err)
	}

	if len(txs) != 2 {
		t.Fatalf("got %d transactions, want 2", len(txs))
	}

	if !strings.Contains(txs[1].Description, "JAGO") {
		t.Errorf("tx2 desc = %q, should contain JAGO continuation", txs[1].Description)
	}
	if !strings.Contains(txs[1].Description, "REF:12345") {
		t.Errorf("tx2 desc = %q, should contain REF:12345 continuation", txs[1].Description)
	}
}

// ── Mandiri tests ─────────────────────────────────────────────────────────────

func TestParseMandiriRows_CreditDebit(t *testing.T) {
	rows := [][]string{
		{"", "Nomor Rekening", "1234567890", "", ""},
		{"", "No", "", "", "Tanggal", "", "", "Keterangan", "", "", "", "", "", "", "", "Kredit", "", "", "Debit", "", "", "Saldo"},
		// Data row 1: credit
		{"", "1", "", "", "01 Jan 2025", "", "", "GAJI MASUK", "", "", "", "", "", "", "", "5.000.000,00", "", "", "", "", "", "10.000.000,00"},
		// Data row 2: time
		{"", "", "", "", "09:00:00 WIB"},
		// Data row 3: debit
		{"", "2", "", "", "02 Jan 2025", "", "", "BELANJA ONLINE", "", "", "", "", "", "", "", "", "", "", "250.000,00", "", "", "9.750.000,00"},
		// Data row 4: time
		{"", "", "", "", "14:30:00 WIB"},
	}

	txs, account, _, err := parseMandiriRows(rows)
	if err != nil {
		t.Fatalf("parseMandiriRows: %v", err)
	}
	if account != "1234567890" {
		t.Errorf("account = %q, want 1234567890", account)
	}
	if len(txs) != 2 {
		t.Fatalf("got %d transactions, want 2", len(txs))
	}

	tx1 := txs[0]
	if tx1.Date != "2025-01-01" {
		t.Errorf("tx1 date = %q, want 2025-01-01", tx1.Date)
	}
	if tx1.Amount != 5000000 {
		t.Errorf("tx1 amount = %d, want 5000000 (credit)", tx1.Amount)
	}
	if tx1.Balance != 10000000 {
		t.Errorf("tx1 balance = %d, want 10000000", tx1.Balance)
	}
	if tx1.Time != "09:00:00" {
		t.Errorf("tx1 time = %q, want 09:00:00", tx1.Time)
	}
	if tx1.Description != "GAJI MASUK" {
		t.Errorf("tx1 desc = %q, want GAJI MASUK", tx1.Description)
	}

	tx2 := txs[1]
	if tx2.Amount != -250000 {
		t.Errorf("tx2 amount = %d, want -250000 (debit)", tx2.Amount)
	}
	if tx2.Time != "14:30:00" {
		t.Errorf("tx2 time = %q, want 14:30:00", tx2.Time)
	}
	if !strings.HasPrefix(tx2.ReferenceID, "mandiri-1234567890-2025-01-02-250000-") {
		t.Errorf("tx2 refID = %q, unexpected prefix", tx2.ReferenceID)
	}
}

func TestParseMandiriRows_HeaderNotFound(t *testing.T) {
	rows := [][]string{
		{"row1col0", "row1col1"},
		{"row2col0", "row2col1"},
	}
	_, _, _, err := parseMandiriRows(rows)
	if err == nil {
		t.Fatal("expected error when header not found")
	}
}

func TestParseIDRAmount(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"5.000.000,00", 5000000},
		{"250.000,50", 250000},
		{"1.234.567,89", 1234567},
		{"0,00", 0},
		{"", 0},
		{"500", 500},
	}
	for _, tt := range tests {
		got := parseIDRAmount(tt.input)
		if got != tt.want {
			t.Errorf("parseIDRAmount(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestBuildMandiriRefID(t *testing.T) {
	ref := buildMandiriRefID("1234567890", "2025-01-01", 5000000, "GAJI MASUK")
	if !strings.HasPrefix(ref, "mandiri-1234567890-2025-01-01-5000000-") {
		t.Errorf("refID = %q, unexpected prefix", ref)
	}
	if len(ref) != len("mandiri-1234567890-2025-01-01-5000000-")+8 {
		t.Errorf("refID length = %d, unexpected", len(ref))
	}
}

// ── Jago tests ────────────────────────────────────────────────────────────────

func TestParseJagoText_CreditDebit(t *testing.T) {
	lines := []string{
		"PT Bank Jago",
		"Main Account",
		"01 Jan 2025 - 31 Jan 2025",
		"Pockets Transactions History",
		"",
		"January 2025",
		"",
		"11 Jan 2025   TRANSFER MASUK                   +50.000.000,00   100.000.000,00",
		"10:30   BCA - 1234567890",
		"ID# TRX001",
		"",
		"15 Jan 2025   BELANJA ONLINE                   -250.000,00      99.750.000,00",
		"14:45   Tokopedia",
		"ID# TRX002",
	}

	txs, pocket, period, err := parseJagoText(lines)
	if err != nil {
		t.Fatalf("parseJagoText: %v", err)
	}
	if pocket != "Main Account" {
		t.Errorf("pocket = %q, want Main Account", pocket)
	}
	if period != "01 Jan 2025 - 31 Jan 2025" {
		t.Errorf("period = %q, want 01 Jan 2025 - 31 Jan 2025", period)
	}
	if len(txs) != 2 {
		t.Fatalf("got %d transactions, want 2", len(txs))
	}

	tx1 := txs[0]
	if tx1.Date != "2025-01-11" {
		t.Errorf("tx1 date = %q, want 2025-01-11", tx1.Date)
	}
	if tx1.Amount != 50000000 {
		t.Errorf("tx1 amount = %d, want 50000000", tx1.Amount)
	}
	if tx1.Balance != 100000000 {
		t.Errorf("tx1 balance = %d, want 100000000", tx1.Balance)
	}
	if tx1.Time != "10:30" {
		t.Errorf("tx1 time = %q, want 10:30", tx1.Time)
	}
	if tx1.ReferenceID != "jago-TRX001" {
		t.Errorf("tx1 refID = %q, want jago-TRX001", tx1.ReferenceID)
	}

	tx2 := txs[1]
	if tx2.Amount != -250000 {
		t.Errorf("tx2 amount = %d, want -250000", tx2.Amount)
	}
	if tx2.ReferenceID != "jago-TRX002" {
		t.Errorf("tx2 refID = %q, want jago-TRX002", tx2.ReferenceID)
	}
}

func TestParseJagoText_Transfer(t *testing.T) {
	lines := []string{
		"PT Bank Jago",
		"Main Account",
		"Pockets Transactions History",
		"11 Jan 2025   Movement between Pockets          -100.000,00      900.000,00",
		"09:00   Main Account → Shopping Pocket",
		"ID# TRX003",
	}

	txs, _, _, err := parseJagoText(lines)
	if err != nil {
		t.Fatalf("parseJagoText: %v", err)
	}
	if len(txs) != 1 {
		t.Fatalf("got %d transactions, want 1", len(txs))
	}
	tx := txs[0]
	if !strings.Contains(tx.Description, "Movement between Pockets") {
		t.Errorf("desc = %q, should contain Movement between Pockets", tx.Description)
	}
	if tx.TransferToAccount != "Shopping Pocket" {
		t.Errorf("TransferToAccount = %q, want Shopping Pocket", tx.TransferToAccount)
	}
	if tx.Amount != -100000 {
		t.Errorf("amount = %d, want -100000", tx.Amount)
	}
}

func TestParseJagoText_FallbackRefID(t *testing.T) {
	lines := []string{
		"PT Bank Jago",
		"Jago Visa",
		"Pockets Transactions History",
		"05 Mar 2025   MERCHANT PAYMENT                  -75.000,00       500.000,00",
		"13:15   Some Store",
	}

	txs, _, _, err := parseJagoText(lines)
	if err != nil {
		t.Fatalf("parseJagoText: %v", err)
	}
	if len(txs) != 1 {
		t.Fatalf("got %d transactions, want 1", len(txs))
	}
	if !strings.HasPrefix(txs[0].ReferenceID, "jago-2025-03-05-") {
		t.Errorf("refID = %q, want jago-2025-03-05-... prefix", txs[0].ReferenceID)
	}
}

func TestParseJagoText_SkipsHeaders(t *testing.T) {
	lines := []string{
		"PT Bank Jago",
		"Main Account",
		"Pockets Transactions History",
		"www.jago.com",
		"Page 1 of 3",
		"December 2024",
		"11 Dec 2024   INCOME                            +1.000.000,00    5.000.000,00",
		"09:00   Employer",
		"ID# TRX100",
		"Page 2 of 3",
		"January 2025",
		"PT Bank Jago",
		"Main Account",
		"Pockets Transactions History",
		"01 Jan 2025   INCOME                            +2.000.000,00    7.000.000,00",
		"10:00   Employer",
		"ID# TRX101",
	}

	txs, _, _, err := parseJagoText(lines)
	if err != nil {
		t.Fatalf("parseJagoText: %v", err)
	}
	if len(txs) != 2 {
		t.Fatalf("got %d transactions, want 2 (skip headers/footers)", len(txs))
	}
}
