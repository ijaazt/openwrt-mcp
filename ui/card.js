import { App } from '@modelcontextprotocol/ext-apps';
const app = new App({ name: 'OpenWrt operation cards', version: '1.0.0' }, { availableDisplayModes: ['inline'] });
const root = document.querySelector('#card');
let current, countdown;
const el = (tag, text, cls) => { const node = document.createElement(tag); if (text != null) node.textContent = String(text); if (cls) node.className = cls; return node; };
const format = value => value == null ? '—' : typeof value === 'object' ? JSON.stringify(value) : String(value);
const bytes = value => { if (typeof value !== 'number') return '—'; const units = ['B','KiB','MiB','GiB']; let i=0; while(value>=1024 && i<3){value/=1024;i++;} return `${value.toFixed(i ? 1 : 0)} ${units[i]}`; };
const uptime = seconds => typeof seconds === 'number' ? `${Math.floor(seconds/86400)}d ${Math.floor(seconds%86400/3600)}h ${Math.floor(seconds%3600/60)}m` : '—';
function error(message) { let node = root.querySelector('#error'); if (!node) { node = el('p'); node.id = 'error'; root.append(node); } node.textContent = message; }
function facts(items) { const group = el('div', null, 'facts'); for (const [label,value] of items) { const fact=el('div',null,'fact'); fact.append(el('span',label),el('strong',format(value)));group.append(fact); }root.append(group); }
function table(title, columns, rows, total = rows.length) {
  root.append(el('h2',title)); const wrapper=el('div',null,'table-wrap'), t=el('table'), head=el('thead'), tr=el('tr');
  for(const label of columns)tr.append(el('th',label));head.append(tr);t.append(head);
  const body=el('tbody');for(const values of rows.slice(0,50)){const row=el('tr');for(const value of values)row.append(el('td',format(value)));body.append(row);}t.append(body);wrapper.append(t);root.append(wrapper);
  if(!rows.length)root.append(el('p','No entries returned.','note'));if(total>50)root.append(el('p',`Showing 50 of ${total} entries. Expand output for the returned snapshot.`,'note'));
}
function flatten(value, prefix='', rows=[]) {
  if(rows.length>=70)return rows;
  if(value && typeof value==='object' && !Array.isArray(value))for(const [key,v] of Object.entries(value)){if(rows.length>=70)break;flatten(v,prefix?`${prefix}.${key}`:key,rows);}
  else rows.push([prefix||'Value',format(value)]);return rows;
}
function readView(c) {
  const d=c.data;
  if(c.tool==='uci_get') {
    const rows=c.details.split('\n').filter(line=>line.includes('=')).map(line=>{const i=line.indexOf('=');return [line.slice(0,i),line.slice(i+1)];});
    table('Current settings',['Setting','Value'],rows);return;
  }
  if(c.tool==='ubus_list') {
    const rows=[];let object='';for(const line of c.details.split('\n')){if(line.startsWith("'"))object=line.split("'")[1];else if(line.trim())rows.push([object,line.trim()]);}
    table('Available router methods',['Object','Method and arguments'],rows);return;
  }
  if(c.tool==='logread') {
    const lines=c.details.split('\n').filter(Boolean);const severity=line=>/\b(err|error|crit|alert|emerg)\b/i.test(line)?'error':/\bwarn(ing)?\b/i.test(line)?'warn':'';
    facts([['Lines returned',lines.length],['Errors',lines.filter(l=>severity(l)==='error').length],['Warnings',lines.filter(l=>severity(l)==='warn').length]]);
    root.append(el('h2','Recent log entries'));const logs=el('div',null,'logs');for(const line of lines.slice(-50))logs.append(el('div',line,`log ${severity(line)}`));root.append(logs);return;
  }
  if(d?.model && d?.release){facts([['Router',d.model],['Hostname',d.hostname],['OpenWrt',d.release.version],['Kernel',d.kernel]]);return;}
  if(d?.memory && d?.uptime!=null){facts([['Uptime',uptime(d.uptime)],['Free memory',bytes(d.memory.free)],['Total memory',bytes(d.memory.total)],['Load',Array.isArray(d.load)?d.load.map(n=>(n/65536).toFixed(2)).join(' / '):'—']]);return;}
  if(typeof d?.up==='boolean') {
    facts([['Connection',d.up?'Up':'Down'],['Protocol',d.proto],['Device',d.l3_device||d.device],['Uptime',uptime(d.uptime)]]);
    const addresses=[...(d['ipv4-address']||[]),...(d['ipv6-address']||[])];if(addresses.length)table('Addresses',['Address','Prefix'],addresses.map(a=>[a.address,a.mask]));
    if(d['dns-server']?.length)table('DNS servers',['Server'],d['dns-server'].map(a=>[a]));return;
  }
  const leases=d?.dhcp_leases||d?.dhcp6_leases;
  if(Array.isArray(leases)){facts([['Leases returned',leases.length]]);table('DHCP leases',['Device','IP address','MAC','Expires'],leases.map(l=>[l.hostname||l.name||'Unnamed',l.ipaddr||l.ip,l.macaddr||l.mac,l.expires]));return;}
  if(d!=null) { table('Router result',['Field','Value'],flatten(d));return; }
  root.append(el('pre',c.details));
}
function render(result) {
  const c=result?.structuredContent?.card;
  if(!c){error(result?.content?.filter(x=>x.type==='text').map(x=>x.text).join('\n')||'No operation card was returned.');return;}
  current=c;clearInterval(countdown);root.replaceChildren();
  const pending=Boolean(c.rollback_deadline)&&c.outcome==='OK'; const status=c.outcome==='DENIED'?'denied':c.outcome==='ERROR'?'error':pending?'pending':'ok';
  const header=el('header'), mark=el('div','◉','mark'), heading=el('div',null,'heading');
  heading.append(el('h1',c.title),el('p',(c.scope||[]).join(' · ')||c.tool,'scope'));header.append(mark,heading,el('span',status==='ok'?'Complete':status==='pending'?'Rollback armed':status==='denied'?'Access denied':'Error',`badge ${status}`));root.append(header);
  if(status==='error'||status==='denied') {
    const banner=el('div',null,`banner ${status}`);banner.append(el('strong',status==='denied'?'This operation is not permitted':'The operation did not complete'),el('p',c.details));root.append(banner);
  } else if(c.tool==='uci_apply') {
    const banner=el('div',null,'banner pending');banner.append(el('strong','Change applied · awaiting confirmation'));
    const clock=el('div',null,'countdown');banner.append(clock,el('p',c.notice));root.append(banner);
    const tick=()=>{const remaining=Math.max(0,Math.ceil((Date.parse(c.rollback_deadline)-Date.now())/1000));clock.textContent=Number.isNaN(remaining)?'Check the returned rollback deadline':remaining?`${remaining}s until automatic rollback`:'Rollback window elapsed — inspect current settings to verify the result.';};tick();countdown=setInterval(tick,1000);
    const changes=Array.isArray(c.changes)?c.changes:[];table('Requested changes',['Setting','Action','Value'],changes.map(x=>[`${x.config}.${x.section}${x.option?'.'+x.option:''}`,x.delete?'Delete':x.type?'Create section':'Set',x.delete?'—':x.type||x.value]));
  } else if(c.tool==='uci_confirm'||c.tool==='mfa_unlock'||c.tool==='wg_new_client') {
    const banner=el('div',null,'banner');banner.append(el('strong',c.tool==='uci_confirm'?'Changes retained':c.tool==='mfa_unlock'?'Access window opened':'Client created'),el('p',c.details));root.append(banner);
  } else readView(c);
  if(c.notice && c.tool!=='uci_apply')root.append(el('p',c.notice,'note'));
  if(c.details){const details=el('details');details.append(el('summary','View returned output'),el('pre',c.details));root.append(details);}
  const footer=el('footer');
  const action=c.refresh||c.inspect;
  if(action){const button=el('button',c.refresh?'Refresh':'Inspect current settings');button.onclick=async()=>{button.disabled=true;try{render(await app.callServerTool(action));}catch{error('Read failed. This snapshot is unchanged. No write was repeated.');}finally{button.disabled=false;}};footer.append(button);}
  const date=new Date(c.observed_at);footer.append(el('small',`Snapshot ${Number.isNaN(date.valueOf())?'just now':date.toLocaleTimeString([],{hour:'2-digit',minute:'2-digit'})} · ${c.duration_ms??0} ms`));root.append(footer);
}
const theme=context=>{if(context?.theme)document.documentElement.dataset.theme=context.theme;};
app.ontoolresult=render;app.ontoolcancelled=()=>error('Operation cancelled. Check current settings before retrying a write.');app.onhostcontextchanged=theme;
// The initial tool result is delivered by the host; never call the tool on startup.
if(window.openai?.toolOutput)render({structuredContent:window.openai.toolOutput});
window.addEventListener('openai:set_globals',event=>{const output=event.detail?.globals?.toolOutput;if(output)render({structuredContent:output});});
app.connect().then(()=>theme(app.getHostContext())).catch(()=>{if(!current)error('This card needs a connected MCP Apps host.');});
