package httpserver

// Only self, explicitly public activity, or mutually-followed friends are visible.
const visibleActivity = `(a.user_id=$1::uuid OR u.activity_visibility='public' OR
 (u.activity_visibility='friends' AND EXISTS(SELECT 1 FROM following f WHERE f.follower_id=$1 AND f.following_id=u.id)
 AND EXISTS(SELECT 1 FROM following f WHERE f.follower_id=u.id AND f.following_id=$1)))`

const feedQuery = `SELECT COALESCE(jsonb_agg(row ORDER BY created_at DESC),'[]'::jsonb) FROM (
 SELECT a.id,a.user_id,u.first_name||' '||u.last_name AS name,a.caption,a.verification_status,a.created_at,
 e.id AS event_id,e.title,o.name AS organization_name,l.city,
 CASE WHEN a.user_id=$1 OR u.hours_visibility='public' OR (u.hours_visibility='friends'
  AND EXISTS(SELECT 1 FROM following f WHERE f.follower_id=$1 AND f.following_id=u.id)
  AND EXISTS(SELECT 1 FROM following f WHERE f.follower_id=u.id AND f.following_id=$1))
 THEN round(EXTRACT(EPOCH FROM (a.ended_at-a.started_at))/3600,2) END AS hours,
 (SELECT count(*) FROM reactions r WHERE r.activity_id=a.id) AS kudos,
 EXISTS(SELECT 1 FROM reactions r WHERE r.activity_id=a.id AND r.user_from=$1) AS liked,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('id',c.id,'name',cu.first_name||' '||cu.last_name,'body',c.body,'created_at',c.created_at) ORDER BY c.created_at)
  FROM activity_comments c JOIN users cu ON cu.id=c.user_id WHERE c.activity_id=a.id),'[]'::jsonb) AS comments,
 COALESCE((SELECT jsonb_agg(t.name) FROM event_tags et JOIN tags t ON t.id=et.tag_id WHERE et.event_id=e.id AND t.category='cause'),'[]'::jsonb) AS causes
 FROM activities a JOIN users u ON u.id=a.user_id JOIN events e ON e.id=a.event_id
 JOIN organizations o ON o.id=e.organization_id JOIN locations l ON l.id=e.location_id
 WHERE ` + visibleActivity + ` AND a.verification_status <> 'rejected'
 AND (e.visibility='public' OR o.owner_id=$1 OR e.created_by=$1 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1 AND m.status='active'))
 ORDER BY a.created_at DESC LIMIT 50
) row`

const profileQuery = `SELECT jsonb_build_object(
 'id',u.id,'first_name',u.first_name,'last_name',u.last_name,'email',u.email,'bio',u.bio,
 'max_distance_km',u.max_distance_km,'activity_visibility',u.activity_visibility,'hours_visibility',u.hours_visibility,
 'is_site_admin',u.is_site_admin,'volunteer_with_friends',u.volunteer_with_friends,'city',l.city,'location_label',l.label,'address',l.address_line1,
 'latitude',ST_Y(l.coordinates::geometry),'longitude',ST_X(l.coordinates::geometry),
 'tags',COALESCE((SELECT jsonb_agg(jsonb_build_object('name',t.name,'category',t.category)) FROM user_tags ut JOIN tags t ON t.id=ut.tag_id WHERE ut.user_id=u.id),'[]'::jsonb),
 'availability',COALESCE((SELECT jsonb_agg(jsonb_build_object('weekday',weekday,'start_time',start_time,'end_time',end_time,'timezone',timezone) ORDER BY weekday,start_time) FROM user_availability WHERE user_id=u.id),'[]'::jsonb),
 'challenges',COALESCE((SELECT jsonb_agg(challenge_key) FROM user_challenges WHERE user_id=u.id),'[]'::jsonb)
 ) FROM users u LEFT JOIN locations l ON l.id=u.location_id WHERE u.id=$1`

const statsQuery = `SELECT jsonb_build_object(
 'hours',COALESCE(round(sum(EXTRACT(EPOCH FROM (a.ended_at-a.started_at))/3600),2),0),
 'verified_hours',COALESCE(round(sum(EXTRACT(EPOCH FROM (a.ended_at-a.started_at))/3600) FILTER(WHERE a.verification_status='verified'),2),0),
 'events',count(DISTINCT a.event_id),'organizations',count(DISTINCT e.organization_id),
 'causes',(SELECT count(DISTINCT t.id) FROM activities aa JOIN event_tags et ON et.event_id=aa.event_id JOIN tags t ON t.id=et.tag_id WHERE aa.user_id=$1 AND aa.verification_status<>'rejected' AND t.category='cause'),
 'active_weeks',count(DISTINCT date_trunc('week',a.started_at)),
 'monthly',COALESCE((SELECT jsonb_agg(jsonb_build_object('month',month,'hours',hours) ORDER BY month) FROM
  (SELECT date_trunc('month',started_at)::date AS month,round(sum(EXTRACT(EPOCH FROM(ended_at-started_at))/3600),2) AS hours FROM activities WHERE user_id=$1 AND verification_status<>'rejected' GROUP BY 1) m),'[]'::jsonb),
 'by_cause',COALESCE((SELECT jsonb_agg(jsonb_build_object('cause',cause,'hours',hours)) FROM
  (SELECT t.name AS cause,round(sum(EXTRACT(EPOCH FROM(aa.ended_at-aa.started_at))/3600),2) AS hours FROM activities aa JOIN event_tags et ON et.event_id=aa.event_id JOIN tags t ON t.id=et.tag_id WHERE aa.user_id=$1 AND aa.verification_status<>'rejected' AND t.category='cause' GROUP BY t.name) c),'[]'::jsonb)
 ) FROM activities a JOIN events e ON e.id=a.event_id WHERE a.user_id=$1 AND a.verification_status<>'rejected'`

const crewListQuery = `SELECT COALESCE(jsonb_agg(row ORDER BY name),'[]'::jsonb) FROM (
 SELECT o.id,o.name,o.description,o.owner_id,o.visibility,
 EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1 AND m.status='active') AS joined,
 (SELECT status FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1) AS membership_status,
 (SELECT count(*) FROM organization_members m WHERE m.organization_id=o.id AND m.status='active') AS members,
 (SELECT COALESCE(round(sum(EXTRACT(EPOCH FROM (a.ended_at-a.started_at))/3600),2),0)
  FROM activities a JOIN events e ON e.id=a.event_id WHERE e.organization_id=o.id AND a.verification_status='verified' AND (e.visibility='public' OR o.owner_id=$1 OR EXISTS(SELECT 1 FROM organization_members visible_member WHERE visible_member.organization_id=o.id AND visible_member.user_id=$1 AND visible_member.status='active'))) AS verified_hours
 FROM organizations o WHERE o.organization_type='volunteer_group' AND
 (o.visibility='public' OR o.owner_id=$1 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1 AND m.status IN('active','invited')))
) row`

const upcomingQuery = `SELECT COALESCE(jsonb_agg(row ORDER BY starts_at),'[]'::jsonb) FROM (
 SELECT e.id,e.title,e.starts_at,e.ends_at,l.city,l.label AS location_label,l.address_line1 AS address,o.name AS organization_name,r.status
 FROM event_registrations r JOIN events e ON e.id=r.event_id JOIN locations l ON l.id=e.location_id JOIN organizations o ON o.id=e.organization_id
 WHERE r.user_id=$1 AND r.status IN('registered','waitlisted') AND e.completed_at IS NULL AND e.starts_at>=now() AND e.status='published'
) row`

const invitationsQuery = `SELECT jsonb_build_object(
 'events',COALESCE((SELECT jsonb_agg(row) FROM (SELECT e.id,e.title,u.first_name||' '||u.last_name AS invited_by FROM event_invitations i JOIN events e ON e.id=i.event_id JOIN users u ON u.id=i.invited_by WHERE i.user_id=$1 AND e.completed_at IS NULL AND e.starts_at>=now() AND e.status='published' AND NOT EXISTS(SELECT 1 FROM event_registrations r WHERE r.event_id=e.id AND r.user_id=$1 AND r.status='registered')) row),'[]'::jsonb),
 'crews',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',o.id,'name',o.name)) FROM organization_members m JOIN organizations o ON o.id=m.organization_id WHERE m.user_id=$1 AND m.status='invited' AND (o.organization_type<>'charity' OR o.approval_status='approved')),'[]'::jsonb))`

const peopleQuery = `SELECT COALESCE(jsonb_agg(row ORDER BY name),'[]'::jsonb) FROM (
 SELECT u.id,u.first_name||' '||u.last_name AS name,
 EXISTS(SELECT 1 FROM following f WHERE f.follower_id=$1 AND f.following_id=u.id) AS following,
 EXISTS(SELECT 1 FROM following f WHERE f.follower_id=u.id AND f.following_id=$1) AS follows_you
 FROM users u WHERE u.id<>$1 AND (u.first_name||' '||u.last_name) ILIKE '%'||$2||'%' ORDER BY u.first_name,u.last_name LIMIT 50
) row`

const leaderboardQuery = `SELECT COALESCE(jsonb_agg(row ORDER BY hours DESC,name),'[]'::jsonb) FROM (
 SELECT u.id,u.first_name||' '||u.last_name AS name,
 round(sum(EXTRACT(EPOCH FROM(a.ended_at-a.started_at))/3600),2) AS hours
 FROM activities a JOIN users u ON u.id=a.user_id WHERE a.verification_status='verified'
 AND (u.id=$1 OR u.hours_visibility='public' OR (u.hours_visibility='friends'
 AND EXISTS(SELECT 1 FROM following f WHERE f.follower_id=$1 AND f.following_id=u.id)
 AND EXISTS(SELECT 1 FROM following f WHERE f.follower_id=u.id AND f.following_id=$1)))
 AND ($2::text='all' OR a.started_at>=date_trunc($2,now()))
 GROUP BY u.id ORDER BY hours DESC LIMIT 50
) row`

const groupLeaderboardQuery = `SELECT COALESCE(jsonb_agg(row ORDER BY hours DESC,name),'[]'::jsonb) FROM (
 SELECT o.id,o.name,round(sum(EXTRACT(EPOCH FROM(a.ended_at-a.started_at))/3600),2) AS hours
 FROM activities a JOIN events e ON e.id=a.event_id JOIN organizations o ON o.id=e.organization_id
 WHERE a.verification_status='verified' AND o.organization_type=$3
 AND (e.visibility='public' OR o.owner_id=$1 OR e.created_by=$1 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1 AND m.status='active'))
 AND ($2::text='all' OR a.started_at>=date_trunc($2,now()))
 GROUP BY o.id ORDER BY hours DESC LIMIT 50
) row`
