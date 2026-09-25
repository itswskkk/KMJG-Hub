//! Developer Tools launcher (docs/PRD.md "Developer Tool Integration",
//! docs/VISION.md "Developer Tools"): opens a terminal, VS Code, or a
//! command-line coding tool (Codex, Claude Code, OpenCode) at a Project's
//! local directory.
//!
//! SECURITY: every command here takes a `project_id`, never a raw path. The
//! only path ever launched is the one already stored in `project_repositories`
//! by the existing `select_git_repository` flow (`resolve_project_path`) — a
//! caller cannot pass an arbitrary filesystem path in. Every process is
//! spawned with `Command::new(program).arg(..)`-style argument arrays; nothing
//! here is ever interpolated into a shell string, so unusual characters in a
//! project path cannot be used for shell injection.

use crate::database;
use rusqlite::params;
use std::{
    env,
    path::{Path, PathBuf},
    process::Command,
};
use tauri::AppHandle;

/// Resolves the local filesystem path stored for `project_id`, the same way
/// `current_git_repository` reads it. This is the only place a project id is
/// turned into a filesystem path — the frontend never supplies a path.
fn resolve_project_path(app: &AppHandle, project_id: &str) -> Result<PathBuf, String> {
    let db = database(app)?;
    let mut statement = db
        .prepare("SELECT path FROM project_repositories WHERE project_id=?1")
        .map_err(|e| e.to_string())?;
    let mut rows = statement
        .query(params![project_id])
        .map_err(|e| e.to_string())?;
    let Some(row) = rows.next().map_err(|e| e.to_string())? else {
        return Err("No local directory configured for this project".into());
    };
    let path: String = row.get(0).map_err(|e| e.to_string())?;
    Ok(PathBuf::from(path))
}

/// Candidate executable names to try for `tool` on this platform. On Windows
/// this mirrors `PATHEXT` closely enough for the small set of tools this
/// module launches; elsewhere the bare name is the only candidate.
fn candidate_names(tool: &str) -> Vec<String> {
    if cfg!(windows) && !tool.contains('.') {
        [".exe", ".cmd", ".bat", ".com"]
            .iter()
            .map(|ext| format!("{tool}{ext}"))
            .collect()
    } else {
        vec![tool.to_string()]
    }
}

fn is_executable_file(path: &Path) -> bool {
    if !path.is_file() {
        return false;
    }
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        std::fs::metadata(path)
            .map(|meta| meta.permissions().mode() & 0o111 != 0)
            .unwrap_or(false)
    }
    #[cfg(not(unix))]
    {
        true
    }
}

/// Searches `dirs` in order for any of `tool`'s candidate names. Kept
/// separate from [`find_on_path`] so the search itself is testable without
/// touching the real `PATH` environment variable.
fn find_in_dirs<I: IntoIterator<Item = PathBuf>>(dirs: I, tool: &str) -> Option<PathBuf> {
    let names = candidate_names(tool);
    for dir in dirs {
        for name in &names {
            let candidate = dir.join(name);
            if is_executable_file(&candidate) {
                return Some(candidate);
            }
        }
    }
    None
}

/// Checks whether `tool` is on `PATH`, without spawning it.
fn find_on_path(tool: &str) -> Option<PathBuf> {
    let path_var = env::var_os("PATH")?;
    find_in_dirs(env::split_paths(&path_var), tool)
}

/// Terminal emulators tried in order, per platform.
fn terminal_candidates() -> &'static [&'static str] {
    if cfg!(target_os = "windows") {
        &["wt.exe", "cmd.exe"]
    } else {
        &["x-terminal-emulator", "gnome-terminal", "konsole", "xterm"]
    }
}

/// One terminal's argv shape for opening at `path`, optionally running
/// `command` inside it. Building this as plain data (rather than spawning
/// directly) keeps it unit-testable; every element becomes a distinct argv
/// entry, never a shell string.
struct TerminalLaunch {
    program: &'static str,
    args: Vec<String>,
}

fn terminal_launch(
    program: &'static str,
    path: &Path,
    command: Option<&[String]>,
) -> TerminalLaunch {
    let mut args = Vec::new();
    match program {
        "gnome-terminal" => {
            args.push(format!("--working-directory={}", path.display()));
            if let Some(command) = command {
                args.push("--".into());
                args.extend(command.iter().cloned());
            }
        }
        "konsole" => {
            args.push("--workdir".into());
            args.push(path.display().to_string());
            if let Some(command) = command {
                args.push("-e".into());
                args.extend(command.iter().cloned());
            }
        }
        "wt.exe" => {
            args.push("-d".into());
            args.push(path.display().to_string());
            if let Some(command) = command {
                args.extend(command.iter().cloned());
            }
        }
        "cmd.exe" => {
            args.push("/K".into());
            if let Some(command) = command {
                args.extend(command.iter().cloned());
            }
        }
        // x-terminal-emulator and xterm both follow xterm's argv-based `-e`
        // convention (the remaining argv is the literal command, not a shell
        // string); the working directory is set via the spawned process's cwd.
        _ => {
            if let Some(command) = command {
                args.push("-e".into());
                args.extend(command.iter().cloned());
            }
        }
    }
    TerminalLaunch { program, args }
}

/// Opens a terminal window at `path`, optionally running `command` in it
/// instead of a bare shell. Tries each candidate terminal in order and
/// returns the last error if none could be spawned.
fn spawn_terminal(path: &Path, command: Option<&[String]>) -> Result<(), String> {
    let mut last_error: Option<String> = None;
    for &program in terminal_candidates() {
        if find_on_path(program).is_none() {
            continue;
        }
        let launch = terminal_launch(program, path, command);
        match Command::new(launch.program)
            .current_dir(path)
            .args(&launch.args)
            .spawn()
        {
            Ok(_) => return Ok(()),
            Err(e) => last_error = Some(format!("Failed to launch {program}: {e}")),
        }
    }
    Err(last_error.unwrap_or_else(|| {
        format!(
            "No terminal emulator found on this system (tried {})",
            terminal_candidates().join(", ")
        )
    }))
}

fn vscode_binary() -> &'static str {
    if cfg!(target_os = "windows") {
        "code.cmd"
    } else {
        "code"
    }
}

/// Maps a frontend tool name to its executable. Deliberately only accepts
/// the CLI tools this launcher is meant for (docs/VISION.md "Developer
/// Tools"), rather than spawning whatever name the frontend passes.
fn tool_binary(tool: &str) -> Result<&'static str, String> {
    match tool {
        "codex" => Ok("codex"),
        "claude" => Ok("claude"),
        "opencode" => Ok("opencode"),
        other => Err(format!("Unknown developer tool: {other}")),
    }
}

/// Opens the platform's default terminal at the Project's local directory.
#[tauri::command]
pub fn launch_terminal(app: AppHandle, project_id: String) -> Result<(), String> {
    let path = resolve_project_path(&app, &project_id)?;
    spawn_terminal(&path, None)
}

/// Opens VS Code (`code <path>`) at the Project's local directory.
#[tauri::command]
pub fn launch_vscode(app: AppHandle, project_id: String) -> Result<(), String> {
    let path = resolve_project_path(&app, &project_id)?;
    let binary = vscode_binary();
    if find_on_path(binary).is_none() {
        return Err("VS Code (code) was not found on PATH".into());
    }
    Command::new(binary)
        .current_dir(&path)
        .arg(&path)
        .spawn()
        .map(|_| ())
        .map_err(|e| format!("Failed to launch VS Code: {e}"))
}

/// Opens a command-line developer tool (codex, claude, opencode) in a new
/// terminal window at the Project's local directory. These are interactive
/// CLI tools, so they are launched inside a terminal rather than headless.
#[tauri::command]
pub fn launch_tool(app: AppHandle, project_id: String, tool: String) -> Result<(), String> {
    let path = resolve_project_path(&app, &project_id)?;
    let binary = tool_binary(&tool)?;
    if find_on_path(binary).is_none() {
        return Err(format!("{binary} was not found on PATH"));
    }
    let path_text = path.to_string_lossy().to_string();
    spawn_terminal(&path, Some(&[binary.to_string(), path_text]))
}

/// Checks whether `tool` is installed (on `PATH`), without spawning it.
/// `"terminal"` checks whether any supported terminal emulator is present.
#[tauri::command]
pub fn is_tool_installed(tool: String) -> Result<bool, String> {
    if tool == "terminal" {
        return Ok(terminal_candidates()
            .iter()
            .any(|t| find_on_path(t).is_some()));
    }
    let binary = match tool.as_str() {
        "vscode" | "code" => vscode_binary(),
        "codex" => "codex",
        "claude" => "claude",
        "opencode" => "opencode",
        other => other,
    };
    Ok(find_on_path(binary).is_some())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    #[cfg(unix)]
    use std::os::unix::fs::PermissionsExt;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn temp_dir() -> PathBuf {
        let path = env::temp_dir().join(format!(
            "kmjg-tools-test-{}-{}",
            std::process::id(),
            SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        fs::create_dir_all(&path).unwrap();
        path
    }

    fn write_executable(dir: &Path, name: &str) {
        let path = dir.join(name);
        fs::write(&path, "#!/bin/sh\n").unwrap();
        #[cfg(unix)]
        fs::set_permissions(&path, fs::Permissions::from_mode(0o755)).unwrap();
    }

    #[test]
    fn tool_binary_only_accepts_known_developer_tools() {
        assert_eq!(tool_binary("codex").unwrap(), "codex");
        assert_eq!(tool_binary("claude").unwrap(), "claude");
        assert_eq!(tool_binary("opencode").unwrap(), "opencode");
        assert!(tool_binary("rm").is_err());
        assert!(tool_binary("").is_err());
    }

    #[test]
    fn find_in_dirs_locates_an_executable_in_a_later_directory() {
        let empty = temp_dir();
        let populated = temp_dir();
        write_executable(&populated, "mytool");

        let found = find_in_dirs(vec![empty.clone(), populated.clone()], "mytool");
        assert_eq!(found, Some(populated.join("mytool")));

        fs::remove_dir_all(empty).unwrap();
        fs::remove_dir_all(populated).unwrap();
    }

    #[test]
    fn find_in_dirs_ignores_non_executable_files() {
        let dir = temp_dir();
        let path = dir.join("mytool");
        fs::write(&path, "not executable").unwrap();
        #[cfg(unix)]
        fs::set_permissions(&path, fs::Permissions::from_mode(0o644)).unwrap();

        #[cfg(unix)]
        assert_eq!(find_in_dirs(vec![dir.clone()], "mytool"), None);

        fs::remove_dir_all(dir).unwrap();
    }

    #[test]
    fn find_in_dirs_returns_none_when_missing_everywhere() {
        let dir = temp_dir();
        assert_eq!(
            find_in_dirs(vec![dir.clone()], "does-not-exist-anywhere"),
            None
        );
        fs::remove_dir_all(dir).unwrap();
    }

    #[test]
    fn gnome_terminal_launch_uses_argv_not_a_shell_string() {
        let path = Path::new("/tmp/some project (weird & chars)");
        let command = vec!["claude".to_string(), path.to_string_lossy().to_string()];
        let launch = terminal_launch("gnome-terminal", path, Some(&command));
        assert_eq!(launch.program, "gnome-terminal");
        assert_eq!(
            launch.args[0],
            format!("--working-directory={}", path.display())
        );
        assert_eq!(launch.args[1], "--");
        assert_eq!(launch.args[2], "claude");
        assert_eq!(launch.args[3], path.to_string_lossy());
    }

    #[test]
    fn konsole_launch_shape() {
        let path = Path::new("/tmp/proj");
        let launch = terminal_launch("konsole", path, None);
        assert_eq!(
            launch.args,
            vec!["--workdir".to_string(), "/tmp/proj".to_string()]
        );
    }

    #[test]
    fn xterm_launch_with_bare_terminal_has_no_extra_args() {
        let path = Path::new("/tmp/proj");
        let launch = terminal_launch("xterm", path, None);
        assert!(launch.args.is_empty());
    }

    #[test]
    fn xterm_launch_with_command_uses_dash_e() {
        let path = Path::new("/tmp/proj");
        let command = vec!["codex".to_string(), "/tmp/proj".to_string()];
        let launch = terminal_launch("xterm", path, Some(&command));
        assert_eq!(
            launch.args,
            vec![
                "-e".to_string(),
                "codex".to_string(),
                "/tmp/proj".to_string()
            ]
        );
    }

    #[test]
    fn vscode_binary_matches_platform() {
        let binary = vscode_binary();
        if cfg!(target_os = "windows") {
            assert_eq!(binary, "code.cmd");
        } else {
            assert_eq!(binary, "code");
        }
    }
}
