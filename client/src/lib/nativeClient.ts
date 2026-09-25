import { invoke } from "@tauri-apps/api/core";
import { AuthResponse } from "./apiClient";

export const isTauri=()=>"__TAURI_INTERNALS__" in window;
interface NativeSavedSession{server_url:string;auth_json:string;token:string}
export interface SavedSession{serverUrl:string;auth:AuthResponse}
export interface GitRepository{path:string;branch:string}

export async function saveSession(serverUrl:string,auth:AuthResponse):Promise<void>{if(!isTauri())return;const {token,...session}=auth.session;await invoke("save_session",{serverUrl,authJson:JSON.stringify({...auth,session}),token})}
export async function loadSessions():Promise<SavedSession[]>{if(!isTauri())return[];const rows=await invoke<NativeSavedSession[]>("load_sessions");return rows.flatMap((row)=>{try{const auth=JSON.parse(row.auth_json) as AuthResponse;auth.session.token=row.token;return[{serverUrl:row.server_url,auth}]}catch{return[]}})}
export async function deleteSession(serverUrl:string):Promise<void>{if(isTauri())await invoke("delete_session",{serverUrl})}
export async function selectGitRepository(projectId:string):Promise<GitRepository|null>{if(!isTauri())throw new Error("Repository detection is available in the desktop app");return invoke("select_git_repository",{projectId})}
export async function currentGitRepository(projectId:string):Promise<GitRepository|null>{if(!isTauri())return null;return invoke("current_git_repository",{projectId})}

/** Developer Tools launcher (docs/VISION.md "Developer Tools"): opens a
 * terminal, VS Code, or a command-line coding tool at a Project's local
 * directory, using the path already stored via `selectGitRepository`. */
export type DeveloperTool="codex"|"claude"|"opencode";
export async function launchTerminal(projectId:string):Promise<void>{if(!isTauri())throw new Error("Developer Tools are available in the desktop app");await invoke("launch_terminal",{projectId})}
export async function launchVSCode(projectId:string):Promise<void>{if(!isTauri())throw new Error("Developer Tools are available in the desktop app");await invoke("launch_vscode",{projectId})}
export async function launchTool(projectId:string,tool:DeveloperTool):Promise<void>{if(!isTauri())throw new Error("Developer Tools are available in the desktop app");await invoke("launch_tool",{projectId,tool})}
export async function isToolInstalled(tool:string):Promise<boolean>{if(!isTauri())return false;return invoke("is_tool_installed",{tool})}

/** Opens an external URL (e.g. GitHub OAuth) in the system browser: via the
 * Tauri opener plugin in the desktop app, or a new tab in a plain browser. */
export async function openExternalUrl(url:string):Promise<void>{if(isTauri()){const {openUrl}=await import("@tauri-apps/plugin-opener");await openUrl(url);return}window.open(url,"_blank","noopener,noreferrer")}
