"use strict";
const $ = (s) => document.querySelector(s),
  esc = (s) =>
    String(s ?? "").replace(
      /[&<>"']/g,
      (c) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[c],
    );
let data = {
    feed: [],
    my_activity: [],
    my_events: [],
    crews: [],
    upcoming: [],
    organizations: [],
    registrations: [],
    invitations: { events: [], crews: [] },
  },
  opportunities = [],
  people = [],
  page = "home",
  feedTab = "following",
  view = "grid",
  crewID = null,
  crewDetails = null,
  ranking = [],
  rankPeriod = "month",
  rankType = "individuals",
  loading = true,
  myEventTab = "all",
  pendingEventSetup = false;
let filters = {
  q: "",
  cause: "",
  weekday: "",
  near: location.pathname === "/matches",
  from: "",
  to: "",
  saved: false,
  joined: false,
};
const current = () => data.profile || null,
  fullName = () =>
    current() ? `${current().first_name} ${current().last_name}` : "Guest";
function opportunityParams() {
  const params = new URLSearchParams({
    q: filters.q,
    cause: filters.cause,
    weekday: filters.weekday,
    near: String(filters.near),
  });
  if (filters.from)
    params.set("from", new Date(filters.from + "T00:00:00").toISOString());
  if (filters.to)
    params.set("to", new Date(filters.to + "T23:59:59").toISOString());
  return params;
}
const initials = (name) =>
  String(name || "")
    .trim()
    .split(/\s+/)
    .map((n) => n[0])
    .slice(0, 2)
    .join("")
    .toUpperCase();
const avatar = (name) => `<span class="avatar">${esc(initials(name))}</span>`;
const button = (label, action, id = "", style = "") =>
  `<button type="button" class="btn ${style}" data-action="${action}" data-id="${esc(id)}">${label}</button>`;
const heading = (eyebrow, title, sub, action = "") =>
  `<div class="page-heading"><div><div class="eyebrow">${eyebrow}</div><h1>${title}</h1><p>${sub}</p></div>${action}</div>`;
const empty = (title, description, action = "") =>
  `<div class="empty"><h3>${title}</h3><p style="margin:9px 0 18px">${description}</p>${action}</div>`;
const signInCard = () =>
  empty(
    "Your next chapter starts here.",
    "Create an account to track your impact and volunteer with your community.",
    '<a class="btn" href="/signup">Create an account →</a> <a class="btn secondary" href="/login">Sign in</a>',
  );
const fmtDate = (s, options = { month: "short", day: "numeric" }) =>
    new Date(s).toLocaleDateString(undefined, options),
  fmtTime = (s) =>
    new Date(s).toLocaleTimeString(undefined, {
      hour: "numeric",
      minute: "2-digit",
    }),
  eventWhen = (e) =>
    `${fmtDate(e.starts_at)} · ${fmtTime(e.starts_at)} – ${fmtTime(e.ends_at)}`;
const causeIcon = (c) =>
  ({
    Environment: "🌿",
    "Food security": "🥕",
    Education: "📚",
    Animals: "🐾",
    Community: "🤝",
    Health: "💛",
    "Arts & culture": "🎨",
  })[c] || "🌱";
let toastTimer;
function toast(message) {
  $("#toast").textContent = message;
  $("#toast").classList.add("visible");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => $("#toast").classList.remove("visible"), 4000);
}
function modal(title, content) {
  $("#modal-content").innerHTML =
    `<div class="modal-header"><h2>${esc(title)}</h2><button class="close" data-action="close" aria-label="Close dialog">×</button></div>${content}`;
  if (!$("#modal").open) $("#modal").showModal();
}
function requireAccount() {
  if (current()) return true;
  modal("Do good. Together.", signInCard());
  return false;
}
async function refresh() {
  data = await apiRequest("/api/community");
  [opportunities, people] = await Promise.all([
    apiRequest("/api/opportunities?" + opportunityParams()).catch((error) => {
      if (error.status === 422 || error.status === 401) {
        toast(error.message);
        return [];
      }
      throw error;
    }),
    current() ? apiRequest("/api/people") : Promise.resolve([]),
  ]);
  loading = false;
  if (crewID && current())
    crewDetails = await apiRequest(
      "/api/crew?id=" + encodeURIComponent(crewID),
    );
  render();
}
async function mutate(path, body, message) {
  const result = await apiRequest(path, body);
  await refresh();
  if (message) toast(message);
  return result;
}
function stats() {
  if (!current()) return "";
  const s = data.stats;
  return `<div class="stats">${[
    [
      "Your volunteer hours",
      Number(s.hours),
      "hrs",
      "Completed service, including pending verification",
      "◷",
    ],
    [
      "Verified impact",
      Number(s.verified_hours),
      "hrs",
      "Confirmed by event organizers",
      "✓",
    ],
    [
      "Events attended",
      s.events,
      "events",
      `${s.organizations} organizations supported`,
      "⚑",
    ],
    [
      "Weeks of giving",
      s.active_weeks,
      "weeks",
      "Distinct weeks with completed service",
      "🌱",
    ],
  ]
    .map(
      (x) =>
        `<div class="stat"><div class="stat-top">${x[0]}<span>${x[4]}</span></div><strong>${x[1]} <small>${x[2]}</small></strong><div class="stat-foot">${x[3]}</div></div>`,
    )
    .join("")}</div>`;
}
function activityCard(a) {
  return `<article class="card"><div class="card-pad"><div class="post-head">${avatar(a.name)}<div><b>${esc(a.name)}</b><p>${fmtDate(a.created_at)} · ${esc(a.city)}</p></div><span class="tag">${esc(a.causes[0] || "Community service")}</span></div><h3 class="post-title">${esc(a.title)}</h3><p class="tiny">with ${esc(a.organization_name)}</p>${a.caption ? `<p class="post-caption" style="margin-top:12px">${esc(a.caption)}</p>` : ""}<div class="post-metrics"><div><strong>${a.hours == null ? "—" : Number(a.hours)}</strong><small>${a.hours == null ? "service hours private" : "hours contributed"}</small></div><span class="verified">${a.verification_status === "verified" ? "◉ Organizer verified" : "○ Awaiting verification"}</span></div></div><div class="post-actions"><button data-action="kudos" data-id="${a.id}" aria-pressed="${a.liked}" class="${a.liked ? "liked" : ""}">♡ ${a.kudos} kudos</button><button data-action="comments" data-id="${a.id}">☏ ${a.comments.length} comments</button></div><div class="comments" id="comments-${a.id}" hidden>${a.comments.map((c) => `<div class="comment"><b>${esc(c.name)}</b> ${esc(c.body)}</div>`).join("")}${current() ? `<form class="comment-form" data-form="comment" data-id="${a.id}"><input name="body" required maxlength="1000" aria-label="Write a comment" placeholder="Leave a little encouragement…"><button class="btn small">Post</button></form>` : '<a class="text-button" href="/login">Sign in to comment</a>'}</div></article>`;
}
function upcomingMini(e) {
  return `<button class="event-mini" data-action="event" data-id="${e.id}" style="text-align:left"><span class="date-tile">${esc(fmtDate(e.starts_at, { month: "short" }).toUpperCase())}<b>${new Date(e.starts_at).getDate()}</b></span><span><h3>${esc(e.title)}</h3><p>${fmtTime(e.starts_at)} – ${fmtTime(e.ends_at)}</p><span class="match">${e.status === "waitlisted" ? "Waitlisted" : "Explore opportunity"} · ${esc(e.address || e.city)}</span></span></button>`;
}
function rightRail() {
  const next = (data.upcoming || []).slice(0, 3),
    suggestions = opportunities.filter((e) => !e.joined).slice(0, 3);
  return `<aside class="right-rail"><div class="card right-card"><h2>Your next good thing</h2><p class="tiny" style="margin-top:7px">A little time, already well spent.</p>${next.length ? next.map(upcomingMini).join("") : '<p class="section-note">Your calendar has room for a little good. Find an opportunity to get started.</p>'}<hr class="divider"><a class="text-button" href="#discover">Explore opportunities →</a></div><div class="card right-card challenge-feature"><div class="eyebrow">GOOD GROWS WITH YOU</div><span class="badge-icon">🌞</span><h2>The 10-hour club</h2><p class="section-note">A personal goal, powered by the service you actually log.</p>${current() ? `<div class="progress"><span style="width:${Math.min(100, Number(data.stats.hours) * 10)}%"></span></div><p class="tiny">${Number(data.stats.hours)} of 10 hours</p>` : ""}<a class="text-button" href="#challenges">Explore your goals →</a></div><div class="card right-card"><h2>Good things close by</h2>${suggestions.length ? suggestions.map((e) => `<button class="recommend-mini" data-action="event" data-id="${e.id}" style="text-align:left"><span class="event-symbol">${causeIcon(e.causes[0])}</span><span><h3>${esc(e.title)}</h3><p>${fmtDate(e.starts_at)} · ${esc(e.address || e.city)}</p><span class="match">${e.distance_km == null ? "Explore this opportunity" : e.distance_km + " km away"}</span></span></button>`).join("") : '<p class="section-note">Published opportunities will appear here as organizers add them.</p>'}</div></aside>`;
}
function home() {
  const following = new Set(people.filter((p) => p.following).map((p) => p.id)),
    feed =
      feedTab === "mine"
        ? data.my_activity || []
        : feedTab === "following" && current()
          ? data.feed.filter(
              (a) => a.user_id === current().id || following.has(a.user_id),
            )
          : data.feed;
  return (
    heading(
      "YOUR EVERYDAY, A LITTLE MORE MEANINGFUL",
      current()
        ? `Hello, ${esc(current().first_name)} ☀`
        : "A little time. A lot of good.",
      "Find your people. Lend a hand. Make a difference, together.",
      current()
        ? button("My events →", "my-events")
        : '<a class="btn" href="/signup">Join your community →</a>',
    ) +
    `<section class="hero"><div class="hero-content"><div class="eyebrow">GOOD THINGS HAPPEN WHEN WE SHOW UP</div><h2>Your community.<br>Your kind of impact.</h2><p>Discover a cause you care about and turn a few hours into something meaningful.</p><a class="btn light" href="#discover">Find an opportunity ↗</a></div><div class="hero-art" aria-hidden="true">🌱</div></section>` +
    stats() +
    `<div class="columns"><section><div class="section-heading"><h2>A little good, all around you</h2>${button("Find your people", "people", "", "secondary small")}</div><div class="tabs">${(current()
      ? [
          ["following", "Following"],
          ["everyone", "Community"],
          ["mine", "My activity"],
        ]
      : [["everyone", "Community"]]
    )
      .map(
        ([key, label]) =>
          `<button data-action="feed" data-id="${key}" class="${feedTab === key || (!current() && key === "everyone") ? "active" : ""}">${label}</button>`,
      )
      .join(
        "",
      )}</div>${feed.length ? feed.map(activityCard).join("") : empty("Good stories start with showing up.", current() ? "There’s no shared activity here yet. Follow people or log your own service." : "Public activities will appear here. Private activity stays private.", current() ? button("Find people to follow", "people", "", "secondary") : '<a class="btn secondary" href="/signup">Start your story</a>')}</section>${rightRail()}</div>`
  );
}
function eventCard(e) {
  return `<article class="card"><div class="event-cover"><img src="/static/images/${e.causes.includes("Environment") ? "garden" : "community"}.jpg" alt="Community inspiration; illustrative stock photograph" loading="lazy"><span>${causeIcon(e.causes[0])}</span>${e.distance_km != null ? `<span class="match">⌖ ${e.distance_km} km away</span>` : ""}<button data-action="save" data-id="${e.id}" aria-label="${e.saved ? "Unsave" : "Save"} ${esc(e.title)}" aria-pressed="${e.saved}">${e.saved ? "▣" : "♧"}</button></div><div class="event-body"><span class="tag">${esc(e.causes.join(" · ") || "Community service")}</span><h3><button data-action="event" data-id="${e.id}" style="padding:0;text-align:left">${esc(e.title)}</button></h3><p>${esc(e.organization_name)}</p><p>◷ ${eventWhen(e)}<br>⌖ ${esc(e.location_label || "")}${e.location_label ? " · " : ""}${esc(e.address || e.city)}</p><p>${esc(e.description.slice(0, 140))}${e.description.length > 140 ? "…" : ""}</p><div class="tiny">${e.spots} spots available${current() ? ` · ${e.friends} people you follow attending` : ""}</div>${button(e.joined ? "✓ Going · Leave event" : e.spots ? "Join the good →" : "Event full", e.joined ? "leave" : e.spots ? "join" : "event", e.id, e.joined ? "secondary" : "")}</div></article>`;
}
function filtered() {
  return opportunities.filter(
    (e) => (!filters.saved || e.saved) && (!filters.joined || e.joined),
  );
}
function resultCards() {
  const rows = filtered();
  return `<div class="section-heading"><p class="tiny">${rows.length} opportunities · earliest upcoming first</p><p class="tiny">Find a time and cause that work for you</p></div>${rows.length ? `<div class="grid ${view === "list" ? "list" : ""}">${rows.map(eventCard).join("")}</div>` : empty("Your next good thing is still growing.", "No published opportunities match these filters. Try a broader search, or publish an event for your organization.", current() ? button("Publish an opportunity", "create-event", "", "secondary") : "")}`;
}
function discover() {
  return (
    heading(
      "FIND YOUR KIND OF GOOD",
      "Good things are close by.",
      "Real opportunities, clear details, and a place for you.",
      current()
        ? `<div class="heading-actions">${button("My events", "my-events", "", "secondary")}${button("Publish an event", "create-event")}</div>`
        : "",
    ) +
    `<div class="card card-pad discovery-note"><div class="section-heading"><h2>Find where you can make a difference</h2>${button("Your preferences", "preferences", "", "secondary small")}</div><p class="tiny">Filter by cause, day, location, and travel distance. Nearby events use your saved location and maximum distance.</p></div><form id="discover-filters" class="filters"><input name="q" value="${esc(filters.q)}" aria-label="Search opportunities" placeholder="Event, organization, city…"><select name="cause" aria-label="Cause"><option value="">All causes</option>${CAUSES.map((c) => `<option ${filters.cause === c ? "selected" : ""}>${c}</option>`).join("")}</select><select name="weekday" aria-label="Day"><option value="">Any day</option>${DAYS.map((d, i) => `<option value="${i}" ${String(i) === filters.weekday ? "selected" : ""}>${d}</option>`).join("")}</select><select name="near" aria-label="Distance"><option value="false">All locations</option><option value="true" ${filters.near ? "selected" : ""}>Within my travel distance</option></select><label>Starts after<input type="date" name="from" value="${esc(filters.from)}"></label><label>Ends before<input type="date" name="to" value="${esc(filters.to)}"></label><button class="btn">Find opportunities</button></form><div class="section-heading"><div class="check-group"><label><input type="checkbox" id="saved-filter" ${filters.saved ? "checked" : ""}>Saved</label><label><input type="checkbox" id="joined-filter" ${filters.joined ? "checked" : ""}>I’m going</label></div><div>${button("▦ Grid", "view", "grid", "secondary small")} ${button("☷ List", "view", "list", "secondary small")}</div></div><div id="event-results">${resultCards()}</div>`
  );
}
function crewCard(c) {
  return `<article class="card"><div class="crew-cover">${c.visibility === "public" ? "🌱" : "🤝"}</div><div class="card-pad"><span class="tag">${c.joined ? "YOUR CREW" : c.membership_status === "invited" ? "YOU’RE INVITED" : esc(c.visibility.replace("_", " "))}</span><h2 style="margin-top:12px">${esc(c.name)}</h2><p class="section-note">${esc(c.description || "A community of people making a difference together.")}</p><div class="crew-stats"><div><b>${c.members}</b><small>members</small></div><div><b>${Number(c.verified_hours)}</b><small>verified event hours</small></div></div>${button("Explore crew →", "crew", c.id, "secondary")} ${!c.joined ? button(c.membership_status === "invited" ? "Accept invitation" : "Join club", "join-crew", c.id, "small") : ""}</div></article>`;
}
function crews() {
  if (crewID) return crewDashboard();
  return (
    heading(
      "BETTER, TOGETHER",
      "Find your people.",
      "Big-hearted humans. Shared adventures. A whole lot of good.",
      button("＋ Create a club", "create-crew"),
    ) +
    (data.crews.length
      ? `<div class="grid">${data.crews.map(crewCard).join("")}</div>`
      : empty(
          "Every great crew starts with one person.",
          "Start a volunteer group or come back as your community grows.",
          button("Create your club", "create-crew", "", "secondary"),
        ))
  );
}
function crewDashboard() {
  const c = data.crews.find((c) => c.id === crewID);
  if (!c)
    return empty(
      "Crew unavailable.",
      "This crew may be private or no longer available.",
      button("All crews", "all-crews", "", "secondary"),
    );
  const d = crewDetails || {};
  return (
    heading(
      "YOUR PEOPLE, YOUR IMPACT",
      esc(c.name),
      `${c.members} members · ${Number(c.verified_hours)} verified event hours`,
      button("← All crews", "all-crews", "", "secondary"),
    ) +
    `<div class="card card-pad"><div class="section-heading"><h2>Make good plans together</h2>${c.joined ? button("Invite people", "invite-crew", c.id, "small") : button("Join club", "join-crew", c.id)}</div><p class="section-note">${esc(c.description)}</p>${c.joined ? `<h3>Your crew’s availability</h3>${(d.members || []).map((m) => `<div class="member-row">${avatar(m.name)}<div><b>${esc(m.name)}</b><p class="tiny">${m.availability.length ? m.availability.map((a) => `${DAYS[a.weekday]} ${a.start_time.slice(0, 5)}–${a.end_time.slice(0, 5)} (${esc(a.timezone)})`).join(" · ") : "No availability set"}${m.has_location ? "" : " · Location needed for group matching"}</p></div></div>`).join("")}<p class="section-note">Group matches require enough spots and must fit every active member’s saved availability and travel distance.</p>` : '<p class="tiny">Join to see member availability and group matches.</p>'}</div><h2 style="margin:25px 0 15px">Upcoming crew events</h2>${(d.events || []).length ? `<div class="card card-pad">${d.events.map(upcomingMini).join("")}</div>` : empty("Good plans are on the way.", "No upcoming events have been published by this crew.")}${c.joined ? `<h2 style="margin:25px 0 15px">Events that fit the whole crew</h2>${(d.matches || []).length ? `<div class="card card-pad">${d.matches.map((e) => `<div class="member-row"><div><h3>${esc(e.title)}</h3><p class="tiny">${eventWhen(e)} · ${esc(e.address || e.city)}</p></div>${button("View event", "event", e.id, "secondary small")}</div>`).join("")}</div>` : empty("No shared match yet.", "Each member needs a location and availability in Preferences.")}<div style="margin-top:20px">${c.owner_id === current()?.id ? button("Publish a crew event", "create-event", c.id, "secondary") : button("Leave crew", "leave-crew", c.id, "secondary")}</div>` : ""}`
  );
}
function challenges() {
  return (
    heading(
      "A LITTLE MOTIVATION GOES A LONG WAY",
      "Make good a habit.",
      "Personal goals that grow with your real service history.",
    ) +
    `<div class="grid">${GOALS.map(
      ([key, title, desc, target, metric, unit, icon]) => {
        const value = Number(data.stats?.[metric] || 0),
          joined = current()?.challenges.includes(key);
        return `<article class="card card-pad"><span style="font-size:36px">${icon}</span><h2 style="margin-top:14px">${title}</h2><p class="section-note">${desc}</p>${current() ? `<div class="progress"><span style="width:${Math.min(100, (value / target) * 100)}%"></span></div><p class="tiny">${value} / ${target} ${unit}${value >= target ? " · Goal achieved!" : ""}</p>` : '<p class="tiny">Sign in to track your progress.</p>'}<div style="margin-top:15px">${button(joined ? "✓ Joined · Leave goal" : "Join this challenge", "challenge", key, joined ? "secondary" : "")}</div></article>`;
      },
    ).join(
      "",
    )}</div><p class="section-note">Progress comes from completed, non-rejected activities. Verified goals count organizer-confirmed hours only.</p><div class="card card-pad"><div class="section-heading"><h2>A community worth cheering for</h2><select id="rank-type" aria-label="Leaderboard type">${[
      ["individuals", "Volunteers"],
      ["crews", "Crews"],
      ["organizations", "Campus & companies"],
    ]
      .map(
        ([v, t]) =>
          `<option value="${v}" ${rankType === v ? "selected" : ""}>${t}</option>`,
      )
      .join(
        "",
      )}</select><select id="rank-period" aria-label="Leaderboard period">${[
      ["week", "This week"],
      ["month", "This month"],
      ["all", "All time"],
    ]
      .map(
        ([v, t]) =>
          `<option value="${v}" ${rankPeriod === v ? "selected" : ""}>${t}</option>`,
      )
      .join(
        "",
      )}</select></div>${current() ? `<div id="rankings">${leaderboard()}</div>` : '<p class="section-note">Sign in to view volunteers who have shared their verified hours.</p>'}</div>`
  );
}
function leaderboard() {
  return ranking.length
    ? `<table class="leaderboard"><thead><tr><th>Rank</th><th>${rankType === "individuals" ? "Volunteer" : "Organization"}</th><th>Verified hours</th></tr></thead><tbody>${ranking.map((r, i) => `<tr><td>${i + 1}</td><td>${esc(r.name)}</td><td>${Number(r.hours)} hrs</td></tr>`).join("")}</tbody></table><p class="section-note">Only verified hours shared with you appear. Your own hours are visible to you.</p>`
    : '<p class="section-note">No shared, verified hours for this period yet.</p>';
}
function badges() {
  const s = data.stats,
    rules = [
      ["🌱", "First Steps", "Complete your first event", s.events >= 1],
      ["🙌", "Helping Hand", "Contribute ten hours", Number(s.hours) >= 10],
      [
        "🌟",
        "Community Hero",
        "Fifty verified hours",
        Number(s.verified_hours) >= 50,
      ],
      [
        "🤝",
        "Team Player",
        "Join a volunteer crew",
        data.crews.some((c) => c.joined),
      ],
      ["🌈", "All-Rounder", "Support five causes", s.causes >= 5],
      ["🔥", "Keep Showing Up", "Service in four weeks", s.active_weeks >= 4],
      [
        "🌳",
        "Earth Advocate",
        "Ten environmental hours",
        s.by_cause.some(
          (c) => c.cause === "Environment" && Number(c.hours) >= 10,
        ),
      ],
      ["💛", "Good Neighbor", "Three organizations", s.organizations >= 3],
    ];
  return `<div class="badges">${rules.map(([icon, title, desc, earned]) => `<div class="badge ${earned ? "earned" : "locked"}" title="${earned ? "Achievement earned" : "Keep going to unlock"}"><span>${icon}</span><b>${title}</b><small>${desc} · ${earned ? "earned" : "not yet"}</small></div>`).join("")}</div>`;
}
function impact() {
  if (!current())
    return (
      heading(
        "EVERY HOUR HAS A STORY",
        "Your good adds up.",
        "Track the difference you make over time.",
      ) + signInCard()
    );
  const s = data.stats,
    months = Array.from({ length: 6 }, (_, i) => {
      const d = new Date();
      d.setDate(1);
      d.setMonth(d.getMonth() - 5 + i);
      const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
      return {
        label: d.toLocaleDateString(undefined, { month: "short" }),
        hours: Number(
          s.monthly.find((m) => m.month.startsWith(key))?.hours || 0,
        ),
      };
    }),
    max = Math.max(1, ...months.map((m) => m.hours));
  return (
    heading(
      "EVERY HOUR HAS A STORY",
      "Look at the good you’ve grown.",
      "Every number comes from your saved service history.",
      button("Review service hours", "review", "", "secondary"),
    ) +
    stats() +
    `<div class="columns"><section><div class="card card-pad"><h2>Your time, well spent</h2><p class="section-note">Completed hours over six calendar months</p><div class="chart">${months.map((m) => `<div class="bar" tabindex="0" style="height:${(m.hours / max) * 145}px" title="${m.label}: ${m.hours} hours" aria-label="${m.label}: ${m.hours} hours"><span>${m.hours}</span><small>${m.label}</small></div>`).join("")}</div><p class="section-note">${Number(s.verified_hours)} verified hours · ${Number(s.hours) - Number(s.verified_hours)} awaiting verification</p></div><div class="card card-pad"><h2>Your community footprint</h2><p class="section-note">${s.events} events · ${s.organizations} organizations · ${s.causes} causes</p><p>Good grows one hour at a time.</p></div></section><aside class="card card-pad" style="align-self:start"><h2>Where your heart goes</h2>${s.by_cause.length ? s.by_cause.map((c) => `<div style="margin-top:20px"><div class="section-heading"><span class="tiny">${causeIcon(c.cause)} ${esc(c.cause)}</span><b class="tiny">${Number(c.hours)} hrs</b></div><div class="progress"><span style="width:${Math.min(100, (Number(c.hours) / Math.max(1, Number(s.hours))) * 100)}%"></span></div></div>`).join("") : '<p class="section-note">Log service at an event with a cause to start your breakdown.</p>'}<p class="section-note">An event may support more than one cause.</p></aside></div><h2>Little badges. Big meaning.</h2>${badges()}`
  );
}
function profile() {
  if (!current())
    return (
      heading(
        "YOUR VOLUNTEER STORY",
        "Good looks like you.",
        "Create a profile and make yourself at home.",
      ) + signInCard()
    );
  const p = current();
  return (
    heading(
      "YOUR VOLUNTEER STORY",
      "Good looks like you.",
      "Your activity and hours are private until you choose to share.",
      button("Edit profile", "edit-profile", "", "secondary"),
    ) +
    `<div class="card"><div class="profile-cover"></div><div class="card-pad"><div class="profile-top">${avatar(fullName())}<div><h2>${esc(fullName())}</h2><p class="tiny">${esc(p.address || p.city || "Set your location to find opportunities")}</p></div><span class="tag">${p.activity_visibility === "private" && p.hours_visibility === "private" ? "PRIVATE BY DEFAULT" : "YOUR SHARING CHOICES"}</span></div><p style="margin-top:20px">${esc(p.bio || "Your story is just getting started.")}</p><p class="section-note">${p.tags
      .filter((t, i, tags) => tags.findIndex(x => x.name.toLowerCase() === t.name.toLowerCase()) === i)
      .map((t) => esc(t.name))
      .join(
        " · ",
      )}</p></div></div><div class="columns"><section><div class="card card-pad"><h2>You choose what to share</h2><p class="section-note">Activity: ${esc(p.activity_visibility)} · Hours: ${esc(p.hours_visibility)}.</p><p class="tiny">Friends means mutual follows. Private activity and hours stay out of other people’s feeds and rankings.</p><div style="margin-top:18px">${button("Manage privacy & profile", "edit-profile", "", "secondary")} ${button("Interests & availability", "preferences", "", "secondary")} ${button("Set location", "set-location", "", "secondary")}</div></div><h2 style="margin:24px 0 15px">Your activity history</h2>${data.my_activity.length ? data.my_activity.map(activityCard).join("") : empty("Your story starts with showing up.", "Join an opportunity. Your organizer will confirm attendance and log hours when the event is complete.", '<a class="btn secondary" href="#discover">Find an opportunity</a>')}</section><aside><div class="card card-pad"><h2>Your people</h2><p class="section-note">${people.filter((p) => p.following).length} following · ${people.filter((p) => p.following && p.follows_you).length} mutual connections</p>${button("Find your community", "people", "", "secondary small")}<hr class="divider"><h3>Your clubs</h3>${
      data.crews
        .filter((c) => c.joined)
        .map(
          (c) =>
            `<button class="event-mini" data-action="crew" data-id="${c.id}"><span>🌱</span><b>${esc(c.name)}</b></button>`,
        )
        .join("") ||
      '<p class="section-note">You haven’t joined a crew yet.</p>'
    }</div><div class="card card-pad"><h2>Your account</h2><p class="section-note">${esc(p.email)}</p>${button("Sign out", "logout", "", "secondary small")}</div></aside></div><h2 style="margin-top:25px">Your achievements</h2>${badges()}`
  );
}
function render() {
  const route = location.hash.slice(1);
  page = [
    "home",
    "discover",
    "my-events",
    "crews",
    "charities",
    "challenges",
    "impact",
    "profile",
  ].includes(route)
    ? route
    : location.pathname === "/events" || location.pathname === "/matches"
      ? "discover"
      : location.pathname === "/profile"
        ? "profile"
        : "home";
  const nav = [
    ["home", "⌂", "Home"],
    ["discover", "⌕", "Discover"],
    ["my-events", "▦", "My Events"],
    ["crews", "♧", "My Clubs"],
    ["charities", "♡", "Charities"],
    ["challenges", "⚑", "Challenges"],
    ["impact", "▥", "My Impact"],
    ["profile", "♙", "Profile"],
  ];
  $("#navigation").innerHTML = nav
    .map(
      ([key, icon, label]) =>
        `<a href="#${key}" class="${page === key ? "active" : ""}" ${page === key ? 'aria-current="page"' : ""}><span class="nav-icon">${icon}</span>${label}</a>`,
    )
    .join("");
  document.title = `${nav.find((n) => n[0] === page)[2]} · Volunteer Together`;
  if (loading) {
    $("#main").innerHTML =
      '<div class="skeleton" role="status" aria-label="Loading your community"></div>';
    return;
  }
  $("#main").innerHTML = {
    home,
    discover,
    "my-events": myEvents,
    crews,
    charities,
    challenges,
    impact,
    profile,
  }[page]();
  document
    .querySelectorAll(".avatar.me")
    .forEach((el) => (el.textContent = current() ? initials(fullName()) : "→"));
  $(".account b").textContent = current() ? fullName() : "Join your community";
  $(".account small").textContent = current()
    ? "Your volunteer profile"
    : "Create an account";
  $("#community-location").textContent = current()?.city || "Your local good";
  $(".workspace span").textContent = current()?.city || "Find your community";
  $(".icon-button i").hidden =
    !current() ||
    !(data.invitations.events.length + data.invitations.crews.length);
  if (page === "challenges" && current()) loadRankings();
}
async function loadRankings() {
  try {
    ranking = await apiRequest(
      "/api/leaderboard?period=" + rankPeriod + "&type=" + rankType,
    );
    if (page === "challenges" && $("#rankings"))
      $("#rankings").innerHTML = leaderboard();
  } catch (e) {
    toast(e.message);
  }
}
function profileBody() {
  const p = current();
  return {
    first_name: p.first_name,
    last_name: p.last_name,
    bio: p.bio,
    max_distance_km: p.max_distance_km,
    activity_visibility: p.activity_visibility,
    hours_visibility: p.hours_visibility,
    volunteer_with_friends: p.volunteer_with_friends,
    tags: p.tags,
    availability: p.availability.map((a) => ({
      ...a,
      start_time: a.start_time.slice(0, 5),
      end_time: a.end_time.slice(0, 5),
    })),
  };
}
function editProfile() {
  if (!requireAccount()) return;
  const p = current();
  modal(
    "Make yourself at home",
    `<form data-form="profile" class="form-stack"><div class="name-fields"><label>First name<input name="first_name" value="${esc(p.first_name)}" required maxlength="100"></label><label>Last name<input name="last_name" value="${esc(p.last_name)}" required maxlength="100"></label></div><label>Your bio<textarea name="bio" maxlength="1000">${esc(p.bio)}</textarea></label>${[
      ["activity_visibility", "Activity history"],
      ["hours_visibility", "Service hours"],
    ]
      .map(
        ([key, label]) =>
          `<label>${label}<select name="${key}">${["private", "friends", "public"].map((v) => `<option ${p[key] === v ? "selected" : ""}>${v}</option>`).join("")}</select></label>`,
      )
      .join(
        "",
      )}<p class="tiny">Friends are mutual follows. Your sharing choices apply as soon as you save.</p><button class="btn">Save profile</button></form>`,
  );
}
function preferences() {
  if (!requireAccount()) return;
  const p = current(),
    av = p.availability[0];
  modal(
    "Find your kind of good",
    `<form data-form="preferences" class="form-stack">${[
      ["interest", "Interests"],
      ["skill", "Skills"],
      ["cause", "Preferred causes"],
    ]
      .map(
        ([cat, label]) =>
          `<fieldset><legend>${label}</legend><p class="tiny">${cat === "skill" ? "Abilities you can offer." : cat === "cause" ? "Issues you want to support." : "Activities you enjoy."}</p><div class="check-group">${[...new Set([...(cat === "skill" ? ["Teaching", "Gardening", "Cooking", "Organizing", "Translation", "Technology"] : cat === "interest" ? ["Outdoors", "Arts", "Mentoring", "Teamwork", "Community"] : CAUSES), ...p.tags.filter(t => t.category === cat).map(t => t.name)])].map(name => `<label class="choice-chip"><input type="checkbox" name="${cat}" value="${esc(name)}" ${p.tags.some(t => t.category === cat && t.name === name) ? "checked" : ""}>${esc(name)}</label>`).join("")}</div></fieldset>`,
      )
      .join(
        "",
      )}<label>Maximum travel distance (km)<input type="number" name="max_distance_km" min="1" max="500" value="${p.max_distance_km}" required></label>${button("Set city or ZIP", "set-location", "", "secondary")}<fieldset><legend>Weekly availability</legend><div class="check-group">${DAYS.map((d, i) => `<label><input type="checkbox" name="day" value="${i}" ${p.availability.some((a) => a.weekday === i) ? "checked" : ""}>${d.slice(0, 3)}</label>`).join("")}</div><div class="name-fields"><label>From<input type="time" name="start_time" value="${av?.start_time.slice(0, 5) || "09:00"}" required></label><label>Until<input type="time" name="end_time" value="${av?.end_time.slice(0, 5) || "13:00"}" required></label></div><label>Timezone<input name="timezone" value="${esc(av?.timezone || Intl.DateTimeFormat().resolvedOptions().timeZone)}" required></label></fieldset><label class="check-line"><input type="checkbox" name="volunteer_with_friends" ${p.volunteer_with_friends ? "checked" : ""}>I prefer volunteering with people I know</label><button class="btn">Save preferences</button></form>`,
  );
}
function organizationForm() {
  if (!requireAccount()) return;
  modal(
    "Good starts with your people",
    `<form data-form="crew" class="form-stack"><label>Club or organization name<input name="name" required maxlength="120"></label><label>Description<textarea name="description" maxlength="2000"></textarea></label><label>Organization type<select name="organization_type"><option value="volunteer_group">Volunteer club</option><option value="charity">Nonprofit / charity</option><option value="company">Company / campus organization</option></select></label><label>Organization street address (required for charities and companies)<input name="address" maxlength="300" placeholder="Street, city, state, ZIP"></label><p class="tiny">Charities need site administrator approval before members can publish events.</p><label>Membership<select name="visibility"><option value="public">Public · anyone can join</option><option value="invite_only">Invite only</option><option value="closed">Closed</option></select></label><button class="btn">Create organization</button></form>`,
  );
}
function eventForm(orgID = "") {
  if (!requireAccount()) return;
  if (!data.organizations.length) {
    pendingEventSetup = true;
    modal(
      "Good plans need an organizer",
      `<p class="section-note">Create your club or organization first. Then publish its volunteering events.</p>${button("Create an organization", "create-crew")}`,
    );
    return;
  }
  modal(
    "Publish an opportunity",
    `<form data-form="event" class="form-stack"><label>Organization<select name="organization_id">${data.organizations.map((o) => `<option value="${o.id}" ${o.id === orgID ? "selected" : ""}>${esc(o.name)}</option>`).join("")}</select></label><label>Who can see and join this event?<select name="visibility"><option value="public">Public · anyone can join</option><option value="crew_only">Crew members only</option></select></label><p class="tiny">Crew-only events are visible only to active members of the hosting crew or organization.</p><label>Event title<input name="title" required maxlength="160"></label><label>Description<textarea name="description" required maxlength="4000"></textarea></label><div class="name-fields"><label>Starts<input name="starts_at" type="datetime-local" required></label><label>Ends<input name="ends_at" type="datetime-local" required></label></div><p class="tiny">Times use your browser’s timezone (${esc(Intl.DateTimeFormat().resolvedOptions().timeZone)}).</p><label>Capacity<input name="capacity" type="number" min="1" max="10000" value="10" required></label><label>Venue name (optional)<input name="label" maxlength="200" placeholder="Community center, park entrance, or meeting point"></label><label>Event address<input name="address" autocomplete="street-address" required maxlength="300" placeholder="110 8th Street, Troy, NY 12180"></label><p class="tiny">Include street number, city, state, and ZIP. U.S. addresses are resolved automatically when you publish.</p><label>Causes (comma separated)<input name="causes" placeholder="Environment, Community" maxlength="400"></label><p class="tiny">The event will use the audience you choose above. Hours are logged only after you confirm attendance and mark the event done.</p><button class="btn">Publish event</button></form>`,
  );
}
function localInput(s) {
  const d = new Date(s);
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
}
function logForm() {
  if (!requireAccount()) return;
  const rows = data.registrations.filter(
    (e) => new Date(e.starts_at) < new Date() && e.status !== "cancelled",
  );
  if (!rows.length) {
    modal(
      "Your service story starts here",
      empty(
        "No completed registrations yet.",
        "Register for an event, then return after it starts to log your time.",
        '<a class="btn" href="#discover" data-action="close">Find an opportunity</a>',
      ),
    );
    return;
  }
  modal(
    "Share a little good",
    `<form data-form="activity" class="form-stack"><label>Registered event<select name="event_id" id="activity-event">${rows.map((e) => `<option value="${e.id}">${esc(e.title)} · ${fmtDate(e.starts_at)}</option>`).join("")}</select></label><label>Service started<input type="datetime-local" name="started_at" required value="${localInput(rows[0].starts_at)}"></label><label>Service ended<input type="datetime-local" name="ended_at" required value="${localInput(new Date(Math.min(Date.now(), new Date(rows[0].ends_at))).toISOString())}"></label><label>Your story<textarea name="caption" maxlength="2000" placeholder="What made this time meaningful?"></textarea></label><p class="tiny">Hours await organizer verification. Activity: ${esc(current().activity_visibility)}. Service hours: ${esc(current().hours_visibility)}.</p><button class="btn">Save activity</button></form>`,
  );
}
async function showEvent(id) {
  const e = await apiRequest("/api/event-details?id=" + encodeURIComponent(id));
  const started = new Date(e.starts_at) <= new Date(),
    finished = Boolean(e.completed_at);
  modal(
    e.title,
    `<span class="tag">${e.visibility === "crew_only" ? "Crew members only" : "Public event"}</span><p style="margin:18px 0">${esc(e.description)}</p><p class="section-note">${esc(e.organization_name)}<br>${eventWhen(e)}<br>${esc(e.location_label || "")}${e.location_label ? " · " : ""}${esc(e.address || e.city)}</p>${finished ? '<p class="match">✓ Completed · participant hours have been logged.</p>' : started ? `<p class="section-note">${e.can_manage ? "Confirm attendance when the event is finished." : "Your organizer will confirm attendance and log hours when this event is done."}</p>${e.can_manage && e.status === "published" ? button("Mark event done", "complete-event", e.id) : ""}` : `<p class="tiny">${e.spots} spots available</p>${button(e.joined ? "Leave this event" : e.spots ? "Join this event" : "Event full", e.joined ? "leave" : e.spots ? "join" : "close", e.id, e.joined ? "secondary" : "")} ${e.joined ? button("Invite people", "invite-event", e.id, "secondary") : ""}`}`,
  );
}
function myEvents() {
  if (!current())
    return (
      heading(
        "YOUR PLANS, YOUR IMPACT",
        "Your events, all together.",
        "Keep track of the events you host and attend.",
      ) + signInCard()
    );
  const rows = (data.my_events || []).filter((e) =>
    myEventTab === "hosting"
      ? e.can_manage
      : myEventTab === "attending"
        ? e.joined
        : myEventTab === "completed"
          ? e.completed_at
          : true,
  );
  return (
    heading(
      "YOUR PLANS, YOUR IMPACT",
      "Your events, all together.",
      "Events stay open until an organizer confirms they are done.",
      button("＋ Create an event", "create-event"),
    ) +
    `<div class="tabs">${[
      ["all", "All events"],
      ["hosting", "Hosting"],
      ["attending", "Attending"],
      ["completed", "Completed"],
    ]
      .map(
        ([key, label]) =>
          `<button data-action="my-events-tab" data-id="${key}" class="${myEventTab === key ? "active" : ""}">${label}</button>`,
      )
      .join("")}</div>${
      rows.length
        ? `<div class="grid">${rows
            .map((e) => {
              const started = new Date(e.starts_at) <= new Date(),
                ended = new Date(e.ends_at) <= new Date();
              return `<article class="card card-pad"><div class="section-heading"><span class="tag">${e.visibility === "crew_only" ? "CREW ONLY" : "PUBLIC"}</span><span class="tiny">${e.completed_at ? "✓ Completed" : e.status === "cancelled" ? "Cancelled" : started ? (ended ? "Awaiting completion" : "In progress") : "Upcoming"}</span></div><h2>${esc(e.title)}</h2><p class="section-note">${esc(e.organization_name)}<br>${eventWhen(e)}<br>${esc(e.address || e.city)}</p><p class="tiny">${e.can_manage ? "You’re organizing" : "You registered"} · ${e.registrations} registered${e.completed_at ? " · " + e.participants + " attended" : ""}</p>${e.completed_at ? `<p class="section-note">${((new Date(e.actual_ended_at) - new Date(e.actual_started_at)) / 3600000).toFixed(2)} hours logged per confirmed participant.</p>` : `<p class="section-note">${e.can_manage ? (started ? "Confirm who participated and the actual service time to log their hours." : "Mark this event done after it starts and service is finished.") : "Your hours will appear after the organizer completes the event and confirms your attendance."}</p>`}<div class="event-manage-actions">${button("View event", "event", e.id, "secondary small")}${e.can_manage && !e.completed_at && started && e.status === "published" ? button("Mark event done", "complete-event", e.id, "small") : ""}</div></article>`;
            })
            .join("")}</div>`
        : empty(
            "Good plans start here.",
            "Create an event or join one to see it in your calendar.",
            '<a class="btn secondary" href="#discover">Find an opportunity</a>',
          )
    }`
  );
}
async function completionForm(id) {
  if (!requireAccount()) return;
  const e = await apiRequest(
    "/api/event-attendance?id=" + encodeURIComponent(id),
  );
  if (e.completed_at) {
    toast("This event is already complete.");
    await refresh();
    return;
  }
  const end = new Date(Math.min(Date.now(), new Date(e.ends_at))).toISOString();
  modal(
    "Confirm attendance & complete event",
    `<form data-form="completion" data-id="${e.id}" class="form-stack"><h3>${esc(e.title)}</h3><p class="tiny">Select the people who actually participated. Uncheck anyone who did not attend.</p><fieldset><legend>Participants</legend>${e.attendees.length ? e.attendees.map((a) => `<label class="check-line"><input type="checkbox" name="participant_id" value="${a.id}" ${a.registered ? "checked" : ""}>${esc(a.name)}${a.registered ? "" : " · I also participated (organizer)"}</label>`).join("") : '<p class="tiny">No registered participants. Completing this event will log no hours.</p>'}</fieldset><div class="name-fields"><label>Actual start<input type="datetime-local" name="started_at" step="1" value="${localInputPrecise(e.starts_at)}" required></label><label>Actual end<input type="datetime-local" name="ended_at" step="1" value="${localInputPrecise(end)}" required></label></div><p id="completion-summary" class="match" aria-live="polite"></p><p class="tiny">Each selected participant receives the confirmed duration as organizer-verified service. Previous individual submissions for this event are replaced in totals to prevent duplicate credit.</p><label class="check-line"><input type="checkbox" name="confirmed" required>I confirm the event is done and this attendance and service time are accurate.</label><button class="btn">Complete event & log hours</button></form>`,
  );
  updateCompletionSummary();
}

function localInputPrecise(s) {
  const d = new Date(s);
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 19);
}
function updateCompletionSummary() {
  const f = document.querySelector('form[data-form="completion"]');
  if (!f) return;
  const d = new FormData(f),
    hours =
      (new Date(d.get("ended_at")) - new Date(d.get("started_at"))) / 3600000;
  const label = f.querySelector("#completion-summary");
  if (label)
    label.textContent =
      Number.isFinite(hours) && hours > 0
        ? `${d.getAll("participant_id").length} participants × ${hours.toFixed(2)} verified hours each`
        : "Choose a valid service start and end time.";
}
function peopleRows(rows, kind) {
  return rows.length
    ? rows
        .map(
          (p) =>
            `<div class="member-row">${avatar(p.name)}<div><b>${esc(p.name)}</b><p class="tiny">${p.following && p.follows_you ? "Mutual connection" : p.follows_you ? "Follows you" : p.following ? "Following" : ""}</p></div>${kind ? `<input type="checkbox" name="user_id" value="${p.id}" aria-label="Invite ${esc(p.name)}">` : button(p.following ? "Following · Unfollow" : "Follow", "follow", p.id, "secondary small")}</div>`,
        )
        .join("")
    : '<p class="section-note">No people found. Your community has room to grow.</p>';
}
async function peopleModal(kind = "", id = "") {
  if (!requireAccount()) return;
  people = await apiRequest("/api/people");
  modal(
    kind ? "Good company makes good happen" : "Find your people",
    `<p class="section-note">${kind ? "Choose people to send an in-app invitation. No email is sent." : "Follow volunteers to see shared activity. Mutual follows are friends."}</p><input id="people-search" aria-label="Search people" placeholder="Search by name…" style="width:100%"><form data-form="invite" data-kind="${kind}" data-id="${id}" class="form-stack"><div id="people-list">${peopleRows(people, kind)}</div>${kind ? '<button class="btn">Send invitations</button>' : ""}</form>`,
  );
}
function notifications() {
  if (!requireAccount()) return;
  const i = data.invitations;
  modal(
    "Good things on your calendar",
    `<h3>Event invitations</h3>${i.events.length ? i.events.map((e) => `<div class="member-row"><div><b>${esc(e.title)}</b><p class="tiny">Invited by ${esc(e.invited_by)}</p></div>${button("View", "event", e.id, "secondary small")}</div>`).join("") : '<p class="section-note">No outstanding event invitations.</p>'}<h3>Crew invitations</h3>${i.crews.length ? i.crews.map((c) => `<div class="member-row"><b>${esc(c.name)}</b>${button("Accept", "join-crew", c.id, "small")}</div>`).join("") : '<p class="section-note">No outstanding crew invitations.</p>'}`,
  );
}
async function review() {
  if (!requireAccount()) return;
  const rows = await apiRequest("/api/review");
  modal(
    "Organizer service review",
    rows.length
      ? rows
          .map(
            (a) =>
              `<div class="card card-pad"><h3>${esc(a.name)} · ${esc(a.title)}</h3><p class="section-note">${eventWhen({ starts_at: a.started_at, ends_at: a.ended_at })} · ${((new Date(a.ended_at) - new Date(a.started_at)) / 3600000).toFixed(2)} hours</p><p>${esc(a.caption)}</p><div style="margin-top:12px">${button("Verify hours", "verify", a.id)} ${button("Reject log", "reject", a.id, "secondary")}</div></div>`,
          )
          .join("")
      : empty(
          "Nothing waiting for review.",
          "Review other volunteers’ pending service at events your organization runs. You cannot verify your own hours.",
        ),
  );
}
async function action(a, id) {
  switch (a) {
    case "close":
      pendingEventSetup = false;
      $("#modal").close();
      break;
    case "profile":
      location.hash = current() ? "profile" : "home";
      if (!current()) requireAccount();
      break;
    case "set-location":
      locationForm();
      break;
    case "approve-charity":
      await apiRequest("/api/charity-approval", {organization_id:id}); await refresh(); break;
    case "preferences":
      preferences();
      break;
    case "edit-profile":
      editProfile();
      break;
    case "create-crew":
      organizationForm();
      break;
    case "create-event":
      eventForm(id);
      break;
    case "log":
    case "my-events":
      location.hash = "my-events";
      break;
    case "my-events-tab":
      myEventTab = id;
      render();
      break;
    case "complete-event":
      await completionForm(id);
      break;
    case "event":
      await showEvent(id);
      break;
    case "notifications":
      notifications();
      break;
    case "people":
      await peopleModal();
      break;
    case "feed":
      feedTab = id;
      render();
      break;
    case "view":
      view = id;
      render();
      break;
    case "comments": {
      const row = $("#comments-" + id);
      row.hidden = !row.hidden;
      if (!row.hidden) row.querySelector("input")?.focus();
      break;
    }
    case "kudos": {
      if (!requireAccount()) return;
      const a = [...data.feed, ...data.my_activity].find((a) => a.id === id);
      await mutate("/api/reaction", { activity_id: id, like: !a.liked });
      break;
    }
    case "save": {
      if (!requireAccount()) return;
      const e = opportunities.find((e) => e.id === id);
      await mutate(
        "/api/save",
        { event_id: id, save: !e.saved },
        e.saved ? "Removed from saved" : "Saved for later",
      );
      break;
    }
    case "join":
      if (!requireAccount()) return;
      await mutate(
        "/api/registration",
        { event_id: id, join: true },
        "You’re registered. Find this event on Home.",
      );
      $("#modal").close();
      break;
    case "leave":
      modal(
        "Leave this event?",
        `<p class="section-note">Your registration will be cancelled and your spot will become available.</p>${button("Keep my spot", "close", "", "secondary")} ${button("Leave event", "confirm-leave", id)}`,
      );
      break;
    case "confirm-leave":
      await mutate(
        "/api/registration",
        { event_id: id, join: false },
        "Registration cancelled.",
      );
      $("#modal").close();
      break;
    case "crew":
      if (!requireAccount()) return;
      crewID = id;
      crewDetails = await apiRequest("/api/crew?id=" + id);
      location.hash = "crews";
      render();
      break;
    case "all-crews":
      crewID = null;
      crewDetails = null;
      render();
      break;
    case "join-crew":
      if (!requireAccount()) return;
      await mutate(
        "/api/membership",
        { organization_id: id, join: true },
        "Welcome to the crew!",
      );
      $("#modal").close();
      break;
    case "leave-crew":
      modal(
        "Leave this crew?",
        `<p class="section-note">You can rejoin a public crew later.</p>${button("Stay in crew", "close", "", "secondary")} ${button("Leave crew", "confirm-leave-crew", id)}`,
      );
      break;
    case "confirm-leave-crew":
      crewID = null;
      await mutate(
        "/api/membership",
        { organization_id: id, join: false },
        "You left the crew.",
      );
      $("#modal").close();
      break;
    case "invite-crew":
      await peopleModal("crew", id);
      break;
    case "invite-event":
      await peopleModal("event", id);
      break;
    case "follow": {
      const p = people.find((p) => p.id === id);
      await mutate("/api/follow", { user_id: id, follow: !p.following });
      await peopleModal();
      break;
    }
    case "challenge":
      if (!requireAccount()) return;
      await mutate(
        "/api/challenge",
        { key: id, join: !current().challenges.includes(id) },
        "Challenge choices saved.",
      );
      break;
    case "review":
      await review();
      break;
    case "verify":
      await mutate(
        "/api/verify",
        { activity_id: id, status: "verified" },
        "Service hours verified.",
      );
      await review();
      break;
    case "reject":
      modal(
        "Reject this service log?",
        `<p class="section-note">Rejected hours will be excluded from impact totals.</p>${button("Keep pending", "close", "", "secondary")} ${button("Reject log", "confirm-reject", id)}`,
      );
      break;
    case "confirm-reject":
      await mutate(
        "/api/verify",
        { activity_id: id, status: "rejected" },
        "Service log rejected.",
      );
      await review();
      break;
    case "logout":
      await apiRequest("/logout", {});
      location.assign("/");
      break;
  }
}
document.addEventListener("click", async (ev) => {
  const el = ev.target.closest("[data-action]");
  if (!el || el.disabled) return;
  el.disabled = true;
  try {
    await action(el.dataset.action, el.dataset.id);
  } catch (error) {
    toast(error.message);
  } finally {
    el.disabled = false;
  }
});
document.addEventListener("submit", async (ev) => {
  const f = ev.target;
  if (f.id !== "discover-filters" && !f.dataset.form) return;
  ev.preventDefault();
  const submit = f.querySelector('button[type="submit"],button:not([type])');
  const originalSubmitLabel = submit?.textContent;
  f.querySelector(".form-error")?.remove();
  if (submit?.disabled) return;
  if (submit) submit.disabled = true;
  const d = new FormData(f);
  if (submit && d.get("address")) submit.textContent = "Checking address…";
  try {
    if (f.id === "discover-filters") {
      filters = {
        ...filters,
        ...Object.fromEntries(d),
        near: d.get("near") === "true",
      };
      const params = new URLSearchParams({
        q: filters.q,
        cause: filters.cause,
        weekday: filters.weekday,
        near: String(filters.near),
      });
      if (filters.from)
        params.set("from", new Date(filters.from + "T00:00:00").toISOString());
      if (filters.to)
        params.set("to", new Date(filters.to + "T23:59:59").toISOString());
      $("#event-results").innerHTML =
        '<div class="skeleton" role="status" aria-label="Finding opportunities"></div>';
      try {
        opportunities = await apiRequest("/api/opportunities?" + params);
        $("#event-results").innerHTML = resultCards();
      } catch (e) {
        $("#event-results").innerHTML = empty(
          "Let’s adjust those filters.",
          esc(e.message),
          e.status === 422
            ? button("Set your location", "preferences", "", "secondary")
            : "",
        );
        throw e;
      }
      return;
    }
    const kind = f.dataset.form;
    let body, path;
    if (kind === "location") {
      const place = await apiRequest("/api/location?q=" + encodeURIComponent(d.get("place")));
      await apiRequest("/api/profile", {...profileBody(), location:place});
      await refresh();
      if (onboarding) onboardingStep(1); else $("#modal").close();
      return;
    }
    if (kind === "onboarding") {
      const body = profileBody();
      if (d.get("step") === "1") body.tags = [...body.tags.filter(t => t.category !== "cause"), ...d.getAll("cause").map(name => ({name, category:"cause"}))];
      else body.availability = d.getAll("day").map(day => ({weekday:Number(day),start_time:d.get("start_time"),end_time:d.get("end_time"),timezone:Intl.DateTimeFormat().resolvedOptions().timeZone}));
      await apiRequest("/api/profile", body); if (d.get("step") === "2") filters.near=true; await refresh(); onboardingStep(Number(d.get("step"))+1); return;
    }
    if (kind === "profile") {
      body = { ...profileBody(), ...Object.fromEntries(d) };
      path = "/api/profile";
    }
    if (kind === "preferences") {
      body = profileBody();
      body.max_distance_km = Number(d.get("max_distance_km"));
      body.volunteer_with_friends = d.has("volunteer_with_friends");
      body.tags = ["interest", "skill", "cause"].flatMap((category) =>
        d.getAll(category)
          .map((name) => ({ name, category })),
      );
      body.availability = d.getAll("day").map((day) => ({
        weekday: Number(day),
        start_time: d.get("start_time"),
        end_time: d.get("end_time"),
        timezone: d.get("timezone"),
      }));
      path = "/api/profile";
    }
    if (kind === "crew") {
      body = Object.fromEntries(d);
      path = "/api/organization";
    }
    if (kind === "event") {
      body = {
        ...Object.fromEntries(d),
        starts_at: new Date(d.get("starts_at")).toISOString(),
        ends_at: new Date(d.get("ends_at")).toISOString(),
        capacity: Number(d.get("capacity")),
        causes: String(d.get("causes"))
          .split(",")
          .map((x) => x.trim())
          .filter(Boolean),
      };
      path = "/api/event";
    }
    if (kind === "completion") {
      body = {
        event_id: f.dataset.id,
        participant_ids: d.getAll("participant_id"),
        started_at: new Date(d.get("started_at")).toISOString(),
        ended_at: new Date(d.get("ended_at")).toISOString(),
      };
      path = "/api/event-complete";
    }
    if (kind === "activity") {
      body = {
        ...Object.fromEntries(d),
        started_at: new Date(d.get("started_at")).toISOString(),
        ended_at: new Date(d.get("ended_at")).toISOString(),
      };
      path = "/api/activity";
    }
    if (kind === "comment") {
      body = { activity_id: f.dataset.id, body: String(d.get("body")).trim() };
      path = "/api/comment";
    }
    if (kind === "invite") {
      const ids = d.getAll("user_id");
      if (!ids.length) throw new Error("Choose at least one person to invite.");
      body = {
        user_ids: ids,
        [f.dataset.kind === "crew" ? "organization_id" : "event_id"]:
          f.dataset.id,
      };
      path = "/api/invite";
    }
    if (!path) return;
    const saved = await mutate(
      path,
      body,
      kind === "invite"
        ? "Invitations sent in the app."
        : "Saved to your account.",
    );
    if (kind === "comment") {
      $("#comments-" + f.dataset.id).hidden = false;
    } else if (kind === "crew" && pendingEventSetup) {
      pendingEventSetup = false;
      eventForm(saved.id);
    } else {
      $("#modal").close();
      if (kind === "event" || kind === "completion") {
        location.hash = "my-events";
        render();
      }
      if (kind === "completion")
        toast(
          saved.already_completed
            ? "This event was already completed."
            : `Event completed. Hours logged for ${saved.participants_credited} participants.`,
        );
    }
  } catch (error) {
    if (f.isConnected) {
      const message = document.createElement("p");
      message.className = "form-error";
      message.setAttribute("role", "alert");
      message.textContent = error.message;
      f.append(message);
    }
    toast(error.message);
  } finally {
    if (submit) {
      submit.disabled = false;
      submit.textContent = originalSubmitLabel;
    }
  }
});
document.addEventListener("change", (ev) => {
  if (ev.target.form?.dataset.form === "completion") updateCompletionSummary();
  if (ev.target.id === "saved-filter" || ev.target.id === "joined-filter") {
    filters[ev.target.id === "saved-filter" ? "saved" : "joined"] =
      ev.target.checked;
    $("#event-results").innerHTML = resultCards();
  }
  if (ev.target.id === "rank-type") {
    rankType = ev.target.value;
    loadRankings();
  }
  if (ev.target.id === "rank-period") {
    rankPeriod = ev.target.value;
    loadRankings();
  }
  if (ev.target.id === "activity-event") {
    const e = data.registrations.find((e) => e.id === ev.target.value),
      form = ev.target.form;
    form.elements.started_at.value = localInput(e.starts_at);
    form.elements.ended_at.value = localInput(
      new Date(Math.min(Date.now(), new Date(e.ends_at))).toISOString(),
    );
  }
});
document.addEventListener("input", (ev) => {
  if (ev.target.form?.dataset.form === "completion") updateCompletionSummary();
  if (ev.target.id === "people-search") {
    const kind = $("#people-list").closest("form").dataset.kind;
    $("#people-list").innerHTML = peopleRows(
      people.filter((p) =>
        p.name.toLowerCase().includes(ev.target.value.toLowerCase()),
      ),
      kind,
    );
  }
});
window.addEventListener("hashchange", () => {
  $("#modal").close();
  render();
  window.scrollTo(0, 0);
});
$("#modal").addEventListener("click", (ev) => {
  if (ev.target === $("#modal")) {
    const r = ev.target.getBoundingClientRect();
    if (
      ev.clientX < r.left ||
      ev.clientX > r.right ||
      ev.clientY < r.top ||
      ev.clientY > r.bottom
    )
      ev.target.close();
  }
});
$("#modal").addEventListener("close", () => {
  pendingEventSetup = false;
});
render();
refresh().then(() => { if (new URLSearchParams(location.search).has("welcome") && current()) { onboarding = true; locationForm(); } }).catch((error) => {
  loading = false;
  $("#main").innerHTML = empty(
    "Your community is taking a moment.",
    esc(error.message),
    '<button class="btn" onclick="location.reload()">Try again</button>',
  );
});

let onboarding = false;
function locationForm() {
  modal(onboarding ? "1 of 4: Where would you like to help?" : "Set location", `<form data-form="location" class="form-stack"><label>City and state, or ZIP code<input name="place" required maxlength="120" placeholder="Troy, NY or 12180" value="${esc(current()?.city || "")}"></label><p class="tiny">Your area helps us find nearby opportunities. You do not need to share a street address.</p><p class="tiny">Location search by <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap contributors</a>.</p><button class="btn">Save location and continue</button></form>`);
}
function onboardingStep(step) {
 if (step === 1) modal("2 of 4: What matters to you?", `<form data-form="onboarding" class="form-stack"><input type="hidden" name="step" value="1"><div class="check-group">${CAUSES.map(c => `<label class="choice-chip"><input type="checkbox" name="cause" value="${esc(c)}">${esc(c)}</label>`).join("")}</div><button class="btn">Continue</button></form>`);
 else if (step === 2) modal("3 of 4: When can you help?", `<form data-form="onboarding" class="form-stack"><input type="hidden" name="step" value="2"><div class="check-group">${DAYS.map((d,i) => `<label><input type="checkbox" name="day" value="${i}">${d}</label>`).join("")}</div><label>From<input type="time" name="start_time" value="09:00" required></label><label>Until<input type="time" name="end_time" value="13:00" required></label><p class="tiny">Leave days unchecked if your schedule varies.</p><button class="btn">Find opportunities</button></form>`);
 else {
  onboarding=false; history.replaceState(null,"","/#discover"); page="discover"; render();
  const causes=current().tags.filter(t => t.category === "cause").map(t => t.name);
  const rows=opportunities.filter(e => (!causes.length || e.causes.some(c => causes.includes(c))) && e.distance_km != null && e.distance_km <= current().max_distance_km && (!current().availability.length || current().availability.some(a => {
    const parts = date => Object.fromEntries(new Intl.DateTimeFormat("en-US", {timeZone:a.timezone,weekday:"short",hour:"2-digit",minute:"2-digit",hourCycle:"h23",year:"numeric",month:"2-digit",day:"2-digit"}).formatToParts(new Date(date)).map(p => [p.type,p.value]));
    const start=parts(e.starts_at), end=parts(e.ends_at);
    return ["Sun","Mon","Tue","Wed","Thu","Fri","Sat"][a.weekday] === start.weekday && start.day===end.day && start.month===end.month && start.year===end.year && start.hour+":"+start.minute >= a.start_time.slice(0,5) && end.hour+":"+end.minute <= a.end_time.slice(0,5);
  }))).slice(0,4);
  modal("4 of 4: Suggested opportunities", rows.length ? rows.map(eventCard).join("") : `<p>No opportunities fit your choices yet. Explore all opportunities or adjust your preferences.</p>${button("Explore opportunities", "close")}`);
 }
}
function charities() {
 return heading("GIVE TOGETHER", "Charities", "Join a charity and help organize its next event.", button("Register a charity", "create-crew")) + `<div class="grid">${(data.charities || []).map(c => `<article class="card card-pad"><h2>${esc(c.name)}</h2><p>${esc(c.description)}</p><span class="tag">${esc(c.approval_status)}</span>${c.approval_status === "approved" ? button(c.joined ? "Create event" : "Join charity", c.joined ? "create-event" : "join-crew", c.id) : ""}${current()?.is_site_admin && c.approval_status === "pending" ? button("Approve charity", "approve-charity", c.id) : ""}</article>`).join("") || empty("Make a difference together", "Approved charities will appear here.")}</div>`;
}
