package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (p product) organization(w http.ResponseWriter, r *http.Request, uid string) (any, error) {
	var in struct {
		Name        string `json:"name"`
		ImageURL    string `json:"image_url"`
		Website     string `json:"website"`
		Address     string `json:"address"`
		Description string `json:"description"`
		Visibility  string `json:"visibility"`
		Type        string `json:"organization_type"`
	}
	if err := decodeProduct(w, r, &in); err != nil {
		return nil, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" || len(in.Name) > 120 || len(in.Description) > 2000 {
		return nil, invalid("Enter a name (up to 120 characters) and a description under 2000 characters.")
	}
	if in.Visibility != "public" && in.Visibility != "invite_only" && in.Visibility != "closed" {
		return nil, invalid("Invalid crew visibility.")
	}
	if in.Type != "volunteer_group" && in.Type != "charity" && in.Type != "company" {
		return nil, invalid("Choose an organization type.")
	}
	for _, link := range []*string{&in.ImageURL, &in.Website} {
		*link = strings.TrimSpace(*link)
		if !validOrganizationURL(*link) {
			return nil, invalid("Use a complete HTTPS link for the picture or website.")
		}
	}
	if (in.Type == "charity") != (r.URL.Path == "/api/charity") {
		return nil, invalid("Register charities from the Charities page. Use the club form for clubs and companies.")
	}
	var orgLocation *resolvedAddress
	if in.Type != "volunteer_group" {
		address, err := p.resolveAddress(r.Context(), in.Address)
		if err != nil {
			return nil, err
		}
		orgLocation = &address
	}
	tx, err := p.db.BeginTx(r.Context(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO organizations(name,description,organization_type,visibility,owner_id,location_id,image_url,website) SELECT $2,$3,$4,$5,id,location_id,$6,$7 FROM users WHERE id=$1 RETURNING id`, uid, in.Name, in.Description, in.Type, in.Visibility, in.ImageURL, in.Website).Scan(&id)
	if err != nil {
		return nil, err
	}
	if orgLocation != nil {
		l := orgLocation
		_, err = tx.ExecContext(r.Context(), `WITH loc AS (INSERT INTO locations(city,coordinates,address_line1,region,postal_code) VALUES($2,ST_SetSRID(ST_MakePoint($3,$4),4326)::geography,$5,$6,$7) RETURNING id) UPDATE organizations SET location_id=loc.id FROM loc WHERE organizations.id=$1`, id, l.City, l.Longitude, l.Latitude, l.Address, l.Region, l.PostalCode)
		if err != nil {
			return nil, err
		}
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO organization_members(organization_id,user_id,role,status) VALUES($1,$2,'owner','active')`, id, uid)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]string{"id": id}, nil
}

func (p product) membership(w http.ResponseWriter, r *http.Request, uid string) error {
	var in struct {
		OrganizationID string `json:"organization_id"`
		Join           bool   `json:"join"`
	}
	if err := decodeProduct(w, r, &in); err != nil {
		return err
	}
	if err := idValid(in.OrganizationID); err != nil {
		return err
	}
	tx, err := p.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var visibility, owner, orgType, approval string
	if err = tx.QueryRowContext(r.Context(), `SELECT visibility,owner_id,organization_type,approval_status FROM organizations WHERE id=$1 FOR UPDATE`, in.OrganizationID).Scan(&visibility, &owner, &orgType, &approval); err != nil {
		return err
	}
	if in.Join {
		if orgType == "charity" && approval != "approved" {
			return denied()
		}
		var invited bool
		if err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM organization_members WHERE organization_id=$1 AND user_id=$2 AND status IN('invited','active'))`, in.OrganizationID, uid).Scan(&invited); err != nil {
			return err
		}
		if visibility != "public" && !invited && owner != uid {
			return denied()
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO organization_members(organization_id,user_id,status) VALUES($1,$2,'active') ON CONFLICT(organization_id,user_id) DO UPDATE SET status='active',joined_at=now()`, in.OrganizationID, uid)
	} else {
		if owner == uid {
			return conflict("Crew owners cannot leave their own crew.")
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE organization_members SET status='removed' WHERE organization_id=$1 AND user_id=$2`, in.OrganizationID, uid)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (p product) invite(w http.ResponseWriter, r *http.Request, uid string) error {
	var in struct {
		OrganizationID string   `json:"organization_id"`
		EventID        string   `json:"event_id"`
		UserIDs        []string `json:"user_ids"`
	}
	if err := decodeProduct(w, r, &in); err != nil {
		return err
	}
	if len(in.UserIDs) < 1 || len(in.UserIDs) > 30 {
		return invalid("Choose between 1 and 30 people.")
	}
	if (in.OrganizationID == "") == (in.EventID == "") {
		return invalid("Choose one crew or event to invite people to.")
	}
	for _, id := range in.UserIDs {
		if err := idValid(id); err != nil {
			return err
		}
		if id == uid {
			return invalid("You cannot invite yourself.")
		}
	}
	tx, err := p.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var allowed bool
	if in.OrganizationID != "" {
		if err = idValid(in.OrganizationID); err != nil {
			return err
		}
		err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM organizations o WHERE o.id=$1 AND (o.organization_type<>'charity' OR o.approval_status='approved') AND (o.owner_id=$2 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$2 AND m.status='active')))`, in.OrganizationID, uid).Scan(&allowed)
	} else {
		if err = idValid(in.EventID); err != nil {
			return err
		}
		err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM events e JOIN organizations o ON o.id=e.organization_id WHERE e.id=$1 AND e.status='published' AND e.completed_at IS NULL AND e.starts_at>now() AND (o.owner_id=$2 OR e.created_by=$2 OR EXISTS(SELECT 1 FROM event_registrations er WHERE er.event_id=e.id AND er.user_id=$2 AND er.status='registered')))`, in.EventID, uid).Scan(&allowed)
	}
	if err != nil {
		return err
	}
	if !allowed {
		return denied()
	}
	for _, id := range in.UserIDs {
		if in.OrganizationID != "" {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO organization_members(organization_id,user_id,status) VALUES($1,$2,'invited') ON CONFLICT(organization_id,user_id) DO UPDATE SET status='invited' WHERE organization_members.status IN('removed','requested')`, in.OrganizationID, id)
		} else {
			var visible bool
			if err = tx.QueryRowContext(r.Context(), `SELECT e.visibility='public' OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=e.organization_id AND m.user_id=$2 AND m.status='active') FROM events e WHERE e.id=$1`, in.EventID, id).Scan(&visible); err != nil {
				return err
			}
			if !visible {
				return invalid("Only current crew members can be invited to a crew-only event.")
			}
			_, err = tx.ExecContext(r.Context(), `INSERT INTO event_invitations(event_id,user_id,invited_by) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, in.EventID, id, uid)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p product) follow(w http.ResponseWriter, r *http.Request, uid string) error {
	var in struct {
		UserID string `json:"user_id"`
		Follow bool   `json:"follow"`
	}
	if err := decodeProduct(w, r, &in); err != nil {
		return err
	}
	if err := idValid(in.UserID); err != nil {
		return err
	}
	if in.UserID == uid {
		return invalid("You cannot follow yourself.")
	}
	var err error
	if in.Follow {
		_, err = p.db.ExecContext(r.Context(), `INSERT INTO following(follower_id,following_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, uid, in.UserID)
	} else {
		_, err = p.db.ExecContext(r.Context(), `DELETE FROM following WHERE follower_id=$1 AND following_id=$2`, uid, in.UserID)
	}
	return err
}

func (p product) challenge(w http.ResponseWriter, r *http.Request, uid string) error {
	var in struct {
		Key  string `json:"key"`
		Join bool   `json:"join"`
	}
	if err := decodeProduct(w, r, &in); err != nil {
		return err
	}
	valid := false
	for _, key := range []string{"ten-hours", "three-events", "fifty-hours", "three-causes", "three-organizations", "four-weeks"} {
		if key == in.Key {
			valid = true
		}
	}
	if !valid {
		return invalid("Unknown challenge.")
	}
	var err error
	if in.Join {
		_, err = p.db.ExecContext(r.Context(), `INSERT INTO user_challenges(user_id,challenge_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, uid, in.Key)
	} else {
		_, err = p.db.ExecContext(r.Context(), `DELETE FROM user_challenges WHERE user_id=$1 AND challenge_key=$2`, uid, in.Key)
	}
	return err
}

func (p product) createEvent(w http.ResponseWriter, r *http.Request, uid string) (any, error) {
	var in struct {
		OrganizationID string    `json:"organization_id"`
		ImageURL       string    `json:"image_url"`
		Title          string    `json:"title"`
		Description    string    `json:"description"`
		StartsAt       time.Time `json:"starts_at"`
		EndsAt         time.Time `json:"ends_at"`
		Capacity       int       `json:"capacity"`
		Label          string    `json:"label"`
		Address        string    `json:"address"`
		City           string    `json:"city"`
		Latitude       float64   `json:"latitude"`
		Longitude      float64   `json:"longitude"`
		Causes         []string  `json:"causes"`
		Visibility     string    `json:"visibility"`
	}
	if err := decodeProduct(w, r, &in); err != nil {
		return nil, err
	}
	if err := idValid(in.OrganizationID); err != nil {
		return nil, err
	}
	in.ImageURL = strings.TrimSpace(in.ImageURL)
	if !validOrganizationURL(in.ImageURL) {
		return nil, invalid("Use a complete HTTPS link for the event picture.")
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Visibility == "" {
		in.Visibility = "public"
	}
	if in.Visibility != "public" && in.Visibility != "crew_only" {
		return nil, invalid("Choose public or crew-only visibility.")
	}
	in.City = strings.TrimSpace(in.City)
	if in.Title == "" || len(in.Title) > 160 || len(in.Description) > 4000 || (in.City == "" && strings.TrimSpace(in.Address) == "") || len(in.City) > 120 || len(in.Label) > 200 || in.Capacity < 1 || in.Capacity > 10000 || in.StartsAt.IsZero() || !in.EndsAt.After(in.StartsAt) || in.EndsAt.Sub(in.StartsAt) > 24*time.Hour || !in.EndsAt.After(time.Now()) || in.Latitude < -90 || in.Latitude > 90 || in.Longitude < -180 || in.Longitude > 180 || len(in.Causes) > 5 {
		return nil, invalid("Check the event title, future dates, capacity, and location.")
	}
	var allowed bool
	err := p.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM organizations o WHERE o.id=$1 AND (o.organization_type<>'charity' OR o.approval_status='approved') AND (o.owner_id=$2 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$2 AND (m.role IN('owner','admin') OR o.organization_type='charity') AND m.status='active')))`, in.OrganizationID, uid).Scan(&allowed)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, denied()
	}
	var address resolvedAddress
	if strings.TrimSpace(in.Address) != "" {
		address, err = p.resolveAddress(r.Context(), in.Address)
		if err != nil {
			return nil, err
		}
		in.City = address.City
		in.Latitude = address.Latitude
		in.Longitude = address.Longitude
	}
	tx, err := p.db.BeginTx(r.Context(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	allowed = false
	err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM organizations o WHERE o.id=$1 AND (o.organization_type<>'charity' OR o.approval_status='approved') AND (o.owner_id=$2 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$2 AND (m.role IN('owner','admin') OR o.organization_type='charity') AND m.status='active')))`, in.OrganizationID, uid).Scan(&allowed)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, denied()
	}
	var loc, id string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO locations(label,city,coordinates,address_line1,region,postal_code) VALUES($1,$2,ST_SetSRID(ST_MakePoint($3,$4),4326)::geography,$5,$6,$7) RETURNING id`, in.Label, in.City, in.Longitude, in.Latitude, address.Address, address.Region, address.PostalCode).Scan(&loc)
	if err != nil {
		return nil, err
	}
	err = tx.QueryRowContext(r.Context(), `INSERT INTO events(organization_id,location_id,title,description,starts_at,ends_at,capacity,status,visibility,created_by,image_url) VALUES($1,$2,$3,$4,$5,$6,$7,'published',$8,$9,$10) RETURNING id`, in.OrganizationID, loc, in.Title, in.Description, in.StartsAt, in.EndsAt, in.Capacity, in.Visibility, uid, in.ImageURL).Scan(&id)
	if err != nil {
		return nil, err
	}
	for _, name := range in.Causes {
		name = strings.TrimSpace(name)
		if name == "" || len(name) > 80 {
			return nil, invalid("Invalid event cause.")
		}
		var tag string
		err = tx.QueryRowContext(r.Context(), `INSERT INTO tags(name,category) VALUES($1,'cause') ON CONFLICT(category,lower(name)) DO UPDATE SET name=tags.name RETURNING id`, name).Scan(&tag)
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO event_tags(event_id,tag_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, tag); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]string{"id": id}, nil
}

func (p product) verify(w http.ResponseWriter, r *http.Request, uid string) error {
	var in struct {
		ActivityID string `json:"activity_id"`
		Status     string `json:"status"`
	}
	if err := decodeProduct(w, r, &in); err != nil {
		return err
	}
	if err := idValid(in.ActivityID); err != nil {
		return err
	}
	if in.Status != "verified" && in.Status != "rejected" {
		return invalid("Choose verified or rejected.")
	}
	result, err := p.db.ExecContext(r.Context(), `UPDATE activities a SET verification_status=$3,verified_by=$2 FROM events e JOIN organizations o ON o.id=e.organization_id WHERE a.id=$1 AND a.event_id=e.id AND a.user_id<>$2 AND a.verification_status='pending' AND (o.owner_id=$2 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$2 AND m.role IN('owner','admin') AND m.status='active'))`, in.ActivityID, uid, in.Status)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return conflict("Only the organizer can review another volunteer's pending activity.")
	}
	return nil
}

func (p product) crew(ctx context.Context, uid, id string) (any, error) {
	if err := idValid(id); err != nil {
		return nil, err
	}
	var allowed, member bool
	err := p.db.QueryRowContext(ctx, `SELECT visibility='public' OR owner_id=$2 OR EXISTS(SELECT 1 FROM organization_members WHERE organization_id=o.id AND user_id=$2 AND status IN('active','invited')),EXISTS(SELECT 1 FROM organization_members WHERE organization_id=o.id AND user_id=$2 AND status='active') FROM organizations o WHERE id=$1 AND organization_type='volunteer_group'`, id, uid).Scan(&allowed, &member)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, missing("Crew not found.")
	}
	result := map[string]json.RawMessage{}
	result["events"], err = p.json(ctx, `SELECT COALESCE(jsonb_agg(row ORDER BY starts_at),'[]'::jsonb) FROM (SELECT e.id,e.title,e.starts_at,e.ends_at,e.visibility,l.city,l.address_line1 AS address FROM events e JOIN locations l ON l.id=e.location_id WHERE e.organization_id=$1 AND e.starts_at>=now() AND e.status='published' AND e.completed_at IS NULL AND (e.visibility='public' OR e.created_by=$2 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=e.organization_id AND m.user_id=$2 AND m.status='active'))) row`, id, uid)
	if err != nil {
		return nil, err
	}
	if !member {
		return result, nil
	}
	result["members"], err = p.json(ctx, `SELECT COALESCE(jsonb_agg(row ORDER BY name),'[]'::jsonb) FROM (SELECT u.id,u.first_name||' '||u.last_name AS name,m.role,u.location_id IS NOT NULL AS has_location,COALESCE((SELECT jsonb_agg(jsonb_build_object('weekday',weekday,'start_time',start_time,'end_time',end_time,'timezone',timezone) ORDER BY weekday) FROM user_availability WHERE user_id=u.id),'[]'::jsonb) AS availability FROM organization_members m JOIN users u ON u.id=m.user_id WHERE m.organization_id=$1 AND m.status='active') row`, id)
	if err != nil {
		return nil, err
	}
	result["matches"], err = p.json(ctx, `SELECT COALESCE(jsonb_agg(row ORDER BY starts_at),'[]'::jsonb) FROM (
 SELECT e.id,e.title,e.starts_at,e.ends_at,o.name AS organization_name,l.city,l.address_line1 AS address FROM events e JOIN organizations o ON o.id=e.organization_id JOIN locations l ON l.id=e.location_id
 WHERE e.status='published' AND e.completed_at IS NULL AND e.starts_at>=now()
 AND (e.visibility='public' OR NOT EXISTS(SELECT 1 FROM organization_members owncrew WHERE owncrew.organization_id=$1 AND owncrew.status='active' AND NOT EXISTS(SELECT 1 FROM organization_members targetcrew WHERE targetcrew.organization_id=e.organization_id AND targetcrew.user_id=owncrew.user_id AND targetcrew.status='active')))
 AND e.capacity-(SELECT count(*) FROM event_registrations er WHERE er.event_id=e.id AND er.status='registered') >=
 (SELECT count(*) FROM organization_members m WHERE m.organization_id=$1 AND m.status='active' AND NOT EXISTS(SELECT 1 FROM event_registrations er WHERE er.event_id=e.id AND er.user_id=m.user_id AND er.status='registered'))
 AND NOT EXISTS(SELECT 1 FROM organization_members m JOIN users u ON u.id=m.user_id LEFT JOIN locations ul ON ul.id=u.location_id WHERE m.organization_id=$1 AND m.status='active' AND
 (ul.id IS NULL OR NOT ST_DWithin(l.coordinates,ul.coordinates,u.max_distance_km*1000.0) OR NOT EXISTS(
 SELECT 1 FROM user_availability av WHERE av.user_id=u.id AND EXTRACT(DOW FROM e.starts_at AT TIME ZONE av.timezone)=av.weekday
 AND (e.starts_at AT TIME ZONE av.timezone)::date=(e.ends_at AT TIME ZONE av.timezone)::date
 AND (e.starts_at AT TIME ZONE av.timezone)::time>=av.start_time AND (e.ends_at AT TIME ZONE av.timezone)::time<=av.end_time)))
 ORDER BY e.starts_at LIMIT 30) row`, id)
	return result, err
}

func (p product) approveCharity(w http.ResponseWriter, r *http.Request, uid string) error {
	var in struct {
		OrganizationID string `json:"organization_id"`
	}
	if err := decodeProduct(w, r, &in); err != nil {
		return err
	}
	if err := idValid(in.OrganizationID); err != nil {
		return err
	}
	result, err := p.db.ExecContext(r.Context(), `UPDATE organizations SET approval_status='approved' WHERE id=$1 AND organization_type='charity' AND EXISTS(SELECT 1 FROM users WHERE id=$2 AND is_site_admin)`, in.OrganizationID, uid)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return denied()
	}
	return nil
}

func validOrganizationURL(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := url.Parse(value)
	return err == nil && len(value) <= 2000 && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil
}
