# Веб-регистрация на событие — `vshage.app/e/<slug>`

Полный цикл без приложения и без аккаунта: страница события → форма →
экран «готово» с мостом в сеть. Организатору — живой список по секретной
ссылке и выгрузка в CSV.

Создание события пока без UI: один PUT с JSON-конфигом. UI — потом.

## Что где лежит

| Часть | Путь |
|---|---|
| Таблицы | `backend/pkg/db/migrations/003_webreg.sql` (`webreg_events`, `webreg_registrations`, `webreg_waitlist`) |
| API | `backend/internal/webreg/` — `/api/e/*`, `/api/webreg/admin/events` |
| Страница события | `frontend/src/routes/e/[slug]/` |
| Список организатора | `frontend/src/routes/e/[slug]/manage/` |
| Выгрузка CSV | `frontend/src/routes/e/[slug]/manage/export/` (прокси на бэкенд) |
| Маршрут `/e/*` | umbrella `compose/Caddyfile.{dev,prod}` — блок `vshage.app` |
| Тесты | `backend/internal/webreg/validate_test.go`, `frontend/tests/e2e/webreg.spec.ts` |

Модуль **намеренно не использует** общие `events` / `event_registrations`:
в `event_registrations` поле `profile_id` — `NOT NULL` с внешним ключом на
живые профили (веб-регистрация без аккаунта туда не ложится иначе как через
мусорные профили в боевой базе), а схема `events` разъехалась между DEV и PROD
(`docs/2026-08-14-isolated-db-design.md`). Свои таблицы не имеют ни одного
внешнего ключа в сторону приложения и переедут вместе с изолированной БД афиши.

## Переменные окружения

| Переменная | Зачем | Если не задана |
|---|---|---|
| `WEBREG_ADMIN_TOKEN` | гейт на создание/правку события | эндпоинт конфига **отключён** (fail closed) |
| `WEBREG_IP_SALT` | соль для хеша IP | случайная на процесс — хеши перестают сравниваться между рестартами |
| `WEBREG_LOG_PATH` | зеркало сабмитов в файл | только stdout (он и так переживает всё, см. ниже) |

## Как завести событие

### 1. Собрать карточку места

```bash
node scripts/venue-card.mjs "точка кипения"
node scripts/venue-card.mjs "точка кипения" --district Гагаринский --pick 1
```

Печатает блок `venue` — вставить в конфиг. Карточка **копируется** в конфиг,
а не подтягивается на лету: страница не должна зависеть от 11-мегабайтного
каталога в момент, когда на неё падает анонс из канала.

Места нет в каталоге — не беда: хватит `venue.address`, страница нарисует
адрес и кнопку маршрута. Совсем без места — блок просто не появится.

### 2. Написать конфиг

```jsonc
{
  "slug": "shag",                          // из a-z, 0-9, дефис. Это и есть URL
  "title": "ШАГ · открытая встреча",
  "tagline": "Одной строкой — то, что видно в превью телеграма",
  "description": "Многострочный текст.\n\nПереносы сохраняются.",
  "cover_url": "https://…/cover.jpg",      // ← ставь: без него в телеге нет картинки
  "starts_at": "2026-08-18T19:00:00+03:00",
  "timezone": "Europe/Moscow",
  "organizer_title": "сообщество ШАГ",
  "venue": { … },                          // из шага 1
  "affiliations": ["НИУ ВШЭ", "МГУ", "Работаю", "Школа"],
  "fields": [                              // поля организатора, рисуются по порядку
    {"key": "source", "label": "Откуда узнал(а)?", "type": "select", "required": true,
     "options": ["Телеграм-канал ШАГ", "От друзей"], "allow_other": true},
    {"key": "project", "label": "Над чем работаешь", "type": "text", "max_len": 200,
     "hint": "Одной строкой"}
  ],
  "bridge": {
    "ios_mode": "testflight",              // testflight | app_store | waitlist | off
    "testflight_url": "https://testflight.apple.com/join/XXXXXXX",
    "app_store_url": "",                   // заполнить заранее — переключение без деплоя
    "invite_code": "vshage23",
    "android_waitlist": true,
    "tg_channel_url": "https://t.me/…",
    "tg_chat_url": ""                      // если есть чат события — он важнее канала
  },
  "capacity": null,                        // число — покажем «осталось N мест»
  "registration_open": true,
  "manage_key": "длинная-случайная-строка" // ← ссылка организатора. Пустая строка при правке = не менять
}
```

Типы полей: `select` (плюс `options`, `allow_other`), `text`, `textarea`,
`checkbox`. Ключи с `__` в начале зарезервированы и будут отвергнуты.

Ключ организатора сгенерировать так:

```bash
openssl rand -hex 16
```

### 3. Залить конфиг

```bash
# DEV
curl -sS -X PUT https://afisha.dev.vshage.app/api/webreg/admin/events \
  -H "X-Admin-Token: $WEBREG_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  --data-binary @event-shag.json

# PROD
curl -sS -X PUT https://afisha.vshage.app/api/webreg/admin/events \
  -H "X-Admin-Token: $WEBREG_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  --data-binary @event-shag.json
```

Повторный PUT с тем же `slug` — правка события, применяется за ≤15 секунд
(столько живёт кэш конфига), **без деплоя**. Пустой `manage_key` при правке
сохраняет старый ключ, чтобы ссылка организатора не протухла от опечатки.

### 4. Ссылки

| Кому | Ссылка |
|---|---|
| В канал | `https://vshage.app/e/shag` |
| Организатору | `https://vshage.app/e/shag/manage?key=<manage_key>` |
| Выгрузка | кнопка «Скачать CSV» на той же странице |

Ссылку организатора **не публиковать**: у кого она есть, тот видит список.
Неверный ключ и несуществующее событие отвечают одинаково — 404, чтобы
перебором нельзя было узнать, что событие существует.

## Что происходит при регистрации

1. Юзернейм нормализуется: `@Ivanov`, `t.me/Ivanov`, `https://t.me/ivanov?start=1`
   — всё это один и тот же человек. Ключ дедупа — строчная форма, в списке
   показывается то, что человек набрал.
2. Сабмит пишется в лог **до** записи в базу — одной JSON-строкой
   `WEBREG_SUBMIT {...}`. Если база отвалится, человек не потерян: строка
   лежит в docker json-file на хосте (3×10 МБ) и восстанавливается. На Loki
   тут не рассчитываем — доставка логов с прода сломана
   (umbrella `rules/pitfalls.md` §Инфра).
3. Вставка идемпотентна по `(событие, юзернейм)`. Повторный тап или ретрай на
   слабой сети обновляет строку, а не плодит вторую; человек видит
   «Ты уже в списке».

## Проверить, что живо

```bash
# конфиг события отдаётся
curl -s https://vshage.app/e/shag -o /dev/null -w '%{http_code}\n'

# счётчик регистраций
curl -s https://afisha.vshage.app/api/e/shag | jq .registered_count

# сколько сабмитов видел бэкенд за сегодня (сверить с базой)
ssh root@<host> "docker logs afisha-backend 2>&1 | grep -c WEBREG_SUBMIT"
```

E2E-набор гоняется против любого стенда:

```bash
cd frontend
BASE_URL=https://vshage.app WEBREG_SLUG=shag WEBREG_MANAGE_KEY=… \
  npx playwright test tests/e2e/webreg.spec.ts --project=chromium
```

Без `WEBREG_SLUG` набор пропускается — обычный прогон CI остаётся зелёным.

## Ловушки, уже пойманные

- **`value={...}` на поле формы стирает набранное.** Svelte переприменяет
  атрибут при любом перерисовывании, поэтому выбор вуза в выпадающем списке
  обнулял уже введённые имя и телеграм — на экране всё выглядело заполненным,
  а сабмит падал на «пустое имя». Все поля — только через `bind:`. Ловится
  тестом «lands on the done screen».
- **`<select>` с выбранным `disabled`-плейсхолдером не попадает в FormData
  вообще** (не пустой строкой, а отсутствием ключа). Сервер обязан считать
  отсутствие поля тем же, что и пустое значение.
- **Лимитер намеренно мягкий** (1 rps, всплеск 20 на IP). Российские
  мобильные операторы держат тысячи абонентов за одним адресом CGNAT: жёсткий
  лимит не остановит скрипт (он меняет адреса), зато тихо отрежет живых людей
  с одного оператора. Дубли ловит уникальный индекс, а не лимитер.
- **CSV — с BOM и через `;`.** Без BOM русский Excel показывает кракозябры.
  Нужна запятая (Google Sheets) — `?sep=,`.
