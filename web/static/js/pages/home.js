"use strict";
function rightRail() {
  const next = (data.upcoming || []).slice(0, 3);
  return `<aside class="right-rail"><div class="card right-card"><h2>Your next good thing</h2><p class="tiny" style="margin-top:7px">A little time, already well spent.</p>${next.length ? next.map(upcomingMini).join("") : '<p class="section-note">Your calendar has room for a little good. Find an opportunity to get started.</p>'}<hr class="divider"><a class="text-button" href="/discover">Explore opportunities →</a></div><div class="card right-card challenge-feature"><div class="eyebrow">GOOD GROWS WITH YOU</div><span class="badge-icon">🌞</span><h2>The 10-hour club</h2><p class="section-note">A personal goal, powered by the service you actually log.</p>${current() ? `<div class="progress"><span style="width:${Math.min(100, Number(data.stats.hours) * 10)}%"></span></div><p class="tiny">${Number(data.stats.hours)} of 10 hours</p>` : ""}<a class="text-button" href="/challenges">Explore your goals →</a></div></aside>`;
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
    `<section class="hero"><div class="hero-content"><div class="eyebrow">GOOD THINGS HAPPEN WHEN WE SHOW UP</div><h2>Your community.<br>Your kind of impact.</h2><p>Discover a cause you care about and turn a few hours into something meaningful.</p><a class="btn light" href="/discover">Find an opportunity ↗</a></div><div class="hero-art" aria-hidden="true">🌱</div></section>` +
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
pageViews["home"] = home;
