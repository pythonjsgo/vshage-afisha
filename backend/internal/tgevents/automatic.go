package tgevents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

const automaticActor = "pipeline:auto-events"

type AutomaticDecision struct {
	ID      string `json:"id"`
	Publish bool   `json:"publish"`
	Reason  string `json:"reason"`
}

type AutomaticResult struct {
	Published []string `json:"published"`
	Excluded  []string `json:"excluded"`
	Protected []string `json:"protected"`
	Missing   []string `json:"missing"`
	Unchanged []string `json:"unchanged"`
}

// AutomaticDecisions consumes reviewed records, never a blanket source flag.
// Row locks make a concurrent human rejection authoritative. Only this writer's
// own exclusions can be reversed automatically; old unreviewed queues can publish.
func (r *Repository) AutomaticDecisions(ctx context.Context, decisions []AutomaticDecision) (AutomaticResult, error) {
	out := AutomaticResult{[]string{}, []string{}, []string{}, []string{}, []string{}}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ordered := append([]AutomaticDecision(nil), decisions...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	for _, d := range ordered {
		var st AdminState
		var by *string
		err := tx.QueryRow(ctx, `SELECT id, feed, anchor, hidden, featured, featured_until,
			listed, hide_reason, hidden_by FROM afisha_tg_events WHERE id=$1 FOR UPDATE`, d.ID).
			Scan(&st.ID, &st.Feed, &st.Anchor, &st.Hidden, &st.Featured, &st.FeaturedUntil, &st.Listed, &st.HideReason, &by)
		if errors.Is(err, pgx.ErrNoRows) {
			out.Missing = append(out.Missing, d.ID)
			continue
		}
		if err != nil {
			return out, err
		}
		owned := by != nil && *by == automaticActor
		if !owned && (!st.Listed || st.HideReason != nil || by != nil) {
			out.Protected = append(out.Protected, d.ID)
			continue
		}
		reason := "auto:" + d.Reason
		if d.Publish && st.Feed && st.Listed && !st.Hidden && st.HideReason == nil ||
			!d.Publish && owned && !st.Feed && !st.Listed && st.HideReason != nil && *st.HideReason == reason {
			out.Unchanged = append(out.Unchanged, d.ID)
			continue
		}
		// Exclusion removes the card from discovery; existing direct links survive.
		// Legacy queued cards stay hidden if rejected, as they never had a public link.
		_, err = tx.Exec(ctx, `UPDATE afisha_tg_events SET feed=$2, listed=$2,
			hidden=CASE WHEN $2 THEN false ELSE hidden END,
			hide_reason=CASE WHEN $2 THEN NULL ELSE $3 END,
			hidden_by=CASE WHEN $2 THEN NULL ELSE $4 END,
			hidden_at=CASE WHEN $2 THEN NULL ELSE NOW() END, updated_at=NOW() WHERE id=$1`,
			d.ID, d.Publish, reason, automaticActor)
		if err != nil {
			return out, err
		}
		st.Feed, st.Listed = d.Publish, d.Publish
		if d.Publish {
			st.Hidden, st.HideReason = false, nil
		} else {
			st.HideReason = &reason
		}
		f := AdminFlags{Actor: automaticActor, Feed: &d.Publish, Listed: &d.Publish, HideReason: &reason}
		if err := logCuration(ctx, tx, d.ID, f, st); err != nil {
			return out, err
		}
		if d.Publish {
			out.Published = append(out.Published, d.ID)
		} else {
			out.Excluded = append(out.Excluded, d.ID)
		}
	}
	return out, tx.Commit(ctx)
}

func (h *Handler) AdminAutomatic(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	var in struct {
		Decisions []struct {
			ID      string `json:"id"`
			Publish *bool  `json:"publish"`
			Reason  string `json:"reason"`
		} `json:"decisions"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || len(in.Decisions) == 0 || len(in.Decisions) > 500 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expected 1..500 automatic decisions"})
		return
	}
	seen := map[string]bool{}
	decisions := make([]AutomaticDecision, 0, len(in.Decisions))
	for _, d := range in.Decisions {
		if !idRe.MatchString(d.ID) || seen[d.ID] || d.Publish == nil || strings.TrimSpace(d.Reason) == "" || len(d.Reason) > 160 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid or duplicate decision: %s", d.ID)})
			return
		}
		if *d.Publish != (d.Reason == "eligible") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "publish and reason disagree"})
			return
		}
		seen[d.ID] = true
		decisions = append(decisions, AutomaticDecision{d.ID, *d.Publish, d.Reason})
	}
	out, err := h.repo.AutomaticDecisions(r.Context(), decisions)
	if err != nil {
		log.Printf("tgevents.AutomaticDecisions: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "automatic publication failed"})
		return
	}
	log.Printf("tgevents.AutomaticDecisions: published=%d excluded=%d protected=%d missing=%d unchanged=%d", len(out.Published), len(out.Excluded), len(out.Protected), len(out.Missing), len(out.Unchanged))
	writeJSON(w, http.StatusOK, out)
}
