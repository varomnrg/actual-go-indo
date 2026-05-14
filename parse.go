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

	"github.com/xuri/excelize/v2"
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

var reMandiriTime = regexp.MustCompile(`\d{2}:\d{2}:\d{2}\s+WIB`)

var reJagoDate    = regexp.MustCompile(`^(\d{1,2})\s+(\w+)\s+(\d{4})\s{2,}`)
var reJagoAmounts = regexp.MustCompile(`([\+\-][\d\.,]+)\s+([\d\.,]+)\s*$`)
var reJagoTime    = regexp.MustCompile(`^(\d{2}:\d{2})\b`)
var reJagoID      = regexp.MustCompile(`\bID#\s*(\S+)`)
var reJagoPage    = regexp.MustCompile(`(?i)page\s+\d+\s+of\s+\d+`)
var reMonthHeader = regexp.MustCompile(`^\w+\s+\d{4}$`)

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

// ── Mandiri ──────────────────────────────────────────────────────────────────

func ParseMandiri(filePath, password string) ([]Transaction, string, string, error) {
	f, err := excelize.OpenFile(filePath, excelize.Options{Password: password})
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to open XLSX (check password): %w", err)
	}
	defer f.Close()

	rows, err := f.GetRows("e-Statement")
	if err != nil {
		return nil, "", "", fmt.Errorf("sheet 'e-Statement' not found: %w", err)
	}
	return parseMandiriRows(rows)
}

func parseMandiriRows(rows [][]string) ([]Transaction, string, string, error) {
	account := extractMandiriAccount(rows)
	period := extractMandiriPeriod(rows)

	headerIdx, colNo, colDate := findMandiriHeader(rows)
	if headerIdx < 0 {
		return nil, account, period, fmt.Errorf("e-Statement header row not found")
	}

	colRemarks := colDate + 3
	colCredit := colDate + 11
	colDebit := colDate + 14
	colBalance := colDate + 17

	var txs []Transaction
	var pending *Transaction

	for _, row := range rows[headerIdx+1:] {
		noVal := getCell(row, colNo)
		dateVal := getCell(row, colDate)

		if noVal != "" && isNumeric(noVal) {
			if pending != nil {
				txs = append(txs, *pending)
			}
			date, err := parseMandiriDate(dateVal)
			if err != nil {
				pending = nil
				continue
			}
			remarks := strings.TrimSpace(strings.ReplaceAll(getCell(row, colRemarks), "\n", " "))
			creditStr := getCell(row, colCredit)
			debitStr := getCell(row, colDebit)
			balanceStr := getCell(row, colBalance)

			var amount int64
			if creditStr != "" {
				amount = parseIDRAmount(creditStr)
			} else if debitStr != "" {
				amount = -parseIDRAmount(debitStr)
			}
			balance := parseIDRAmount(balanceStr)
			absAmount := int64(math.Abs(float64(amount)))

			pending = &Transaction{
				Date:        date,
				Description: remarks,
				Amount:      amount,
				Balance:     balance,
				ReferenceID: buildMandiriRefID(account, date, absAmount, remarks),
				Status:      "pending",
			}
		} else if noVal == "" && pending != nil && reMandiriTime.MatchString(dateVal) {
			parts := strings.Fields(dateVal)
			if len(parts) > 0 {
				pending.Time = parts[0] // "HH:MM:SS"
			}
		}
	}
	if pending != nil {
		txs = append(txs, *pending)
	}
	return txs, account, period, nil
}

func findMandiriHeader(rows [][]string) (headerIdx, colNo, colDate int) {
	headerIdx, colNo, colDate = -1, -1, -1
	for i, row := range rows {
		for j, cell := range row {
			if strings.TrimSpace(cell) == "Tanggal" {
				headerIdx = i
				colDate = j
				break
			}
		}
		if headerIdx >= 0 {
			break
		}
	}
	if headerIdx < 0 {
		return
	}
	for j, cell := range rows[headerIdx] {
		if strings.TrimSpace(cell) == "No" {
			colNo = j
			break
		}
	}
	if colNo < 0 {
		colNo = colDate - 3
		if colNo < 0 {
			colNo = 0
		}
	}
	return
}

func extractMandiriAccount(rows [][]string) string {
	limit := min(30, len(rows))
	for _, row := range rows[:limit] {
		for j, cell := range row {
			if strings.Contains(strings.ToLower(cell), "rekening") {
				for k := j + 1; k < len(row); k++ {
					v := strings.TrimSpace(row[k])
					if v == "" {
						continue
					}
					digits := extractDigits(v)
					if len(digits) >= 6 {
						return digits
					}
					if v != cell {
						return v
					}
				}
			}
		}
	}
	return "unknown"
}

func extractMandiriPeriod(rows [][]string) string {
	limit := min(30, len(rows))
	for _, row := range rows[:limit] {
		for j, cell := range row {
			lower := strings.ToLower(strings.TrimSpace(cell))
			if lower == "periode" || lower == "period" {
				for k := j + 1; k < len(row); k++ {
					if v := strings.TrimSpace(row[k]); v != "" {
						return v
					}
				}
			}
		}
	}
	return ""
}

func parseMandiriDate(s string) (string, error) {
	parts := strings.Fields(strings.TrimSpace(s))
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid date: %q", s)
	}
	day := parts[0]
	if len(day) == 1 {
		day = "0" + day
	}
	month, ok := monthNumbers[strings.ToUpper(parts[1])]
	if !ok {
		return "", fmt.Errorf("unknown month: %q", parts[1])
	}
	return parts[2] + "-" + month + "-" + day, nil
}

func parseIDRAmount(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f)
}

func buildMandiriRefID(account, date string, amount int64, remarks string) string {
	h := sha1.New()
	io.WriteString(h, remarks)
	hash := fmt.Sprintf("%x", h.Sum(nil))[:8]
	return fmt.Sprintf("mandiri-%s-%s-%d-%s", account, date, amount, hash)
}

func getCell(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func extractDigits(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// ── Jago ─────────────────────────────────────────────────────────────────────

func ParseJago(reader io.Reader) ([]Transaction, string, string, error) {
	cmd := exec.Command("pdftotext", "-layout", "-", "-")
	cmd.Stdin = reader
	output, err := cmd.Output()
	if err != nil {
		return nil, "", "", fmt.Errorf("pdftotext: %w", err)
	}
	return parseJagoText(strings.Split(string(output), "\n"))
}

func parseJagoText(lines []string) ([]Transaction, string, string, error) {
	pocket, period := extractJagoHeader(lines)

	var txs []Transaction
	var cur *Transaction
	var descParts []string

	flush := func() {
		if cur == nil {
			return
		}
		cur.Description = strings.TrimSpace(strings.Join(descParts, " "))
		if cur.ReferenceID == "" {
			h := sha1.New()
			io.WriteString(h, cur.Description)
			hash := fmt.Sprintf("%x", h.Sum(nil))[:8]
			cur.ReferenceID = fmt.Sprintf("jago-%s-%d-%s",
				cur.Date, int64(math.Abs(float64(cur.Amount))), hash)
		}
		txs = append(txs, *cur)
		cur = nil
		descParts = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || isJagoSkipLine(trimmed) {
			continue
		}

		if m := reJagoDate.FindStringSubmatchIndex(line); m != nil {
			flush()
			day := line[m[2]:m[3]]
			if len(day) == 1 {
				day = "0" + day
			}
			month, ok := monthNumbers[strings.ToUpper(line[m[4]:m[5]])]
			if !ok {
				continue
			}
			date := line[m[6]:m[7]] + "-" + month + "-" + day
			rest := strings.TrimSpace(line[m[1]:])

			var amount, balance int64
			if am := reJagoAmounts.FindStringSubmatchIndex(rest); am != nil {
				amount = parseJagoAmount(rest[am[2]:am[3]])
				balance = parseIDRAmount(rest[am[4]:am[5]])
				rest = strings.TrimSpace(rest[:am[0]])
			}

			cur = &Transaction{Date: date, Amount: amount, Balance: balance, Status: "pending"}
			descParts = nil
			if rest != "" {
				descParts = append(descParts, rest)
			}
			continue
		}

		if cur == nil {
			continue
		}

		if cur.Time == "" {
			if tm := reJagoTime.FindStringSubmatch(trimmed); tm != nil {
				cur.Time = tm[1]
				if rest := strings.TrimSpace(trimmed[len(tm[0]):]); rest != "" {
					descParts = append(descParts, rest)
				}
				continue
			}
		}

		if idm := reJagoID.FindStringSubmatch(trimmed); idm != nil {
			cur.ReferenceID = "jago-" + idm[1]
			continue
		}

		descParts = append(descParts, trimmed)
	}
	flush()

	for i := range txs {
		if strings.Contains(txs[i].Description, "Movement between Pockets") {
			txs[i].TransferToAccount = extractJagoTransferDest(txs[i].Description)
		}
	}

	return txs, pocket, period, nil
}

func isJagoSkipLine(trimmed string) bool {
	return strings.Contains(trimmed, "PT Bank Jago") ||
		strings.Contains(trimmed, "Pockets Transactions History") ||
		strings.Contains(trimmed, "www.jago.com") ||
		reJagoPage.MatchString(trimmed) ||
		reMonthHeader.MatchString(trimmed)
}

func extractJagoHeader(lines []string) (pocket, period string) {
	seenBank := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "PT Bank Jago") {
			seenBank = true
			continue
		}
		if strings.Contains(trimmed, "Pockets Transactions History") || reJagoDate.MatchString(line) {
			break
		}
		if !seenBank {
			continue
		}
		if strings.Contains(trimmed, "www.jago.com") || reJagoPage.MatchString(trimmed) {
			continue
		}
		if reMonthHeader.MatchString(trimmed) {
			if period == "" {
				period = trimmed
			}
			continue
		}
		// Period line: "01 Oct 2022 - 31 Oct 2022" or similar
		if pocket != "" && strings.Contains(trimmed, " - ") && len(trimmed) > 15 {
			period = trimmed
			continue
		}
		if pocket == "" {
			pocket = trimmed
		}
	}
	return
}

func parseJagoAmount(s string) int64 {
	s = strings.TrimSpace(s)
	sign := int64(1)
	if strings.HasPrefix(s, "-") {
		sign = -1
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	return sign * parseIDRAmount(s)
}

func extractJagoTransferDest(description string) string {
	for _, sep := range []string{" → ", " -> ", " ke ", " to "} {
		if idx := strings.Index(description, sep); idx >= 0 {
			dest := strings.TrimSpace(description[idx+len(sep):])
			if spaceIdx := strings.Index(dest, "  "); spaceIdx >= 0 {
				dest = strings.TrimSpace(dest[:spaceIdx])
			}
			if dest != "" {
				return dest
			}
		}
	}
	return ""
}
