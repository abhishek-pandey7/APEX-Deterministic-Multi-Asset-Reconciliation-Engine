package ingest

import (
	"crypto/sha256"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/money"
)

// MaxReportedErrors caps how many row errors are kept; ErrorCount still counts them all.
const MaxReportedErrors = 100

// Files exported from Excel often start with a byte order mark glued to the first header.
var utf8BOM = string(rune(0xFEFF))

// RowError is a problem with one field of one row. Line is the 1-based line in the file.
type RowError struct {
	Line   int
	Column string
	Err    error
}

func (e RowError) Error() string { return fmt.Sprintf("line %d, %s: %v", e.Line, e.Column, e.Err) }

// File is the result of parsing one source file. A file with any row errors must not be
// loaded: partial loads would leave the reconciliation silently incomplete.
type File[T any] struct {
	SHA256     [32]byte
	Records    []T
	Errors     []RowError
	ErrorCount int
}

func (f *File[T]) OK() bool { return f.ErrorCount == 0 }

func (f *File[T]) fail(line int, column string, err error) {
	f.ErrorCount++
	if len(f.Errors) < MaxReportedErrors {
		f.Errors = append(f.Errors, RowError{line, column, err})
	}
}

// row gives named access to the current record.
type row struct {
	idx    map[string]int
	record []string
}

func (r row) get(column string) string {
	if column == "" {
		return ""
	}
	return strings.TrimSpace(r.record[r.idx[column]])
}

// scan reads a CSV with a header row, checks that every required column exists, and
// calls fn for each data row. The returned hash covers the file's exact bytes.
func scan(src io.Reader, required []string, fn func(line int, r row)) ([32]byte, error) {
	h := sha256.New()
	cr := csv.NewReader(io.TeeReader(src, h))
	cr.ReuseRecord = true

	header, err := cr.Read()
	if err != nil {
		return [32]byte{}, fmt.Errorf("ingest: reading header: %w", err)
	}
	idx := make(map[string]int, len(header))
	for i, name := range header {
		idx[strings.TrimSpace(strings.TrimPrefix(name, utf8BOM))] = i
	}
	var missing []string
	for _, c := range required {
		if _, ok := idx[c]; c != "" && !ok {
			missing = append(missing, c)
		}
	}
	if len(missing) > 0 {
		return [32]byte{}, fmt.Errorf("ingest: missing columns %q", missing)
	}

	for {
		record, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return [32]byte{}, fmt.Errorf("ingest: %w", err)
		}
		line, _ := cr.FieldPos(0)
		fn(line, row{idx, record})
	}
	// Drain anything the CSV reader didn't consume so the hash covers the whole file.
	if _, err := io.Copy(h, src); err != nil {
		return [32]byte{}, err
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

// ParseCash reads a cash file described by l.
func ParseCash(src io.Reader, l CashLayout) (*File[domain.CashEntry], error) {
	f := &File[domain.CashEntry]{}
	seen := make(map[string]int)
	required := []string{l.ID, l.Account, l.Currency, l.Amount, l.ValueDate,
		l.TradeDate, l.TxnType, l.Reference, l.Counterparty, l.DirectionColumn}

	sum, err := scan(src, required, func(line int, r row) {
		e := domain.CashEntry{
			Side:         l.Side,
			ExternalID:   r.get(l.ID),
			AccountID:    r.get(l.Account),
			TxnType:      r.get(l.TxnType),
			Reference:    r.get(l.Reference),
			Counterparty: r.get(l.Counterparty),
		}
		if e.TxnType == "" {
			e.TxnType = l.DefaultTxnType
		}
		bad := false
		check := func(column string, err error) {
			if err != nil {
				f.fail(line, column, err)
				bad = true
			}
		}

		if e.ExternalID == "" {
			check(l.ID, errors.New("empty id"))
		} else if first, dup := seen[e.ExternalID]; dup {
			check(l.ID, fmt.Errorf("duplicate id %q (first on line %d)", e.ExternalID, first))
		} else {
			seen[e.ExternalID] = line
		}
		if e.AccountID == "" {
			check(l.Account, errors.New("empty account"))
		}

		ccy, err := money.LookupCurrency(r.get(l.Currency))
		check(l.Currency, err)
		e.Currency = ccy.Code
		if err == nil {
			e.AmountMinor, err = parseAmount(r, l, ccy)
			check(l.Amount, err)
		}

		e.ValueDate, err = domain.ParseDate(l.DateFormat, r.get(l.ValueDate))
		check(l.ValueDate, err)
		if s := r.get(l.TradeDate); s != "" {
			e.TradeDate, err = domain.ParseDate(l.DateFormat, s)
			e.HasTradeDate = err == nil
			check(l.TradeDate, err)
		}

		if !bad {
			f.Records = append(f.Records, e)
		}
	})
	if err != nil {
		return nil, err
	}
	f.SHA256 = sum
	return f, nil
}

func parseAmount(r row, l CashLayout, ccy money.Currency) (int64, error) {
	raw := r.get(l.Amount)
	if l.Sign == Signed {
		return money.ParseFixed(raw, ccy.Exponent)
	}
	if strings.HasPrefix(raw, "-") || strings.HasPrefix(raw, "+") {
		return 0, fmt.Errorf("amount %q must be unsigned when a direction column is used", raw)
	}
	v, err := money.ParseFixed(raw, ccy.Exponent)
	if err != nil {
		return 0, err
	}
	switch flag := r.get(l.DirectionColumn); flag {
	case l.CreditFlag:
		return v, nil
	case l.DebitFlag:
		return -v, nil
	default:
		return 0, fmt.Errorf("direction %q is neither %q nor %q", flag, l.CreditFlag, l.DebitFlag)
	}
}

// ParsePositions reads a position file described by l.
func ParsePositions(src io.Reader, l PositionLayout) (*File[domain.Position], error) {
	f := &File[domain.Position]{}
	seen := make(map[[3]string]int)
	required := []string{l.Account, l.AsOf, l.Security, l.AssetClass, l.Currency,
		l.Quantity, l.Price, l.MarketValue}

	sum, err := scan(src, required, func(line int, r row) {
		p := domain.Position{
			Side:       l.Side,
			AccountID:  r.get(l.Account),
			SecurityID: r.get(l.Security),
			AssetClass: r.get(l.AssetClass),
		}
		if p.AssetClass == "" {
			p.AssetClass = l.DefaultAssetClass
		}
		bad := false
		check := func(column string, err error) {
			if err != nil {
				f.fail(line, column, err)
				bad = true
			}
		}

		if p.AccountID == "" {
			check(l.Account, errors.New("empty account"))
		}
		if p.SecurityID == "" {
			check(l.Security, errors.New("empty security id"))
		}
		var err error
		p.AsOf, err = domain.ParseDate(l.DateFormat, r.get(l.AsOf))
		check(l.AsOf, err)

		key := [3]string{p.AccountID, r.get(l.AsOf), p.SecurityID}
		if first, dup := seen[key]; dup {
			check(l.Security, fmt.Errorf("duplicate position (first on line %d)", first))
		} else {
			seen[key] = line
		}

		ccy, err := money.LookupCurrency(r.get(l.Currency))
		check(l.Currency, err)
		p.Currency = ccy.Code
		p.QuantityE6, err = money.ParseFixed(r.get(l.Quantity), money.QuantityScale)
		check(l.Quantity, err)
		p.PriceE8, err = money.ParseFixed(r.get(l.Price), money.PriceScale)
		check(l.Price, err)
		if ccy.Code != "" {
			p.MarketValueMinor, err = money.ParseFixed(r.get(l.MarketValue), ccy.Exponent)
			check(l.MarketValue, err)
		}

		if !bad {
			f.Records = append(f.Records, p)
		}
	})
	if err != nil {
		return nil, err
	}
	f.SHA256 = sum
	return f, nil
}
