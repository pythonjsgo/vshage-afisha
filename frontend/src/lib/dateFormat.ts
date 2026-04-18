const MONTHS_SHORT = ['ЯНВ','ФЕВ','МАР','АПР','МАЯ','ИЮН','ИЮЛ','АВГ','СЕН','ОКТ','НОЯ','ДЕК'];

function sameDay(a: Date, b: Date) {
  return a.getFullYear() === b.getFullYear()
    && a.getMonth() === b.getMonth()
    && a.getDate() === b.getDate();
}

export function formatEventDate(iso: string, now = new Date()): string {
  const d = new Date(iso);
  const tomorrow = new Date(now); tomorrow.setDate(now.getDate() + 1);
  const h = d.getHours().toString().padStart(2, '0');
  const m = d.getMinutes().toString().padStart(2, '0');
  if (sameDay(d, now)) return `СЕГОДНЯ В ${h}:${m}`;
  if (sameDay(d, tomorrow)) return `ЗАВТРА В ${h}:${m}`;
  return `${d.getDate()} ${MONTHS_SHORT[d.getMonth()]} · ${h}:${m}`;
}

export function formatEventDateLong(iso: string): string {
  const d = new Date(iso);
  const day = d.getDate();
  const mon = MONTHS_SHORT[d.getMonth()];
  const h = d.getHours().toString().padStart(2, '0');
  const m = d.getMinutes().toString().padStart(2, '0');
  return `${day} ${mon} · ${h}:${m}`;
}
