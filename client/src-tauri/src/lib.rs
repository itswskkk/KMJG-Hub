use rusqlite::{params, Connection};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::{
    fs,
    path::{Path, PathBuf},
};
use tauri::{AppHandle, Manager};

const KEYRING_SERVICE: &str = "com.kmjghub.client";

fn database(app: &AppHandle) -> Result<Connection, String> {
    let dir = app.path().app_data_dir().map_err(|e| e.to_string())?;
    fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    let db = Connection::open(dir.join("client.sqlite3")).map_err(|e| e.to_string())?;
    db.execute_batch("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;
        CREATE TABLE IF NOT EXISTS saved_sessions(server_url TEXT PRIMARY KEY, auth_json TEXT NOT NULL, updated_at INTEGER NOT NULL);
        CREATE TABLE IF NOT EXISTS project_repositories(project_id TEXT PRIMARY KEY, path TEXT NOT NULL);")
        .map_err(|e| e.to_string())?;
    Ok(db)
}

fn keyring_account(server_url: &str) -> String {
    format!("session:{:x}", Sha256::digest(server_url.as_bytes()))
}

#[derive(Serialize, Deserialize)]
struct SavedSession {
    server_url: String,
    auth_json: String,
    token: String,
}

#[tauri::command]
fn save_session(
    app: AppHandle,
    server_url: String,
    auth_json: String,
    token: String,
) -> Result<(), String> {
    keyring::Entry::new(KEYRING_SERVICE, &keyring_account(&server_url))
        .map_err(|e| e.to_string())?
        .set_password(&token)
        .map_err(|e| e.to_string())?;
    database(&app)?.execute("INSERT INTO saved_sessions(server_url,auth_json,updated_at) VALUES(?1,?2,strftime('%s','now')) ON CONFLICT(server_url) DO UPDATE SET auth_json=excluded.auth_json,updated_at=excluded.updated_at",params![server_url,auth_json]).map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
fn load_sessions(app: AppHandle) -> Result<Vec<SavedSession>, String> {
    let db = database(&app)?;
    let mut statement = db
        .prepare("SELECT server_url,auth_json FROM saved_sessions ORDER BY updated_at DESC")
        .map_err(|e| e.to_string())?;
    let rows = statement
        .query_map([], |row| {
            Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?))
        })
        .map_err(|e| e.to_string())?;
    let mut sessions = Vec::new();
    for row in rows {
        let (server_url, auth_json) = row.map_err(|e| e.to_string())?;
        if let Ok(token) = keyring::Entry::new(KEYRING_SERVICE, &keyring_account(&server_url))
            .and_then(|entry| entry.get_password())
        {
            sessions.push(SavedSession {
                server_url,
                auth_json,
                token,
            })
        }
    }
    Ok(sessions)
}

#[tauri::command]
fn delete_session(app: AppHandle, server_url: String) -> Result<(), String> {
    if let Ok(entry) = keyring::Entry::new(KEYRING_SERVICE, &keyring_account(&server_url)) {
        let _ = entry.delete_credential();
    }
    database(&app)?
        .execute(
            "DELETE FROM saved_sessions WHERE server_url=?1",
            params![server_url],
        )
        .map_err(|e| e.to_string())?;
    Ok(())
}

#[derive(Serialize)]
struct GitRepository {
    path: String,
    branch: String,
}

fn git_dir(path: &Path) -> Result<PathBuf, String> {
    let dot_git = path.join(".git");
    if dot_git.is_dir() {
        return Ok(dot_git);
    }
    if dot_git.is_file() {
        let value = fs::read_to_string(&dot_git).map_err(|e| e.to_string())?;
        if let Some(relative) = value.trim().strip_prefix("gitdir: ") {
            let candidate = PathBuf::from(relative);
            return Ok(if candidate.is_absolute() {
                candidate
            } else {
                path.join(candidate)
            });
        }
    }
    Err("The selected folder is not a Git repository".into())
}

fn read_branch(path: &Path) -> Result<String, String> {
    let head = fs::read_to_string(git_dir(path)?.join("HEAD")).map_err(|e| e.to_string())?;
    let head = head.trim();
    Ok(head
        .strip_prefix("ref: refs/heads/")
        .map(str::to_owned)
        .unwrap_or_else(|| head.chars().take(12).collect()))
}

#[tauri::command]
fn select_git_repository(
    app: AppHandle,
    project_id: String,
) -> Result<Option<GitRepository>, String> {
    let Some(path) = rfd::FileDialog::new()
        .set_title("Select this Project's Git repository")
        .pick_folder()
    else {
        return Ok(None);
    };
    let branch = read_branch(&path)?;
    let path_text = path.to_string_lossy().to_string();
    database(&app)?.execute("INSERT INTO project_repositories(project_id,path) VALUES(?1,?2) ON CONFLICT(project_id) DO UPDATE SET path=excluded.path",params![project_id,path_text]).map_err(|e|e.to_string())?;
    Ok(Some(GitRepository {
        path: path.to_string_lossy().to_string(),
        branch,
    }))
}

#[tauri::command]
fn current_git_repository(
    app: AppHandle,
    project_id: String,
) -> Result<Option<GitRepository>, String> {
    let db = database(&app)?;
    let mut statement = db
        .prepare("SELECT path FROM project_repositories WHERE project_id=?1")
        .map_err(|e| e.to_string())?;
    let mut rows = statement
        .query(params![project_id])
        .map_err(|e| e.to_string())?;
    let Some(row) = rows.next().map_err(|e| e.to_string())? else {
        return Ok(None);
    };
    let path: String = row.get(0).map_err(|e| e.to_string())?;
    let branch = read_branch(Path::new(&path))?;
    Ok(Some(GitRepository { path, branch }))
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .invoke_handler(tauri::generate_handler![
            save_session,
            load_sessions,
            delete_session,
            select_git_repository,
            current_git_repository
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

#[cfg(test)]
mod tests {
    use super::read_branch;
    use std::{
        fs,
        time::{SystemTime, UNIX_EPOCH},
    };

    fn repository(head: &str) -> std::path::PathBuf {
        let path = std::env::temp_dir().join(format!(
            "kmjg-git-test-{}-{}",
            std::process::id(),
            SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        fs::create_dir_all(path.join(".git")).unwrap();
        fs::write(path.join(".git/HEAD"), head).unwrap();
        path
    }

    #[test]
    fn reads_named_branch() {
        let path = repository("ref: refs/heads/feature/session\n");
        assert_eq!(read_branch(&path).unwrap(), "feature/session");
        fs::remove_dir_all(path).unwrap()
    }

    #[test]
    fn reports_short_detached_commit() {
        let path = repository("1234567890abcdef\n");
        assert_eq!(read_branch(&path).unwrap(), "1234567890ab");
        fs::remove_dir_all(path).unwrap()
    }
}
