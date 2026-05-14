package main

import (
	"testing"
)

func TestSuggestCategoryMatch(t *testing.T) {
	tests := []struct {
		desc string
		want string
	}{
		{"GOPAY payment", "Food"},
		{"makan siang", "Food"},
		{"nasi goreng", "Food"},
		{"starbucks coffee", "Coffee"},
		{"kopi kenangan", "Coffee"},
		{"netflix subscription", "Subscription"},
		{"spotify premium", "Subscription"},
		{"tokopedia belanja", "Shopping"},
		{"shopee order", "Shopping"},
		{"grab ride", "Transport"},
		{"gojek trip", "Transport"},
		{"bensin pertamina", "Transport"},
		{"bills payment", "Bills"},
		{"tagihan listrik", "Bills"},
		{"gaji bulanan", "Salary"},
		{"hiburan nonton", "Entertainment"},
		{"cinema ticket", "Entertainment"},
		{"transfer bifast", "Transfer"},
	}
	for _, tt := range tests {
		got := SuggestCategory(tt.desc)
		if got != tt.want {
			t.Errorf("SuggestCategory(%q) = %q, want %q", tt.desc, got, tt.want)
		}
	}
}

func TestSuggestCategoryNoMatch(t *testing.T) {
	got := SuggestCategory("random description xyz")
	if got != "" {
		t.Errorf("SuggestCategory(%q) = %q, want empty string", "random description xyz", got)
	}
}

func TestSuggestCategoryCaseInsensitive(t *testing.T) {
	got := SuggestCategory("KOPI KENANGAN")
	if got != "Coffee" {
		t.Errorf("SuggestCategory(%q) = %q, want Coffee", "KOPI KENANGAN", got)
	}

	got = SuggestCategory("StArBuCkS")
	if got != "Coffee" {
		t.Errorf("SuggestCategory(%q) = %q, want Coffee", "StArBuCkS", got)
	}
}

func TestSuggestCategoryEmpty(t *testing.T) {
	got := SuggestCategory("")
	if got != "" {
		t.Errorf("SuggestCategory(%q) = %q, want empty string", "", got)
	}
}
