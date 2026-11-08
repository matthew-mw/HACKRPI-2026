"use strict";
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
              return `<article class="card card-pad">${eventImage(e, "event-detail-image")}<div class="section-heading"><span class="tag">${e.visibility === "crew_only" ? "CREW ONLY" : "PUBLIC"}</span><span class="tiny">${e.completed_at ? "✓ Completed" : e.status === "cancelled" ? "Cancelled" : started ? (ended ? "Awaiting completion" : "In progress") : "Upcoming"}</span></div><h2>${esc(e.title)}</h2><p class="section-note">${esc(e.organization_name)}<br>${eventWhen(e)}<br>${esc(e.address || e.city)}</p><p class="tiny">${e.can_manage ? "You’re organizing" : "You registered"} · ${e.registrations} registered${e.completed_at ? " · " + e.participants + " attended" : ""}</p>${e.completed_at ? `<p class="section-note">${((new Date(e.actual_ended_at) - new Date(e.actual_started_at)) / 3600000).toFixed(2)} hours logged per confirmed participant.</p>` : `<p class="section-note">${e.can_manage ? (started ? "Confirm who participated and the actual service time to log their hours." : "Mark this event done after it starts and service is finished.") : "Your hours will appear after the organizer completes the event and confirms your attendance."}</p>`}<div class="event-manage-actions">${button("View event", "event", e.id, "secondary small")}${e.can_manage && !e.completed_at && started && e.status === "published" ? button("Mark event done", "complete-event", e.id, "small") : ""}</div></article>`;
            })
            .join("")}</div>`
        : empty(
            "Good plans start here.",
            "Create an event or join one to see it in your calendar.",
            '<a class="btn secondary" href="/discover">Find an opportunity</a>',
          )
    }`
  );
}
pageViews["my-events"] = myEvents;
