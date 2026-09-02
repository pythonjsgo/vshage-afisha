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

// Условие видимости встречается в пяти запросах, и правку легко внести в один
// из них. Тест читает сам файл: голое `COALESCE(d.visibility …)` мимо двух
// именованных констант — это ровно тот случай, когда доска и ссылка тихо
// разъезжаются.
func TestEveryQueryUsesTheNamedVisibilityRule(t *testing.T) {
	src, err := os.ReadFile("repository.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	// Два вхождения — сами объявления констант; чтение visibility в
	// RegisterPublic идёт отдельной колонкой и проверяется ниже.
	if n := strings.Count(body, "COALESCE(d.visibility, 'public') "); n != 2 {
		t.Errorf("условий видимости мимо констант: %d (ожидали только два объявления)", n)
	}
	if strings.Count(body, "visibleOnBoard") != 4 { // объявление + три списка
		t.Error("не все списки фильтруют по visibleOnBoard")
	}
	if strings.Count(body, "visibleByLink") != 2 { // объявление + карточка
		t.Error("карточка события фильтрует не по visibleByLink")
	}
	// Запись должна пускать ровно то же, что и карточка: страница открылась,
	// а форма отвечает «событие не найдено» — худший из возможных исходов.
	if !strings.Contains(body, `ev.Visibility != "public" && ev.Visibility != "unlisted"`) {
		t.Error("запись на unlisted-событие отклоняется — форма на открывшейся странице не сработает")
	}
}
