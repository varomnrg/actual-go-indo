package main

import (
	"database/sql"
	"strings"
)

var categoryKeywords = map[string]string{
	"gopay":       "Food",
	"gojek":       "Transport",
	"grab":        "Transport",
	"grabcar":     "Transport",
	"grabfood":    "Food",
	"makan":       "Food",
	"nasi":        "Food",
	"kopi":        "Coffee",
	"coffee":      "Coffee",
	"cafe":        "Coffee",
	"starbucks":   "Coffee",
	"bills":       "Bills",
	"tagihan":     "Bills",
	"listrik":     "Bills",
	"pdam":        "Bills",
	"telkom":      "Bills",
	"indihome":    "Subscription",
	"spotify":     "Subscription",
	"netflix":     "Subscription",
	"gym":         "Subscription",
	"subscription": "Subscription",
	"tokopedia":   "Shopping",
	"shopee":      "Shopping",
	"lazada":      "Shopping",
	"blibli":      "Shopping",
	"alfamart":    "Shopping",
	"indomaret":   "Shopping",
	"supermarket": "Shopping",
	"bahan pokok": "Shopping",
	"bensin":      "Transport",
	"pertamina":   "Transport",
	"toll":        "Transport",
	"tol":         "Transport",
	"transit":     "Transport",
	"gaji":        "Salary",
	"salary":      "Salary",
	"transfer":    "Transfer",
	"bifast":      "Transfer",
	"qr":          "Transfer",
	"hiburan":     "Entertainment",
	"nonton":      "Entertainment",
	"cinema":      "Entertainment",
	"bioskop":     "Entertainment",
	"game":        "Entertainment",
}

func SuggestCategory(description string) string {
	desc := strings.ToLower(description)
	for keyword, category := range categoryKeywords {
		if strings.Contains(desc, keyword) {
			return category
		}
	}
	return ""
}

var defaultCategories = []string{
	"Food", "Transport", "Shopping", "Bills", "Transfer",
	"Salary", "Entertainment", "Coffee", "Subscription", "Other",
}

func seedCategories(db *sql.DB) error {
	cats, err := listCategories(db)
	if err != nil {
		return err
	}
	if len(cats) > 0 {
		return nil
	}
	for _, name := range defaultCategories {
		if _, err := insertCategory(db, name); err != nil {
			return err
		}
	}
	return nil
}
