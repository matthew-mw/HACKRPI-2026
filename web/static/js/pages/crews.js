"use strict";
function crewCard(c) {
  return `<article class="card"><div class="crew-cover">${c.visibility === "public" ? "🌱" : "🤝"}</div><div class="card-pad"><span class="tag">${c.joined ? "YOUR CLUB" : c.membership_status === "invited" ? "YOU’RE INVITED" : esc(c.visibility.replace("_", " "))}</span><h2 style="margin-top:12px">${esc(c.name)}</h2><p class="section-note">${esc(c.description || "A community of people making a difference together.")}</p><div class="crew-stats"><div><b>${c.members}</b><small>members</small></div><div><b>${Number(c.verified_hours)}</b><small>verified event hours</small></div></div>${button("Explore club →", "crew", c.id, "secondary")} ${!c.joined ? button(c.membership_status === "invited" ? "Accept invitation" : "Join club", "join-crew", c.id, "small") : ""}</div></article>`;
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
          "Every great club starts with one person.",
          "Start a volunteer group or come back as your community grows.",
          button("Create your club", "create-crew", "", "secondary"),
        ))
  );
}
function crewDashboard() {
  const c = data.crews.find((c) => c.id === crewID);
  if (!c)
    return empty(
      "Club unavailable.",
      "This club may be private or no longer available.",
      button("All clubs", "all-crews", "", "secondary"),
    );
  const d = crewDetails || {};
  return (
    heading(
      "YOUR PEOPLE, YOUR IMPACT",
      esc(c.name),
      `${c.members} members · ${Number(c.verified_hours)} verified event hours`,
      button("← All clubs", "all-crews", "", "secondary"),
    ) +
    `<div class="card card-pad"><div class="section-heading"><h2>Make good plans together</h2>${c.joined ? button("Invite people", "invite-crew", c.id, "small") : button("Join club", "join-crew", c.id)}</div><p class="section-note">${esc(c.description)}</p>${c.joined ? `<h3>Your club’s availability</h3>${(d.members || []).map((m) => `<div class="member-row">${avatar(m.name)}<div><b>${esc(m.name)}</b><p class="tiny">${m.availability.length ? m.availability.map((a) => `${DAYS[a.weekday]} ${a.start_time.slice(0, 5)}–${a.end_time.slice(0, 5)} (${esc(a.timezone)})`).join(" · ") : "No availability set"}${m.has_location ? "" : " · Location needed for group matching"}</p></div></div>`).join("")}<p class="section-note">Group matches require enough spots and must fit every active member’s saved availability and travel distance.</p>` : '<p class="tiny">Join to see member availability and group matches.</p>'}</div><h2 style="margin:25px 0 15px">Upcoming club events</h2>${(d.events || []).length ? `<div class="card card-pad">${d.events.map(upcomingMini).join("")}</div>` : empty("Good plans are on the way.", "No upcoming events have been published by this club.")}${c.joined ? `<h2 style="margin:25px 0 15px">Events that fit the whole club</h2>${(d.matches || []).length ? `<div class="card card-pad">${d.matches.map((e) => `<div class="member-row"><div><h3>${esc(e.title)}</h3><p class="tiny">${eventWhen(e)} · ${esc(e.address || e.city)}</p></div>${button("View event", "event", e.id, "secondary small")}</div>`).join("")}</div>` : empty("No shared match yet.", "Each member needs a location and availability in Preferences.")}<div style="margin-top:20px">${c.owner_id === current()?.id ? button("Publish a club event", "create-event", c.id, "secondary") : button("Leave club", "leave-crew", c.id, "secondary")}</div>` : ""}`
  );
}
pageViews["crews"] = crews;
