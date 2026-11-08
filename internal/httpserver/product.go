package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/matthew-mw/HACKRPI-2026/internal/api"
	"github.com/matthew-mw/HACKRPI-2026/internal/auth"
)

type product struct{ db *sql.DB }

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type productError struct {
	status  int
	message string
}

func (e *productError) Error() string { return e.message }
func invalid(message string) error    { return &productError{400, message} }
func denied() error                   { return &productError{403, "You do not have permission to do that."} }
func missing(message string) error    { return &productError{404, message} }
func conflict(message string) error   { return &productError{409, message} }
func idValid(id string) error {
	if !uuidPattern.MatchString(id) {
		return invalid("Invalid record ID.")
	}
	return nil
}

func productFailure(w http.ResponseWriter, err error) {
	var pe *productError
	if errors.As(err, &pe) {
		api.Error(w, pe.status, pe.message)
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		api.Error(w, 404, "Record not found.")
		return
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		if pg.Code == "23505" {
			api.Error(w, 409, "That record already exists.")
			return
		}
		if pg.Code == "23503" {
			api.Error(w, 400, "A referenced record is unavailable.")
			return
		}
		if pg.Code == "23514" || pg.Code == "22007" || pg.Code == "22008" {
			api.Error(w, 400, "Please check the entered values.")
			return
		}
		log.Printf("product database error: code=%s constraint=%s", pg.Code, pg.ConstraintName)
	} else {
		log.Printf("product request error: %T", err)
	}
	api.Error(w, 503, "The service is temporarily unavailable. Please try again.")
}
func decodeProduct(w http.ResponseWriter, r *http.Request, value any) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return &productError{415, "Content-Type must be application/json"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(value); err != nil {
		return invalid("Invalid request body.")
	}
	if d.Decode(new(any)) != io.EOF {
		return invalid("Expected a single JSON object.")
	}
	return nil
}
func (p product) json(ctx context.Context, q string, args ...any) (json.RawMessage, error) {
	var b []byte
	err := p.db.QueryRowContext(ctx, q, args...).Scan(&b)
	return json.RawMessage(b), err
}

func (p product) serve(w http.ResponseWriter, r *http.Request) {
	if p.db == nil {
		api.Error(w, 503, "Database unavailable.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	uid, _ := auth.UserID(ctx)
	var user any
	if uid != "" {
		user = uid
	}
	path := r.URL.Path
	var result any
	var err error
	if r.Method == "GET" || r.Method == "HEAD" {
		switch path {
		case "/api/community", "/api/dashboard":
			out := map[string]any{}
			queries := map[string]string{"feed": feedQuery, "crews": crewListQuery}
			if uid != "" {
				queries["profile"] = profileQuery
				queries["stats"] = statsQuery
				queries["upcoming"] = upcomingQuery
				queries["invitations"] = invitationsQuery
				queries["my_activity"] = strings.Replace(feedQuery, "WHERE "+visibleActivity, "WHERE a.user_id=$1", 1)
				queries["registrations"] = `SELECT COALESCE(jsonb_agg(row ORDER BY starts_at DESC),'[]'::jsonb) FROM (SELECT e.id,e.title,e.starts_at,e.ends_at,e.status,r.status AS registration_status FROM event_registrations r JOIN events e ON e.id=r.event_id WHERE r.user_id=$1 AND r.status='registered') row`
				queries["organizations"] = `SELECT COALESCE(jsonb_agg(jsonb_build_object('id',id,'name',name)),'[]'::jsonb) FROM organizations o WHERE (o.organization_type<>'charity' OR o.approval_status='approved') AND (owner_id=$1 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1 AND m.status='active' AND (m.role IN('owner','admin') OR o.organization_type='charity')))`
				queries["my_events"] = myEventsQuery
				queries["charities"] = `SELECT COALESCE(jsonb_agg(row),'[]'::jsonb) FROM (SELECT o.id,o.name,o.description,o.image_url,o.website,o.visibility,o.approval_status,(SELECT city FROM locations WHERE id=o.location_id) AS city,(SELECT count(*) FROM organization_members WHERE organization_id=o.id AND status='active') AS members,EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1 AND m.status='active') AS joined FROM organizations o WHERE o.organization_type='charity' AND (o.approval_status='approved' OR o.owner_id=$1 OR EXISTS(SELECT 1 FROM users WHERE id=$1 AND is_site_admin))) row`
			}
			scope := r.URL.Query().Get("page")
			fields, scoped := pageDataFields[scope]
			if scope != "" && !scoped {
				api.Error(w, 400, "Unknown page.")
				return
			}
			for key, q := range queries {
				if scoped && !strings.Contains(" "+fields+" ", " "+key+" ") {
					continue
				}

				var value json.RawMessage
				value, err = p.json(ctx, q, user)
				if err != nil {
					break
				}
				out[key] = value
			}
			result = out
		case "/api/location":
			result, err = lookupArea(ctx, r.URL.Query().Get("q"))
		case "/api/opportunities":
			result, err = p.opportunities(r, user)
		case "/api/my-events":
			result, err = p.json(ctx, myEventsQuery, user)
		case "/api/event-attendance":
			result, err = p.completionDetails(ctx, uid, r.URL.Query().Get("id"))
		case "/api/event-details":
			result, err = p.eventDetails(ctx, uid, r.URL.Query().Get("id"))
		case "/api/people":
			result, err = p.json(ctx, peopleQuery, user, strings.TrimSpace(r.URL.Query().Get("q")))
		case "/api/leaderboard":
			period := r.URL.Query().Get("period")
			kind := r.URL.Query().Get("type")
			if period != "week" && period != "month" && period != "all" {
				err = invalid("Choose week, month, or all.")
			} else if kind == "crews" || kind == "organizations" {
				orgType := "volunteer_group"
				if kind == "organizations" {
					orgType = "company"
				}
				result, err = p.json(ctx, groupLeaderboardQuery, user, period, orgType)
			} else if kind != "" && kind != "individuals" {
				err = invalid("Choose individuals, crews, or organizations.")
			} else {
				result, err = p.json(ctx, leaderboardQuery, user, period)
			}
		case "/api/review":
			result, err = p.json(ctx, `SELECT COALESCE(jsonb_agg(row ORDER BY created_at),'[]'::jsonb) FROM (SELECT a.id,a.caption,a.started_at,a.ended_at,a.created_at,u.first_name||' '||u.last_name AS name,e.title FROM activities a JOIN users u ON u.id=a.user_id JOIN events e ON e.id=a.event_id JOIN organizations o ON o.id=e.organization_id WHERE a.verification_status='pending' AND a.user_id<>$1 AND (o.owner_id=$1 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1 AND m.status='active' AND m.role IN('owner','admin')))) row`, user)
		case "/api/crew":
			result, err = p.crew(ctx, uid, r.URL.Query().Get("id"))
		default:
			api.Error(w, 404, "Route not found.")
			return
		}
	} else if r.Method == "POST" {
		if uid == "" {
			api.Error(w, 401, "Sign in to continue.")
			return
		}
		switch path {
		case "/api/charity-approval":
			err = p.approveCharity(w, r, uid)
		case "/api/profile":
			err = p.updateProfile(w, r, uid)
		case "/api/registration":
			err = p.registration(w, r, uid)
		case "/api/save":
			err = p.saveEvent(w, r, uid)
		case "/api/activity":
			err = p.activity(w, r, uid)
		case "/api/reaction":
			err = p.reaction(w, r, uid)
		case "/api/comment":
			err = p.comment(w, r, uid)
		case "/api/organization", "/api/charity":
			result, err = p.organization(w, r, uid)
		case "/api/membership":
			err = p.membership(w, r, uid)
		case "/api/invite":
			err = p.invite(w, r, uid)
		case "/api/follow":
			err = p.follow(w, r, uid)
		case "/api/challenge":
			err = p.challenge(w, r, uid)
		case "/api/event":
			result, err = p.createEvent(w, r, uid)
		case "/api/event-complete":
			result, err = p.completeEvent(w, r, uid)
		case "/api/verify":
			err = p.verify(w, r, uid)
		default:
			api.Error(w, 404, "Route not found.")
			return
		}
		if result == nil {
			result = map[string]bool{"ok": true}
		}
	} else {
		w.Header().Set("Allow", "GET, HEAD, POST")
		api.Error(w, 405, "Method not allowed.")
		return
	}
	if err != nil {
		productFailure(w, err)
		return
	}
	api.JSON(w, 200, result)
}

func (p product) opportunities(r *http.Request, user any) (json.RawMessage, error) {
	q := r.URL.Query()
	near := q.Get("near") == "true"
	if near && user == nil {
		return nil, &productError{401, "Sign in to find nearby opportunities."}
	}
	var from, to any
	var start, end time.Time
	for _, key := range []string{"from", "to"} {
		if s := q.Get(key); s != "" {
			t, e := time.Parse(time.RFC3339, s)
			if e != nil {
				return nil, invalid("Invalid date filter.")
			}
			if key == "from" {
				from = t
				start = t
			} else {
				to = t
				end = t
			}
		}
	}
	if !start.IsZero() && !end.IsZero() && !end.After(start) {
		return nil, invalid("The end filter must be after the start filter.")
	}
	var weekday, distance any
	if s := q.Get("weekday"); s != "" {
		n, e := strconv.Atoi(s)
		if e != nil || n < 0 || n > 6 {
			return nil, invalid("Invalid day filter.")
		}
		weekday = n
	}
	if s := q.Get("distance"); s != "" {
		n, e := strconv.Atoi(s)
		if e != nil || n < 1 || n > 500 {
			return nil, invalid("Distance must be between 1 and 500 km.")
		}
		distance = n
	}
	if near {
		var exists bool
		if err := p.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND location_id IS NOT NULL)`, user).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, &productError{422, "Add your location in Preferences to find nearby opportunities."}
		}
	}
	return p.json(r.Context(), `SELECT COALESCE(jsonb_agg(row ORDER BY starts_at,id),'[]'::jsonb) FROM (
 SELECT e.id,e.title,e.description,e.image_url,e.starts_at,e.ends_at,e.capacity,e.visibility,o.id AS organization_id,o.name AS organization_name,l.label AS location_label,l.city,l.address_line1 AS address,
 CASE WHEN ul.id IS NOT NULL THEN round((ST_Distance(l.coordinates,ul.coordinates)/1000.0)::numeric,1) END AS distance_km,
 GREATEST(0,e.capacity-(SELECT count(*) FROM event_registrations er WHERE er.event_id=e.id AND er.status='registered')) AS spots,
 EXISTS(SELECT 1 FROM event_registrations er WHERE er.event_id=e.id AND er.user_id=$1 AND er.status='registered') AS joined,
 EXISTS(SELECT 1 FROM saved_events se WHERE se.event_id=e.id AND se.user_id=$1) AS saved,
 (SELECT count(*) FROM event_registrations er JOIN following f ON f.following_id=er.user_id AND f.follower_id=$1 WHERE er.event_id=e.id AND er.status='registered') AS friends,
 COALESCE((SELECT jsonb_agg(t.name) FROM event_tags et JOIN tags t ON t.id=et.tag_id WHERE et.event_id=e.id AND t.category='cause'),'[]'::jsonb) AS causes
 FROM events e JOIN organizations o ON o.id=e.organization_id JOIN locations l ON l.id=e.location_id
 LEFT JOIN users u ON u.id=$1::uuid LEFT JOIN locations ul ON ul.id=u.location_id
 WHERE e.status='published' AND e.completed_at IS NULL AND e.starts_at>=now()
 AND (e.visibility='public' OR o.owner_id=$1 OR e.created_by=$1 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1 AND m.status='active'))
 AND (NOT $2::boolean OR ST_DWithin(l.coordinates,ul.coordinates,LEAST(u.max_distance_km,COALESCE($8::integer,u.max_distance_km))*1000.0))
 AND (e.title||' '||o.name||' '||l.city||' '||COALESCE(l.label,'')||' '||COALESCE(l.address_line1,'')) ILIKE '%'||$3||'%'
 AND ($4::text='' OR EXISTS(SELECT 1 FROM event_tags et JOIN tags t ON t.id=et.tag_id WHERE et.event_id=e.id AND t.name=$4 AND t.category='cause'))
 AND ($5::timestamptz IS NULL OR e.starts_at>=$5) AND ($6::timestamptz IS NULL OR e.ends_at<=$6)
 AND ($7::integer IS NULL OR EXTRACT(DOW FROM e.starts_at AT TIME ZONE 'America/New_York')=$7)
 ORDER BY e.starts_at,e.id LIMIT 200
 ) row`, user, near, strings.TrimSpace(q.Get("q")), q.Get("cause"), from, to, weekday, distance)
}

type profileInput struct {
	FirstName          string `json:"first_name"`
	LastName           string `json:"last_name"`
	Bio                string `json:"bio"`
	MaxDistance        int    `json:"max_distance_km"`
	ActivityVisibility string `json:"activity_visibility"`
	HoursVisibility    string `json:"hours_visibility"`
	WithFriends        bool   `json:"volunteer_with_friends"`
	ClearLocation      bool   `json:"clear_location"`
	Location           *struct {
		Address   string  `json:"address"`
		Label     string  `json:"label"`
		City      string  `json:"city"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"location"`
	Tags []struct {
		Name     string `json:"name"`
		Category string `json:"category"`
	} `json:"tags"`
	Availability []struct {
		Weekday  int    `json:"weekday"`
		Start    string `json:"start_time"`
		End      string `json:"end_time"`
		Timezone string `json:"timezone"`
	} `json:"availability"`
}

func (p product) updateProfile(w http.ResponseWriter, r *http.Request, uid string) error {
	var in profileInput
	if err := decodeProduct(w, r, &in); err != nil {
		return err
	}
	in.FirstName = strings.TrimSpace(in.FirstName)
	in.LastName = strings.TrimSpace(in.LastName)
	if in.FirstName == "" || in.LastName == "" || len(in.FirstName) > 100 || len(in.LastName) > 100 || len(in.Bio) > 1000 {
		return invalid("Enter your first and last name and a bio under 1000 characters.")
	}
	validVisibility := func(s string) bool { return s == "private" || s == "friends" || s == "public" }
	if !validVisibility(in.ActivityVisibility) || !validVisibility(in.HoursVisibility) || in.MaxDistance < 1 || in.MaxDistance > 500 {
		return invalid("Check your visibility and distance settings.")
	}
	if len(in.Tags) > 30 || len(in.Availability) > 21 {
		return invalid("Too many preferences.")
	}
	var address resolvedAddress
	if in.ClearLocation && in.Location != nil {
		return invalid("Choose a new location or remove your saved location.")
	}
	if in.Location != nil && strings.TrimSpace(in.Location.Address) != "" {
		var err error
		address, err = p.resolveAddress(r.Context(), in.Location.Address)
		if err != nil {
			return err
		}
		in.Location.City = address.City
		in.Location.Latitude = address.Latitude
		in.Location.Longitude = address.Longitude
	}
	tx, err := p.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var loc any
	if in.Location != nil {
		l := in.Location
		l.City = strings.TrimSpace(l.City)
		if l.City == "" || len(l.City) > 120 || len(l.Label) > 200 || l.Latitude < -90 || l.Latitude > 90 || l.Longitude < -180 || l.Longitude > 180 {
			return invalid("Choose a city and state or ZIP code.")
		}
		var id string
		err = tx.QueryRowContext(r.Context(), `INSERT INTO locations(label,city,coordinates,address_line1,region,postal_code) VALUES($1,$2,ST_SetSRID(ST_MakePoint($3,$4),4326)::geography,$5,$6,$7) RETURNING id`, l.Label, l.City, l.Longitude, l.Latitude, address.Address, address.Region, address.PostalCode).Scan(&id)
		if err != nil {
			return err
		}
		loc = id
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE users SET first_name=$2,last_name=$3,bio=$4,max_distance_km=$5,activity_visibility=$6,hours_visibility=$7,volunteer_with_friends=$8,location_id=CASE WHEN $10::boolean THEN NULL ELSE COALESCE($9::uuid,location_id) END,updated_at=now() WHERE id=$1`, uid, in.FirstName, in.LastName, in.Bio, in.MaxDistance, in.ActivityVisibility, in.HoursVisibility, in.WithFriends, loc, in.ClearLocation)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM user_tags WHERE user_id=$1`, uid); err != nil {
		return err
	}
	for _, t := range in.Tags {
		t.Name = strings.TrimSpace(t.Name)
		if t.Name == "" || len(t.Name) > 80 || (t.Category != "interest" && t.Category != "skill" && t.Category != "cause") {
			return invalid("Invalid preference tag.")
		}
		var id string
		if err = tx.QueryRowContext(r.Context(), `INSERT INTO tags(name,category) VALUES($1,$2) ON CONFLICT(category,lower(name)) DO UPDATE SET name=tags.name RETURNING id`, t.Name, t.Category).Scan(&id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO user_tags(user_id,tag_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, uid, id); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM user_availability WHERE user_id=$1`, uid); err != nil {
		return err
	}
	for _, a := range in.Availability {
		if a.Weekday < 0 || a.Weekday > 6 {
			return invalid("Invalid available day.")
		}
		start, e1 := time.Parse("15:04", a.Start)
		end, e2 := time.Parse("15:04", a.End)
		if e1 != nil || e2 != nil || !end.After(start) {
			return invalid("Availability must end after it starts.")
		}
		if _, err = time.LoadLocation(a.Timezone); err != nil {
			return invalid("Invalid availability timezone.")
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO user_availability(user_id,weekday,start_time,end_time,timezone) VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id,weekday,start_time) DO UPDATE SET end_time=EXCLUDED.end_time,timezone=EXCLUDED.timezone`, uid, a.Weekday, a.Start, a.End, a.Timezone); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p product) registration(w http.ResponseWriter, r *http.Request, uid string) error {
	var in struct {
		EventID string `json:"event_id"`
		Join    bool   `json:"join"`
	}
	if err := decodeProduct(w, r, &in); err != nil {
		return err
	}
	if err := idValid(in.EventID); err != nil {
		return err
	}
	tx, err := p.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var capacity int
	var status, org, visibility, owner, creator string
	var completed sql.NullTime
	var start time.Time
	err = tx.QueryRowContext(r.Context(), `SELECT e.capacity,e.status,e.starts_at,o.id,e.visibility,o.owner_id,COALESCE(e.created_by::text,''),e.completed_at FROM events e JOIN organizations o ON o.id=e.organization_id WHERE e.id=$1 FOR UPDATE OF e`, in.EventID).Scan(&capacity, &status, &start, &org, &visibility, &owner, &creator, &completed)
	if err != nil {
		return err
	}
	if completed.Valid {
		return conflict("This event is completed; its attendance is finalized.")
	}
	if in.Join {
		if status != "published" || !start.After(time.Now()) {
			return conflict("This event is no longer open for registration.")
		}
		if visibility != "public" && owner != uid && creator != uid {
			var member bool
			if err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM organization_members WHERE organization_id=$1 AND user_id=$2 AND status='active')`, org, uid).Scan(&member); err != nil {
				return err
			}
			if !member {
				return denied()
			}
		}
		var count int
		var already bool
		if err = tx.QueryRowContext(r.Context(), `SELECT count(*),COALESCE(bool_or(user_id=$2),false) FROM event_registrations WHERE event_id=$1 AND status='registered'`, in.EventID, uid).Scan(&count, &already); err != nil {
			return err
		}
		if count >= capacity && !already {
			return conflict("This event is full.")
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO event_registrations(event_id,user_id,status) VALUES($1,$2,'registered') ON CONFLICT(event_id,user_id) DO UPDATE SET status='registered',registered_at=now()`, in.EventID, uid)
	} else {
		_, err = tx.ExecContext(r.Context(), `UPDATE event_registrations SET status='cancelled' WHERE event_id=$1 AND user_id=$2`, in.EventID, uid)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (p product) saveEvent(w http.ResponseWriter, r *http.Request, uid string) error {
	var in struct {
		EventID string `json:"event_id"`
		Save    bool   `json:"save"`
	}
	if err := decodeProduct(w, r, &in); err != nil {
		return err
	}
	if err := idValid(in.EventID); err != nil {
		return err
	}
	var err error
	if in.Save {
		var allowed bool
		err = p.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM events e JOIN organizations o ON o.id=e.organization_id WHERE e.id=$2 AND e.status='published' AND (e.visibility='public' OR o.owner_id=$1 OR e.created_by=$1 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1 AND m.status='active')))`, uid, in.EventID).Scan(&allowed)
		if err != nil {
			return err
		}
		if !allowed {
			return missing("Event not found.")
		}
		_, err = p.db.ExecContext(r.Context(), `INSERT INTO saved_events(user_id,event_id) SELECT $1,e.id FROM events e JOIN organizations o ON o.id=e.organization_id WHERE e.id=$2 AND e.status='published' AND (e.visibility='public' OR o.owner_id=$1 OR e.created_by=$1 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1 AND m.status='active')) ON CONFLICT DO NOTHING`, uid, in.EventID)
	} else {
		_, err = p.db.ExecContext(r.Context(), `DELETE FROM saved_events WHERE user_id=$1 AND event_id=$2`, uid, in.EventID)
	}
	return err
}

func (p product) activity(w http.ResponseWriter, r *http.Request, uid string) error {
	var in struct {
		EventID   string    `json:"event_id"`
		StartedAt time.Time `json:"started_at"`
		EndedAt   time.Time `json:"ended_at"`
		Caption   string    `json:"caption"`
	}
	if err := decodeProduct(w, r, &in); err != nil {
		return err
	}
	if err := idValid(in.EventID); err != nil {
		return err
	}
	in.Caption = strings.TrimSpace(in.Caption)
	duration := in.EndedAt.Sub(in.StartedAt)
	if in.StartedAt.IsZero() || duration <= 0 || duration > 24*time.Hour || in.EndedAt.After(time.Now()) || len(in.Caption) > 2000 {
		return invalid("Log a completed activity of up to 24 hours with a caption under 2000 characters.")
	}
	tx, err := p.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var registered bool
	var start, end time.Time
	var completed sql.NullTime
	err = tx.QueryRowContext(r.Context(), `SELECT e.starts_at,e.ends_at,EXISTS(SELECT 1 FROM event_registrations er WHERE er.event_id=e.id AND er.user_id=$2 AND er.status='registered'),e.completed_at FROM events e WHERE e.id=$1 FOR UPDATE`, in.EventID, uid).Scan(&start, &end, &registered, &completed)
	if err != nil {
		return err
	}
	if completed.Valid {
		return conflict("This event is complete. Its confirmed service hours have already been logged.")
	}
	if !registered {
		return conflict("Register for the event before logging your service.")
	}
	if in.StartedAt.Before(start) || in.EndedAt.After(end) {
		return invalid("Your logged time must fall within the event's start and end.")
	}
	var overlap bool
	err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM activities WHERE user_id=$1 AND verification_status<>'rejected' AND started_at<$3 AND COALESCE(ended_at,now())>$2)`, uid, in.StartedAt, in.EndedAt).Scan(&overlap)
	if err != nil {
		return err
	}
	if overlap {
		return conflict("You already logged activity during this time.")
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO activities(user_id,event_id,started_at,ended_at,caption) VALUES($1,$2,$3,$4,$5)`, uid, in.EventID, in.StartedAt, in.EndedAt, in.Caption)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (p product) canSeeActivity(ctx context.Context, uid, id string) (bool, error) {
	var ok bool
	err := p.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM activities a JOIN users u ON u.id=a.user_id JOIN events e ON e.id=a.event_id JOIN organizations o ON o.id=e.organization_id WHERE a.id=$2 AND a.verification_status<>'rejected' AND `+visibleActivity+` AND (e.visibility='public' OR o.owner_id=$1 OR e.created_by=$1 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1 AND m.status='active')))`, uid, id).Scan(&ok)
	return ok, err
}
func (p product) reaction(w http.ResponseWriter, r *http.Request, uid string) error {
	var in struct {
		ActivityID string `json:"activity_id"`
		Like       bool   `json:"like"`
	}
	if err := decodeProduct(w, r, &in); err != nil {
		return err
	}
	if err := idValid(in.ActivityID); err != nil {
		return err
	}
	ok, err := p.canSeeActivity(r.Context(), uid, in.ActivityID)
	if err != nil {
		return err
	}
	if !ok {
		return missing("Activity not found.")
	}
	if in.Like {
		_, err = p.db.ExecContext(r.Context(), `INSERT INTO reactions(activity_id,user_from,reaction_type) VALUES($1,$2,'clap') ON CONFLICT(activity_id,user_from) DO NOTHING`, in.ActivityID, uid)
	} else {
		_, err = p.db.ExecContext(r.Context(), `DELETE FROM reactions WHERE activity_id=$1 AND user_from=$2`, in.ActivityID, uid)
	}
	return err
}
func (p product) comment(w http.ResponseWriter, r *http.Request, uid string) error {
	var in struct {
		ActivityID string `json:"activity_id"`
		Body       string `json:"body"`
	}
	if err := decodeProduct(w, r, &in); err != nil {
		return err
	}
	if err := idValid(in.ActivityID); err != nil {
		return err
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" || len(in.Body) > 1000 {
		return invalid("Write a comment of 1–1000 characters.")
	}
	ok, err := p.canSeeActivity(r.Context(), uid, in.ActivityID)
	if err != nil {
		return err
	}
	if !ok {
		return missing("Activity not found.")
	}
	_, err = p.db.ExecContext(r.Context(), `INSERT INTO activity_comments(activity_id,user_id,body) VALUES($1,$2,$3)`, in.ActivityID, uid, in.Body)
	return err
}

// Page requests execute only the queries needed by that view. The unscoped
// dashboard remains available to existing API clients.
var pageDataFields = map[string]string{
	"home":       "profile invitations stats upcoming feed my_activity",
	"discover":   "profile invitations",
	"my-events":  "profile invitations my_events",
	"crews":      "profile invitations crews",
	"charities":  "profile invitations charities",
	"challenges": "profile invitations stats",
	"impact":     "profile invitations stats crews",
	"profile":    "profile invitations stats crews my_activity",
	"event-form": "organizations",
}
