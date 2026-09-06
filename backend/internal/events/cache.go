package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	rdb *redis.Client
}

func NewCache(url string) (*Cache, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return &Cache{rdb: redis.NewClient(opts)}, nil
}

func (c *Cache) GetList(ctx context.Context, key string) (*ListResult, bool) {
	b, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil || len(b) == 0 {
		return nil, false
	}
	var out ListResult
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, false
	}
	return &out, true
}

func (c *Cache) SetList(ctx context.Context, key string, v ListResult, ttl time.Duration) {
	b, _ := json.Marshal(v)
	c.rdb.Set(ctx, key, b, ttl)
}

// eventCacheKey — ключ карточки события: `afisha:events:v2:<дата МСК>:<id>`.
//
// ВЕРСИЯ `v2` — про выкатку, а не про сутки. В окно деплоя записи, положенные
// СТАРЫМ бэкендом, доживают свои пять минут и приезжают новому фронту без
// `kind` и `multiday`: детальная страница выставки эти пять минут рисует
// довалновую подпись. Дата от этого не спасает — день тот же самый; спасает
// только смена префикса, после которой старые записи недостижимы мгновенно.
// Цена — холодный кэш карточек сразу после выкатки, то есть пять минут чуть
// более частых попаданий в базу. Следующая правка формы PublicEvent, меняющая
// смысл уже отданных полей, обязана поднять версию снова.
//
// Дата МСК в нём по той же причине, что и в ключе списка (см. Filter.CacheKey):
// полоса `kind` считается относительно «сегодня», а запись живёт пять минут и
// полночь переживает. Карточка, положенная в 23:58 с `kind: "timed"`, до 00:03
// отдавалась бы уже наступившим сегодня с прежней полосой. Пять минут в сутки
// на детальной странице — мало, но список от этого защищён специально, и
// асимметрия читалась бы как «у карточки просто забыли».
//
// Ключ строит ОДНА функция на все три обращения: чтение, запись и сброс после
// записи на событие. Собери его руками в третьем месте — и сброс удалял бы
// ключ, которого нет: карточка держала бы устаревший счётчик записавшихся все
// пять минут, причём молча. Прибор на это — TestОдинСтроительКлючаКарточки.
func eventCacheKey(id string, now time.Time) string {
	return "afisha:events:v2:" + now.In(mskZone).Format("2006-01-02") + ":" + id
}

func (c *Cache) GetEvent(ctx context.Context, id string, now time.Time) (*PublicEvent, bool) {
	b, err := c.rdb.Get(ctx, eventCacheKey(id, now)).Bytes()
	if err != nil || len(b) == 0 {
		return nil, false
	}
	var out PublicEvent
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, false
	}
	return &out, true
}

func (c *Cache) SetEvent(ctx context.Context, ev PublicEvent, ttl time.Duration, now time.Time) {
	b, _ := json.Marshal(ev)
	c.rdb.Set(ctx, eventCacheKey(ev.ID, now), b, ttl)
}

// InvalidateEvent сбрасывает карточку. Отдельным методом, а не общим
// Invalidate со склеенным на месте ключом: склейка на месте и была бы тем
// самым третьим строителем ключа.
//
// Ключ вчерашней даты при этом не удаляется и не должен: его никто уже не
// прочитает (чтение идёт по сегодняшней дате), и он уйдёт по своему TTL.
func (c *Cache) InvalidateEvent(ctx context.Context, id string, now time.Time) {
	c.rdb.Del(ctx, eventCacheKey(id, now))
}

func (c *Cache) Invalidate(ctx context.Context, keys ...string) {
	c.rdb.Del(ctx, keys...)
}
