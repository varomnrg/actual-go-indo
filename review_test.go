package main

import (
	"testing"
)

func TestSuggestCatIDRegistered(t *testing.T) {
	fn, ok := tmplFuncs["suggestCatID"].(func(string, []Category) string)
	if !ok {
		t.Fatal("suggestCatID function not registered in tmplFuncs")
	}
	_ = fn
}

func TestSuggestCatIDMatch(t *testing.T) {
	fn, ok := tmplFuncs["suggestCatID"].(func(string, []Category) string)
	if !ok {
		t.Fatal("suggestCatID function not registered in tmplFuncs")
	}

	cats := []Category{
		{ID: "cat-1", Name: "Food"},
		{ID: "cat-2", Name: "Coffee"},
		{ID: "cat-3", Name: "Transport"},
		{ID: "cat-4", Name: "Shopping"},
	}

	got := fn("GOPAY payment", cats)
	if got != "cat-1" {
		t.Errorf("suggestCatID(%q, cats) = %q, want cat-1 (Food)", "GOPAY payment", got)
	}

	got = fn("starbucks coffee", cats)
	if got != "cat-2" {
		t.Errorf("suggestCatID(%q, cats) = %q, want cat-2 (Coffee)", "starbucks coffee", got)
	}

	got = fn("grab ride", cats)
	if got != "cat-3" {
		t.Errorf("suggestCatID(%q, cats) = %q, want cat-3 (Transport)", "grab ride", got)
	}
}

func TestSuggestCatIDNoMatch(t *testing.T) {
	fn, ok := tmplFuncs["suggestCatID"].(func(string, []Category) string)
	if !ok {
		t.Fatal("suggestCatID function not registered in tmplFuncs")
	}

	cats := []Category{
		{ID: "cat-1", Name: "Food"},
		{ID: "cat-2", Name: "Coffee"},
	}

	got := fn("random description", cats)
	if got != "" {
		t.Errorf("suggestCatID(%q, cats) = %q, want empty string", "random description", got)
	}
}

func TestSuggestCatIDSuggestedNameNotFound(t *testing.T) {
	fn, ok := tmplFuncs["suggestCatID"].(func(string, []Category) string)
	if !ok {
		t.Fatal("suggestCatID function not registered in tmplFuncs")
	}

	cats := []Category{
		{ID: "cat-1", Name: "Transport"},
	}

	got := fn("starbucks coffee", cats)
	if got != "" {
		t.Errorf("suggestCatID(%q, cats with no Coffee) = %q, want empty string (category not in list)", "starbucks coffee", got)
	}
}

func TestBuildReviewDataWithCutoff(t *testing.T) {
	imp := Import{ID: "test-1", Bank: "bca"}
	txs := []Transaction{
		{ID: "tx1", Date: "2025-01-01", Amount: 1000, Status: "pending"},
		{ID: "tx2", Date: "2025-01-15", Amount: 2000, Status: "pending"},
		{ID: "tx3", Date: "2025-02-01", Amount: 3000, Status: "pending"},
		{ID: "tx4", Date: "2025-01-10", Amount: 4000, Status: "approved"},
		{ID: "tx5", Date: "2025-01-20", Amount: 5000, Status: "skipped"},
		{ID: "tx6", Date: "2025-01-25", Amount: 6000, Status: "duplicate"},
	}

	data := buildReviewData(imp, txs, nil, "2025-01-14", "", "")
	if data.Pending != 2 {
		t.Errorf("Pending = %d, want 2 (tx2, tx3)", data.Pending)
	}
	if data.BeforeCutoff != 1 {
		t.Errorf("BeforeCutoff = %d, want 1 (tx1)", data.BeforeCutoff)
	}
	if data.Approved != 1 {
		t.Errorf("Approved = %d, want 1", data.Approved)
	}
	if data.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", data.Skipped)
	}
	if data.Duplicate != 1 {
		t.Errorf("Duplicate = %d, want 1", data.Duplicate)
	}

	data2 := buildReviewData(imp, txs, nil, "", "", "")
	if data2.Pending != 3 {
		t.Errorf("Pending without cutoff = %d, want 3", data2.Pending)
	}
	if data2.BeforeCutoff != 0 {
		t.Errorf("BeforeCutoff without cutoff = %d, want 0", data2.BeforeCutoff)
	}
}

func TestGroupByDateWithCutoff(t *testing.T) {
	txs := []Transaction{
		{ID: "tx1", Date: "2025-01-01", Description: "before cutoff"},
		{ID: "tx2", Date: "2025-01-15", Description: "after cutoff"},
	}

	groups := groupByDate(txs, nil, "2025-01-14")
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}

	if !groups[0].Transactions[0].BeforeCutoff {
		t.Error("tx1 (2025-01-01) should be before cutoff")
	}
	if groups[1].Transactions[0].BeforeCutoff {
		t.Error("tx2 (2025-01-15) should NOT be before cutoff")
	}

	groups2 := groupByDate(txs, nil, "")
	for i, g := range groups2 {
		for _, tx := range g.Transactions {
			if tx.BeforeCutoff {
				t.Errorf("tx %s in group %d should NOT be before cutoff when cutoff is empty", tx.Transaction.ID, i)
			}
		}
	}
}
