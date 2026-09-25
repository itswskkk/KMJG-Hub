import { beforeEach, describe, expect, it, vi } from "vitest";

const {invoke}=vi.hoisted(()=>({invoke:vi.fn()}));
vi.mock("@tauri-apps/api/core",()=>({invoke}));
import { loadSessions, saveSession } from "./nativeClient";

beforeEach(()=>{invoke.mockReset();Object.defineProperty(window,"__TAURI_INTERNALS__",{value:{},configurable:true})});

describe("secure native session bridge",()=>{
	it("passes the token separately from SQLite metadata",async()=>{invoke.mockResolvedValue(undefined);await saveSession("https://hub.test",{user:{id:"u1",username:"k",email:"k@example.test",created_at:"now"},session:{token:"top-secret",expires_at:"later"}});expect(invoke).toHaveBeenCalledWith("save_session",expect.objectContaining({token:"top-secret"}));const payload=invoke.mock.calls[0][1];expect(payload.authJson).not.toContain("top-secret")});
	it("reassembles a session returned from the OS credential vault",async()=>{invoke.mockResolvedValue([{server_url:"https://hub.test",auth_json:JSON.stringify({user:{id:"u1"},session:{expires_at:"later"}}),token:"secret"}]);const sessions=await loadSessions();expect(sessions[0].auth.session.token).toBe("secret")});
});
