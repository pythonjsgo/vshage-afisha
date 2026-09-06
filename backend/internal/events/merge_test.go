package events

import (
	"testing"
	"time"
)

func ev(id string, min int) PublicEvent {
	return PublicEvent{ID: id, StartTime: time.Date(2026, 9, 1, 0, min, 0, 0, time.UTC)}
}

func ids(list []PublicEvent) []string {
	out := make([]string, len(list))
	for i, e := range list {
		out[i] = e.ID
	}
	return out
}

func eq(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Прибор на тот самый дефект, ради которого MergePage и написана: до неё
// дополнительный источник приклеивался к странице целиком, поэтому вторая
// страница повторяла его события. Здесь это проверяется прямо: ни один id не
// имеет права встретиться на двух страницах подряд.
func TestMergePage_НеДублируетМеждуСтраницами(t *testing.T) {
	main := []PublicEvent{ev("m1", 10), ev("m2", 30), ev("m3", 50)}
	extra := []PublicEvent{ev("e1", 20), ev("e2", 40)}

	p1 := MergePage([][]PublicEvent{main, extra}, 2, 0, false)
	p2 := MergePage([][]PublicEvent{main, extra}, 2, 2, false)

	if !eq(ids(p1), "m1", "e1") {
		t.Fatalf("страница 1: жду [m1 e1], получил %v", ids(p1))
	}
	if !eq(ids(p2), "m2", "e2") {
		t.Fatalf("страница 2: жду [m2 e2], получил %v", ids(p2))
	}
	seen := map[string]bool{}
	for _, e := range append(append([]PublicEvent{}, p1...), p2...) {
		if seen[e.ID] {
			t.Fatalf("id %s встретился на двух страницах — дубль между страницами", e.ID)
		}
		seen[e.ID] = true
	}
}

// Порядок обязан быть строго по времени начала, а не «сначала чужие».
func TestMergePage_ПорядокПоВремени(t *testing.T) {
	main := []PublicEvent{ev("m1", 10), ev("m2", 50)}
	extra := []PublicEvent{ev("e1", 20), ev("e2", 30)}

	got := ids(MergePage([][]PublicEvent{main, extra}, 10, 0, false))
	if !eq(got, "m1", "e1", "e2", "m2") {
		t.Fatalf("жду [m1 e1 e2 m2], получил %v", got)
	}
}

// Одинаковое время начала — не повод тасовать ленту между запросами.
// У студсобытий без указанного времени start_time = 00:00 МСК у всех сразу,
// и без явного тай-брейка порядок «стабилен» только в пределах одного вызова.
func TestMergePage_РавноеВремяРешаетсяId(t *testing.T) {
	a := []PublicEvent{ev("ev_b", 0)}
	b := []PublicEvent{ev("ev_a", 0)}

	first := ids(MergePage([][]PublicEvent{a, b}, 10, 0, false))
	second := ids(MergePage([][]PublicEvent{b, a}, 10, 0, false))
	if !eq(first, "ev_a", "ev_b") || !eq(second, "ev_a", "ev_b") {
		t.Fatalf("порядок зависит от порядка источников: %v против %v", first, second)
	}
}

// Хвост за пределами данных — пустой список, а не паника и не повтор.
func TestMergePage_ЗаПределамиХвоста(t *testing.T) {
	main := []PublicEvent{ev("m1", 10)}
	if got := MergePage([][]PublicEvent{main}, 5, 10, false); len(got) != 0 {
		t.Fatalf("жду пусто за хвостом, получил %v", ids(got))
	}
	if got := MergePage([][]PublicEvent{main}, 5, 0, false); !eq(ids(got), "m1") {
		t.Fatalf("жду [m1], получил %v", ids(got))
	}
}

// evEnd — карточка с концом. Полоса «идёт сейчас» сливается по КОНЦУ, и без
// собственной фикстуры это не проверить: у события из ev() конца нет вовсе.
func evEnd(id string, startMin, endMin int) PublicEvent {
	e := ev(id, startMin)
	t := time.Date(2026, 9, 1, 0, endMin, 0, 0, time.UTC)
	e.EndTime = &t
	return e
}

// Полоса «идёт сейчас» отвечает на вопрос «успею ли»: сверху то, что
// закрывается раньше. Ключ слияния тут — конец, и начало на порядок не влияет
// вовсе: у идущих программ оно в прошлом и говорит только о том, кто открылся
// раньше.
//
// Порядок обязан совпасть с ORDER BY источников. Разойдись они — окно
// [offset, offset+limit) перестало бы быть честным: событие, стоящее у
// источника за окном, попало бы в него по другому ключу, страница потеряла бы
// карточки и выглядела бы полной.
func TestMergePage_ИдущиеПоКонцу(t *testing.T) {
	// Начала намеренно в обратном порядке к концам: сортировка по началу дала
	// бы ровно обратный список, и тест отличит одно от другого.
	main := []PublicEvent{evEnd("m_late", 10, 90), evEnd("m_soon", 40, 20)}
	extra := []PublicEvent{evEnd("e_mid", 20, 50)}

	got := ids(MergePage([][]PublicEvent{main, extra}, 10, 0, true))
	if !eq(got, "m_soon", "e_mid", "m_late") {
		t.Fatalf("жду [m_soon e_mid m_late], получил %v", got)
	}
	// Тот же набор по началу — другой порядок. Иначе тест проходил бы и при
	// полностью проигнорированном byEnd.
	byStart := ids(MergePage([][]PublicEvent{main, extra}, 10, 0, false))
	if eq(byStart, got...) {
		t.Fatalf("порядок по началу совпал с порядком по концу — byEnd ничего не переключает: %v", byStart)
	}
}

// Бессрочная программа отвечает на «успею ли» словом «всегда» — значит её
// место в самом конце полосы. Без явного ключа нулевой EndTime отправил бы её
// в начало, то есть карточка «идёт постоянно» стояла бы первой там, где сверху
// обязано быть закрывающееся завтра.
func TestMergePage_БессрочноеВКонцеПолосы(t *testing.T) {
	forever := ev("forever", 5)
	forever.OpenEnded = true
	pages := [][]PublicEvent{{forever}, {evEnd("soon", 10, 20), evEnd("later", 15, 60)}}

	if got := ids(MergePage(pages, 10, 0, true)); !eq(got, "soon", "later", "forever") {
		t.Fatalf("жду [soon later forever], получил %v", got)
	}
	// В полосе времени бессрочное ведёт себя как обычное: там ключ — начало.
	if got := ids(MergePage(pages, 10, 0, false)); !eq(got, "forever", "soon", "later") {
		t.Fatalf("порядок по началу: жду [forever soon later], получил %v", got)
	}
}

// Событие без конца и без признака «бессрочно» кончается тогда же, когда
// началось — то же COALESCE(end, start), что и в SQL. Иначе нулевой EndTime
// в полосе «идёт сейчас» ставил бы такую карточку впереди всех.
func TestMergePage_БезКонцаСчитаетсяПоНачалу(t *testing.T) {
	pages := [][]PublicEvent{{ev("noend", 45)}, {evEnd("soon", 10, 20), evEnd("later", 10, 60)}}
	if got := ids(MergePage(pages, 10, 0, true)); !eq(got, "soon", "noend", "later") {
		t.Fatalf("жду [soon noend later], получил %v", got)
	}
}
