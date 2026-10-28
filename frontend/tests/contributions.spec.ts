import { test, expect } from "@playwright/test";

test.beforeEach(async ({page}) => {
  await page.addInitScript(() => {
    const saved = new Map<string,string>();
    const legacy = {version:1,widgets:[{id:"legacy",kind:"queue",title:"Keep layout",connectionId:"custom",targetId:"",expression:"",unit:"",period:900,refreshSeconds:0,x:0,y:0,w:3,h:2}]};
    saved.set("dashboard.layout",JSON.stringify(legacy));
    let enabled = true;
    const schema = {type:"object",properties:{region:{type:"string",title:"Region",enum:["local","remote"],default:"local"},limit:{type:"integer",title:"Limit",minimum:1,maximum:10,default:3}},required:["region","limit"]};
    const cards = ["table","scalar","timeseries"].map(renderer => ({id:renderer,title:`Inventory ${renderer}`,capability:"data",renderer,configSchema:schema}));
    const connection = {id:"custom",kind:"inventory",name:"Inventory source",url:"https://example.invalid",browserUrl:"",auth:"none",username:"",caFile:"",targets:[],rules:[],revision:"1",config:{region:"remote",limit:4}};
    const app = {
      GetPreference: async (key:string) => saved.get(key) || "",
      SavePreference: async (key:string,value:string) => {saved.set(key,value)},
      GetState: async () => ({error:"",demo:false,connections:[connection],snapshots:[],providers:[{kind:"inventory",name:"Inventory",category:"custom",capabilities:["data"],authMethods:["none"],defaultAuth:"none"}]}),
      Plugins: async () => [{enabled,manifest:{id:"inventory",version:"1",contractVersion:1,providers:[{id:"inventory",name:"Inventory",configSchema:schema}],cards,dashboards:[{id:"overview",title:"Inventory dashboard",cards:cards.map(c=>c.id)}],queries:[{id:"lookup",title:"Inventory lookup",capability:"data",renderer:"table",inputSchema:schema}]}}],
      SetPluginEnabled: async (_id:string,value:boolean) => {enabled=value;connection.revision=String(Number(connection.revision)+1)},
      Sources: async () => {if(!enabled)throw Error("disabled");return [{id:"default",name:"Warehouse",capabilities:["data"],revision:1}]},
      ExecuteContribution: async (_c:string,_s:string,r:{id:string,input:Record<string,unknown>}) => {
        if (!enabled) throw Error("disabled");
        if(r.id==="scalar")return {type:"scalar",value:42};
        if(r.id==="timeseries")return {type:"timeseries",series:[{labels:{service:"inventory"},points:[{time:1000,value:1},{time:1010,value:3}]}]};
        return {type:"table",columns:[{name:"Region",type:"string"},{name:"ID",type:"string"}],rows:[[r.input.region,"9223372036854775807"],["<img src=x onerror=alert(1)>","2"]]};
      },
      Presets:async()=>[], Refresh:async()=>{}, SaveConnection:async(v:{connection:typeof connection})=>{Object.assign(connection,v.connection);return connection.id},
    };
    Object.defineProperty(window,"go",{value:{main:{App:app}}});
  });
});

test("manifest dashboard renders generic data and preserves v1 layout through add and disable", async ({page}) => {
  await page.goto("/");
  await expect(page.getByText("Keep layout",{exact:true})).toBeVisible();
  await page.locator('nav [data-page="plugin:inventory:overview"]').click();
  const table = page.locator("#plugin-contributions > section").filter({has:page.getByRole("heading",{name:"Inventory table"})});
  await expect(table.getByRole("cell",{name:"9223372036854775807",exact:true})).toBeVisible();
  await expect(table.locator("img")).toHaveCount(0);
  await expect(page.locator(".db-current")).toHaveText("42");
  await expect(page.getByRole("img",{name:"시계열 그래프"})).toBeVisible();
  await page.screenshot({path:`test-results/contributions-${page.viewportSize()!.width}.png`,fullPage:true});
  await table.getByLabel("Region").selectOption("1");
  await table.getByRole("button",{name:"조회",exact:true}).click();
  await expect(table.getByRole("cell",{name:"remote",exact:true})).toBeVisible();
  await table.getByRole("button",{name:"내 대시보드에 추가"}).click();
  await page.getByRole("button",{name:"저장 · 배치 완료"}).click();
  const preferences = await page.evaluate(async () => ({layout:JSON.parse(await window.go!.main.App.GetPreference("dashboard.layout")),backup:JSON.parse(await window.go!.main.App.GetPreference("dashboard.layout.v1.backup"))}));
  expect(preferences.layout.version).toBe(2); expect(preferences.backup.version).toBe(1);
  expect(preferences.layout.widgets[0]).toEqual(preferences.backup.widgets[0]);
  expect(preferences.layout.widgets[1].extension.input.region).toBe("remote");
  await page.locator('nav [data-page="plugins"]').click();
  await page.getByRole("button",{name:"비활성화",exact:true}).click();
  await expect(page.getByRole("button",{name:"활성화",exact:true})).toBeVisible();
  await page.locator('nav [data-page="dashboard"]').click();
  await expect(page.getByText("플러그인 또는 기여 항목을 사용할 수 없습니다.",{exact:false})).toBeVisible();
  await page.locator('nav [data-page="plugins"]').click();
  await page.getByRole("button",{name:"활성화",exact:true}).click();
  await expect(page.getByRole("button",{name:"비활성화",exact:true})).toBeVisible();
  await page.locator('nav [data-page="dashboard"]').click();
  await expect(page.getByRole("cell",{name:"remote",exact:true})).toBeVisible();
});

test("query and provider forms are generated from the declared schema", async ({page}) => {
  await page.goto("/");
  await page.locator('nav [data-page="plugin:inventory:overview"]').click();
  const query = page.locator("#plugin-contributions > section").filter({has:page.getByRole("heading",{name:"Inventory lookup"})});
  await query.getByLabel("Limit").fill("99");
  await query.getByRole("button",{name:"조회",exact:true}).click();
  await expect(query.getByRole("cell")).toHaveCount(0);
  await query.getByLabel("Limit").fill("4");
  await query.getByRole("button",{name:"조회",exact:true}).click();
  await expect(query.getByRole("cell",{name:"local",exact:true})).toBeVisible();
  await page.locator('nav [data-page="connections"]').click();
  await page.getByRole("button",{name:"설정 편집",exact:true}).click();
  await expect(page.locator("#provider-config").getByLabel("Region")).toHaveValue("1");
  await expect(page.locator("#provider-config").getByLabel("Limit")).toHaveValue("4");
});

test("generic series keep gaps and required boolean accepts false", async ({page}) => {
  await page.goto("/");
  const result = await page.evaluate(async () => {
    // @ts-expect-error Browser import resolved by Vite.
    const {renderData, schemaForm, readSchema} = await import("/src/components/extensions.ts");
    const root = document.createElement("div");
    const schema = {type:"object",properties:{enabled:{type:"boolean",default:false}},required:["enabled"]};
    root.innerHTML = `<form>${schemaForm(schema)}</form>`;
    const valid = root.querySelector("form")!.checkValidity();
    const values = readSchema(root,schema);
    root.innerHTML = renderData({type:"timeseries",series:[{labels:{},points:[{time:1,value:1},{time:2,value:2},{time:3,value:null},{time:4,value:4}]}]});
    return {valid, values, lines:root.querySelectorAll("polyline").length, dots:root.querySelectorAll("circle").length};
  });
  expect(result).toEqual({valid:true,values:{enabled:false},lines:1,dots:1});
});
