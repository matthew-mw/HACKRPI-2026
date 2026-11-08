"use strict";
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
      .filter((t, i, tags) => tags.findIndex(x => x.name.trim().toLowerCase() === t.name.trim().toLowerCase()) === i)
      .map((t) => esc(t.name))
      .join(
        " · ",
      )}</p></div></div><div class="columns"><section><div class="card card-pad"><h2>You choose what to share</h2><p class="section-note">Activity: ${esc(p.activity_visibility)} · Hours: ${esc(p.hours_visibility)}.</p><p class="tiny">Friends means mutual follows. Private activity and hours stay out of other people’s feeds and rankings.</p><div style="margin-top:18px">${button("Manage privacy & profile", "edit-profile", "", "secondary")} ${button("Interests & availability", "preferences", "", "secondary")} ${button("Set location", "set-location", "", "secondary")}</div></div><h2 style="margin:24px 0 15px">Your activity history</h2>${data.my_activity.length ? data.my_activity.map(activityCard).join("") : empty("Your story starts with showing up.", "Join an opportunity. Your organizer will confirm attendance and log hours when the event is complete.", '<a class="btn secondary" href="/discover">Find an opportunity</a>')}</section><aside><div class="card card-pad"><h2>Your people</h2><p class="section-note">${people.filter((p) => p.following).length} following · ${people.filter((p) => p.following && p.follows_you).length} mutual connections</p>${button("Find your community", "people", "", "secondary small")}<hr class="divider"><h3>Your clubs</h3>${
      data.crews
        .filter((c) => c.joined)
        .map(
          (c) =>
            `<button class="event-mini" data-action="crew" data-id="${c.id}"><span>🌱</span><b>${esc(c.name)}</b></button>`,
        )
        .join("") ||
      '<p class="section-note">You haven’t joined a club yet.</p>'
    }</div><div class="card card-pad"><h2>Your account</h2><p class="section-note">${esc(p.email)}</p>${button("Sign out", "logout", "", "secondary small")}</div></aside></div><h2 style="margin-top:25px">Your achievements</h2>${badges()}`
  );
}
pageViews["profile"] = profile;
