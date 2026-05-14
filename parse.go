package main

import (
	"crypto/sha1"
	"fmt"
	"io"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var monthNumbers = map[string]string{
	"JANUARY": "01", "FEBRUARY": "02", "MARCH": "03", "APRIL": "04",
	"MAY": "05", "JUNE": "06", "JULY": "07", "AUGUST": "08",
	"SEPTEMBER": "09", "OCTOBER": "10", "NOVEMBER": "11", "DECEMBER": "12",
	"JAN": "01", "FEB": "02", "MAR": "03", "APR": "04",
	"JUN": "06", "JUL": "07", "AUG": "08",
	"SEP": "09", "OCT": "10", "NOV": "11", "DEC": "12",
	"JANUARI": "01", "FEBRUARI": "02", "MARET": "03",
	"MEI": "05", "JUNI": "06", "JULI": "07", "AGUSTUS": "08",
	"OKTOBER": "10", "DESEMBER": "12",
}

var rePeriode = regexp.MustCompile(`(?i)PERIODE\s*:\s*(\w+)\s+(\d{4})`)
var reRekening = regexp.MustCompile(`(?i)NO\.?\s*REKENING\s*:\s*(\d+)`)
var reDateLine = regexp.MustCompile(`^\s*(\d{2})/(\d{2})\s+(.*)`)
var reAmount = regexp.MustCompile(`([\d,]+\.\d{2})\s*(DB)?`)

func ParseBCA(reader io.Reader) ([]Transaction, string, string, error) {
	cmd := exec.Command("pdftotext", "-layout", "-", "-")
	cmd.Stdin = reader
	output, err := cmd.Output()
	if err != nil {
		return nil, "", "", fmt.Errorf("pdftotext: %w", err)
	}
	lines := strings.Split(string(output), "\n")
	return parseBCAText(lines)
}

func parseBCAText(lines []string) ([]Transaction, string, string, error) {
	var period, account string
	var transactions []Transaction
	var currentTx *Transaction
	annualPeriod := ""

	for _, line := range lines {
		if m := rePeriode.FindStringSubmatch(line); m != nil {
			monthName := strings.ToUpper(m[1])
			year := m[2]
			monthNum, ok := monthNumbers[monthName]
			if !ok {
				return nil, "", "", fmt.Errorf("unknown month: %s", monthName)
			}
			period = fmt.Sprintf("%s %s", m[1], year)
			annualPeriod = year + "-" + monthNum
			continue
		}

		if m := reRekening.FindStringSubmatch(line); m != nil {
			account = m[1]
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.Contains(trimmed, "TANGGAL") && strings.Contains(trimmed, "KETERANGAN") {
			continue
		}

		if strings.Contains(trimmed, "SALDO AWAL") || strings.Contains(trimmed, "SALDO AKHIR") ||
			strings.Contains(trimmed, "MUTASI CR") || strings.Contains(trimmed, "MUTASI DB") ||
			strings.Contains(trimmed, "SALDO RATA") {
			continue
		}

		if m := reDateLine.FindStringSubmatch(line); m != nil {
			if currentTx != nil {
				transactions = append(transactions, *currentTx)
			}

			day := m[1]
			month := m[2]
			rest := m[3]

			amountValue, isDebit, balanceValue, err := extractAmounts(rest)
			if err != nil {
				continue
			}

			amount := amountValue
			if isDebit {
				amount = -amount
			}

			desc := extractDescriptionBeforeAmount(rest, amountValue, isDebit)

			fullDate := annualPeriod[:4] + "-" + month + "-" + day
			absAmount := int64(math.Abs(float64(amount)))
			refID := buildBCARefID(account, day+"/"+month, absAmount, desc)

			currentTx = &Transaction{
				Date:        fullDate,
				Description: desc,
				Amount:      amount,
				Balance:     balanceValue,
				ReferenceID: refID,
				Status:      "pending",
			}
		} else if currentTx != nil {
			t := strings.TrimSpace(line)
			if t != "" {
				currentTx.Description += " " + t
			}
		}
	}

	if currentTx != nil {
		transactions = append(transactions, *currentTx)
	}

	if account == "" {
		account = "unknown"
	}

	return transactions, account, period, nil
}

func extractAmounts(s string) (amount int64, isDebit bool, balance int64, err error) {
	matches := reAmount.FindAllStringSubmatch(s, -1)
	if len(matches) < 2 {
		return 0, false, 0, fmt.Errorf("not enough amount fields")
	}

	balanceMatch := matches[len(matches)-1]
	balance = parseBCAAmount(balanceMatch[1])

	amountMatch := matches[len(matches)-2]
	amount = parseBCAAmount(amountMatch[1])
	isDebit = strings.TrimSpace(amountMatch[2]) == "DB"

	return amount, isDebit, balance, nil
}

func extractDescriptionBeforeAmount(s string, amount int64, isDebit bool) string {
	matches := reAmount.FindAllStringSubmatchIndex(s, -1)
	if len(matches) < 2 {
		return strings.TrimSpace(s)
	}

	// Text before the second-to-last amount match (the transaction amount)
	amountStart := matches[len(matches)-2][0]
	desc := strings.TrimSpace(s[:amountStart])
	// Trim any trailing short CBG code (e.g. "1111")
	parts := strings.Fields(desc)
	if len(parts) > 1 && isNumeric(parts[len(parts)-1]) {
		desc = strings.Join(parts[:len(parts)-1], " ")
	}
	return desc
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func parseBCAAmount(s string) int64 {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f)
}

func buildBCARefID(account, date string, amount int64, description string) string {
	h := sha1.New()
	io.WriteString(h, description)
	hash := fmt.Sprintf("%x", h.Sum(nil))
	if len(hash) > 8 {
		hash = hash[:8]
	}
	return fmt.Sprintf("bca-%s-%s-%d-%s", account, date, amount, hash)
}

func ParseMandiri(reader io.Reader, password string) ([]Transaction, string, string, error) {
	return nil, "", "", fmt.Errorf("mandiri parser not implemented yet")
}

func ParseJago(reader io.Reader) ([]Transaction, string, string, error) {
	return nil, "", "", fmt.Errorf("jago parser not implemented yet")
}
