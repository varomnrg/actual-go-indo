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
