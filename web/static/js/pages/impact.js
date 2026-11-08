"use strict";
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
pageViews["impact"] = impact;
