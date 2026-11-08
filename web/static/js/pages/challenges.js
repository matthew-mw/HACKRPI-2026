"use strict";
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
      ["crews", "Clubs"],
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
pageViews["challenges"] = challenges;
