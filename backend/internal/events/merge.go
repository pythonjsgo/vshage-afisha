package events

import (
	"sort"
	"time"
)

// openEndedKey — ключ порядка у события без конца. Полоса «идёт сейчас»
// отвечает на вопрос «успею ли», и бессрочная программа отвечает на него
// «всегда» — значит её место в самом конце, а не в начале, куда её отправил бы
// нулевой EndTime. Дата за пределами любого реального события и совпадает по
// смыслу с сентинелом источника (`date_end = 9999-01-01`), который в SQL
// уезжает в конец сам.
var openEndedKey = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

// MergePage сливает страницы нескольких источников ленты в одну.
// Editorial pins precede the chronological order in every source and here.
//
// Зачем отдельная функция. До 30.08 лента знала ровно один дополнительный
// источник и приклеивала его ЦЕЛИКОМ к уже обрезанной странице основного:
// `result.All = append(extra, result.All...)`. Пока источник был один и
// отдавал единицы событий, это работало; с двумя источниками и настоящим
// объёмом (студсобытия — десятки карточек) тот же код даёт два разных дефекта
// сразу, и оба тихие:
//
//  1. offset игнорируется — вторая страница возвращает те же самые события
//     дополнительного источника, что и первая;
//  2. total считается как «страница плюс всё лишнее», то есть не равен числу
//     событий, которые лента способна отдать.
//
// Правило слияния: каждый источник отдаёт СВОИ первые offset+limit событий,
// уже отсортированные по времени начала; здесь они сливаются и режется окно
// [offset, offset+limit). Это честно ровно потому, что событие с номером
// offset+limit+1 у любого источника не может попасть в это окно — перед ним
// в его же источнике стоит offset+limit более ранних.
//
// Сортировка стабильная и с явным тай-брейком по id: без него два события с
// одинаковым временем начала (а у студсобытий время часто «00:00 МСК» —
// признак «время не указано») меняются местами между запросами, и человек
// видит, как лента перетасовывается сама собой при перелистывании.
//
// byEnd переключает КЛЮЧ слияния на время конца — для полосы «идёт сейчас»,
// которая отвечает на «успею ли»: сверху то, что закрывается раньше. Это
// параметр, а не второй метод, СПЕЦИАЛЬНО: подрезка окна честна ровно тогда,
// когда каждый источник отдал свои первые offset+limit строк по ТОМУ ЖЕ ключу,
// каким сливают здесь. Смена сигнатуры ломает компиляцию у всех вызывающих —
// это и есть прибор, который нельзя забыть запустить; второй метод рядом
// оставил бы старый вызов молча работающим и молча неверным.
func MergePage(pages [][]PublicEvent, limit, offset int, byEnd bool) []PublicEvent {
	if limit <= 0 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}

	key := func(e PublicEvent) time.Time {
		if !byEnd {
			return e.StartTime
		}
		switch {
		case e.OpenEnded:
			return openEndedKey
		case e.EndTime != nil:
			return *e.EndTime
		}
		// Конца нет и бессрочным событие не объявлено — значит оно кончается
		// тогда же, когда началось. То же COALESCE(end, start), что и в SQL.
		return e.StartTime
	}

	merged := make([]PublicEvent, 0, limit)
	for _, p := range pages {
		merged = append(merged, p...)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		a, b := merged[i], merged[j]
		if a.IsFeatured != b.IsFeatured {
			return a.IsFeatured
		}
		if a.IsFeatured && a.FeaturedPosition != nil && b.FeaturedPosition != nil && *a.FeaturedPosition != *b.FeaturedPosition {
			return *a.FeaturedPosition < *b.FeaturedPosition
		}
		ki, kj := key(a), key(b)
		if ki.Equal(kj) {
			return merged[i].ID < merged[j].ID
		}
		return ki.Before(kj)
	})

	if offset >= len(merged) {
		return []PublicEvent{}
	}
	end := offset + limit
	if end > len(merged) {
		end = len(merged)
	}
	return merged[offset:end]
}
