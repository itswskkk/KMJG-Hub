import { afterEach, describe, expect, it, vi } from "vitest";
import { listProjectMessages, updateProjectWorkContext, uploadProjectAttachment } from "./apiClient";

afterEach(()=>vi.unstubAllGlobals());
const response=(body:unknown,status=200)=>new Response(JSON.stringify(body),{status,headers:{"Content-Type":"application/json"}});

describe("Project Chat API",()=>{
	it("uses an opaque cursor and authenticated pagination",async()=>{const fetchMock=vi.fn().mockResolvedValue(response({messages:[],next_cursor:"next"}));vi.stubGlobal("fetch",fetchMock);await expect(listProjectMessages("https://hub.test","secret","project 1","opaque+/=")).resolves.toEqual({messages:[],next_cursor:"next"});const [url,init]=fetchMock.mock.calls[0];expect(url).toContain("cursor=opaque%2B%2F%3D");expect(init.headers.Authorization).toBe("Bearer secret")});
	it("lets the browser set the multipart boundary",async()=>{const fetchMock=vi.fn().mockResolvedValue(response({id:"m1",attachments:[]},201));vi.stubGlobal("fetch",fetchMock);await uploadProjectAttachment("https://hub.test","secret","p1",new File(["hello"],"note.txt",{type:"text/plain"}),"");const init=fetchMock.mock.calls[0][1];expect(init.body).toBeInstanceOf(FormData);expect(init.headers["Content-Type"]).toBeUndefined();expect(init.headers.Authorization).toBe("Bearer secret")});
});

describe("Work context API",()=>{it("sends explicit manual state",async()=>{const fetchMock=vi.fn().mockResolvedValue(response({project_id:"p1",user_id:"u1",working:false,status_mode:"manual"}));vi.stubGlobal("fetch",fetchMock);await updateProjectWorkContext("https://hub.test","secret","p1",{working:false,status_mode:"manual",current_branch:"main"});expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({working:false,status_mode:"manual",current_branch:"main"})})});
