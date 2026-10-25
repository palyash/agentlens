const esc=s=>String(s).replace(/[&<>"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c])),$=s=>document.querySelector(s),COL=['#2a78d6','#eb6834','#1baf7a','#eda100','#6250d6','#e34948'];
const money=n=>n==null?'n/a':'$'+n.toFixed(n<10?2:1),tk=n=>n>=1e6?(n/1e6).toFixed(1)+'M':n>=1e3?(n/1e3).toFixed(1)+'k':Math.round(n)+'';
const when=t=>t?new Date(t).toLocaleString([], {month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}):'';
let D=null,mc={};const col=m=>mc[m]||(mc[m]=COL[Object.keys(mc).length%COL.length]);
const kpi=(l,v,s)=>`<div class="k"><small>${l}</small><b>${v}</b><small>${s||''}</small></div>`;
const bars=(a,f)=>{const mx=Math.max(...a.map(x=>x.v),1e-9);return '<table>'+a.map(x=>`<tr><td style="width:30%">${x.n}</td><td><div class="tr"><div class="bar" style="width:${100*x.v/mx}%"></div></div></td><td class="num">${f(x.v)}</td></tr>`).join('')+'</table>'};
async function load(){const p=$('#proj').value;D=await (await fetch(`/api/data?days=${$('#days').value}&project=${encodeURIComponent(p)}`)).json();
const s=$('#proj'),cur=s.value;s.innerHTML='<option value="">All projects</option>'+D.projectList.map(x=>`<option ${x==cur?'selected':''}>${x}</option>`).join('');route()}
function overview(){const k=D.kpi,ms=D.models.map(m=>m.n);if(!k.calls)return '<p class="mut">No requests in this range.</p>';
const mx=Math.max(...D.daily.map(d=>Object.values(d.m).reduce((a,b)=>a+b,0)),1e-9);
const ch=D.daily.map(d=>`<div class="col" title="${d.d}">${ms.filter(m=>d.m[m]).map(m=>`<i style="height:${170*d.m[m]/mx}px;background:${col(m)}"></i>`).join('')}</div>`).join('');
return `<div class="kpis">${kpi('Spend (API-equivalent est.)',money(k.cost),k.sessions+' sessions, '+money(k.cost/Math.max(k.sessions,1))+' avg')}${kpi('Tokens in / out',tk(k.i+k.cr+k.cw)+' / '+tk(k.o),'in includes cache')}${kpi('Cache hit rate',k.cache+'%',k.cache<80?'below 80%':'healthy')}${kpi('Requests',tk(k.calls),'thinking ~'+tk(k.think)+' tok est.')}</div>
<div class="c"><h3>Cost per day by model</h3><div class="lg">${ms.map(m=>`<span><i style="background:${col(m)}"></i>${m}</span>`).join('')}</div><div class="chart">${ch}</div><div class="lbl">${D.daily.map(d=>`<span>${d.d.slice(5)}</span>`).join('')}</div></div>
<div class="g2"><div class="c"><h3>Spend by project</h3>${bars(D.projects.map(x=>({n:x.n,v:x.v})),money)}</div>
<div class="c"><h3>Spend by token type</h3>${bars(Object.entries(D.kinds).map(([n,v])=>({n,v})),money)}</div></div>
<div class="c"><h3>Spend by model</h3>${bars(D.models,money)}</div>`}
function sessions(){return `<div class="c"><table><tr><th>Started</th><th>Project</th><th>Task</th><th class="num">Reqs</th><th class="num">Tools</th><th class="num">Tokens</th><th class="num">Cache</th><th class="num">Cost</th></tr>`+
D.sessions.map(s=>`<tr class="r" onclick="location.hash='#/session/${s.id}'"><td>${when(s.start)}</td><td>${s.project}</td><td style="max-width:280px;overflow:hidden;text-overflow:ellipsis">${esc(s.prompt)||'<span class="mut">no prompt</span>'}</td><td class="num">${s.reqs}</td><td class="num">${s.tools}</td><td class="num">${tk(s.tokens)}</td><td class="num ${s.cache<70?'bad':''}">${s.cache}%</td><td class="num">${money(s.cost)}</td></tr>`).join('')+'</table></div>'}
async function detail(id){const s=await (await fetch('/api/session/'+id)).json();if(!s.calls)return '<p>Session not found.</p>';
const mx=Math.max(...s.calls.map(c=>c.i+c.cr+c.cw)),tot=s.calls.reduce((a,c)=>a+(c.cost||0),0);
return `<p><a href="#/sessions">Back to sessions</a></p><h3>${esc(s.prompt||s.id)}</h3><p class="mut">${s.project} ${s.branch?'on '+s.branch:''}, ${s.calls.length} requests, ${money(tot)}</p>
<div class="c"><table><tr><th>#</th><th>Time</th><th>Model</th><th class="num">Input</th><th class="num">Cache read</th><th class="num">Cache write</th><th class="num">Output</th><th class="num">Cost</th><th>Context</th><th>Tools</th></tr>`+
s.calls.map((c,i)=>`<tr><td>${i+1}</td><td>${when(c.ts)}</td><td>${c.model}</td><td class="num">${tk(c.i)}</td><td class="num">${tk(c.cr)}</td><td class="num">${tk(c.cw)}</td><td class="num">${tk(c.o)}${c.think?' <span class="mut">(~'+tk(c.think)+' think)</span>':''}</td><td class="num">${money(c.cost)}</td><td><div class="tr"><div class="bar" style="width:${100*(c.i+c.cr+c.cw)/mx}%"></div></div></td><td>${c.tools.map(t=>`<span class="chip ${t.n.includes('MCP')?'m':''}" title="${esc(t.arg)}">${t.n}${t.tok>2000?' ~'+tk(t.tok):''}</span>`).join('')||'<span class="mut">reply</span>'}</td></tr>`).join('')+'</table></div>'}
function tools(){const b=D.bash,s=b.sum,pc=(e,c)=>c?Math.round(100*e/c):0,bd=p=>p>=10?'bad':'',mx=Math.max(...b.days.map(d=>d.tok),1);
return `<div class="kpis">${kpi('Bash calls',tk(s.calls),pc(s.err,s.calls)+'% failed')}${kpi('Carry cost of Bash output',money(s.carry),'est., re-read in context')}${kpi('Saved if output capped at 2k',money(s.save),'est.')}${kpi('Retries after a failure',s.retries,'same call right after it failed')}</div>
<div class="c"><div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-bottom:8px"><h3 style="margin:0;flex:1">Tool usage per day</h3><select onchange="TM=this.value;drawTools()"><option value="tok" ${TM=='tok'?'selected':''}>Est. tokens</option><option value="carry" ${TM=='carry'?'selected':''}>Price (carry cost, est.)</option></select><select onchange="TT=this.value;drawTools()"><option value="">All tools, stacked</option>${D.tools.map(t=>`<option ${TT==t.n?'selected':''}>${esc(t.n)}</option>`).join('')}</select></div><div id="tchart"></div></div>
<div class="g2"><div class="c"><h3>Bash by command</h3><table><tr><th>Command</th><th class="num">Calls</th><th class="num">Failed</th><th class="num">Est. tokens</th><th class="num">Carry</th></tr>${b.fams.map(f=>`<tr><td>${esc(f.n)}</td><td class="num">${f.calls}</td><td class="num ${bd(pc(f.err,f.calls))}">${pc(f.err,f.calls)}%</td><td class="num">~${tk(f.tok)}</td><td class="num">${money(f.carry)}</td></tr>`).join('')}</table></div>
<div class="c"><h3>Largest Bash outputs</h3><table><tr><th>Command</th><th class="num">Est. tokens</th><th class="num">Carry</th></tr>${b.top.map(t=>`<tr><td style="max-width:240px;overflow:hidden;text-overflow:ellipsis" title="${esc(t.a)}">${t.err?'<span class="bad">failed </span>':''}${esc(t.a)}</td><td class="num">~${tk(t.tok)}</td><td class="num">${money(t.carry)}</td></tr>`).join('')}</table></div></div>
<div class="c"><h3>All tools: outcomes and carry cost</h3><table><tr><th>Tool</th><th class="num">Calls</th><th class="num">Failed</th><th class="num">Denied</th><th class="num">Fail rate</th><th class="num">Est. tokens</th><th class="num">Carry</th></tr>${D.tools.map(t=>`<tr><td>${esc(t.n)}</td><td class="num">${t.calls}</td><td class="num ${t.err?'bad':''}">${t.err}</td><td class="num">${t.den}</td><td class="num ${bd(pc(t.err,t.calls))}">${pc(t.err,t.calls)}%</td><td class="num">~${tk(t.chars/4)}</td><td class="num">${money(t.carry)}</td></tr>`).join('')}</table></div>
<div class="g2"><div class="c"><h3>Most-failed commands</h3>${D.fails.map(f=>`<div class="ins"><span class="bad">${f.v}x</span> ${esc(f.n)} ${esc(f.a)}</div>`).join('')||'<p class="mut">No failures.</p>'}</div>
<div class="c"><h3>Waste finder</h3>${D.insights.map(x=>`<div class="ins">${esc(x)}</div>`).join('')||'<p class="mut">Nothing flagged.</p>'}</div></div>
<p class="mut">Failed = tool result flagged as an error. Denied = you rejected the call. Token counts and carry cost are estimates: result size / 4, times the cache-read price of every later request in that session.</p>`}
let TM='tok',TT='';
function drawTools(){const m=TM,days=D.tday,tot={};
days.forEach(d=>Object.entries(d.t).forEach(([n,v])=>tot[n]=(tot[n]||0)+v[m]));
const names=Object.keys(tot).sort((a,b)=>tot[b]-tot[a]),keep=TT?[TT]:names.slice(0,6),other=!TT&&names.length>6;
const rows=days.map(d=>{const o={};let ot=0;Object.entries(d.t).forEach(([n,v])=>{if(keep.includes(n))o[n]=v[m];else ot+=v[m]});if(other)o.Other=ot;return o});
const series=other?[...keep,'Other']:keep,colr=n=>n=='Other'?'#898781':COL[keep.indexOf(n)%COL.length],f=m=='tok'?tk:money;
const sum=r=>Object.values(r).reduce((a,b)=>a+b,0),mx=Math.max(...rows.map(sum),1e-9);
const p10=Math.pow(10,Math.floor(Math.log10(mx/4))),st=[1,2,2.5,5,10].map(x=>x*p10).find(x=>x*4>=mx),top=st*4;
const W=900,H=320,L=62,B=30,T=22,R=10,pw=W-L-R,ph=H-T-B,n=Math.max(rows.length,1),bw=Math.min(44,pw/n*.7),y=v=>T+ph-ph*v/top,tx='style="fill:var(--mut);font-size:11px"';
let g='';for(let i=0;i<=4;i++){const v=st*i;g+=`<line x1="${L}" x2="${W-R}" y1="${y(v)}" y2="${y(v)}" style="stroke:var(--ln)"/><text x="${L-8}" y="${y(v)+4}" text-anchor="end" ${tx}>${f(v)}</text>`}
const skip=Math.ceil(n/14);
rows.forEach((r,i)=>{const cx=L+(i+.5)*pw/n;let acc=0;
series.forEach(k=>{const v=r[k]||0;if(v<=0)return;g+=`<rect x="${cx-bw/2}" y="${y(acc+v)}" width="${bw}" height="${Math.max(y(acc)-y(acc+v)-1,0)}" rx="2" fill="${colr(k)}"><title>${esc(k)}: ${f(v)}</title></rect>`;acc+=v});
if(acc>0&&n<=16)g+=`<text x="${cx}" y="${y(acc)-5}" text-anchor="middle" ${tx}>${f(acc)}</text>`;
if(i%skip==0)g+=`<text x="${cx}" y="${H-10}" text-anchor="middle" ${tx}>${days[i].d.slice(5)}</text>`});
const lg=series.map(k=>`<span><i style="background:${colr(k)}"></i>${esc(k)} ${f(k=='Other'?rows.reduce((a,r)=>a+(r.Other||0),0):tot[k])}</span>`).join('');
$('#tchart').innerHTML=`<div class="lg">${lg}</div><svg viewBox="0 0 ${W} ${H}" width="100%" role="img" aria-label="Stacked bars of ${m=='tok'?'estimated tokens':'estimated price'} per day by tool">${g}</svg><p class="mut" style="margin:4px 0 0">${m=='tok'?'Estimated tokens each tool returned into context (result size / 4).':'Estimated price of keeping those results in context: tokens x later requests x cache-read price.'}</p>`}
const cp=i=>navigator.clipboard.writeText(D.fixes[i].fix);
function fixes(){if(!D.fixes.length)return '<p class="mut">Nothing to fix yet. Use Claude Code for a while and check back.</p>';
return '<p class="mut">Ranked by estimated saving per week. These are estimates, not billed amounts. This page uses all your data and ignores the filters above.</p>'+D.fixes.map((f,i)=>`<div class="c"><div style="display:flex;gap:10px;align-items:baseline;flex-wrap:wrap"><h3 style="margin:0;flex:1">${esc(f.title)}</h3><b>~${money(f.save)} / week</b></div><ul>${f.evidence.map(x=>`<li>${esc(x)}</li>`).join('')}</ul><pre class="pre">${esc(f.fix)}</pre><button class="btn" onclick="cp(${i});this.textContent='Copied'">Copy fix</button></div>`).join('')}
async function route(){const h=location.hash||'#/';document.querySelectorAll('#nav a').forEach(a=>a.classList.toggle('on',a.getAttribute('href')==h));
if(!D)return;$('#v').innerHTML=h.startsWith('#/session/')?await detail(h.slice(10)):h=='#/sessions'?sessions():h=='#/tools'?tools():h=='#/overview'?overview():fixes();if(h=='#/tools')drawTools()}
$('#days').onchange=$('#proj').onchange=load;addEventListener('hashchange',route);load();
