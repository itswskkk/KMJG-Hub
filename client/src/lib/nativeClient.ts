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

/** Opens an external URL (e.g. GitHub OAuth) in the system browser: via the
 * Tauri opener plugin in the desktop app, or a new tab in a plain browser. */
export async function openExternalUrl(url:string):Promise<void>{if(isTauri()){const {openUrl}=await import("@tauri-apps/plugin-opener");await openUrl(url);return}window.open(url,"_blank","noopener,noreferrer")}
