#!/usr/bin/env bash
# Разбор анкет на вход в закрытую сеть (vshage.app/join) без админки.
#
# Второй путь к тому же — HTTP под админским JWT афиши:
#   GET   /api/admin/join?status=new
#   PATCH /api/admin/join/<id>  {"status":"approved"}
# Этот скрипт нужен, когда токена под рукой нет, а решение принять надо.
#
#   scripts/join-requests.sh [dev|prod] list [new|approved|rejected]
#   scripts/join-requests.sh [dev|prod] show <id>
#   scripts/join-requests.sh [dev|prod] approve <id>
#   scripts/join-requests.sh [dev|prod] reject  <id>
#   scripts/join-requests.sh [dev|prod] stats
#   scripts/join-requests.sh [dev|prod] queue    — застрявшие уведомления
#
# Стенд обязателен первым аргументом и НЕ имеет значения по умолчанию:
# «approve 12» без стенда однажды поедет не в ту базу, и это необратимо.
set -euo pipefail

SSH_KEY="${SSH_KEY:-$HOME/.ssh/vshage-deploy}"

usage() { sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }

[ $# -ge 2 ] || usage
STAND="$1"; shift
CMD="$1"; shift

case "$STAND" in
  dev)  HOST=64.188.80.28; DIR=/opt/vshage-dev/compose ;;
  prod) HOST=2.27.28.53;   DIR=/opt/vshage-prod/compose ;;
  *) echo "Неизвестный стенд: $STAND (нужно dev или prod)" >&2; exit 2 ;;
esac

# psql запускается с -c: SQL идёт АРГУМЕНТОМ, не через stdin. `docker exec -i`
# внутри ssh-скрипта съедает остаток скрипта — на этом у нас однажды молча не
# выполнился сброс кэша, и вывод выглядел просто короче обычного.
psql_run() {
  local sql="$1"
  ssh -i "$SSH_KEY" -o ConnectTimeout=15 "root@$HOST" \
    "cd $DIR && docker compose exec -T postgres psql -U slava -d slava -v ON_ERROR_STOP=1 -c \"\$(cat <<'SQL'
$sql
SQL
)\""
}

# id проверяется здесь, а не в SQL: он подставляется в запрос текстом, и
# единственная защита от инъекции — то, что дальше пройдут только цифры.
check_id() {
  [[ "${1:-}" =~ ^[0-9]+$ ]] || { echo "Нужен числовой id анкеты" >&2; exit 2; }
}

case "$CMD" in
  list)
    STATUS="${1:-new}"
    case "$STATUS" in new|approved|rejected|all) ;; *) echo "Статус: new|approved|rejected|all" >&2; exit 2 ;; esac
    WHERE="WHERE status = '$STATUS'"
    [ "$STATUS" = all ] && WHERE=""
    psql_run "
      SELECT id,
             to_char(created_at AT TIME ZONE 'Europe/Moscow', 'DD.MM HH24:MI') AS \"когда\",
             name AS \"имя\",
             CASE WHEN university = 'other' THEN COALESCE(university_other, 'другой') ELSE university END AS \"вуз\",
             course AS \"курс\",
             '@' || telegram AS \"телеграм\",
             left(about, 48) AS \"занимается\",
             COALESCE(utm_campaign, '—') AS \"кампания\"
      FROM join_requests
      $WHERE
      ORDER BY created_at DESC
      LIMIT 200;"
    ;;

  show)
    check_id "${1:-}"
    psql_run "SELECT * FROM join_requests WHERE id = $1;" | sed 's/^/  /'
    ;;

  approve|reject)
    check_id "${1:-}"
    NEW=approved; [ "$CMD" = reject ] && NEW=rejected
    # RETURNING, а не «UPDATE ... ; SELECT»: пустой вывод тогда означает
    # «строки с таким id нет», и опечатка в номере не выглядит успехом.
    # decided_at ставится и здесь: срок хранения политика считает от даты
    # решения, а этот путь пишет в базу мимо ручки, которая её проставляет.
    psql_run "UPDATE join_requests SET status = '$NEW', decided_at = NOW() WHERE id = $1 RETURNING id, name, '@' || telegram AS telegram, status, decided_at;"
    ;;

  queue)
    # Прибор на застрявшие уведомления. Очередь ретраит вечно, поэтому
    # протухший токен бота выглядит как тишина: анкеты в базе есть, в чате
    # их нет, и узнать об этом неоткуда. Строка старше часа = канал мёртв.
    psql_run "
      SELECT count(*) FILTER (WHERE sent_at IS NULL) AS \"в очереди\",
             count(*) FILTER (WHERE sent_at IS NULL AND attempts > 20) AS \"застряло\",
             COALESCE(max(EXTRACT(EPOCH FROM (NOW() - created_at)) FILTER (WHERE sent_at IS NULL))::int, 0) AS \"старшей, сек\"
      FROM registration_notify_outbox;
      SELECT id, attempts, left(COALESCE(last_error,''), 60) AS \"ошибка\",
             left(payload, 40) AS \"текст\"
      FROM registration_notify_outbox
      WHERE sent_at IS NULL ORDER BY id LIMIT 10;"
    ;;

  stats)
    psql_run "
      SELECT status,
             count(*) AS \"всего\",
             count(*) FILTER (WHERE created_at > NOW() - INTERVAL '7 days') AS \"за неделю\"
      FROM join_requests GROUP BY status ORDER BY status;
      SELECT COALESCE(utm_campaign, '— без метки') AS \"кампания\", count(*) AS \"анкет\"
      FROM join_requests GROUP BY 1 ORDER BY 2 DESC LIMIT 20;"
    ;;

  *) usage ;;
esac
