const { chromium } = require('playwright');

(async()=>{
  const browser = await chromium.launch({headless:true});
  const page = await browser.newPage({viewport:{width:1100,height:900}});
  await page.goto('http://127.0.0.1:8878/', {waitUntil:'networkidle'});
  await page.waitForTimeout(500);

  await page.click('button[data-page="accounts"]');
  await page.waitForTimeout(300);
  await page.click('.account-row button[data-act="edit"]');
  await page.waitForTimeout(300);

  const card = await page.locator('.modal-card').evaluate(e=>({clientWidth:e.clientWidth,scrollWidth:e.scrollWidth}));
  if(card.scrollWidth > card.clientWidth + 1) throw new Error(`horizontal overflow remains: ${JSON.stringify(card)}`);
  const footer = await page.locator('.account-editor-footer').boundingBox();
  const modalBody = await page.locator('#modal-body').boundingBox();
  if(!footer || !modalBody || footer.x < modalBody.x - 1 || footer.x + footer.width > modalBody.x + modalBody.width + 1) throw new Error('footer background exceeds modal content bounds');
  if(await page.locator('.vless-flow').isVisible().catch(()=>false)) throw new Error('Vision Flow is visible for XHTTP + NONE');

  await page.click('#account-advanced summary');
  await page.waitForTimeout(250);
  if(!await page.locator('#expert-xhttp-mode').count()) throw new Error('XHTTP mode field missing');
  if(!await page.locator('#expert-xhttp-padding').count()) throw new Error('XHTTP padding field missing');
  if(!await page.locator('#expert-xhttp-streamsecs').count()) throw new Error('XHTTP stream field missing');
  await page.screenshot({path:'edit-account-v015.png',fullPage:true});

  await page.click('#modal-close');
  await page.click('#add-account');
  await page.waitForTimeout(300);
  if(await page.locator('input[name="port"]').inputValue() !== '42317') throw new Error('suggested port was not prefilled');
  const range = await page.locator('.account-port-note').textContent();
  if(!range.includes('20000') || !range.includes('60000')) throw new Error(`configured port range missing: ${range}`);
  await page.selectOption('#network-select','xhttp');
  await page.selectOption('#security-select','reality');
  if(await page.locator('.vless-flow').isVisible()) throw new Error('Vision Flow is visible on new XHTTP');
  await page.click('#account-advanced summary');
  await page.waitForTimeout(150);
  if(!await page.locator('#expert-xhttp-mode').count() || !await page.locator('#expert-xhttp-buffered').count()) throw new Error('new-account XHTTP advanced fields missing');
  await page.screenshot({path:'new-xhttp-v015.png',fullPage:true});

  await page.click('#modal-close');
  await page.click('button[data-page="system"]');
  await page.waitForTimeout(300);
  const panelPort = page.locator('#settings-form [name="panelListen"]');
  const basePath = page.locator('#settings-form [name="panelBasePath"]');
  if(await panelPort.inputValue() !== '50010') throw new Error('panel port is not displayed as a port-only value');
  if(await page.getByText('面板端口',{exact:true}).count() !== 1) throw new Error('面板端口 label missing');
  if(await page.locator('#settings-form [name="currentPassword"]').count()) throw new Error('permanent current-password field still exists');
  const a = await panelPort.boundingBox(), b = await basePath.boundingBox();
  if(Math.abs(a.y-b.y) > 2 || Math.abs(a.height-b.height) > 2) throw new Error(`panel port/Base Path misaligned: ${JSON.stringify({a,b})}`);
  await page.locator('[name="newPassword"]').fill('new-password-123');
  if(await page.locator('#settings-current-password-wrap input[type="password"]').count() !== 1) throw new Error('conditional current-password confirmation did not appear');
  await page.locator('[name="newPassword"]').fill('');
  if(await page.locator('#settings-current-password-wrap').count()) throw new Error('conditional current-password confirmation did not disappear');
  await page.screenshot({path:'system-v015.png',fullPage:true});

  await browser.close();
})().catch(err=>{console.error(err);process.exit(1)});
