import { describe, it, expect } from 'vitest';
import { metrikaId } from '../../src/lib/analytics';

describe('номер счётчика Метрики', () => {
  it('принимает настоящий номер и обрезает пробелы', () => {
    expect(metrikaId('112367587')).toBe('112367587');
    expect(metrikaId('  112367587 \n')).toBe('112367587');
  });

  it('пусто и не задано означают «счётчика нет» — это состояние стенда', () => {
    for (const x of ['', '   ', undefined, null]) expect(metrikaId(x)).toBe('');
  });

  // Номер уезжает ВНУТРЬ тела <script>, поэтому всё, что не цифра, оттуда
  // исполнится в браузере каждого посетителя. Значение из нашего же .env,
  // но опечатка не должна становиться исполняемым кодом.
  it('всё, что не цифра, отбрасывает целиком', () => {
    for (const x of [
      '1);alert(1);//',
      "1','x');ym(1,'init'",
      '112367587<\/script><script>alert(1)<\/script>',
      'id=112367587',
      '11 23',
      '-1',
      '1e5',
      '0x10'
    ]) {
      expect(metrikaId(x)).toBe('');
    }
  });

  it('не пускает номер длиннее разумного', () => {
    expect(metrikaId('1'.repeat(12))).toBe('1'.repeat(12));
    expect(metrikaId('1'.repeat(13))).toBe('');
  });
});
