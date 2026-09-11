package events

// NotEndedSQL uses an explicit end when available. Without one, retain the
// event through its Moscow calendar day rather than inventing a duration or
// treating its start as its end. Inputs are trusted SQL column/parameter names.
func NotEndedSQL(start, end, at string) string {
	return "((" + end + " IS NOT NULL AND " + end + " > " + at + "::timestamptz) OR (" + end + " IS NULL AND " + start +
		" >= (date_trunc('day', " + at + "::timestamptz AT TIME ZONE 'Europe/Moscow') AT TIME ZONE 'Europe/Moscow')))"
}
