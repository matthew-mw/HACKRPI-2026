"use strict";
function charityImage(c) {
  return c.image_url && /^https:\/\//i.test(c.image_url)
    ? `<img class="charity-image" src="${esc(c.image_url)}" alt="${esc(c.name)}" loading="lazy" referrerpolicy="no-referrer">`
    : '<div class="crew-cover" aria-hidden="true">♡</div>';
}
function charityActions(c) {
  return (c.approval_status === "approved"
    ? (c.joined ? button("Create event", "create-event", c.id) : c.visibility === "public" ? button("Join charity", "join-crew", c.id) : '<p class="tiny">Contact this charity to ask about membership.</p>')
    : '<p class="tiny">Awaiting site administrator approval before members can publish events.</p>')
    + (current()?.is_site_admin && c.approval_status === "pending" ? button("Approve charity", "approve-charity", c.id) : "");
}
function charityDetails(id) {
  const c = (data.charities || []).find(c => c.id === id);
  if (!c) return;
  modal(c.name, `${charityImage(c)}<h3>About this charity</h3><p class="charity-description">${esc(c.description || "This charity has not added its story yet.")}</p><p class="section-note">${esc(c.city || "")} · ${Number(c.members || 0)} members</p>${c.website && /^https:\/\//i.test(c.website) ? `<p><a href="${esc(c.website)}" target="_blank" rel="noopener noreferrer">Visit website ↗</a></p>` : ""}<span class="tag">${esc(c.approval_status)}</span><div class="charity-actions">${charityActions(c)}</div>`);
}
function charities() {
 return heading("GIVE TOGETHER", "Charities", "Learn about local charities and help organize their next event.", button("Register a charity", "register-charity")) + `<div class="grid">${(data.charities || []).map(c => `<article class="card">${charityImage(c)}<div class="card-pad"><h2>${esc(c.name)}</h2><p>${esc(c.description || "Get to know this charity and its community.")}</p><span class="tag">${esc(c.approval_status)}</span><div class="charity-actions">${button("Learn more →", "charity", c.id, "secondary")}${charityActions(c)}</div></div></article>`).join("") || empty("Make a difference together", "Approved charities will appear here.")}</div>`;
}
pageViews["charities"] = charities;
