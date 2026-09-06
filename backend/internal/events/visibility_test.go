package events

import (
	"os"
	"strings"
	"testing"
)

// Правило, которое держит этот тест: «спрятать с доски» и «закрыть событие» —
// разные вещи, и спутать их дорого. Событие 10.09 роздано людям ссылкой; если
// unlisted начнёт означать 404, ссылка умрёт у всех, кому её уже отправили, и
// заметит это не прибор, а человек у входа.
func TestUnlistedIsHiddenFromTheBoardButOpensByLink(t *testing.T) {
	if strings.Contains(visibleOnBoard, "unlisted") {
		t.Error("доска показывает unlisted — значит «спрятать» ничего не прячет")
	}
	if !strings.Contains(visibleByLink, "unlisted") {
		t.Error("по ссылке unlisted не открывается — это мёртвая ссылка")
	}
	if !strings.Contains(visibleOnBoard, "'public'") || !strings.Contains(visibleByLink, "'public'") {
		t.Error("обычное событие перестало быть видимым")
	}
}

// repositoryCode — исходник репозитория без строк-комментариев.
//
// Комментарии выброшены намеренно: тест считает ВХОЖДЕНИЯ, а имя правила
// упоминается и в объяснении рядом с ним. Пока считались все строки подряд,
// правка комментария меняла число и красное означало «кто-то дописал абзац»,
// а не «кто-то забыл условие». Прибор, который краснеет не на том, перестают
// читать.
func repositoryCode(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("repository.go")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, line := range strings.Split(string(src), "\n") {
		trimmed := strings.TrimSpace(line)
		// Отбрасываем комментарии ОБОИХ видов: Go-шные и SQL-ные внутри
		// строковых литералов запросов. Иначе абзац, объясняющий предикат
		// доски и называющий его по имени, считается вхождением — и прибор
		// краснеет на объяснении, а не на расхождении. Ровно так он и
		// покраснел 06.09 на комментарии про price_type.
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "--") {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// Условие видимости и предикат доски существуют в ЕДИНСТВЕННОМ экземпляре, и
// каждый запрос доски обязан идти через них. Тест читает сам файл: голое
// `COALESCE(d.visibility …)` мимо двух именованных констант — это ровно тот
// случай, когда доска и ссылка тихо разъезжаются, а собственный `e.status =
// 'published' AND e.start_time >= $1` в новом запросе — тот случай, когда
// разъезжаются список и счётчик, и подпись «показано N из M» врёт на разницу.
func TestEveryQueryUsesTheNamedVisibilityRule(t *testing.T) {
	body := repositoryCode(t)
	// Два вхождения — сами объявления констант; чтение visibility в
	// RegisterPublic идёт отдельной колонкой и проверяется ниже.
	if n := strings.Count(body, "COALESCE(d.visibility, 'public') "); n != 2 {
		t.Errorf("условий видимости мимо констант: %d (ожидали только два объявления)", n)
	}
	// Объявление + boardBase: доска знает правило видимости в одном месте.
	if n := strings.Count(body, "visibleOnBoard"); n != 2 {
		t.Errorf("visibleOnBoard встречается %d раз (ожидали объявление и boardBase) — правило видимости расползлось", n)
	}
	if n := strings.Count(body, "visibleByLink"); n != 2 { // объявление + карточка
		t.Error("карточка события фильтрует не по visibleByLink")
	}
	// Объявление + четыре запроса доски: закреплённое, список, счётчик,
	// фасеты. Новый запрос со своим предикатом это число не изменит — и
	// именно поэтому число проверяется: пятый запрос обязан либо взять
	// boardBase, либо объяснить себя правкой этого теста.
	if n := strings.Count(body, "boardBase"); n != 5 {
		t.Errorf("boardBase встречается %d раз (ожидали объявление и четыре запроса доски) — предикат доски разошёлся", n)
	}
	// Запись должна пускать ровно то же, что и карточка: страница открылась,
	// а форма отвечает «событие не найдено» — худший из возможных исходов.
	if !strings.Contains(body, `ev.Visibility != "public" && ev.Visibility != "unlisted"`) {
		t.Error("запись на unlisted-событие отклоняется — форма на открывшейся странице не сработает")
	}
}
