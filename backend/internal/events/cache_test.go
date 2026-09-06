package events

import (
	"context"
	"os"
	"testing"
	"time"
)

// Прогон кэша карточки против НАСТОЯЩЕГО редиса. Ключ здесь собирают три
// обращения — чтение, запись и сброс, — и до 07.09 каждое склеивало его своей
// копией строки. Пока копии совпадали, всё работало; расходятся такие копии
// молча и в одну сторону: сброс удаляет ключ, которого нет, страница отвечает
// 200 старым содержимым, и правка события в кабинете не доезжает до публичной
// карточки пять минут.
//
// Проверять это чтением кода нельзя — совпадение трёх строк и есть предмет
// проверки. Нужен круг: положили, сбросили, промахнулись.
//
//	AFISHA_TEST_REDIS_URL=redis://127.0.0.1:36379/15 go test ./internal/events/ -run Кэш
//
// Без переменной тест пропускается, и пропуск — не «зелёный».
func cacheForTest(t *testing.T) *Cache {
	t.Helper()
	url := os.Getenv("AFISHA_TEST_REDIS_URL")
	if url == "" {
		t.Skip("AFISHA_TEST_REDIS_URL не задан — прогон против настоящего редиса пропущен")
	}
	c, err := NewCache(url)
	if err != nil {
		t.Fatalf("подключение к редису: %v", err)
	}
	if err := c.rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("редис не отвечает: %v", err)
	}
	return c
}

// Круг «положили → сбросили → промахнулись». Главный прибор на сцепку трёх
// обращений к ключу карточки.
func TestКэшКарточкиСбрасываетсяТемЖеКлючом(t *testing.T) {
	c := cacheForTest(t)
	ctx := context.Background()
	now := mskDate(2026, 9, 7, 12)
	// Свой id на каждый прогон не нужен — база отдельная (индекс 15), но
	// прибрать за собой обязаны: чужой оставленный ключ читался бы следующим
	// прогоном как «попадание» и прятал бы отказ.
	const id = "ev_cache_probe"
	t.Cleanup(func() { c.InvalidateEvent(ctx, id, now) })

	ev := PublicEvent{ID: id, Title: "Проба", Kind: KindRunning, Multiday: true}

	// КОНТРОЛЬ ДО: пусто. Иначе «промах после сброса» ничего не доказывает —
	// он мог быть промахом и до записи.
	c.InvalidateEvent(ctx, id, now)
	if _, ok := c.GetEvent(ctx, id, now); ok {
		t.Fatal("карточка нашлась ДО записи — прибор меряет не то")
	}

	c.SetEvent(ctx, ev, 5*time.Minute, now)
	got, ok := c.GetEvent(ctx, id, now)
	if !ok {
		t.Fatal("положили и не нашли — чтение и запись строят разные ключи")
	}
	// Заодно: то, что легло, обязано пережить сериализацию с новыми полями.
	// Карточка без Kind — это довалновая запись, ровно та, ради которой в
	// ключе стоит версия.
	if got.Kind != KindRunning || !got.Multiday {
		t.Errorf("из кэша приехала карточка kind=%q multiday=%v", got.Kind, got.Multiday)
	}

	c.InvalidateEvent(ctx, id, now)
	if _, ok := c.GetEvent(ctx, id, now); ok {
		t.Error("после сброса карточка ещё в кэше — сброс строит НЕ ТОТ ключ, и правка события не доедет до страницы пять минут")
	}
}

// Запись вчерашнего дня не отдаётся сегодня. Это вторая половина того же
// правила: полоса `kind` считается относительно «сегодня», а запись живёт пять
// минут и полночь переживает.
func TestКэшКарточкиНеОтдаётВчерашнююЗапись(t *testing.T) {
	c := cacheForTest(t)
	ctx := context.Background()
	late := mskDate(2026, 9, 7, 23) // запись легла вечером
	next := mskDate(2026, 9, 8, 0)  // читаем уже в новых сутках
	const id = "ev_cache_midnight"
	t.Cleanup(func() {
		c.InvalidateEvent(ctx, id, late)
		c.InvalidateEvent(ctx, id, next)
	})

	c.SetEvent(ctx, PublicEvent{ID: id, Kind: KindTimed}, 5*time.Minute, late)
	if _, ok := c.GetEvent(ctx, id, next); ok {
		t.Error("вчерашняя запись отдалась сегодня — полоса приедет вчерашняя")
	}
	// КОНТРОЛЬ: в свои сутки она читается. Без него тест проходил бы и при
	// кэше, который не отдаёт ничего никогда.
	if _, ok := c.GetEvent(ctx, id, late); !ok {
		t.Error("запись не читается даже в свои сутки — сломан кэш, а не дата в ключе")
	}
}
