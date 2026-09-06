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
