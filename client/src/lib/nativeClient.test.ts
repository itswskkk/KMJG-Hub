import { beforeEach, describe, expect, it, vi } from "vitest";

const {invoke}=vi.hoisted(()=>({invoke:vi.fn()}));
vi.mock("@tauri-apps/api/core",()=>({invoke}));
import { isToolInstalled, launchTerminal, launchTool, launchVSCode, loadSessions, saveSession } from "./nativeClient";

beforeEach(()=>{invoke.mockReset();Object.defineProperty(window,"__TAURI_INTERNALS__",{value:{},configurable:true})});

describe("secure native session bridge",()=>{
	it("passes the token separately from SQLite metadata",async()=>{invoke.mockResolvedValue(undefined);await saveSession("https://hub.test",{user:{id:"u1",username:"k",email:"k@example.test",created_at:"now"},session:{token:"top-secret",expires_at:"later"}});expect(invoke).toHaveBeenCalledWith("save_session",expect.objectContaining({token:"top-secret"}));const payload=invoke.mock.calls[0][1];expect(payload.authJson).not.toContain("top-secret")});
	it("reassembles a session returned from the OS credential vault",async()=>{invoke.mockResolvedValue([{server_url:"https://hub.test",auth_json:JSON.stringify({user:{id:"u1"},session:{expires_at:"later"}}),token:"secret"}]);const sessions=await loadSessions();expect(sessions[0].auth.session.token).toBe("secret")});
});

describe("developer tools launcher",()=>{
	it("launches a terminal by project id only, never a raw path",async()=>{invoke.mockResolvedValue(undefined);await launchTerminal("proj-1");expect(invoke).toHaveBeenCalledWith("launch_terminal",{projectId:"proj-1"})});
	it("launches VS Code by project id",async()=>{invoke.mockResolvedValue(undefined);await launchVSCode("proj-1");expect(invoke).toHaveBeenCalledWith("launch_vscode",{projectId:"proj-1"})});
	it("launches a CLI tool by project id and tool name",async()=>{invoke.mockResolvedValue(undefined);await launchTool("proj-1","claude");expect(invoke).toHaveBeenCalledWith("launch_tool",{projectId:"proj-1",tool:"claude"})});
	it("checks tool installation via PATH lookup",async()=>{invoke.mockResolvedValue(true);expect(await isToolInstalled("codex")).toBe(true);expect(invoke).toHaveBeenCalledWith("is_tool_installed",{tool:"codex"})});
	it("refuses to launch outside the desktop app",async()=>{delete (window as {__TAURI_INTERNALS__?:unknown}).__TAURI_INTERNALS__;await expect(launchTerminal("proj-1")).rejects.toThrow();expect(invoke).not.toHaveBeenCalled()});
});
