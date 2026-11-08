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
  crewID = location.pathname === "/clubs" ? new URLSearchParams(location.search).get("id") : null,
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
  const requestedPage = pageForPath(location.pathname);
  await loadPageScript(requestedPage);
  data = {feed: [], my_activity: [], my_events: [], crews: [], charities: [], upcoming: [], organizations: [], registrations: [], invitations: {events: [], crews: []}, ...await apiRequest("/api/community?page=" + requestedPage)};
  [opportunities, people] = await Promise.all([
    (requestedPage === "discover" ? apiRequest("/api/opportunities?" + opportunityParams()) : Promise.resolve([])).catch((error) => {
      if (error.status === 422 || error.status === 401) {
        toast(error.message);
        return [];
      }
      throw error;
    }),
    current() && ["home", "profile"].includes(requestedPage) ? apiRequest("/api/people") : Promise.resolve([]),
  ]);
  loading = false;
  if (requestedPage === "crews" && crewID && current())
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


function eventImage(e, className = "") {
  const custom = /^https:\/\//i.test(e.image_url || "");
  const src = custom ? e.image_url : `/static/images/${(e.causes || []).includes("Environment") ? "garden" : "community"}.jpg`;
  return `<img class="${className}" src="${esc(src)}" alt="${custom ? esc(e.title) : "Community inspiration; illustrative stock photograph"}" loading="lazy" referrerpolicy="no-referrer">`;
}
function showFormError(form, error) {
  const message = document.createElement("p");
  message.className = "form-error";
  message.setAttribute("role", "alert");
  message.tabIndex = -1;
  message.textContent = error.message;
  form.prepend(message);
  message.focus({preventScroll:true});
  message.scrollIntoView({block:"center",behavior:"instant"});
}
function eventCard(e) {
  return `<article class="card"><div class="event-cover">${eventImage(e)}<span>${causeIcon(e.causes[0])}</span>${e.distance_km != null ? `<span class="match">⌖ ${e.distance_km} km away</span>` : ""}<button data-action="save" data-id="${e.id}" aria-label="${e.saved ? "Unsave" : "Save"} ${esc(e.title)}" aria-pressed="${e.saved}">${e.saved ? "▣" : "♧"}</button></div><div class="event-body"><span class="tag">${esc(e.causes.join(" · ") || "Community service")}</span><h3><button data-action="event" data-id="${e.id}" style="padding:0;text-align:left">${esc(e.title)}</button></h3><p>${esc(e.organization_name)}</p><p>◷ ${eventWhen(e)}<br>⌖ ${esc(e.location_label || "")}${e.location_label ? " · " : ""}${esc(e.address || e.city)}</p><p>${esc(e.description.slice(0, 140))}${e.description.length > 140 ? "…" : ""}</p><div class="tiny">${e.spots} spots available${current() ? ` · ${e.friends} people you follow attending` : ""}</div>${button(e.joined ? "✓ Going · Leave event" : e.spots ? "Join the good →" : "Event full", e.joined ? "leave" : e.spots ? "join" : "event", e.id, e.joined ? "secondary" : "")}</div></article>`;
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


const pageViews = {};
const pageScripts = new Map();
function loadPageScript(key) {
  if (!pageScripts.has(key)) {
    const promise = new Promise((resolve, reject) => {
      const script = document.createElement("script");
      script.src = "/static/js/pages/" + key + ".js";
      script.onload = resolve;
      script.onerror = () => { pageScripts.delete(key); reject(new Error("Unable to load this page. Please try again.")); };
      document.head.append(script);
    });
    pageScripts.set(key, promise);
  }
  return pageScripts.get(key);
}
const pagePaths = {
  home: "/", discover: "/discover", "my-events": "/my-events",
  crews: "/clubs", charities: "/charities", challenges: "/challenges",
  impact: "/impact", profile: "/profile",
};
function pageForPath(path) {
  if (path === "/events" || path === "/matches") return "discover";
  return Object.keys(pagePaths).find(key => pagePaths[key] === path) || "home";
}
function navigate(key) {
  if (pagePaths[key]) location.assign(pagePaths[key]);
}
function render() {
  page = pageForPath(location.pathname);
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
        `<a href="${pagePaths[key]}" class="${page === key ? "active" : ""}" ${page === key ? 'aria-current="page"' : ""}><span class="nav-icon">${icon}</span>${label}</a>`,
    )
    .join("");
  document.title = `${nav.find((n) => n[0] === page)[2]} · Volunteer Together`;
  if (loading) {
    $("#main").innerHTML =
      '<div class="skeleton" role="status" aria-label="Loading your community"></div>';
    return;
  }
  $("#main").innerHTML = pageViews[page]();
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
function organizationForm(type = "volunteer_group") {
  if (!requireAccount()) return;
  const charity = type === "charity";
  if (charity && location.pathname !== "/charities") { navigate("charities"); return; }
  modal(
    charity ? "Register a charity" : "Good starts with your people",
    `<form data-form="${charity ? "charity" : "crew"}" class="form-stack"><label>${charity ? "Charity name" : "Club or organization name"}<input name="name" required maxlength="120"></label><label>Description<textarea name="description" maxlength="2000"></textarea></label><label>Picture link (optional)<input name="image_url" type="url" maxlength="2000" placeholder="https://example.org/photo.jpg"></label><p class="tiny">Use an HTTPS link to a photo or logo that represents your organization.</p><label>Website (optional)<input name="website" type="url" maxlength="2000" placeholder="https://example.org"></label>${charity ? '<input type="hidden" name="organization_type" value="charity">' : '<label>Organization type<select name="organization_type"><option value="volunteer_group">Volunteer club</option><option value="company">Company / campus organization</option></select></label>'}<label>Street address (required for charities and companies)<input name="address" ${charity ? "required" : ""} autocomplete="street-address" maxlength="300" placeholder="Street number and name, city, state, ZIP"></label>${charity ? '<p class="tiny">Only you and site admins can see your charity until a site admin approves it. Members can create events after approval.</p>' : ""}<label>Membership<select name="visibility"><option value="public">Public · anyone can join</option><option value="invite_only">Invite only</option><option value="closed">Closed</option></select></label><button class="btn">${charity ? "Submit for approval" : "Create organization"}</button></form>`,
  );
}
async function eventForm(orgID = "") {
  if (!requireAccount()) return;
  const organizerData = await apiRequest("/api/community?page=event-form");
  data.organizations = organizerData.organizations || [];
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
    `<form data-form="event" class="form-stack"><label>Organization<select name="organization_id">${data.organizations.map((o) => `<option value="${o.id}" ${o.id === orgID ? "selected" : ""}>${esc(o.name)}</option>`).join("")}</select></label><label>Who can see and join this event?<select name="visibility"><option value="public">Public · anyone can join</option><option value="crew_only">Club members only</option></select></label><p class="tiny">Club-only events are visible only to active members of the hosting club or organization.</p><label>Event title<input name="title" required maxlength="160"></label><label>Description<textarea name="description" required maxlength="4000"></textarea></label><label>Event picture (optional)<input name="image_url" type="url" maxlength="2000" placeholder="https://example.org/event-photo.jpg"></label><p class="tiny">Paste an HTTPS link to your photo or flyer. Leave blank to use a community photo.</p><div class="name-fields"><label>Starts<input name="starts_at" type="datetime-local" required></label><label>Ends<input name="ends_at" type="datetime-local" required></label></div><p class="tiny">Times use your browser’s timezone (${esc(Intl.DateTimeFormat().resolvedOptions().timeZone)}).</p><label>Capacity<input name="capacity" type="number" min="1" max="10000" value="10" required></label><label>Venue name (optional)<input name="label" maxlength="200" placeholder="Community center, park entrance, or meeting point"></label><label>Event address<input name="address" autocomplete="street-address" required maxlength="300" placeholder="110 8th Street, Troy, NY 12180"></label><p class="tiny">Include street number, city, state, and ZIP. U.S. addresses are resolved automatically when you publish.</p><label>Causes (comma separated)<input name="causes" placeholder="Environment, Community" maxlength="400"></label><p class="tiny">The event will use the audience you choose above. Hours are logged only after you confirm attendance and mark the event done.</p><button class="btn">Publish event</button></form>`,
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
        '<a class="btn" href="/discover" data-action="close">Find an opportunity</a>',
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
    `${eventImage(e, "event-detail-image")}<span class="tag">${e.visibility === "crew_only" ? "Club members only" : "Public event"}</span><p style="margin:18px 0">${esc(e.description)}</p><p class="section-note">${esc(e.organization_name)}<br>${eventWhen(e)}<br>${esc(e.location_label || "")}${e.location_label ? " · " : ""}${esc(e.address || e.city)}</p>${finished ? '<p class="match">✓ Completed · participant hours have been logged.</p>' : started ? `<p class="section-note">${e.can_manage ? "Confirm attendance when the event is finished." : "Your organizer will confirm attendance and log hours when this event is done."}</p>${e.can_manage && e.status === "published" ? button("Mark event done", "complete-event", e.id) : ""}` : `<p class="tiny">${e.spots} spots available</p>${button(e.joined ? "Leave this event" : e.spots ? "Join this event" : "Event full", e.joined ? "leave" : e.spots ? "join" : "close", e.id, e.joined ? "secondary" : "")} ${e.joined ? button("Invite people", "invite-event", e.id, "secondary") : ""}`}`,
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
    `<h3>Event invitations</h3>${i.events.length ? i.events.map((e) => `<div class="member-row"><div><b>${esc(e.title)}</b><p class="tiny">Invited by ${esc(e.invited_by)}</p></div>${button("View", "event", e.id, "secondary small")}</div>`).join("") : '<p class="section-note">No outstanding event invitations.</p>'}<h3>Club invitations</h3>${i.crews.length ? i.crews.map((c) => `<div class="member-row"><b>${esc(c.name)}</b>${button("Accept", "join-crew", c.id, "small")}</div>`).join("") : '<p class="section-note">No outstanding club invitations.</p>'}`,
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
      navigate(current() ? "profile" : "home");
      if (!current()) requireAccount();
      break;
    case "set-location":
      locationForm();
      break;
    case "charity":
      charityDetails(id); break;
    case "register-charity":
      organizationForm("charity"); break;
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
      await eventForm(id);
      break;
    case "log":
    case "my-events":
      navigate("my-events");
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
      location.assign("/clubs?id=" + encodeURIComponent(id));
      break;
    case "all-crews":
      navigate("crews");
      break;
    case "join-crew":
      if (!requireAccount()) return;
      await mutate(
        "/api/membership",
        { organization_id: id, join: true },
        "Welcome to the community!",
      );
      $("#modal").close();
      break;
    case "leave-crew":
      modal(
        "Leave this club?",
        `<p class="section-note">You can rejoin a public club later.</p>${button("Stay in club", "close", "", "secondary")} ${button("Leave club", "confirm-leave-crew", id)}`,
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
            ? button("Set your location", "set-location", "", "secondary")
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
      if (onboarding) await onboardingStep(1); else $("#modal").close();
      return;
    }
    if (kind === "onboarding") {
      const body = profileBody();
      if (d.get("step") === "1") body.tags = [...body.tags.filter(t => t.category !== "cause"), ...d.getAll("cause").map(name => ({name, category:"cause"}))];
      else body.availability = d.getAll("day").map(day => ({weekday:Number(day),start_time:d.get("start_time"),end_time:d.get("end_time"),timezone:Intl.DateTimeFormat().resolvedOptions().timeZone}));
      await apiRequest("/api/profile", body); if (d.get("step") === "2") filters.near=true; await refresh(); await onboardingStep(Number(d.get("step"))+1); return;
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
    if (kind === "crew" || kind === "charity") {
      body = Object.fromEntries(d);
      path = kind === "charity" ? "/api/charity" : "/api/organization";
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
      await eventForm(saved.id);
    } else {
      $("#modal").close();
      if (kind === "event" || kind === "completion") {
        navigate("my-events");
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
      showFormError(f, error);
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
// Older shared links perform a real page visit before any data is requested.
const legacyPage = location.hash.slice(1);
const legacyPath = Object.hasOwn(pagePaths, legacyPage) ? pagePaths[legacyPage] : null;
if (legacyPath) location.replace(legacyPath + location.search);
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
// START PAGE LOAD
render();
if (!legacyPath) refresh().then(() => { if (new URLSearchParams(location.search).has("welcome") && current()) { onboarding = true; locationForm(); } }).catch((error) => {
  loading = false;
  $("#main").innerHTML = empty(
    "Your community is taking a moment.",
    esc(error.message),
    '<button class="btn" onclick="location.reload()">Try again</button>',
  );
});

// END PAGE LOAD
let onboarding = false;
function locationForm() {
  modal(onboarding ? "1 of 4: Where would you like to help?" : "Set location", `<form data-form="location" class="form-stack"><label>City and state, or ZIP code<input name="place" required maxlength="120" placeholder="Troy, NY or 12180" value="${esc(current()?.city || "")}"></label><p class="tiny">Your area helps us find nearby opportunities. You do not need to share a street address.</p><p class="tiny">Location search by <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap contributors</a>.</p><button class="btn">${onboarding ? "Save location and continue" : "Save location"}</button></form>`);
}
async function onboardingStep(step) {
 if (step === 1) modal("2 of 4: What matters to you?", `<form data-form="onboarding" class="form-stack"><input type="hidden" name="step" value="1"><div class="check-group">${CAUSES.map(c => `<label class="choice-chip"><input type="checkbox" name="cause" value="${esc(c)}">${esc(c)}</label>`).join("")}</div><button class="btn">Continue</button></form>`);
 else if (step === 2) modal("3 of 4: When can you help?", `<form data-form="onboarding" class="form-stack"><input type="hidden" name="step" value="2"><div class="check-group">${DAYS.map((d,i) => `<label><input type="checkbox" name="day" value="${i}">${d}</label>`).join("")}</div><label>From<input type="time" name="start_time" value="09:00" required></label><label>Until<input type="time" name="end_time" value="13:00" required></label><p class="tiny">Leave days unchecked if your schedule varies.</p><button class="btn">Find opportunities</button></form>`);
 else {
  onboarding=false; history.replaceState(null,"","/discover"); page="discover"; await refresh();
  const causes=current().tags.filter(t => t.category === "cause").map(t => t.name);
  const rows=opportunities.filter(e => (!causes.length || e.causes.some(c => causes.includes(c))) && e.distance_km != null && e.distance_km <= current().max_distance_km && (!current().availability.length || current().availability.some(a => {
    const parts = date => Object.fromEntries(new Intl.DateTimeFormat("en-US", {timeZone:a.timezone,weekday:"short",hour:"2-digit",minute:"2-digit",hourCycle:"h23",year:"numeric",month:"2-digit",day:"2-digit"}).formatToParts(new Date(date)).map(p => [p.type,p.value]));
    const start=parts(e.starts_at), end=parts(e.ends_at);
    return ["Sun","Mon","Tue","Wed","Thu","Fri","Sat"][a.weekday] === start.weekday && start.day===end.day && start.month===end.month && start.year===end.year && start.hour+":"+start.minute >= a.start_time.slice(0,5) && end.hour+":"+end.minute <= a.end_time.slice(0,5);
  }))).slice(0,4);
  modal("4 of 4: Suggested opportunities", rows.length ? rows.map(eventCard).join("") : `<p>No opportunities fit your choices yet. Explore all opportunities or adjust your preferences.</p>${button("Explore opportunities", "close")}`);
 }
}
