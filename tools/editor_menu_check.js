// Run with playwright-cli run-code after logging into an isolated local fixture.
// State responses are synthetic; the script cancels all edit/delete dialogs.
async page => {
  await page.unroute('**/api/v1/state');
  await page.route('**/api/v1/state', async route => {
    const response = await route.fetch();
    const state = await response.json();
    const c = state.draft.config;
    c.subscriptions = [1,2].map(n=>({id:'s'+n,name:'测试订阅'+n,url:'https://example.com/sub',pool:'测试池',enabled:true,interval:3600,healthCheck:true}));
    c.groups = [1,2].map(n=>({id:'g'+n,name:'测试策略'+n,type:'select',pools:['测试池'],builtins:['DIRECT'],interval:300}));
    c.ruleSets = [1,2].map(n=>({id:'r'+n,name:'测试规则集'+n,kind:'local',entries:[]}));
    c.rules = [1,2].map(n=>({id:'rule'+n,type:'DOMAIN',value:'example.com',target:'DIRECT'}));
    state.devices = [1,2].map(n=>({id:'d'+n,name:'测试设备'+n,state:'active',revision:1}));
    state.releases = [1,2].map(n=>({number:n,created:'2026-11-01T00:00:00Z',note:'测试发布',config:JSON.parse(JSON.stringify(c))}));
    state.active = 2;
    await route.fulfill({response,json:state});
  });
  await page.reload();
  await page.setViewportSize({width:1440,height:900});
  const assertClosed = async label => {
    if(await page.locator('.row-menu[open]').count()) throw Error(label);
  };
  for(const name of ['订阅','策略组','规则集','分流规则','发布记录','设备']) {
    await page.getByRole('link',{name,exact:true}).click();
    const triggers=page.locator('.row-menu summary');
    await triggers.first().click();
    await page.locator('h1').click();
    await assertClosed(name+' outside');
    await triggers.nth(1).click();
    await triggers.first().click();
    if(await page.locator('.row-menu[open]').count()!==1) throw Error(name+' mutual exclusion');
    await page.keyboard.press('Escape');
    await assertClosed(name+' escape');
    if(!await triggers.first().evaluate(el=>el===document.activeElement)) throw Error(name+' return focus');
    await triggers.first().focus();
    await page.keyboard.press('Enter');
    if(!await page.locator('.row-menu[open] button:not(:disabled)').first().evaluate(el=>el===document.activeElement)) throw Error(name+' keyboard open');
    await page.keyboard.press('End');
    await page.keyboard.press('Tab');
    await assertClosed(name+' tab out');
  }
  await page.getByRole('link',{name:'规则集',exact:true}).click();
  await page.locator('.row-menu summary').first().click();
  await page.getByRole('menuitem',{name:'编辑名称与来源'}).click();
  await assertClosed('drawer transition');
  await page.getByRole('button',{name:'取消',exact:true}).click();
  if(!await page.locator('.row-menu summary').first().evaluate(el=>el===document.activeElement)) throw Error('drawer focus return');
  await page.locator('.row-menu summary').first().click();
  await page.getByRole('menuitem',{name:'删除',exact:true}).click();
  await assertClosed('confirm transition');
  await page.getByRole('button',{name:'取消',exact:true}).click();
  await assertClosed('cancel confirmation');
  await page.setViewportSize({width:375,height:812});
  await page.getByRole('button',{name:'展开导航',exact:true}).click();
  await page.keyboard.press('Escape');
  if(await page.locator('#sidebar.open').count()) throw Error('navigation escape');
  await page.getByRole('button',{name:'展开导航',exact:true}).click();
  await page.locator('h1').click({position:{x:300,y:10},force:true});
  if(await page.locator('#sidebar.open').count()) throw Error('navigation outside');
  await page.locator('.row-menu summary').first().click();
  await page.locator('h1').dispatchEvent('pointerdown',{pointerType:'touch',bubbles:true});
  await assertClosed('touch outside');
  await page.screenshot({path:'output/playwright/menu-review-mobile.png'});
  await page.unroute('**/api/v1/state');
  await page.reload();
}
