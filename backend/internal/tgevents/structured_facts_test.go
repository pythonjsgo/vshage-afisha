package tgevents

import (
	"encoding/json"
	"testing"
)

func TestStructuredFactsUseSourceData(t *testing.T) {
	row := kdRow("facts", "2026-09-18", "2026-09-18", "2026-09-18", strPtr("18:00"))
	row.Card.IsFree = new(bool)
	err := json.Unmarshal([]byte(`{"time_start":"18:00","time_end":"20:00","price":{"amount":2500,"currency":"RUB"},"org":{"url":"https://host.example/events"},"performers":[{"type":"Person","name":"Анна Иванова"}],"registration":{"valid_from":"2026-09-01T10:00:00+03:00"}}`), &row.Card.Payload)
	if err != nil {
		t.Fatal(err)
	}
	ev := toPublic(row, kdNow)
	if ev.EndDate == nil || *ev.EndDate != "2026-09-18T20:00:00+03:00" {
		t.Fatalf("end: %v", ev.EndDate)
	}
	if ev.PriceMin == nil || *ev.PriceMin != 2500 || ev.Currency == nil || *ev.Currency != "RUB" {
		t.Fatal("numeric source price lost")
	}
	if ev.OrganizerURL == nil || *ev.OrganizerURL != "https://host.example/events" {
		t.Fatal("host URL lost")
	}
	if len(ev.Performers) != 1 || ev.Performers[0].Name != "Анна Иванова" {
		t.Fatal("performer lost")
	}
	if ev.OffersValidFrom == nil || *ev.OffersValidFrom != "2026-09-01T10:00:00+03:00" {
		t.Fatal("sale opening lost")
	}
	if ev.EndTime != nil {
		t.Fatal("SEO precision must not change the legacy sorting window")
	}
}

func TestStructuredFactsDoNotInventMissingDetails(t *testing.T) {
	row := kdRow("unknown", "2026-09-18", "", "2026-09-18", strPtr("18:00"))
	row.Card.SourceURL = strPtr("https://aggregator.example/host-event")
	json.Unmarshal([]byte(`{"time_start":"18:00","time_end":"05:00","imported_at":"2026-09-01T10:00:00Z","price":{"raw":"от 300 до 500 рублей","amount":0,"currency":"RUB"},"org":{"name":"Host","channel":"publisher","url":"javascript:alert(1)"},"performers":[{"type":"Organization","name":"Publisher"}],"registration":{"deadline":"2026-09-15T00:00:00Z"}}`), &row.Card.Payload)
	ev := toPublic(row, kdNow)
	if ev.EndDate != nil || ev.PriceMin != nil || ev.OrganizerURL != nil || len(ev.Performers) > 0 || ev.OffersValidFrom != nil {
		t.Fatalf("invented facts: %+v", ev)
	}
}

func TestStructuredEndDatePrecision(t *testing.T) {
	for _, c := range []struct{ end, clock, want string }{
		{"2026-09-18", "", "2026-09-18"},
		{"2026-09-19", "01:00", "2026-09-19T01:00:00+03:00"},
		{"2026-09-18", "25:00", "2026-09-18"},
		{"9999-01-01", "20:00", ""},
	} {
		row := kdRow("end", "2026-09-18", c.end, "2026-09-18", strPtr("18:00"))
		row.Card.Payload = map[string]any{"time_start": "18:00", "time_end": c.clock}
		ev := toPublic(row, kdNow)
		got := ""
		if ev.EndDate != nil {
			got = *ev.EndDate
		}
		if got != c.want {
			t.Errorf("%+v: %q", c, got)
		}
	}
}

func TestWholePriceLiteralOnly(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want int
	}{
		{"600 рублей", 600}, {"2 500 ₽", 2500}, {"1500 руб.", 1500}, {"2500р", 2500},
		{"от 600 рублей", 600}, {"300–600 рублей", 0}, {"от 10 000 ₽", 10000}, {"от 0 до 300 рублей", 0}, {"1500 руб/час", 0}, {"0 рублей", 0},
		{"1500 рублей для членов клуба", 0}, {"600 долларов", 0}, {"600", 0},
	} {
		n := literalRublePrice(c.raw)
		got := 0
		if n != nil {
			got = *n
		}
		if got != c.want {
			t.Errorf("%q: got %d, want %d", c.raw, got, c.want)
		}
	}
}
