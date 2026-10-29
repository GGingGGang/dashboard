import { test, expect } from '@playwright/test';

test('Atlas home keeps keyboard focus through polling and all workspace routes remain available', async ({page}) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.goto('/');
  await expect(page.getByRole('heading', {name:'좋은 하루예요.'})).toBeVisible();
  const open = page.getByRole('button', {name:'내 대시보드 열기'});
  await open.focus();
  await page.waitForTimeout(2300);
  await expect(open).toBeFocused();
  await open.press('Enter');
  await expect(page.locator('.dashboard-page')).toBeVisible();
  for (const [route, heading] of [['overview','수집 상세'],['history','빌드 이력'],['metrics','지표 탐색'],['connections','연결 설정']]) {
    const link = page.locator(`nav [data-page="${route}"]`);
    await link.click();
    await expect(link).toHaveAttribute('aria-current','page');
    await expect(page.locator('#page-title')).toHaveText(heading);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  }
  const saved = await page.evaluate(async () => {
    const {api} = await import('/src/bridge/index.ts');
    return JSON.parse(await api.GetPreference('view')).page;
  });
  expect(saved).toBe('connections');
  expect(errors).toEqual([]);
});

test('Atlas narrow home escapes long names and distinguishes on-demand, failed and stale data', async ({page}) => {
  await page.setViewportSize({width:375,height:812});
  await page.goto('/');
  await expect(page.getByRole('heading', {name:'좋은 하루예요.'})).toBeVisible();
  await page.evaluate(async () => {
    const {renderHome} = await import('/src/components/home.ts');
    const {api} = await import('/src/bridge/index.ts');
    const state = await api.GetState();
    state.connections[0].name = '<img src=x onerror=alert(1)>' + '긴연결이름'.repeat(30);
    state.snapshots[0].lastSuccess = '2000-01-01T00:00:00Z';
    state.snapshots[1].error = 'upstream unavailable';
    const metrics = state.providers.find(p => p.kind === 'prometheus')!;
    metrics.onDemand = true;
    renderHome(document.querySelector<HTMLElement>('#page')!, state);
  });
  await expect(page.locator('#page img')).toHaveCount(0);
  await expect(page.getByText('필요할 때 조회',{exact:true})).toBeVisible();
  await expect(page.getByText('수집 대기 · 지연',{exact:true})).toBeVisible();
  await expect(page.getByText('확인 필요',{exact:true})).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});
