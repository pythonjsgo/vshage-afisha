package join

import (
	"sort"
	"strings"
	"testing"
)

// TestUniversityCodes фиксирует НАБОР кодов вузов целиком.
//
// Это половина связки: вторая копия списка живёт на фронте
// (frontend/src/lib/join.ts) и закреплена своим тестом с тем же перечислением.
// Разъехавшись, они дают тихий отказ — человек выбирает вуз, которого сервер
// не знает, и теряется на валидации, — а тихий отказ на платном трафике стоит
// денег. Правка любой стороны обязана уронить её тест.
func TestUniversityCodes(t *testing.T) {
	want := []string{
		"bmstu", "finu", "hse", "mgimo", "mipt", "msu", "other", "plekhanov", "ranepa",
	}
	got := make([]string, 0, len(Universities))
	for k, label := range Universities {
		got = append(got, k)
		if strings.TrimSpace(label) == "" {
			t.Errorf("у кода %q пустая подпись", k)
		}
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("набор кодов вузов изменился:\n  сервер: %v\n  ожидали: %v\n"+
			"Поправь frontend/src/lib/join.ts и его тест в этом же коммите.", got, want)
	}
}

func validSubmission() Submission {
	return Submission{
		Name:       "Иван Петров",
		University: "hse",
		Course:     "3",
		About:      "Пишу бота для расписания и играю в го",
		Telegram:   "@ivanov_hse",
		Consent:    true,
	}
}

func TestValidate(t *testing.T) {
	t.Run("валидная анкета", func(t *testing.T) {
		c, errs := Validate(validSubmission())
		if errs != nil {
			t.Fatalf("неожиданные ошибки: %v", errs)
		}
		// Юзернейм обязан лечь в базу голым и в нижнем регистре — это ключ
		// дедупа, и '@Ivanov' с 'ivanov' должны быть одним человеком.
		if c.Telegram != "ivanov_hse" {
			t.Fatalf("telegram = %q, ждали ivanov_hse", c.Telegram)
		}
	})

	t.Run("ссылку на профиль тоже принимаем", func(t *testing.T) {
		in := validSubmission()
		in.Telegram = "https://t.me/Ivanov_HSE?start=1"
		c, errs := Validate(in)
		if errs != nil {
			t.Fatalf("ссылка на профиль должна приниматься, получили %v", errs)
		}
		if c.Telegram != "ivanov_hse" {
			t.Fatalf("telegram = %q", c.Telegram)
		}
	})

	t.Run("невалидный телеграм", func(t *testing.T) {
		in := validSubmission()
		in.Telegram = "иванов"
		_, errs := Validate(in)
		if errs["telegram"] == "" {
			t.Fatalf("ждали ошибку по полю telegram, получили %v", errs)
		}
	})

	t.Run("без согласия", func(t *testing.T) {
		in := validSubmission()
		in.Consent = false
		_, errs := Validate(in)
		if errs["consent"] == "" {
			t.Fatalf("ждали ошибку по полю consent, получили %v", errs)
		}
	})

	t.Run("чужой вуз требует названия", func(t *testing.T) {
		in := validSubmission()
		in.University = "other"
		_, errs := Validate(in)
		if errs["university_other"] == "" {
			t.Fatalf("ждали ошибку university_other, получили %v", errs)
		}
		in.UniversityOther = "  МИСиС  "
		c, errs := Validate(in)
		if errs != nil {
			t.Fatalf("неожиданные ошибки: %v", errs)
		}
		if c.UniversityOther != "МИСиС" {
			t.Fatalf("university_other = %q, ждали «МИСиС»", c.UniversityOther)
		}
	})

	t.Run("несуществующий вуз отвергается", func(t *testing.T) {
		in := validSubmission()
		in.University = "oxford"
		_, errs := Validate(in)
		if errs["university"] == "" {
			t.Fatalf("ждали ошибку university, получили %v", errs)
		}
	})

	t.Run("about длиннее 140 знаков", func(t *testing.T) {
		in := validSubmission()
		in.About = strings.Repeat("я", 141)
		_, errs := Validate(in)
		if errs["about"] == "" {
			t.Fatalf("ждали ошибку about, получили %v", errs)
		}
		// Ровно 140 — можно: граница включительная, иначе человек с ровным
		// текстом получает отказ, которого не ждёт.
		in.About = strings.Repeat("я", 140)
		if _, errs := Validate(in); errs != nil {
			t.Fatalf("140 знаков должны проходить, получили %v", errs)
		}
	})
}

// TestTrackingMarksAreSingleLine — метка кампании целиком в руках того, кто
// открыл ссылку. Перевод строки внутри неё дорисовал бы модератору ОТДЕЛЬНУЮ
// строку сообщения («Телеграм: @чужой»), и читал бы её человек, принимающий
// решение.
//
// Инвариант тут ровно один и проверяется буквально: сколько бы переводов
// строки ни прислали, число строк сообщения не меняется. Требовать, чтобы
// слово «Телеграм» не встречалось внутри значения метки, — перебор: оставшись
// в СВОЕЙ строке, оно уже ничего не подделывает, а вычищать из чужих меток
// слова значит портить настоящие названия кампаний.
func TestTrackingMarksAreSingleLine(t *testing.T) {
	clean := func(in Submission) Clean {
		t.Helper()
		c, errs := Validate(in)
		if errs != nil {
			t.Fatalf("метка не должна валить валидацию: %v", errs)
		}
		return c
	}

	honest := clean(validSubmission())
	honest.UTMCampaign = "join_msk"
	want := strings.Count(NotifyText(honest), "\n")

	in := validSubmission()
	in.UTMCampaign = "join_msk\nТелеграм: @attacker\nВуз: МГУ"
	c := clean(in)
	if strings.ContainsAny(c.UTMCampaign, "\n\r") {
		t.Fatalf("в метке остался перевод строки: %q", c.UTMCampaign)
	}
	if got := strings.Count(NotifyText(c), "\n"); got != want {
		t.Fatalf("строк в сообщении %d вместо %d — метка дорисовала свои:\n%s",
			got+1, want+1, NotifyText(c))
	}
}

// TestNotifyText — сообщение читает человек в чате, и в нём должны быть все
// шесть полей директивы: имя, вуз, курс, занятие, юзернейм, кампания.
func TestNotifyText(t *testing.T) {
	c, errs := Validate(validSubmission())
	if errs != nil {
		t.Fatalf("фикстура невалидна: %v", errs)
	}
	c.UTMCampaign = "join_msk_students"

	txt := NotifyText(c)
	for _, want := range []string{
		"Новая анкета", "Иван Петров", "ВШЭ", "3 курс",
		"Пишу бота для расписания", "@ivanov_hse", "join_msk_students",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("в сообщении нет %q:\n%s", want, txt)
		}
	}

	// Без кампании строки «Кампания:» быть не должно вовсе — пустая подпись
	// в чате читается как «кампания неизвестна», а это другое утверждение.
	c.UTMCampaign = ""
	if strings.Contains(NotifyText(c), "Кампания") {
		t.Errorf("пустая кампания не должна печататься:\n%s", NotifyText(c))
	}
}

// TestNotifyTextOtherUniversity — у «Другого» вуза в чат обязано уехать то,
// что человек написал, а не слово «Другой»: иначе сообщение бесполезно.
func TestNotifyTextOtherUniversity(t *testing.T) {
	in := validSubmission()
	in.University = "other"
	in.UniversityOther = "МИСиС"
	c, errs := Validate(in)
	if errs != nil {
		t.Fatalf("фикстура невалидна: %v", errs)
	}
	txt := NotifyText(c)
	if !strings.Contains(txt, "МИСиС") || strings.Contains(txt, "Вуз: Другой") {
		t.Fatalf("ждали название вуза из поля, получили:\n%s", txt)
	}
}
