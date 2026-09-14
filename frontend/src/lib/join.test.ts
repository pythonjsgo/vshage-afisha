import { describe, expect, it } from 'vitest';
import {
  COURSES,
  UNIVERSITIES,
  looksLikeTelegram,
  platformFromUA,
  readTracking
} from './join';

describe('список вузов', () => {
  // Вторая половина связки с сервером: тот же набор кодов перечислен в
  // backend/internal/join/join.go и закреплён там TestUniversityCodes.
  // Разъехавшись, стороны дают тихий отказ — человек выбирает вуз, которого
  // сервер не знает, и теряется на валидации. Правка одной стороны обязана
  // уронить её тест, а не всплыть на платном трафике.
  it('набор кодов совпадает с серверным', () => {
    expect([...UNIVERSITIES.map((u) => u.code)].sort()).toEqual([
      'bmstu',
      'finu',
      'hse',
      'mgimo',
      'mipt',
      'msu',
      'other',
      'plekhanov',
      'ranepa'
    ]);
  });

  it('у каждого вуза есть подпись, и «Другой» стоит последним', () => {
    for (const u of UNIVERSITIES) expect(u.label.trim()).not.toBe('');
    expect(UNIVERSITIES.at(-1)?.code).toBe('other');
  });

  it('курсы покрывают 1–6 и выпускника', () => {
    expect(COURSES.map((c) => c.code)).toEqual(['1', '2', '3', '4', '5', '6', 'graduate']);
  });
});

describe('юзернейм телеграма', () => {
  it('принимает те формы, которые люди реально вставляют', () => {
    for (const raw of [
      'ivanov_hse',
      '@ivanov_hse',
      'IVANOV_HSE',
      't.me/ivanov_hse',
      'https://t.me/ivanov_hse',
      'https://t.me/ivanov_hse?start=1',
      'https://telegram.me/ivanov_hse/',
      // Этот префикс принимает сервер (webreg/validate.go) — клиент обязан
      // не подсвечивать его красным.
      'https://telegram.dog/ivanov_hse'
    ]) {
      expect(looksLikeTelegram(raw), raw).toBe(true);
    }
  });

  it('отвергает то, что юзернеймом не является', () => {
    for (const raw of ['', '   ', 'иванов', 'Иван Петров', '@ivan', 'ivan-ov', '@' + 'a'.repeat(40)]) {
      expect(looksLikeTelegram(raw), raw).toBe(false);
    }
  });
});

describe('метки кампании', () => {
  it('снимаются из query и обрезаются', () => {
    const url = new URL(
      'https://vshage.app/join?utm_source=yandex&utm_medium=cpc&utm_campaign=join_msk&yclid=123&foo=bar'
    );
    expect(readTracking(url)).toEqual({
      utm_source: 'yandex',
      utm_medium: 'cpc',
      utm_campaign: 'join_msk',
      yclid: '123'
    });
  });

  it('пустой query даёт пустой набор, а не поля с пустыми строками', () => {
    // До базы это различие НЕ доезжает и не должно: скрытые поля формы всё
    // равно отправляют '', а `NULLIF` в миграции 019 кладёт NULL — то есть
    // «прямой заход» и «метка потерялась» в таблице неразличимы, и это
    // осознанно (различать их нечем: клик без метки выглядит так же).
    // Проверка здесь про другое — что в объект не попадают ключи-пустышки,
    // иначе `Object.keys(tracking).length` перестал бы отвечать на вопрос
    // «пришёл ли человек по кампании» в самом коде страницы.
    expect(readTracking(new URL('https://vshage.app/join'))).toEqual({});
  });
});

describe('платформа для кнопки на странице «спасибо»', () => {
  it('айфон и айпад — App Store', () => {
    expect(platformFromUA('Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)')).toBe('ios');
    expect(platformFromUA('Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X)')).toBe('ios');
  });

  it('андроид и десктоп — афиша', () => {
    expect(platformFromUA('Mozilla/5.0 (Linux; Android 14; Pixel 8)')).toBe('other');
    expect(platformFromUA('Mozilla/5.0 (Windows NT 10.0; Win64; x64)')).toBe('other');
  });

  it('мак и пустой UA — обе ссылки', () => {
    // iPadOS 13+ представляется Macintosh: отличить его от настоящего мака
    // на сервере нечем, поэтому обе кнопки, а не угадывание.
    expect(platformFromUA('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)')).toBe('unknown');
    expect(platformFromUA('')).toBe('unknown');
    expect(platformFromUA(null)).toBe('unknown');
  });
});
