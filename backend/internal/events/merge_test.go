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

	p1 := MergePage([][]PublicEvent{main, extra}, 2, 0)
	p2 := MergePage([][]PublicEvent{main, extra}, 2, 2)

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

	got := ids(MergePage([][]PublicEvent{main, extra}, 10, 0))
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

	first := ids(MergePage([][]PublicEvent{a, b}, 10, 0))
	second := ids(MergePage([][]PublicEvent{b, a}, 10, 0))
	if !eq(first, "ev_a", "ev_b") || !eq(second, "ev_a", "ev_b") {
		t.Fatalf("порядок зависит от порядка источников: %v против %v", first, second)
	}
}

// Хвост за пределами данных — пустой список, а не паника и не повтор.
func TestMergePage_ЗаПределамиХвоста(t *testing.T) {
	main := []PublicEvent{ev("m1", 10)}
	if got := MergePage([][]PublicEvent{main}, 5, 10); len(got) != 0 {
		t.Fatalf("жду пусто за хвостом, получил %v", ids(got))
	}
	if got := MergePage([][]PublicEvent{main}, 5, 0); !eq(ids(got), "m1") {
		t.Fatalf("жду [m1], получил %v", ids(got))
	}
}
