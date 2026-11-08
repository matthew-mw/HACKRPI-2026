"use strict";
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
pageViews["discover"] = discover;
