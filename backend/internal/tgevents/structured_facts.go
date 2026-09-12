package tgevents

import (
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/pythonjsgo/vshage-afisha/internal/events"
)

// Project explicit facts from the existing collector payload. Never use
// source_url as the host's URL: the source may be a third-party aggregator.
// Likewise imported_at/extracted_at are not ticket sale opening dates.
func applyStructuredFacts(ev *events.PublicEvent, c Card) {
	p := c.Payload
	obj := func(key string) map[string]any { v, _ := p[key].(map[string]any); return v }
	text := func(m map[string]any, key string) string { v, _ := m[key].(string); return strings.TrimSpace(v) }
	if u := text(obj("org"), "url"); validHTTPURL(u) {
		ev.OrganizerURL = &u
	}
	price := obj("price")
	currency := strings.ToUpper(text(price, "currency"))
	validCurrency := len(currency) == 3
	for _, r := range currency {
		validCurrency = validCurrency && r >= 'A' && r <= 'Z'
	}
	amount, ok := price["amount"].(float64)
	// Numeric extraction only, no parsing of "from ...", membership fees,
	// child/adult ranges or other human price wording. Existing price API is
	// integer-valued, so fractional values are omitted instead of rounded.
	if ok && validCurrency && !math.IsNaN(amount) && !math.IsInf(amount, 0) && amount >= 0 && amount <= math.MaxInt32 && math.Trunc(amount) == amount && (amount > 0 || c.IsFree != nil && *c.IsFree) {
		n := int(amount)
		ev.PriceMin = &n
		ev.Currency = &currency
	}
	// A whole price literal or explicit minimum ("от 600 рублей") is a
	// known price_min. Ranges, subscriptions, discounts and prose do not match.
	if ev.PriceMin == nil && c.PriceRaw != nil && (currency == "" || currency == "RUB") && (c.IsFree == nil || !*c.IsFree) {
		if n := literalRublePrice(*c.PriceRaw); n != nil {
			ev.PriceMin = n
			ev.Currency = strPtr("RUB")
		}
	}
	if from := text(obj("registration"), "valid_from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil && !t.After(ev.StartTime) {
			ev.OffersValidFrom = &from
		}
	}
	if performers, ok := p["performers"].([]any); ok {
		for _, entry := range performers {
			m, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			kind, name := text(m, "type"), text(m, "name")
			if (kind == "Person" || kind == "PerformingGroup") && name != "" && len([]rune(name)) <= 200 {
				ev.Performers = append(ev.Performers, events.Performer{Type: kind, Name: name})
			}
		}
	}
	if ev.OpenEnded {
		return
	}
	day := c.Date
	if c.DateEnd != nil && *c.DateEnd != "" {
		if _, err := time.Parse(dateLayout, *c.DateEnd); err != nil || *c.DateEnd < c.Date || *c.DateEnd >= openEndedFrom {
			return
		}
		day = *c.DateEnd
		ev.EndDate = &day
	}
	hhmm := text(p, "time_end")
	if hhmm == "" {
		hhmm = text(p, "end_time")
	}
	if _, err := time.Parse("15:04", hhmm); err != nil {
		return
	}
	end, err := time.ParseInLocation(dateLayout+" 15:04", day+" "+hhmm, msk)
	if err != nil {
		return
	}
	startTime := text(p, "time_start")
	start := parseMSK(c.Date, strPtr(startTime))
	// An earlier clock time without an explicit next-day end date is
	// ambiguous. Do not invent overnight duration from a missing date.
	if end.Before(start) {
		ev.EndDate = nil
		return
	}
	value := end.Format(time.RFC3339)
	ev.EndDate = &value
}

func validHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil
}

var rublePriceLiteral = regexp.MustCompile(`(?i)^(?:от[ \x{00a0}\x{202f}]+)?([1-9][0-9]*(?:[ \x{00a0}\x{202f}][0-9]{3})*)[ \x{00a0}\x{202f}]*(?:₽|руб\.?|рублей|рубля|р\.?)$`)

func literalRublePrice(raw string) *int {
	match := rublePriceLiteral.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return nil
	}
	digits := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, match[1])
	n, err := strconv.Atoi(digits)
	if err != nil || n > math.MaxInt32 {
		return nil
	}
	return &n
}
