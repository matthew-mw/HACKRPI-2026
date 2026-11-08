package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

const eventAudience = `(e.visibility='public' OR o.owner_id=$1 OR e.created_by=$1 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1 AND m.status='active'))`
const eventManager = `COALESCE((o.owner_id=$1 OR e.created_by=$1 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1 AND m.role IN('owner','admin') AND m.status='active')),false)`

const myEventsQuery = `SELECT COALESCE(jsonb_agg(row ORDER BY starts_at DESC,id),'[]'::jsonb) FROM (
 SELECT e.id,e.title,e.description,e.starts_at,e.ends_at,e.visibility,e.status,e.completed_at,e.actual_started_at,e.actual_ended_at,
 o.name AS organization_name,l.label AS location_label,l.city,l.address_line1 AS address,
 ` + eventManager + ` AS can_manage,
 EXISTS(SELECT 1 FROM event_registrations r WHERE r.event_id=e.id AND r.user_id=$1 AND r.status='registered') AS joined,
 (SELECT count(*) FROM event_registrations r WHERE r.event_id=e.id AND r.status='registered') AS registrations,
 (SELECT count(*) FROM event_registrations r WHERE r.event_id=e.id AND r.participated) AS participants
 FROM events e JOIN organizations o ON o.id=e.organization_id JOIN locations l ON l.id=e.location_id
 WHERE ` + eventManager + ` OR EXISTS(SELECT 1 FROM event_registrations r WHERE r.event_id=e.id AND r.user_id=$1)
) row`

func (p product) completionDetails(ctx context.Context, uid, id string) (any, error) {
	if err := idValid(id); err != nil {
		return nil, err
	}
	var allowed bool
	err := p.db.QueryRowContext(ctx, `SELECT `+eventManager+` FROM events e JOIN organizations o ON o.id=e.organization_id WHERE e.id=$2`, uid, id).Scan(&allowed)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, denied()
	}
	return p.json(ctx, `SELECT jsonb_build_object('id',e.id,'title',e.title,'starts_at',e.starts_at,'ends_at',e.ends_at,'completed_at',e.completed_at,
 'attendees',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',attendee.id,'name',attendee.name,'registered',attendee.registered) ORDER BY attendee.name) FROM (
 SELECT u.id,u.first_name||' '||u.last_name AS name,true AS registered FROM event_registrations r JOIN users u ON u.id=r.user_id WHERE r.event_id=e.id AND r.status='registered'
 UNION ALL SELECT u.id,u.first_name||' '||u.last_name,false FROM users u WHERE u.id=$2 AND NOT EXISTS(SELECT 1 FROM event_registrations r WHERE r.event_id=e.id AND r.user_id=u.id AND r.status='registered')
 ) attendee),'[]'::jsonb)) FROM events e WHERE e.id=$1`, id, uid)
}

func (p product) eventDetails(ctx context.Context, uid, id string) (any, error) {
	if err := idValid(id); err != nil {
		return nil, err
	}
	var user any
	if uid != "" {
		user = uid
	}
	return p.json(ctx, `SELECT jsonb_build_object('id',e.id,'title',e.title,'description',e.description,'starts_at',e.starts_at,'ends_at',e.ends_at,
 'status',e.status,'visibility',e.visibility,'completed_at',e.completed_at,'organization_name',o.name,'location_label',l.label,'city',l.city,'address',l.address_line1,
 'can_manage',`+eventManager+`,
 'joined',EXISTS(SELECT 1 FROM event_registrations r WHERE r.event_id=e.id AND r.user_id=$1 AND r.status='registered'),
 'spots',GREATEST(0,e.capacity-(SELECT count(*) FROM event_registrations r WHERE r.event_id=e.id AND r.status='registered')),
 'causes',COALESCE((SELECT jsonb_agg(t.name) FROM event_tags et JOIN tags t ON t.id=et.tag_id WHERE et.event_id=e.id AND t.category='cause'),'[]'::jsonb))
 FROM events e JOIN organizations o ON o.id=e.organization_id JOIN locations l ON l.id=e.location_id
 WHERE e.id=$2 AND (e.status='published' OR `+eventManager+`) AND `+eventAudience, user, id)
}

func (p product) completeEvent(w http.ResponseWriter, r *http.Request, uid string) (any, error) {
	var in struct {
		EventID        string    `json:"event_id"`
		ParticipantIDs []string  `json:"participant_ids"`
		StartedAt      time.Time `json:"started_at"`
		EndedAt        time.Time `json:"ended_at"`
	}
	if err := decodeProduct(w, r, &in); err != nil {
		return nil, err
	}
	if err := idValid(in.EventID); err != nil {
		return nil, err
	}
	if len(in.ParticipantIDs) > 10000 {
		return nil, invalid("Too many participants.")
	}
	ids := map[string]bool{}
	for _, id := range in.ParticipantIDs {
		if err := idValid(id); err != nil {
			return nil, err
		}
		if ids[id] {
			return nil, invalid("A participant was selected more than once.")
		}
		ids[id] = true
	}
	tx, err := p.db.BeginTx(r.Context(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var allowed bool
	var completed sql.NullTime
	var start time.Time
	var status string
	err = tx.QueryRowContext(r.Context(), `SELECT `+eventManager+`,e.completed_at,e.starts_at,e.status FROM events e JOIN organizations o ON o.id=e.organization_id WHERE e.id=$2 FOR UPDATE OF e`, uid, in.EventID).Scan(&allowed, &completed, &start, &status)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, denied()
	}
	if completed.Valid {
		return map[string]any{"ok": true, "already_completed": true}, nil
	}
	if status != "published" {
		return nil, conflict("Only a published event can be completed.")
	}
	now := time.Now()
	duration := in.EndedAt.Sub(in.StartedAt)
	if start.After(now) || in.StartedAt.Before(start) || in.StartedAt.IsZero() || in.EndedAt.After(now) || duration <= 0 || duration > 24*time.Hour {
		return nil, invalid("Confirm an actual service period after the event starts, ending now or earlier, of up to 24 hours.")
	}
	rows, err := tx.QueryContext(r.Context(), `SELECT user_id FROM event_registrations WHERE event_id=$1 AND status='registered'`, in.EventID)
	if err != nil {
		return nil, err
	}
	registered := map[string]bool{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		registered[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for id := range ids {
		if !registered[id] && id != uid {
			return nil, invalid("Only registered participants can be credited. Refresh the attendance list.")
		}
	}
	if ids[uid] && !registered[uid] {
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO event_registrations(event_id,user_id,status) VALUES($1,$2,'registered') ON CONFLICT(event_id,user_id) DO UPDATE SET status='registered'`, in.EventID, uid); err != nil {
			return nil, err
		}
	}
	// Organizer-confirmed attendance is authoritative. Keep previous submissions for audit,
	// but exclude them from totals and feeds so service cannot be counted twice.
	if _, err = tx.ExecContext(r.Context(), `UPDATE activities SET verification_status='rejected',superseded_by_completion=true WHERE event_id=$1 AND NOT completion_generated`, in.EventID); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE event_registrations SET participated=false WHERE event_id=$1`, in.EventID); err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		selected, _ := json.Marshal(in.ParticipantIDs)
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO activities(user_id,event_id,started_at,ended_at,verification_status,verified_by,completion_generated)
  SELECT user_id,event_id,$3,$4,'verified',$5,true FROM event_registrations WHERE event_id=$1 AND status='registered' AND user_id::text IN(SELECT jsonb_array_elements_text($2::jsonb))`, in.EventID, string(selected), in.StartedAt, in.EndedAt, uid); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(r.Context(), `UPDATE event_registrations SET participated=true WHERE event_id=$1 AND user_id::text IN(SELECT jsonb_array_elements_text($2::jsonb))`, in.EventID, string(selected)); err != nil {
			return nil, err
		}
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE events SET completed_at=now(),completed_by=$2,actual_started_at=$3,actual_ended_at=$4 WHERE id=$1`, in.EventID, uid, in.StartedAt, in.EndedAt); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "participants_credited": len(ids), "hours_each": duration.Hours()}, nil
}
