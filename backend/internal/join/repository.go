package join

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pythonjsgo/vshage-afisha/internal/events"
)

// ErrDuplicate — этот юзернейм присылал анкету меньше суток назад. Не ошибка
// человека: он просто не увидел подтверждения и нажал ещё раз, — поэтому
// наверх уходит спокойное «анкета уже есть», а не отказ.
var ErrDuplicate = errors.New("join: анкета за последние сутки уже есть")

// dedupWindow — сколько повтор считается тем же обращением. Сутки, а не
// «навсегда»: человек, которому не ответили в сентябре, имеет право прийти
// снова в марте, и уникальный индекс это запретил бы.
const dedupWindow = 24 * time.Hour

type Repository struct {
	pool   *pgxpool.Pool
	ipSalt string
}

func NewRepository(pool *pgxpool.Pool, ipSalt string) *Repository {
	return &Repository{pool: pool, ipSalt: ipSalt}
}

// Create пишет анкету и ставит уведомление в очередь В ТОЙ ЖЕ ТРАНЗАКЦИИ.
// Либо есть и строка, и сообщение в очереди, либо нет ничего: анкета, о
// которой никто не узнал, для нас равна потерянной.
func (r *Repository) Create(ctx context.Context, c Clean) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// Блокировка на время транзакции по ключу юзернейма. Без неё два
	// одновременных нажатия «Отправить» проходят проверку на дубль оба и
	// кладут две строки: проверка «нет ли за сутки» читает состояние, которое
	// сосед ещё не закоммитил. Уникальным индексом это не закрыть — окно
	// скользящее, а не вечное.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, c.Telegram); err != nil {
		return 0, err
	}

	var dup bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM join_requests
			WHERE telegram = $1
			  AND created_at > NOW() - make_interval(hours => $2)
		)`, c.Telegram, int(dedupWindow.Hours())).Scan(&dup)
	if err != nil {
		return 0, err
	}
	if dup {
		return 0, ErrDuplicate
	}

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO join_requests (
			name, university, university_other, course, about, telegram,
			consent_at, utm_source, utm_medium, utm_campaign, utm_content,
			utm_term, yclid, referrer, user_agent, ip_hash
		) VALUES (
			$1, $2, NULLIF($3,''), $4, $5, $6,
			NOW(), NULLIF($7,''), NULLIF($8,''), NULLIF($9,''), NULLIF($10,''),
			NULLIF($11,''), NULLIF($12,''), NULLIF($13,''), NULLIF($14,''), NULLIF($15,'')
		) RETURNING id`,
		c.Name, c.University, c.UniversityOther, c.Course, c.About, c.Telegram,
		c.UTMSource, c.UTMMedium, c.UTMCampaign, c.UTMContent,
		c.UTMTerm, c.YClid, c.Referrer, c.UserAgent, r.hashIP(c.IP),
	).Scan(&id)
	if err != nil {
		return 0, err
	}

	if err := events.EnqueueTelegram(ctx, tx, NotifyText(c)); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return id, nil
}

// Row — анкета глазами модератора.
type Row struct {
	ID              int64     `json:"id"`
	CreatedAt       time.Time `json:"created_at"`
	Name            string    `json:"name"`
	University      string    `json:"university"`
	UniversityOther string    `json:"university_other,omitempty"`
	Course          string    `json:"course"`
	About           string    `json:"about"`
	Telegram        string    `json:"telegram"`
	Status          string    `json:"status"`
	UTMCampaign     string    `json:"utm_campaign,omitempty"`
	UTMSource       string    `json:"utm_source,omitempty"`
}

// List отдаёт анкеты для разбора. Пустой status = все: усечение по умолчанию
// читается как полнота, поэтому фильтр здесь явный, а limit возвращается
// вызывающему вместе с данными (см. handler).
func (r *Repository) List(ctx context.Context, status string, limit int) ([]Row, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, created_at, name, university, COALESCE(university_other,''),
		       course, about, telegram, status,
		       COALESCE(utm_campaign,''), COALESCE(utm_source,'')
		FROM join_requests
		WHERE ($1 = '' OR status = $1)
		ORDER BY created_at DESC
		LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Row{}
	for rows.Next() {
		var x Row
		if err := rows.Scan(&x.ID, &x.CreatedAt, &x.Name, &x.University,
			&x.UniversityOther, &x.Course, &x.About, &x.Telegram, &x.Status,
			&x.UTMCampaign, &x.UTMSource); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	// rows.Err() обязателен: без него отказ прав или обрыв соединения
	// приезжает сюда как «строк нет» — ровно так помощник кабинета однажды
	// показал пустой список вместо permission denied.
	return out, rows.Err()
}

// SetStatus — решение по анкете. Возвращает false, если такой строки нет:
// «не нашли» и «поменяли» обязаны различаться, иначе опечатка в id читается
// как успешная модерация.
func (r *Repository) SetStatus(ctx context.Context, id int64, status string) (bool, error) {
	// decided_at ставится вместе со статусом: срок хранения отклонённой
	// анкеты политика считает от даты РЕШЕНИЯ, а не подачи.
	tag, err := r.pool.Exec(ctx,
		`UPDATE join_requests SET status = $2, decided_at = NOW() WHERE id = $1`,
		id, status)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) hashIP(ip string) string {
	if ip == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(r.ipSalt + "|" + ip))
	return hex.EncodeToString(sum[:])[:32]
}
