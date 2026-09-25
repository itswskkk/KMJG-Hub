import { useEffect, useRef, useState } from "react";
import { ProjectWorkContext, updateProjectWorkContext } from "../../lib/apiClient";
import { GitRepository, currentGitRepository, isTauri, selectGitRepository } from "../../lib/nativeClient";
import { useProjectWorkContexts } from "./useProjectWorkContexts";
import "./WorkContextPanel.css";

interface Props{serverUrl:string;token:string;projectId:string;viewerUserId:string}

export default function WorkContextPanel({serverUrl,token,projectId,viewerUserId}:Props){
	const {contexts,loading,reload}=useProjectWorkContexts(serverUrl,token,projectId);const current=contexts[viewerUserId];
	const currentRef=useRef(current);useEffect(()=>{currentRef.current=current},[current]);
	const [repo,setRepo]=useState<GitRepository|null>(null);const [error,setError]=useState<string|null>(null);const [saving,setSaving]=useState(false);
	async function report(repository:GitRepository|null,override?:Partial<ProjectWorkContext>){const latest=currentRef.current;const value={working:override?.working??latest?.working??Boolean(repository),status_mode:override?.status_mode??latest?.status_mode??"automatic",current_branch:repository?.branch??latest?.current_branch??""} as const;setSaving(true);try{const updated=await updateProjectWorkContext(serverUrl,token,projectId,value);currentRef.current=updated;reload()}catch(err){setError(err instanceof Error?err.message:"Could not update work status.")}finally{setSaving(false)}}
	useEffect(()=>{if(!isTauri())return;currentGitRepository(projectId).then(setRepo).catch((err)=>setError(String(err)));const timer=window.setInterval(()=>{currentGitRepository(projectId).then((repository)=>{if(repository){setRepo(repository);if(currentRef.current?.status_mode!=="manual")void report(repository,{working:true,status_mode:"automatic"})}}).catch(()=>{})},15000);return()=>window.clearInterval(timer);// eslint-disable-next-line react-hooks/exhaustive-deps
	},[projectId]);
	useEffect(()=>{if(repo&&!loading&&current?.status_mode!=="manual")void report(repo,{working:true,status_mode:"automatic"});// eslint-disable-next-line react-hooks/exhaustive-deps
	},[repo?.path,loading]);
	async function choose(){setError(null);try{const repository=await selectGitRepository(projectId);if(repository){setRepo(repository);await report(repository,{working:true,status_mode:"automatic"})}}catch(err){setError(err instanceof Error?err.message:String(err))}}
	return <section className="work-context" aria-label="Work status">
		<div><strong>{current?.working?"Working":"Not working"}</strong>{current?.current_branch&&<span> · Branch: <code>{current.current_branch}</code></span>}{repo&&<small title={repo.path}>{repo.path}</small>}</div>
		<div className="work-context__actions"><button type="button" onClick={()=>void report(repo,{working:!current?.working,status_mode:"manual"})} disabled={saving}>{current?.working?"Stop Working":"Start Working"}</button><button type="button" onClick={()=>void report(repo,{working:Boolean(repo),status_mode:"automatic"})} disabled={saving}>Use Automatic</button><button type="button" onClick={()=>void choose()} disabled={saving||!isTauri()}>Select Repository</button></div>
		{!isTauri()&&<small>Branch detection is available in the desktop app.</small>}{error&&<small className="work-context__error">{error}</small>}
	</section>
}
