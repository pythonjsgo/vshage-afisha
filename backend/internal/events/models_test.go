package events

import (
	"encoding/json"
	"testing"
	"time"
)

// kdNow — момент, относительно которого разбирается вся доска в тестах полосы:
// понедельник 07.09.2026, полдень МСК. Фиксируется руками, потому что «идёт
// сейчас» считается по календарному дню МСК, и прогон в 23:30 иначе давал бы
// другой ответ, чем в 09:00, — флака, которую читают как регрессию.
var kdNow = time.Date(2026, 9, 7, 12, 0, 0, 0, mskZone)

// kdEvent собирает карточку из дат в МСК. `start` и `end` — «YYYY-MM-DD HH:MM»
// либо пусто; `actual` — настоящий первый день у карточки со сдвинутым стартом.
func kdEvent(id, start, end, actual string, openEnded bool) PublicEvent {
	ev := PublicEvent{ID: id, OpenEnded: openEnded}
	ev.StartTime = kdTime(start)
	if end != "" {
		t := kdTime(end)
		ev.EndTime = &t
	}
	if actual != "" {
		a := actual
		ev.ActualStartDate = &a
	}
	return ev
}

func kdTime(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, mskZone)
	if err != nil {
		panic("kdTime(" + s + "): " + err.Error())
	}
	return t
}

// Разбиение обязано быть ПОЛНЫМ и НЕПЕРЕСЕКАЮЩИМСЯ: каждое событие доски
// попадает ровно в одну полосу. Если бы полосы задавались двумя независимо
// написанными условиями, карточка, не подошедшая ни под одно, не показалась бы
// НИГДЕ — и заметить это можно было бы только пересчитав доску руками.
func TestClassifyПолосаОднаНаСобытие(t *testing.T) {
	cases := []struct {
		name     string
		ev       PublicEvent
		kind     string
		multiday bool
	}{
		{"концерт сегодня вечером",
			kdEvent("a", "2026-09-07 19:00", "2026-09-07 22:00", "", false), KindTimed, false},
		{"концерт без указанного конца",
			kdEvent("b", "2026-09-07 19:00", "", "", false), KindTimed, false},
		// Ночная вечеринка пересекает полночь, но многодневной НЕ является:
		// девять часов — это одна посадка, и подпись обязана остаться
		// «в 21:00», а не стать периодом «7 — 8 СЕН» (правка 07.09, порог
		// multidayMinSpan).
		{"ночная вечеринка до утра",
			kdEvent("c", "2026-09-07 21:00", "2026-09-08 06:00", "", false), KindTimed, false},
		{"выставка идёт с августа",
			kdEvent("d", "2026-08-01 00:00", "2026-09-30 23:59", "", false), KindRunning, true},
		// Программа, открывающаяся СЕГОДНЯ, — ещё «по дате и времени»: у неё
		// есть осмысленный ответ на «когда». Идущей она станет завтра сама.
		{"неделя открывается сегодня",
			kdEvent("e", "2026-09-07 10:00", "2026-09-13 20:00", "", false), KindTimed, true},
		{"неделя открывается завтра",
			kdEvent("f", "2026-09-08 10:00", "2026-09-13 20:00", "", false), KindTimed, true},
		// Конец пришёлся на вчера: программа кончилась, полоса «идёт» её не
		// берёт, хотя началась она раньше сегодняшнего дня.
		{"кончилась вчера",
			kdEvent("g", "2026-09-01 10:00", "2026-09-06 20:00", "", false), KindTimed, true},
		// Конец сегодня — последний день, программа всё ещё идёт.
		{"последний день сегодня",
			kdEvent("h", "2026-09-01 10:00", "2026-09-07 20:00", "", false), KindRunning, true},
		{"бессрочная, открылась в марте",
			kdEvent("i", "2026-03-01 00:00", "", "", true), KindRunning, true},
		{"бессрочная, открывается завтра",
			kdEvent("j", "2026-09-08 00:00", "", "", true), KindTimed, true},
	}

	for _, c := range cases {
		ev := c.ev
		ev.Classify(kdNow)
		if ev.Kind != c.kind {
			t.Errorf("%s: полоса %q, ожидали %q", c.name, ev.Kind, c.kind)
		}
		if ev.Multiday != c.multiday {
			t.Errorf("%s: multiday=%v, ожидали %v", c.name, ev.Multiday, c.multiday)
		}
		if ev.Kind != KindRunning && ev.Kind != KindTimed {
			t.Errorf("%s: полоса %q вне словаря — карточка не попадёт ни в одну", c.name, ev.Kind)
		}
	}
}

// САМОЕ ВАЖНОЕ МЕСТО КЛАССИФИКАТОРА. У tgevents StartTime идущей программы
// СДВИНУТ на сегодня ради сортировки (см. tgevents.selectCard): выставка,
// открывшаяся в июне, приезжает со стартом «сегодня». Наивное сравнение
// сказало бы про неё «начинается сегодня, значит не идёт» — то есть все 88
// идущих программ (замер 07.09 на DEV) уехали бы в полосу времени, и полоса
// «идёт сейчас» осталась бы пустой при полной доске выставок.
func TestClassifyБерётНастоящееНачалоАНеСдвинутое(t *testing.T) {
	shifted := kdEvent("expo", "2026-09-07 00:00", "2026-09-30 23:59", "2026-08-01", false)
	shifted.Classify(kdNow)
	if shifted.Kind != KindRunning {
		t.Errorf("выставка со сдвинутым стартом попала в %q — сдвиг сортировки прочитан как настоящая дата", shifted.Kind)
	}
	if !shifted.Multiday {
		t.Error("выставка со сдвинутым стартом не считается многодневной")
	}

	// Обратная сторона: карточка БЕЗ сдвига (ActualStartDate нет) со стартом
	// сегодня — это событие сегодня, и полоса у него временная.
	plain := kdEvent("gig", "2026-09-07 19:00", "", "", false)
	plain.Classify(kdNow)
	if plain.Kind != KindTimed {
		t.Errorf("сегодняшний концерт попал в %q", plain.Kind)
	}

	// Мусор в ActualStartDate не имеет права переклассифицировать карточку:
	// поле приезжает строкой из чужого конвейера, и разбор его может не
	// удаться. Тогда правдой остаётся StartTime.
	broken := kdEvent("junk", "2026-09-07 00:00", "2026-09-30 23:59", "не дата", false)
	broken.Classify(kdNow)
	if broken.Kind != KindTimed {
		t.Errorf("неразобранная actual_start_date изменила полосу на %q", broken.Kind)
	}
}

// Полночь МСК, а не UTC: контейнер живёт в UTC, и с полуночи до трёх ночи по
// Москве «сегодня» там вчерашнее. Выставка, кончившаяся вчера, в этот час
// считалась бы идущей — и стояла бы в полосе «идёт сейчас» до трёх утра.
func TestClassifyСчитаетДеньПоМоскве(t *testing.T) {
	night := time.Date(2026, 9, 6, 22, 30, 0, 0, time.UTC) // 07.09 01:30 МСК
	ev := kdEvent("expo", "2026-09-01 10:00", "2026-09-06 20:00", "", false)
	ev.Classify(night)
	if ev.Kind != KindTimed {
		t.Errorf("в 01:30 МСК вчерашняя программа считается %q — день взят по UTC", ev.Kind)
	}
	still := kdEvent("expo2", "2026-09-01 10:00", "2026-09-07 20:00", "", false)
	still.Classify(night)
	if still.Kind != KindRunning {
		t.Errorf("в 01:30 МСК сегодняшняя программа считается %q", still.Kind)
	}
}

// Форма карточки — контракт с фронтом. `kind` присутствует ВСЕГДА (по нему
// страница выбирает подпись даты), `multiday` и `open_ended` — только когда
// они истинны: писать false в каждую из четырёхсот карточек незачем.
//
// Существующие поля обязаны остаться на месте с прежними именами: по доске
// ходят старые страницы, и переименование поля компилируется, деплоится и
// молча оставляет карточку без даты (`.claude/rules/compatibility.md` §API).
func TestКарточкаНеслаБыНовыеПоляНеТеряяСтарых(t *testing.T) {
	ev := kdEvent("expo", "2026-09-07 00:00", "2026-09-30 23:59", "2026-08-01", false)
	ev.Title = "Выставка"
	ev.Tags = json.RawMessage("[]")
	ev.Classify(kdNow)

	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "title", "start_time", "end_time", "status", "tags",
		"actual_start_date", "kind", "multiday"} {
		if _, ok := got[key]; !ok {
			t.Errorf("в карточке нет поля %q", key)
		}
	}
	if string(got["kind"]) != `"running"` {
		t.Errorf("kind = %s", got["kind"])
	}
	if _, ok := got["open_ended"]; ok {
		t.Error("open_ended попал в обычную карточку — фронт прочитает «конца нет» у выставки с датой конца")
	}

	// Точечное событие не несёт ни одного из двух признаков, но `kind` несёт.
	plain := kdEvent("gig", "2026-09-07 19:00", "2026-09-07 22:00", "", false)
	plain.Tags = json.RawMessage("[]")
	plain.Classify(kdNow)
	raw, err = json.Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	got = map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["multiday"]; ok {
		t.Error("multiday=false попал в карточку — лишний байт в каждой из четырёхсот")
	}
	if string(got["kind"]) != `"timed"` {
		t.Errorf("kind точечного события = %s", got["kind"])
	}
}

// «Конец пришёлся на следующий день» и «событие многодневное» — разные
// утверждения, и путает их ровно ночная посадка. Вечеринка 21:00→06:00
// пересекает полночь; подпись у неё обязана остаться «в 21:00», иначе на
// карточке появится период «7 — 8 СЕН» — про клубный вечер это неправда.
//
// Контроль обязателен и стоит рядом: настоящая двухдневная программа обязана
// остаться многодневной. Без него тест проходил бы и при правиле, которое
// всегда отвечает «нет», — то есть при полностью убитом признаке.
func TestНочнаяПосадкаНеМногодневная(t *testing.T) {
	for _, c := range []struct {
		name     string
		ev       PublicEvent
		multiday bool
	}{
		{"вечеринка 21:00 → 06:00", kdEvent("party", "2026-09-07 21:00", "2026-09-08 06:00", "", false), false},
		{"клубный сет 23:00 → 05:00", kdEvent("club", "2026-09-07 23:00", "2026-09-08 05:00", "", false), false},
		// КОНТРОЛЬ: маркет с полудня до конца следующего дня — настоящая
		// двухдневная программа, признак обязан стоять.
		{"маркет 12:00 → 23:59 назавтра", kdEvent("market", "2026-09-07 12:00", "2026-09-08 23:59", "", false), true},
		// КОНТРОЛЬ: импортированная двухдневная карточка в худшем для порога
		// виде — старт в 23:00, конец синтезирован как 23:59 последнего дня,
		// то есть 24:59. Ради неё порог и не «сутки».
		{"импорт 23:00 → 23:59 назавтра", kdEvent("tg2d", "2026-09-07 23:00", "2026-09-08 23:59", "", false), true},
	} {
		ev := c.ev
		ev.Classify(kdNow)
		if ev.Multiday != c.multiday {
			t.Errorf("%s: multiday=%v, ожидали %v", c.name, ev.Multiday, c.multiday)
		}
		// Полосы это не касается вовсе: всё перечисленное начинается сегодня и
		// отвечает на вопрос «когда».
		if ev.Kind != KindTimed {
			t.Errorf("%s: полоса %q, ожидали %q", c.name, ev.Kind, KindTimed)
		}
	}
}

// Порог именно там, где написан, и проверяется С ОБЕИХ сторон. Число взято из
// формы данных (см. multidayMinSpan), а такие числа двигают наугад чаще всего:
// граница без второй половины пропустила бы сдвиг «чуть-чуть» молча, и
// «7 — 8 СЕН» вернулось бы на карточку клубного вечера.
func TestПорогМногодневностиРовноТам(t *testing.T) {
	start := kdTime("2026-09-07 21:00")
	span := func(d time.Duration) PublicEvent {
		end := start.Add(d)
		ev := PublicEvent{ID: "x", StartTime: start, EndTime: &end}
		ev.Classify(kdNow)
		return ev
	}
	if ev := span(multidayMinSpan - time.Minute); ev.Multiday {
		t.Errorf("на минуту короче порога (%v) событие уже многодневное", multidayMinSpan-time.Minute)
	}
	if ev := span(multidayMinSpan); !ev.Multiday {
		t.Errorf("ровно на пороге (%v) событие ещё не многодневное — граница не включающая", multidayMinSpan)
	}
	// Обе стороны обязаны лежать в РАЗНЫХ календарных днях, иначе граница
	// проверялась бы вторым условием, а не порогом.
	for _, d := range []time.Duration{multidayMinSpan - time.Minute, multidayMinSpan} {
		if !mskToday(start.Add(d)).After(mskToday(start)) {
			t.Fatalf("образец на %v не пересекает полночь — тест меряет не то", d)
		}
	}
}
