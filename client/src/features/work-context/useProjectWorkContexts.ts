import { useEffect, useMemo, useState } from "react";
import { ProjectWorkContext, listProjectWorkContexts } from "../../lib/apiClient";
import { useProjectPresence, useProjectWorkContextEvents } from "../presence/PresenceProvider";

export function useProjectWorkContexts(serverUrl:string,token:string,projectId:string){
	const [values,setValues]=useState<Record<string,ProjectWorkContext>>({});
	const [loading,setLoading]=useState(true);
	const [reloadKey,setReloadKey]=useState(0);
	const events=useProjectWorkContextEvents(projectId);const presence=useProjectPresence(projectId);
	useEffect(()=>{let cancelled=false;listProjectWorkContexts(serverUrl,token,projectId).then((contexts)=>{if(!cancelled)setValues(Object.fromEntries(contexts.map((value)=>[value.user_id,value]))) }).catch(()=>{}).finally(()=>{if(!cancelled)setLoading(false)});return()=>{cancelled=true}},[serverUrl,token,projectId,reloadKey]);
	useEffect(()=>{const latest=events[events.length-1];if(latest)setValues((current)=>({...current,[latest.user_id]:latest}))},[events]);
	const visible=useMemo(()=>Object.fromEntries(Object.entries(values).filter(([userId])=>presence.isOnline(userId)===true)),[values,presence]);
	return {contexts:visible,loading,reload:()=>setReloadKey((value)=>value+1)};
}
