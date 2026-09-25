import { afterEach, describe, expect, it, vi } from "vitest";
import {
  getOwnProfile,
  getPublicProfile,
  listProjectMessages,
  setProfilePrivacy,
  updateProfile,
  updateProjectWorkContext,
  uploadProjectAttachment,
} from "./apiClient";

afterEach(()=>vi.unstubAllGlobals());
const response=(body:unknown,status=200)=>new Response(JSON.stringify(body),{status,headers:{"Content-Type":"application/json"}});

describe("Project Chat API",()=>{
	it("uses an opaque cursor and authenticated pagination",async()=>{const fetchMock=vi.fn().mockResolvedValue(response({messages:[],next_cursor:"next"}));vi.stubGlobal("fetch",fetchMock);await expect(listProjectMessages("https://hub.test","secret","project 1","opaque+/=")).resolves.toEqual({messages:[],next_cursor:"next"});const [url,init]=fetchMock.mock.calls[0];expect(url).toContain("cursor=opaque%2B%2F%3D");expect(init.headers.Authorization).toBe("Bearer secret")});
	it("lets the browser set the multipart boundary",async()=>{const fetchMock=vi.fn().mockResolvedValue(response({id:"m1",attachments:[]},201));vi.stubGlobal("fetch",fetchMock);await uploadProjectAttachment("https://hub.test","secret","p1",new File(["hello"],"note.txt",{type:"text/plain"}),"");const init=fetchMock.mock.calls[0][1];expect(init.body).toBeInstanceOf(FormData);expect(init.headers["Content-Type"]).toBeUndefined();expect(init.headers.Authorization).toBe("Bearer secret")});
});

describe("Work context API",()=>{it("sends explicit manual state",async()=>{const fetchMock=vi.fn().mockResolvedValue(response({project_id:"p1",user_id:"u1",working:false,status_mode:"manual"}));vi.stubGlobal("fetch",fetchMock);await updateProjectWorkContext("https://hub.test","secret","p1",{working:false,status_mode:"manual",current_branch:"main"});expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({working:false,status_mode:"manual",current_branch:"main"})})});

describe("Profile API", () => {
  it("fetches the caller's own profile with auth headers", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      response({ user_id: "u1", username: "alice", email: "alice@example.com", privacy: [] }),
    );
    vi.stubGlobal("fetch", fetchMock);
    await expect(getOwnProfile("https://hub.test", "secret")).resolves.toMatchObject({ username: "alice" });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("https://hub.test/api/v1/users/me");
    expect(init.headers.Authorization).toBe("Bearer secret");
  });

  it("fetches another user's filtered public profile", async () => {
    const fetchMock = vi.fn().mockResolvedValue(response({ user_id: "u2", username: "bob" }));
    vi.stubGlobal("fetch", fetchMock);
    await getPublicProfile("https://hub.test", "secret", "u2");
    const [url] = fetchMock.mock.calls[0];
    expect(url).toBe("https://hub.test/api/v1/users/u2/profile");
  });

  it("sends only the profile fields being changed", async () => {
    const fetchMock = vi.fn().mockResolvedValue(response({ user_id: "u1", username: "alice" }));
    vi.stubGlobal("fetch", fetchMock);
    await updateProfile("https://hub.test", "secret", { bio: "hello" });
    const [, init] = fetchMock.mock.calls[0];
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body)).toEqual({ bio: "hello" });
  });

  it("sends privacy settings as a list", async () => {
    const fetchMock = vi.fn().mockResolvedValue(response({ user_id: "u1", username: "alice" }));
    vi.stubGlobal("fetch", fetchMock);
    await setProfilePrivacy("https://hub.test", "secret", [{ field: "bio", audience: "friends" }]);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("https://hub.test/api/v1/users/profile/privacy");
    expect(JSON.parse(init.body)).toEqual({ settings: [{ field: "bio", audience: "friends" }] });
  });
});
