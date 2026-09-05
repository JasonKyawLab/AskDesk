package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Analytics runs read-only aggregate queries over the conversation log for the
// admin dashboard: answer rate, most-asked questions, and busy times.
type Analytics struct {
	pool *pgxpool.Pool
}

// NewAnalytics constructs an Analytics store.
func NewAnalytics(pool *pgxpool.Pool) *Analytics {
	return &Analytics{pool: pool}
}

// AnswerStats is the answered/unanswered breakdown over a window.
type AnswerStats struct {
	Total      int
	Answered   int
	Unanswered int
}

// AnsweredPct returns the answered share as a 0-100 percentage (0 when empty).
func (s AnswerStats) AnsweredPct() int {
	if s.Total == 0 {
		return 0
	}
	return int(float64(s.Answered)*100/float64(s.Total) + 0.5)
}

// QuestionCount is one distinct question and how often it was asked.
type QuestionCount struct {
	Question   string
	Count      int
	Unanswered int // of Count, how many the AI could not answer
}

// HourBucket is a conversation count for one hour of the day (0-23, UTC).
type HourBucket struct {
	Hour  int
	Count int
}

// DayBucket is a conversation count for one day of the week (0=Sunday..6, UTC).
type DayBucket struct {
	Weekday int
	Count   int
}

// windowClause limits to the last `days` days (0 = all time).
func windowClause(days int) string {
	if days <= 0 {
		return ""
	}
	return " AND created_at >= now() - make_interval(days => $2)"
}

func (a *Analytics) args(businessID int64, days int) []any {
	if days <= 0 {
		return []any{businessID}
	}
	return []any{businessID, days}
}

// AnswerRate returns the answered/unanswered totals over the last `days` days
// (0 = all time).
func (a *Analytics) AnswerRate(ctx context.Context, businessID int64, days int) (AnswerStats, error) {
	q := `
		SELECT count(*),
		       count(*) FILTER (WHERE was_answered),
		       count(*) FILTER (WHERE NOT was_answered)
		FROM conversations
		WHERE business_id = $1` + windowClause(days)

	var s AnswerStats
	if err := a.pool.QueryRow(ctx, q, a.args(businessID, days)...).Scan(&s.Total, &s.Answered, &s.Unanswered); err != nil {
		return AnswerStats{}, fmt.Errorf("answer rate: %w", err)
	}
	return s, nil
}

// TopQuestions returns the most-frequently-asked questions over the last `days`
// days (0 = all time). When onlyUnanswered is true, only questions the AI could
// not answer are counted — the FAQ gaps worth filling.
func (a *Analytics) TopQuestions(ctx context.Context, businessID int64, days, limit int, onlyUnanswered bool) ([]QuestionCount, error) {
	filter := ""
	if onlyUnanswered {
		filter = " AND NOT was_answered"
	}
	// $2 is days when windowed; limit is always the last positional arg.
	q := `
		SELECT question, count(*) AS n,
		       count(*) FILTER (WHERE NOT was_answered) AS unanswered
		FROM conversations
		WHERE business_id = $1` + windowClause(days) + filter + `
		GROUP BY question
		ORDER BY n DESC, unanswered DESC
		LIMIT ` + limitParam(days)

	args := append(a.args(businessID, days), limit)
	rows, err := a.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("top questions: %w", err)
	}
	defer rows.Close()

	var out []QuestionCount
	for rows.Next() {
		var it QuestionCount
		if err := rows.Scan(&it.Question, &it.Count, &it.Unanswered); err != nil {
			return nil, fmt.Errorf("scan question: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// BusyHours returns conversation counts per hour of day (0-23, UTC) over the
// last `days` days (0 = all time). Hours with no activity are omitted.
func (a *Analytics) BusyHours(ctx context.Context, businessID int64, days int) ([]HourBucket, error) {
	q := `
		SELECT extract(hour from created_at)::int AS h, count(*)
		FROM conversations
		WHERE business_id = $1` + windowClause(days) + `
		GROUP BY h ORDER BY h`
	rows, err := a.pool.Query(ctx, q, a.args(businessID, days)...)
	if err != nil {
		return nil, fmt.Errorf("busy hours: %w", err)
	}
	defer rows.Close()

	var out []HourBucket
	for rows.Next() {
		var b HourBucket
		if err := rows.Scan(&b.Hour, &b.Count); err != nil {
			return nil, fmt.Errorf("scan hour: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BusyDays returns conversation counts per weekday (0=Sunday..6, UTC) over the
// last `days` days (0 = all time). Days with no activity are omitted.
func (a *Analytics) BusyDays(ctx context.Context, businessID int64, days int) ([]DayBucket, error) {
	q := `
		SELECT extract(dow from created_at)::int AS d, count(*)
		FROM conversations
		WHERE business_id = $1` + windowClause(days) + `
		GROUP BY d ORDER BY d`
	rows, err := a.pool.Query(ctx, q, a.args(businessID, days)...)
	if err != nil {
		return nil, fmt.Errorf("busy days: %w", err)
	}
	defer rows.Close()

	var out []DayBucket
	for rows.Next() {
		var b DayBucket
		if err := rows.Scan(&b.Weekday, &b.Count); err != nil {
			return nil, fmt.Errorf("scan day: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// limitParam returns the positional placeholder for LIMIT: $2 with no window,
// $3 when the window already consumed $2.
func limitParam(days int) string {
	if days <= 0 {
		return "$2"
	}
	return "$3"
}
