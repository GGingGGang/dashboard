import { test, expect } from "@playwright/test";
test("package changes show pending restart and do not alter active plugins", async ({page}) => {
 await page.addInitScript(() => {
  let pending = false, removed = false;
  const manifest = {id:"external",version:"1",contractVersion:1,providers:[],cards:[],queries:[],dashboards:[]};
  const app = {
   GetPreference:async()=>"", SavePreference:async()=>{}, Refresh:async()=>{},
   GetState:async()=>({connections:[],providers:[],snapshots:[],error:"",demo:false}),
   Plugins:async()=>[{manifest,enabled:true}],
   InstalledPlugins:async()=>[{manifest:{...manifest,version:pending?"2":"1"},pendingRestart:pending||removed,pendingRemoval:removed}],
   InstallPlugin:async(trusted:boolean)=>{if(!trusted)throw Error("trust required");pending=true;return "external"},
   UninstallPlugin:async()=>{removed=true},
  };
  Object.defineProperty(window,"go",{value:{main:{App:app}}});
 });
 await page.goto("/");await page.locator('nav [data-page="plugins"]').click();
 const install=page.getByRole("button",{name:"신뢰한 ZIP 설치 · 업그레이드"});await expect(install).toBeDisabled();
 await page.locator("#plugin-trust").check();await install.click();
 await expect(page.getByText("설치/변경 예약 · 재시작 필요",{exact:true})).toBeVisible();
 await expect(page.getByRole("heading",{name:"external 1",exact:true})).toBeVisible();
 await page.getByRole("button",{name:"다음 시작에서 제거"}).click();
 await expect(page.getByText("제거 예약 · 재시작 필요",{exact:true})).toBeVisible();
 await expect(page.getByRole("heading",{name:"external 1",exact:true})).toBeVisible();
});
