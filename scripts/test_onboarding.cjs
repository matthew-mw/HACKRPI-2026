const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const source = fs.readFileSync('web/static/js/app.js','utf8');
let html = '';
const element = {addEventListener(){}, close(){}, showModal(){}, classList:{add(){},remove(){}}, get innerHTML(){return html}, set innerHTML(v){html=v}};
const context = vm.createContext({document:{querySelector:()=>element,addEventListener(){},querySelectorAll:()=>[]},window:{addEventListener(){}},location:{pathname:'/',search:'',hash:''},Intl,URLSearchParams,console,setTimeout,clearTimeout,CAUSES:['Environment','Education'],DAYS:['Sunday','Monday','Tuesday','Wednesday','Thursday','Friday','Saturday']});
// Avoid starting network loading; retain the actual UI functions and event handlers.
vm.runInContext(source.replace(/\/\/ START PAGE LOAD[\s\S]*?\/\/ END PAGE LOAD/, ''), context);
for (const file of fs.readdirSync('web/static/js/pages')) vm.runInContext(fs.readFileSync('web/static/js/pages/'+file,'utf8'),context);
vm.runInContext(`data.profile={id:'volunteer',first_name:'Test',last_name:'Person',tags:[{name:'Environment',category:'cause'},{name:'Environment',category:'interest'}],availability:[],max_distance_km:25}; modal=(title,content)=>{globalThis.title=title;globalThis.content=content}; preferences();`,context);
assert.match(context.content,/Abilities you can offer/);
assert.match(context.content,/type="checkbox" name="cause"/);
assert.doesNotMatch(context.content,/comma separated/);
vm.runInContext('locationForm()',context);
assert.match(context.content,/City and state, or ZIP code/);
vm.runInContext('onboardingStep(1)',context);
assert.match(context.title,/2 of 4/);
vm.runInContext('onboardingStep(2)',context);
assert.match(context.content,/type="time"/);
vm.runInContext('data.charities=[{id:"charity",name:"Test charity",approval_status:"pending"}]; globalThis.card=charities()',context);
assert.doesNotMatch(context.card,/Approve charity/);
vm.runInContext('data.profile.is_site_admin=true; globalThis.card=charities()',context);
assert.match(context.card,/Approve charity/);
console.log('Preference chips, location, onboarding, and charity approval UI checks passed.');
vm.runInContext('organizationForm()',context);
assert.doesNotMatch(context.content, /value="charity"/);
vm.runInContext('location.pathname="/charities"; organizationForm("charity")',context);
assert.match(context.content, /data-form="charity"/);
assert.match(context.content, /type="hidden" name="organization_type" value="charity"/);
assert.doesNotMatch(context.content, /<select name="organization_type"/);
assert.match(context.content, /Only you and site admins/);
assert.match(context.content, /name="image_url"/);
vm.runInContext(`data.charities=[{id:"charity",name:"Community aid",description:"Food & friendship",image_url:"https://example.org/photo.jpg",website:"https://example.org",approval_status:"approved",visibility:"public",members:3}]; charityDetails("charity")`,context);
assert.match(context.content, /Food &amp; friendship/);
assert.match(context.content, /charity-image/);
assert.match(context.content, /Visit website/);
assert.match(context.content, /Join charity/);
assert.doesNotMatch(context.content, /Create event/);
vm.runInContext('data.charities[0].joined=true; charityDetails("charity")',context);
assert.match(context.content, /Create event/);
vm.runInContext('data.charities[0].joined=false; data.charities[0].visibility="closed"; charityDetails("charity")',context);
assert.doesNotMatch(context.content, /Join charity/);
console.log('Charity registration, details, and membership actions passed.');
vm.runInContext(`
window.scrollTo=()=>{};
location.assign=path=>{location.pathname=path;};
for (const [key,path] of Object.entries(pagePaths)) {
 if (pageForPath(path)!==key) throw new Error("Wrong route for "+path);
 navigate(key);
 if (location.pathname!==path) throw new Error("Navigation failed: "+key);
}

`, context);
assert.doesNotMatch(source, /href="#|location\.hash\s*=/);
console.log('Clean page URLs and document navigation passed.');
(async () => {
  vm.runInContext(`
    globalThis.requests=[]; globalThis.scripts=[];
    loadPageScript=async key=>scripts.push(key);
    render=()=>{};
    apiRequest=async path=>{requests.push(path); return path.startsWith('/api/community') ? {profile:{id:'test'},invitations:{events:[],crews:[]}} : [];};
  `,context);
  for (const [path,key] of [['/charities','charities'],['/profile','profile'],['/discover','discover'],['/','home'],['/clubs','crews'],['/my-events','my-events'],['/impact','impact'],['/challenges','challenges']]) {
    context.location.pathname=path;
    vm.runInContext('requests.length=0; scripts.length=0;',context);
    await vm.runInContext('refresh()',context);
    assert.equal(context.scripts.join(','),key);
    const expected=['/api/community?page='+key];
    if(key==='discover') expected.push('/api/opportunities?');
    if(key==='home'||key==='profile') expected.push('/api/people');
    assert.equal(context.requests.length,expected.length, path);
    expected.forEach((prefix,i)=>assert.ok(context.requests[i].startsWith(prefix),path));
  }
  console.log('Every page loads only its own script and expected data requests.');
})().catch(error=>{console.error(error);process.exitCode=1;});
vm.runInContext(`
  data.profile={id:'test',first_name:'Test',last_name:'Person',tags:[{name:'Environment',category:'cause'},{name:' environment ',category:'interest'}]};
  data.stats={events:0,hours:0,verified_hours:0,organizations:0,causes:0,active_weeks:0,by_cause:[]};
  data.crews=[]; data.my_activity=[]; people=[];
  globalThis.profileMarkup=profile();
`,context);
assert.equal((context.profileMarkup.match(/Environment/g)||[]).length,1);
assert.doesNotMatch(context.profileMarkup,/ environment /);
assert.match(context.profileMarkup,/Set location/);
vm.runInContext(`
 globalThis.customImage=eventImage({title:'Food & friends', image_url:'https://example.org/event.jpg',causes:[]});
 globalThis.fallbackImage=eventImage({title:'Event',image_url:'javascript:alert(1)',causes:['Environment']});
 globalThis.errorSteps=[];
 document.createElement=()=>({setAttribute(k,v){errorSteps.push([k,v]);},focus(){errorSteps.push('focused');},scrollIntoView(){errorSteps.push('scrolled');}});
 showFormError({prepend(message){globalThis.visibleError=message;errorSteps.push('prepended');}},new Error('Enter a full street address, including city, state, and ZIP code.'));
`,context);
assert.match(context.customImage,/https:\/\/example.org\/event.jpg/);
assert.match(context.customImage,/Food &amp; friends/);
assert.match(context.fallbackImage,/garden.jpg/);
assert.doesNotMatch(context.fallbackImage,/javascript:/);
assert.equal(context.visibleError.textContent,'Enter a full street address, including city, state, and ZIP code.');
assert.deepEqual(Array.from(context.errorSteps).slice(-3),['prepended','focused','scrolled']);
console.log('Event pictures and visible, focused validation errors passed.');
