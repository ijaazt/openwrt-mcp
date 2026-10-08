"""Exercise all operation cards in an isolated host, including themes and mobile width."""
import os
import shutil
import json
import pathlib
import time
from playwright.sync_api import sync_playwright

root = pathlib.Path(__file__).resolve().parent.parent
html = (root / 'ui/card.html').read_text()
out = root / 'ui/test-results'
out.mkdir(exist_ok=True)
host = '''<iframe title="Router card" style="width:100%;height:950px;border:0" src="/widget"></iframe>
<script>window.requests=[];addEventListener('message',e=>{const m=e.data;requests.push(m);if(m.id!==undefined){const result=m.method==='ui/initialize'?{protocolVersion:'2026-01-26',hostInfo:{name:'test',version:'1'},hostCapabilities:{serverTools:{}},hostContext:{theme:'light',displayMode:'inline',availableDisplayModes:['inline']}}:m.method==='tools/call'?window.fixture:{};e.source.postMessage({jsonrpc:'2.0',id:m.id,result},'*')}})</script>'''
def card(tool, **kwargs):
    return {'structuredContent': {'card': {'tool': tool, 'title': tool, 'outcome': 'OK', 'summary': 'fixture', 'scope': ['fixture'], 'observed_at': '2026-10-08T12:00:00Z', 'duration_ms': 12, 'details': 'fixture output', **kwargs}}}
fixtures = [
    card('ubus_list', details="'system' @abc\n  \"board\":{}", refresh={'name':'ubus_list','arguments':{}}),
    card('ubus_call', data={'model':'Linksys WHW03','hostname':'fixture','kernel':'6.12','release':{'version':'25.12.5'}}, refresh={'name':'ubus_call','arguments':{'object':'system','method':'board'}}),
    card('uci_get', details="network.lan.proto='static'\nwireless.radio.key=<redacted>"),
    card('uci_apply', changes=[{'config':'system','section':'fixture','option':'description','value':'<img src=x onerror=alert(1)>'}], rollback_deadline=time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime(time.time()+90)), notice='Verify behavior before confirming.', inspect={'name':'uci_get','arguments':{'config':'system'}}),
    card('uci_confirm', details='Confirmed. Rollback cancelled.'),
    card('exec', details='fixture stdout'),
    card('logread', details='daemon.info Service started\ndaemon.err Fixture error\ndaemon.warn Fixture warning'),
    card('wg_new_client', details='Client created. Private key is not copied into this card.'),
    card('mfa_unlock', details='Unlocked for 15 minutes.'),
    card('exec', outcome='DENIED', details='denied: no policy grants exec'),
    card('uci_apply', outcome='ERROR', details='No such configuration'),
]
with sync_playwright() as p:
    browser = p.chromium.launch(headless=True, executable_path=os.environ.get("CHROMIUM_PATH") or shutil.which("chromium"))
    page = browser.new_page(viewport={'width': 390, 'height': 1100})
    failures=[]
    page.on('pageerror',lambda e:failures.append(str(e)))
    page.route('http://card.test/',lambda route:route.fulfill(content_type='text/html',body=host))
    page.route('http://card.test/widget',lambda route:route.fulfill(content_type='text/html',body=html))
    page.goto('http://card.test/')
    page.wait_for_function("requests.some(r=>r.method==='ui/notifications/initialized')")
    frame = page.frames[1]
    for i,fixture in enumerate(fixtures):
        page.evaluate("data=>{window.fixture=data;document.querySelector('iframe').contentWindow.postMessage({jsonrpc:'2.0',method:'ui/notifications/tool-result',params:data},'*')}",fixture)
        frame.get_by_role('heading',name=fixture['structuredContent']['card']['title'],exact=True).wait_for()
        assert frame.locator('img').count()==0, 'Unescaped router data'
        assert not frame.evaluate('document.documentElement.scrollWidth > innerWidth'), 'Horizontal overflow'
        expected=fixture['structuredContent']['card']
        if expected.get('refresh'):
            frame.get_by_role('button',name='Refresh',exact=True).click()
            page.wait_for_function("requests.some(r=>r.method==='tools/call')")
        if expected['tool']=='uci_apply' and expected['outcome']=='OK':
            frame.get_by_role('button',name='Inspect current settings',exact=True).click()
            page.wait_for_function("requests.some(r=>r.method==='tools/call'&&r.params.name==='uci_get')")
            assert not page.evaluate("requests.some(r=>r.method==='tools/call'&&['uci_apply','uci_confirm'].includes(r.params.name))"), 'Mutation replayed'
        page.screenshot(path=str(out/f'{i:02}-{expected["tool"]}-{expected["outcome"]}.png'))
    page.evaluate("()=>document.querySelector('iframe').contentWindow.postMessage({jsonrpc:'2.0',method:'ui/notifications/host-context-changed',params:{theme:'dark'}},'*')")
    frame.wait_for_function("document.documentElement.dataset.theme==='dark'")
    page.screenshot(path=str(out/'dark.png'))
    assert not failures, failures
    browser.close()
print(json.dumps({'result':'PASS','cards':len(fixtures),'checks':['SDK handshake','all nine operations','errors and denials','read-only refresh','mutation not replayed','escaping','mobile width','dark theme']}))
