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
