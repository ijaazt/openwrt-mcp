import json,pathlib,runpy,shutil
from playwright.sync_api import sync_playwright
root=pathlib.Path(__file__).resolve().parent.parent
baseline=runpy.run_path(str(root/'scripts/verify-ui.py'))
fixtures=baseline['fixtures'];html=(root/'ui/card.html').read_text()
host='''<meta name="viewport" content="width=device-width,initial-scale=1"><style>body{margin:0}iframe{width:100%;border:0}</style><iframe src="/widget"></iframe><script>window.requests=[];addEventListener('message',e=>{const m=e.data;requests.push(m);if(m.method==='ui/notifications/size-changed')document.querySelector('iframe').style.height=m.params.height+'px';if(m.id!==undefined){let result=m.method==='ui/initialize'?{protocolVersion:'2026-01-26',hostInfo:{name:'mobile-test',version:'1'},hostCapabilities:{serverTools:{}},hostContext:{theme:window.theme,displayMode:'inline',availableDisplayModes:['inline']}}:m.method==='tools/call'?window.fixture:{};e.source.postMessage({jsonrpc:'2.0',id:m.id,result},'*')}})</script>'''
results=[];issues=[];out=root/'ui/test-results/mobile';out.mkdir(parents=True,exist_ok=True)
with sync_playwright() as p:
 browser=p.chromium.launch(headless=True,executable_path=shutil.which('chromium'))
 for width,height in [(320,568),(375,667),(390,844),(430,932),(844,390)]:
  for theme in ['light','dark']:
   context=browser.new_context(viewport={'width':width,'height':height},is_mobile=True,has_touch=True,device_scale_factor=2,color_scheme=theme)
   page=context.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
   page.route('http://mobile.test/',lambda r:r.fulfill(content_type='text/html',body=host));page.route('http://mobile.test/widget',lambda r:r.fulfill(content_type='text/html',body=html))
   page.add_init_script('window.theme='+json.dumps(theme));page.goto('http://mobile.test/');page.wait_for_function("requests.some(r=>r.method==='ui/notifications/initialized')");frame=page.frames[1]
   for i,fixture in enumerate(fixtures):
    page.evaluate("data=>{window.fixture=data;document.querySelector('iframe').contentWindow.postMessage({jsonrpc:'2.0',method:'ui/notifications/tool-result',params:data},'*')}",fixture)
    frame.get_by_role('heading',name=fixture['structuredContent']['card']['title'],exact=True).wait_for()
    assert not frame.evaluate('document.documentElement.scrollWidth > innerWidth'),(width,theme,i,'card overflow')
    assert not page.evaluate('document.documentElement.scrollWidth > innerWidth'),(width,theme,i,'host overflow')
    details=frame.locator('summary');details.tap();assert frame.locator('details').evaluate('(n)=>n.open')
    assert not frame.evaluate('document.documentElement.scrollWidth > innerWidth'),(width,theme,i,'expanded overflow')
    details.tap();assert not frame.locator('details').evaluate('(n)=>n.open')
    for node in [*frame.locator('button').all(),details]:
     box=node.bounding_box()
     if box['height']<44:issues.append({'width':width,'theme':theme,'tool':fixture['structuredContent']['card']['tool'],'target':node.inner_text(),'height':box['height']})
    button=frame.locator('button')
    if button.count():
     before=page.evaluate('requests.filter(r=>r.method==="tools/call").length');button.tap();page.wait_for_function('before=>requests.filter(r=>r.method==="tools/call").length>before',arg=before)
    assert not page.evaluate("requests.some(r=>r.method==='tools/call'&&['uci_apply','uci_confirm'].includes(r.params.name))")
    frame.evaluate('getSelection().removeAllRanges()')
    if i in [1,3,6,9]:page.screenshot(path=str(out/f'{width}-{theme}-{i}.png'),full_page=True)
    results.append({'width':width,'height':height,'theme':theme,'tool':fixture['structuredContent']['card']['tool'],'outcome':fixture['structuredContent']['card']['outcome'],'passed':True})
   page.evaluate("()=>document.querySelector('iframe').contentWindow.postMessage({jsonrpc:'2.0',method:'ui/notifications/host-context-changed',params:{styles:{variables:{'--color-text-secondary':'rgb(123, 124, 125)','--color-background-primary':'rgb(30, 31, 32)','--font-sans':'serif','--font-heading-sm-size':'19px','--border-radius-lg':'18px'}}}},'*')")
   frame.wait_for_function("getComputedStyle(document.documentElement).getPropertyValue('--color-text-secondary').trim()==='rgb(123, 124, 125)'")
   assert frame.locator('body').evaluate("n=>getComputedStyle(n).backgroundColor")=='rgb(30, 31, 32)'
   assert frame.locator('h1').evaluate("n=>getComputedStyle(n).fontSize")=='19px'
   assert 'serif' in frame.locator('h1').evaluate("n=>getComputedStyle(n).fontFamily")
   assert not errors,errors
   context.close()
 browser.close()
report={'cases':len(results),'result':'PASS' if not issues else 'TAP_TARGET_ISSUES','touch_target_issues':issues,'viewports':[320,375,390,430,844],'checks':['touch emulation','host style updates','light/dark','all nine operations','errors/denials','page/card overflow','expand/collapse output','read controls','no mutation replay'],'results':results}
(out/'report.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps({k:v for k,v in report.items() if k!='results'}))
