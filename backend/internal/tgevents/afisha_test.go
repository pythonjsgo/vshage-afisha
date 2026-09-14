package tgevents

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pythonjsgo/vshage-afisha/internal/events"
)

// kdNow — понедельник 07.09.2026, полдень МСК. Момент фиксируется руками:
// полоса считается по календарному дню МСК, и прогон в 23:30 иначе давал бы
// другой ответ, чем в 09:00.
var kdNow = time.Date(2026, 9, 7, 12, 0, 0, 0, msk)

// kdRow собирает строку витрины так, как её отдаёт selectCard: eff — уже
// посчитанные в SQL дата и время показа, Card — сырые date/date_end.
func kdRow(id, date, dateEnd, effDate string, effTime *string) cardRow {
	c := Card{ID: id, Title: "Событие", Annonce: "Наш текст.", Date: date, AccessLevel: "open"}
	if dateEnd != "" {
		c.DateEnd = &dateEnd
	}
	return cardRow{Card: c, Eff: eff{Date: effDate, Time: effTime}}
}

// Сентинел «бессрочно» приезжает из конвейера (vshage-geo/collect) и означает
// «конца в анонсе не было». Отдать его как дату значит нарисовать на карточке
// «до 1 ЯНВ» — утверждение о чужом событии, которого никто не делал.
//
// Замер 07.09: три такие карточки на DEV и три на PROD. Чинить источник —
// отдельный заход в другом репозитории; здесь сентинел ПЕРЕВОДИТСЯ в признак.
func TestБессрочноеБезEndTime(t *testing.T) {
	ev := toPublic(kdRow("ev_forever", "2026-03-01", "9999-01-01", "2026-09-07", nil), kdNow)

	if !ev.OpenEnded {
		t.Error("карточка с date_end=9999-01-01 не помечена open_ended")
	}
	if ev.EndTime != nil {
		t.Errorf("у бессрочной карточки есть end_time (%v) — на экране будет «до 1 ЯНВ»", ev.EndTime)
	}
	if ev.Kind != events.KindRunning {
		t.Errorf("бессрочная программа, открывшаяся в марте, попала в полосу %q", ev.Kind)
	}
	if !ev.Multiday {
		t.Error("бессрочная программа не помечена multiday")
	}

	// Отсутствие поля в JSON — это и есть то, что читают все потребители,
	// включая разметку schema.org. Нулевое время или строка «9999-01-01»
	// прочитались бы как настоящий конец.
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["end_time"]; ok {
		t.Errorf("end_time попал в ответ у бессрочной карточки: %s", got["end_time"])
	}
	if string(got["open_ended"]) != "true" {
		t.Errorf("open_ended = %s", got["open_ended"])
	}

	// Контроль на заведомо конечной программе: если бы end_time пропадал у
	// всех, тест выше прошёл бы при полностью сломанном маппере.
	normal := toPublic(kdRow("ev_expo", "2026-08-01", "2026-09-30", "2026-09-07", nil), kdNow)
	if normal.EndTime == nil {
		t.Fatal("у обычной многодневной карточки пропал end_time — сломан маппер, а не сентинел")
	}
	if normal.OpenEnded {
		t.Error("обычная многодневная карточка помечена open_ended")
	}
	if normal.Kind != events.KindRunning {
		t.Errorf("выставка, идущая с августа, попала в полосу %q", normal.Kind)
	}
}

// Полоса решается по НАСТОЯЩЕМУ началу. У идущей программы eff_date сдвинут на
// сегодня ради сортировки, и наивное сравнение сказало бы «начинается сегодня,
// значит не идёт» — то есть все идущие выставки (88 из 260 на DEV, замер
// 07.09) уехали бы в полосу времени и встали над сегодняшним концертом.
func TestПолосаTgБерётНастоящееНачало(t *testing.T) {
	at19 := "19:00"
	cases := []struct {
		name     string
		row      cardRow
		kind     string
		multiday bool
	}{
		{"выставка идёт с августа",
			kdRow("ev_expo", "2026-08-01", "2026-09-30", "2026-09-07", nil),
			events.KindRunning, true},
		{"концерт сегодня вечером",
			kdRow("ev_gig", "2026-09-07", "", "2026-09-07", &at19),
			events.KindTimed, false},
		{"программа открывается завтра",
			kdRow("ev_soon", "2026-09-08", "2026-09-20", "2026-09-08", nil),
			events.KindTimed, true},
		// Кончилась вчера: доска держит вчерашнее ещё сутки, сдвига нет
		// (selectCard сдвигает только то, что ещё идёт), полоса временная.
		{"кончилась вчера",
			kdRow("ev_over", "2026-09-01", "2026-09-06", "2026-09-01", nil),
			events.KindTimed, true},
		// Последний день сегодня: программа всё ещё идёт.
		{"последний день сегодня",
			kdRow("ev_last", "2026-09-01", "2026-09-07", "2026-09-07", nil),
			events.KindRunning, true},
	}

	for _, c := range cases {
		ev := toPublic(c.row, kdNow)
		if ev.Kind != c.kind {
			t.Errorf("%s: полоса %q, ожидали %q", c.name, ev.Kind, c.kind)
		}
		if ev.Multiday != c.multiday {
			t.Errorf("%s: multiday=%v, ожидали %v", c.name, ev.Multiday, c.multiday)
		}
	}
}

// Порядок в полосе «идёт сейчас» — по концу, всё остальное — по началу.
// Разойдись он с ключом слияния (events.MergePage), окно [offset, offset+limit)
// перестало бы быть честным: строка, стоящая у источника за окном, попала бы в
// него по другому ключу, и страница потеряла бы карточки, выглядя полной.
func TestOrderCardПереключаетКлюч(t *testing.T) {
	byStart, byEnd := orderCard(false), orderCard(true)
	if byStart == byEnd {
		t.Fatal("orderCard отдаёт один текст на обе полосы — порядок не переключается")
	}
	if want := `) t ORDER BY COALESCE(de, d), id`; byEnd != want {
		t.Errorf("порядок полосы «идёт сейчас» = %q, ожидали %q", byEnd, want)
	}
	if want := `) t ORDER BY eff_date, COALESCE(eff_time, '00:00'), id`; byStart != want {
		t.Errorf("порядок по началу = %q, ожидали %q", byStart, want)
	}
}

// The public website used to lose both paid status and the source price,
// making a paid event indistinguishable from a registration-only listing.
//
// Число берётся ТОЛЬКО из явного литерала. «от 10 000 ₽» — это объявленный
// минимум (015ac94: Google читает offers.price как нижнюю доступную цену),
// а диапазон минимума не объявляет и числом не становится. Проверяются обе
// стороны, потому что до 13.09 фикстура «от 10 000 ₽» стояла здесь в роли
// прозы — тест противоречил правилу из structured_facts.go, и CI бэкенда
// был красным двое суток, блокируя выкатку соседних фич.
func TestExternalPriceSurvivesPublicMapping(t *testing.T) {
	for _, tc := range []struct {
		name    string
		price   string
		free    *bool
		want    string
		wantMin *int
	}{
		{name: "paid", price: "от 10 000 ₽", free: boolPtr(false), want: "paid", wantMin: intPtr(10000)},
		{name: "free", price: "от 10 000 ₽", free: boolPtr(true), want: "free"},
		{name: "unknown", price: "от 10 000 ₽", want: "", wantMin: intPtr(10000)},
		// Диапазон нижней цены не объявляет — угадывать её нечем.
		{name: "range", price: "300–600 рублей", free: boolPtr(false), want: "paid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := kdRow("ev_price", "2026-09-22", "", "2026-09-22", nil)
			price := tc.price
			row.Card.PriceRaw, row.Card.IsFree = &price, tc.free
			ev := toPublic(row, kdNow)
			if ev.PriceText == nil || *ev.PriceText != price {
				t.Fatal("source price was lost")
			}
			if tc.want == "" {
				if ev.PriceType != nil {
					t.Fatal("unknown price was guessed")
				}
			} else if ev.PriceType == nil || *ev.PriceType != tc.want {
				t.Fatalf("price type = %v", ev.PriceType)
			}
			switch {
			case tc.wantMin == nil && ev.PriceMin != nil:
				t.Fatalf("цена додумана из прозы: price_min = %d", *ev.PriceMin)
			case tc.wantMin != nil && (ev.PriceMin == nil || *ev.PriceMin != *tc.wantMin):
				t.Fatalf("price_min = %v, ждали %d", ev.PriceMin, *tc.wantMin)
			}
			if ev.PriceMax != nil {
				t.Fatal("верхняя цена нигде не объявлена — её нельзя угадывать")
			}
		})
	}
}

func boolPtr(value bool) *bool { return &value }
func intPtr(value int) *int    { return &value }

func TestCoverReplacementChangesPublicURL(t *testing.T) {
	row := kdRow("ev_cover", "2026-09-22", "", "2026-09-22", nil)
	row.HasCover = true
	row.CoverRevision = "20260911090000000000"
	before := toPublic(row, kdNow)
	row.CoverRevision = "20260911100000000000"
	after := toPublic(row, kdNow)
	if before.PhotoURL == nil || after.PhotoURL == nil || *before.PhotoURL == *after.PhotoURL {
		t.Fatal("updated cover retained the stale cached URL")
	}
	row.HasCover = false
	if toPublic(row, kdNow).PhotoURL != nil {
		t.Fatal("revision invented a missing image")
	}
}
